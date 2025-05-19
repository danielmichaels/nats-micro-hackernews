package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
)

func main() {
	if err := run(); err != nil {
		slog.Error("failed to run", "err", err)
		os.Exit(1)
	}
}

func connect() (*nats.Conn, error) {
	natsURL := nats.DefaultURL
	if os.Getenv("NATS_URL") != "" {
		natsURL = os.Getenv("NATS_URL")
	}
	nc, err := nats.Connect(natsURL)
	return nc, err
}

func run() error {
	nc, err := connect()
	if err != nil {
		return err
	}
	js, err := nc.JetStream()
	if err != nil {
		return err
	}
	kv, err := js.CreateKeyValue(&nats.KeyValueConfig{
		Bucket: "hacker_news",
		TTL:    1 * time.Hour,
	})
	obj, err := js.CreateObjectStore(&nats.ObjectStoreConfig{
		Bucket: "hacker_news",
	})

	svc, err := micro.AddService(nc, micro.Config{
		Name:        "HackerNews",
		Description: "Get most voted entries in a the last 24 hours",
		Version:     "1.0.0",
		DoneHandler: func(srv micro.Service) {
			info := srv.Info()
			fmt.Printf("stopped service %q with ID %q\n", info.Name, info.ID)
		},
		ErrorHandler: func(srv micro.Service, err *micro.NATSError) {
			info := srv.Info()
			fmt.Printf("Service %q returned an error on subject %q: %s", info.Name, err.Subject, err.Description)
		},
	})
	if err != nil {
		return err
	}
	hn := svc.AddGroup("hn")

	fetch := hn.AddGroup("fetch", micro.WithGroupQueueGroup("fetch-group"))
	if err := fetch.AddEndpoint("fetch", handleFetchIDs(kv, nc), micro.WithEndpointSubject("ids")); err != nil {
		return err
	}
	if err := fetch.AddEndpoint("list", listFetchedIDs(kv), micro.WithEndpointSubject("list")); err != nil {
		return err
	}

	process := hn.AddGroup("process", micro.WithGroupQueueGroup("process-group"))
	if err := process.AddEndpoint("process", processFetchedIDs(kv), micro.WithEndpointSubject("id")); err != nil {
		return err
	}
	if err := process.AddEndpoint("process", processFetchedIDsReply(kv), micro.WithEndpointSubject("id.reply")); err != nil {
		return err
	}

	sort := hn.AddGroup("sort", micro.WithGroupQueueGroup("sort-group"))
	if err := sort.AddEndpoint("sort", sortByScore(kv, obj), micro.WithEndpointSubject("ids")); err != nil {
		return err
	}
	if err := sort.AddEndpoint("top", listByScoreCount(obj), micro.WithEndpointSubject("top")); err != nil {
		return err
	}

	slog.Info("HackerNews service started")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	err = svc.Stop()
	if err != nil {
		slog.Error("failed to stop service", "err", err)
	}
	slog.Info("HackerNews service stopping")
	return nil
}
