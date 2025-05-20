# NATS Micro - HackerNews

> Fetch and sort the top posts in the last 24 hours using NATS micro

This project demonstrates [NATS Micro](https://github.com/nats-io/nats.go/tree/main/micro) capabilities by fetching top HackerNews posts, processing them, and sorting them by points. It uses NATS [KeyValue](https://docs.nats.io/nats-concepts/jetstream/key-value-store) for storage and [ObjectStore](https://docs.nats.io/nats-concepts/jetstream/obj_store) for presenting results.

```shell
# nats req hn.sort.top 3 | jq .
20:07:58 Sending request on "hn.sort.top"
20:07:58 Received with rtt 6.323333ms
{
  "count": 3,
  "posts": [
    {
      "id": 44031385,
      "title": "The Windows Subsystem for Linux is now open source",
      "url": "https://blogs.windows.com/windowsdeveloper/2025/05/19/the-windows-subsystem-for-linux-is-now-open-source/",
      "score": 1341,
      "time": "2025-05-20T02:14:15+10:00"
    },
    {
      "id": 44030850,
      "title": "Zod 4",
      "url": "https://zod.dev/v4",
      "score": 706,
      "time": "2025-05-20T01:24:58+10:00"
    },
    {
      "id": 44028153,
      "title": "Don't guess my language",
      "url": "https://vitonsky.net/blog/2025/05/17/language-detection/",
      "score": 630,
      "time": "2025-05-19T20:12:53+10:00"
    }
  ]
}
```

## Prerequisites

- [NATS Server][ns] with JetStream enabled
- [NATS CLI](https://github.com/nats-io/natscli)
- [Go](https://golang.org/doc/install) 
- [jq](https://stedolan.github.io/jq/download/) (for parsing JSON responses)
- [Task](https://taskfile.dev/) (for running commands)

## Installation

1. Clone the repository:
```bash
git clone https://github.com/danielmichaels/nats-micro-hackernews.git
cd nats-micro-hackernews
```

2. Start a NATS server by following the [instructions][ns], you'll need to enable JetStream.

3. Run the service:
```bash
task dev
# or 
# go run .
```
4. Check services
```bash
nats micro ls
```

## Usage

The service provides several endpoints that can be accessed using the NATS CLI or the provided Task commands.

### Workflow

1. **Fetch:** Get the latest top stories from HackerNews
2. **Process:** Process each story to extract details
3. **Sort:** Sort the stories by score
4. **Display:** Show the top N stories by points

### Available Commands

#### Fetch Commands

```bash
# Fetch top stories from HackerNews and store in KV
task fetch

# List all stored story IDs
task fetch:list
```

#### Process Commands

```bash
# Process a single ID (e.g., 12345) without reply
task process:pub:12345

# Process a single ID and get the post info as JSON
task process:reply:12345
```

#### Sort and Display Commands

```bash
# Sort all stored posts by score and save to Object Store
task sort

# Display the top N posts (e.g., show top 10)
task top:10
```

## How It Works

1. The service connects to a NATS server and creates KeyValue and ObjectStore buckets
2. When you run `task fetch`, it fetches HackerNews top story IDs and publishes each ID for processing
3. The process handlers fetch detailed information for each story
4. The sort handler groups stories from the last 24 hours and sorts them by score
5. The top handler retrieves the latest sorted list and returns the specified number of posts

## Environment Variables

- `NATS_URL`: NATS server URL (defaults to `nats://localhost:4222`)

You can set environment variables in a `.env` file or directly in your shell.

## Example Usage Flow

```bash
# Start the service
task dev
# Multiple instances can be run simultaneously

# Fetch latest HackerNews stories
task fetch

# Sort the stories by points
task sort

# Display top 10 stories
task top:10
```

## NATS Micro/Service Stats

The NATS cli offers two very useful commands; `info` and `stats`.

Here's an example of `info`:

This is great for getting a quick snapshot of your running services.

```
╭─────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│                                        HackerNews Service Statistics                                        │
├────────────────────────┬───────────────┬──────────┬───────────────┬────────┬─────────────────┬──────────────┤
│ ID                     │ Endpoint      │ Requests │ Queue Group   │ Errors │ Processing Time │ Average Time │
├────────────────────────┼───────────────┼──────────┼───────────────┼────────┼─────────────────┼──────────────┤
│ IZHP70J2v9KWHI6djny2Hy │ fetch         │ 0        │ fetch-group   │ 0      │ 0s              │ 0s           │
│                        │ list          │ 1        │ fetch-group   │ 0      │ 590µs           │ 590µs        │
│                        │ process       │ 244      │ process-group │ 0      │ 49.70s          │ 204ms        │
│                        │ process-reply │ 3        │ process-group │ 0      │ 622ms           │ 207ms        │
│                        │ sort          │ 2        │ sort-group    │ 0      │ 164ms           │ 82ms         │
│                        │ top           │ 2        │ sort-group    │ 0      │ 11ms            │ 5ms          │
│ f46PZ61mGCg520JW9HaphC │ fetch         │ 1        │ fetch-group   │ 0      │ 488ms           │ 488ms        │
│                        │ list          │ 1        │ fetch-group   │ 0      │ 725µs           │ 725µs        │
│                        │ process       │ 261      │ process-group │ 0      │ 1m1s            │ 234ms        │
│                        │ process-reply │ 1        │ process-group │ 0      │ 416ms           │ 416ms        │
│                        │ sort          │ 1        │ sort-group    │ 0      │ 124ms           │ 124ms        │
│                        │ top           │ 1        │ sort-group    │ 0      │ 7ms             │ 7ms          │
├────────────────────────┼───────────────┼──────────┼───────────────┼────────┼─────────────────┼──────────────┤
│                        │               │ 518      │               │ 0      │ 1m52s           │ 218ms        │
╰────────────────────────┴───────────────┴──────────┴───────────────┴────────┴─────────────────┴──────────────╯
```

And `stats`:

This is excellent for getting a better view into all the endpoints configured
for your service.

```
# nats micro info HackerNews
Service Information

          Service: HackerNews (IZHP70J2v9KWHI6djny2Hy)
      Description: Get most voted entries in a the last 24 hours
          Version: 1.0.0

Endpoints:

               Name: fetch
            Subject: hn.fetch.ids
        Queue Group: fetch-group
  
               Name: list
            Subject: hn.fetch.list
        Queue Group: fetch-group
  
               Name: process
            Subject: hn.process.id
        Queue Group: process-group
  
               Name: process-reply
            Subject: hn.process.reply
        Queue Group: process-group
  
               Name: sort
            Subject: hn.sort.ids
        Queue Group: sort-group
  
               Name: top
            Subject: hn.sort.top
        Queue Group: sort-group

Statistics for 6 Endpoint(s):

  fetch Endpoint Statistics:

           Requests: 0 in group "fetch-group"
    Processing Time: 0s (average 0s)
            Started: 2025-05-20 20:14:33 (2m8s ago)
             Errors: 0

  list Endpoint Statistics:

           Requests: 1 in group "fetch-group"
    Processing Time: 590µs (average 590µs)
            Started: 2025-05-20 20:14:33 (2m8s ago)
             Errors: 0

  process Endpoint Statistics:

           Requests: 244 in group "process-group"
    Processing Time: 49.70s (average 204ms)
            Started: 2025-05-20 20:14:33 (2m8s ago)
             Errors: 0

  process-reply Endpoint Statistics:

           Requests: 3 in group "process-group"
    Processing Time: 622ms (average 207ms)
            Started: 2025-05-20 20:14:33 (2m8s ago)
             Errors: 0

  sort Endpoint Statistics:

           Requests: 2 in group "sort-group"
    Processing Time: 164ms (average 82ms)
            Started: 2025-05-20 20:14:33 (2m8s ago)
             Errors: 0

  top Endpoint Statistics:

           Requests: 2 in group "sort-group"
    Processing Time: 11ms (average 5ms)
            Started: 2025-05-20 20:14:33 (2m8s ago)
             Errors: 0
```


[ns]: https://docs.nats.io/running-a-nats-service/introduction/installation