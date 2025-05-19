package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
)

type HackerNewsPost struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Points    int       `json:"score"`
	Timestamp time.Time `json:"time"`
}

func handleFetchIDs(kv nats.KeyValue, nc *nats.Conn) micro.HandlerFunc {
	return func(req micro.Request) {
		slog.Info("Fetching latest Hacker News IDs")
		resp, err := http.Get("https://hacker-news.firebaseio.com/v0/topstories.json")
		if err != nil {
			slog.Error("Failed to fetch top story IDs", "error", err)
			_ = req.Error("500", "Failed to fetch IDs", nil)
			return
		}
		defer resp.Body.Close()

		var ids []int
		if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
			slog.Error("Failed to decode response", "error", err)
			_ = req.Error("500", "Failed to decode IDs", nil)
			return
		}
		slog.Info("Fetched %v IDs", len(ids))

		if len(ids) > 0 {
			idBytes, _ := json.Marshal(ids)
			if _, err := kv.Put("hn_ids_latest", idBytes); err != nil {
				slog.Error("Failed to store IDs in KV", "error", err)
				_ = req.Error("500", "Failed to store IDs", nil)
				return
			}

			for _, id := range ids {
				if err := nc.Publish("hn.process.id", []byte(fmt.Sprintf("%d", id))); err != nil {
					slog.Error("Failed to publish ID for processing", "error", err)
				}
			}
			slog.Info("Fetched Hacker News IDs", "count", len(ids))
		}

		_ = req.Respond([]byte(fmt.Sprintf("Fetched %v post IDs", len(ids))))
	}
}

func listFetchedIDs(kv nats.KeyValue) micro.HandlerFunc {
	return func(req micro.Request) {
		ids, err := kv.Get("hn_ids_latest")
		if err != nil {
			slog.Error("Failed to fetch latest HackerNews IDs", "error", err)
			_ = req.Error("404", "Failed to fetch lastest HackerNews IDs. Re-run fetch to populate KV.", nil)
			return
		}
		_ = req.Respond(ids.Value())
	}
}

func processHackerNewsID(id int, kv nats.KeyValue) ([]byte, error) {
	url := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch post: %w", err)
	}
	defer resp.Body.Close()

	var post map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&post); err != nil {
		return nil, fmt.Errorf("failed to decode post: %w", err)
	}

	if post["type"] == "story" && post["title"] != nil && post["score"] != nil && post["time"] != nil {
		ts := time.Unix(int64(post["time"].(float64)), 0)
		p := HackerNewsPost{
			ID:        id,
			Title:     post["title"].(string),
			URL:       fmt.Sprintf("%v", post["url"]),
			Points:    int(post["score"].(float64)),
			Timestamp: ts,
		}

		pb, err := json.Marshal(p)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal post: %w", err)
		}
		key := fmt.Sprintf("hn_post_%d", id)
		if _, err := kv.Put(key, pb); err != nil {
			return nil, fmt.Errorf("failed to store post in KV: %w", err)
		}
		slog.Info("Stored Hacker News ID", "id", id, "points", p.Points, "title", p.Title)
		return pb, nil
	}

	return nil, fmt.Errorf("skipped non-story item: %d", id)
}

func processFetchedIDs(kv nats.KeyValue) micro.HandlerFunc {
	return func(req micro.Request) {
		idStr := string(req.Data())
		id, err := strconv.Atoi(idStr)
		if err != nil {
			slog.Error("Failed to parse ID", "error", err, "id", id)
		}
		_, err = processHackerNewsID(id, kv)
		if err != nil {
			slog.Warn("Failed to process HackerNews ID", "error", err, "id", id)
		}
	}
}

func processFetchedIDsReply(kv nats.KeyValue) micro.HandlerFunc {
	return func(req micro.Request) {
		idStr := string(req.Data())
		id, err := strconv.Atoi(idStr)
		if err != nil {
			slog.Error("Failed to parse ID", "id", idStr, "error", err)
			_ = req.Error("400", "Invalid ID", nil)
			return
		}
		result, err := processHackerNewsID(id, kv)
		if err != nil {
			slog.Warn("Processing failed", "id", id, "error", err)
			_ = req.Error("500", err.Error(), nil)
			return
		}
		_ = req.Respond(result)
	}
}

