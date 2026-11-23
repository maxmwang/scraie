# scraie

Scrapes Google Flights prices for a list of itineraries daily using [goflights](https://pkg.go.dev/github.com/maxmwang/goflights) and stores results in a Supabase database, building a price history suitable for graphing.

## Setup

To run locally:

```bash
# ./packages/scraie/flights
FLIGHTS_SERPAPI_KEY=_ \
FLIGHTS_DB_URI=_ \
DISCORD_BOT_TOKEN=_ \
go run ./cmd/scraie -nosearch -readonly
```

## Deployment

GitHub Actions automatically builds and pushes the latest commit to Docker Hub as two containers:
`maxmwang/scraie:latest` for scheduled jobs and `maxmwang/scraie-server:latest` for the long-lived server.
The Carpi Quadlets use `Container.Pull=always`, so the latest commits are pulled on restart or deployment.
