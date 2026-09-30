# scraie

Scrapes Google Flights prices for a list of itineraries daily using [goflights](https://pkg.go.dev/github.com/maxmwang/goflights) and stores results in a Supabase database, building a price history suitable for graphing.

## Setup

To run locally:

```bash
# ./packages/scraie/flights
FLIGHTS_SERPAPI_KEY=_ \
FLIGHTS_DB_URI=_ \
SCRAIE_DISCORD_WEBHOOK=_ \
go run ./cmd/scraie -nosearch -readonly
```

## Deployment

Github Actions automatically build and push the latest commit into a container and onto Docker Hub. `carpi.container` is configured with `Container.Pull=always`, so the latest commit should always be pulled.
