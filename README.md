# scraie

Scrapes Google Flights prices for a list of itineraries daily using [SerpAPI](https://serpapi.com/google-flights-api) and stores results in a Supabase database, building a price history suitable for graphing.

## Setup

1. Copy `.env` and fill in your SerpAPI key:

   ```
   SCRAIE_SERPAPI_KEY=_
   SCRAIE_DB_URI=_
   SCRAIE_DISCORD_WEBHOOK=_
   ```

2. Local run:
   ```bash
   # ./packages/scraie/flights
   go run ./cmd/scraie
   ```