func sortByScore(kv nats.KeyValue, obj nats.ObjectStore) micro.HandlerFunc {
	return func(req micro.Request) {
		slog.Info("Sorting latest HackerNews IDs by score")
		keys, err := kv.Keys()
		if err != nil {
			slog.Error("Failed to fetch latest HackerNews IDs", "error", err)
			_ = req.Error("500", "Failed to fetch latest HackerNews IDs", nil)
			return
		}

		var posts []HackerNewsPost
		now := time.Now()
		cutoff := now.Add(-24 * time.Hour)

		for _, key := range keys {
			entry, err := kv.Get(key)
			if err != nil {
				slog.Error("Failed to fetch latest HackerNews IDs from KV", "id", key, "error", err)
				continue
			}
			var post HackerNewsPost
			if key == "hn_ids_latest" {
				continue
			}
			if err := json.Unmarshal(entry.Value(), &post); err != nil {
				slog.Error("Failed to decode response", "id", key, "error", err)
				continue
			}
			if post.Timestamp.After(cutoff) {
				posts = append(posts, post)
			}
		}

		sort.Slice(posts, func(i, j int) bool {
			return posts[i].Points > posts[j].Points
		})

		report, err := json.MarshalIndent(posts, "", " ")
		if err != nil {
			slog.Error("Failed to encode response", "error", err)
			_ = req.Error("500", "Failed to encode response", nil)
			return
		}

		objName := fmt.Sprintf("hn_posts_%s", now.Format("2006-01-02"))
		_, err = obj.PutString(objName, string(report))
		if err != nil {
			slog.Error("Failed to store sorted posts in OBJ", "error", err)
			_ = req.Error("500", "Failed to store sorted posts in OBJ", nil)
			return
		}
		slog.Info("Stored HackerNews sorted posts", "id", objName)
		_ = req.Respond([]byte(fmt.Sprintf("Stored HackerNews sorted posts: %s", objName)))
	}
}

func listByScoreCount(obj nats.ObjectStore) micro.HandlerFunc {
	return func(req micro.Request) {
		countStr := string(req.Data())
		count, err := strconv.Atoi(countStr)
		if err != nil || count <= 0 {
			slog.Error("Invalid count parameter", "count", countStr)
			_ = req.Error("500", "Invalid count parameter. Must be a positive integer", nil)
			return
		}

		objs, err := obj.List()
		if err != nil {
			slog.Error("Failed to list objects in store", "error", err)
			_ = req.Error("500", "Failed to access stored objects", nil)
			return
		}
		if len(objs) == 0 {
			_ = req.Error("404", "No posts found in object store", nil)
			return
		}

		var latestObject *nats.ObjectInfo
		for i, obj := range objs {
			if i == 0 || obj.ModTime.After(latestObject.ModTime) {
				latestObject = objs[i]
			}
		}

		objData, err := obj.GetBytes(latestObject.Name)
		if err != nil {
			slog.Error("Failed to retrieve posts", "name", latestObject.Name, "error", err)
			_ = req.Error("500", "Failed to retrieve posts", nil)
			return
		}

		var posts []HackerNewsPost
		if err := json.Unmarshal(objData, &posts); err != nil {
			slog.Error("Failed to parse report data", "error", err)
			_ = req.Error("500", "Failed to parse report data", nil)
			return
		}

		if count < len(posts) {
			posts = posts[:count]
		}

		response, err := json.Marshal(struct {
			Count int              `json:"count"`
			Posts []HackerNewsPost `json:"posts"`
		}{
			Count: len(posts),
			Posts: posts,
		})
		if err != nil {
			slog.Error("Failed to serialize response", "error", err)
			_ = req.Error("500", "Failed to serialize response", nil)
			return
		}

		slog.Info("Retrieved top posts by score", "count", len(posts), "requested", count)
		_ = req.Respond(response)
	}
}
