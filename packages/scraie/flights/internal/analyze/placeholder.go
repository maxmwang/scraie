package analyze

// TEMPORARY placeholder — remove this file (and cmd/placeholder) when done.
// It sends a hardcoded price-change notification through the real embed + webhook
// path so the formatting can be eyeballed in the actual Discord channel.

import (
	"context"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
)

func SendPlaceholder(ctx context.Context) error {
	it := db.Itinerary{
		DepartureID:  "TEST",
		ArrivalID:    "TEST",
		OutboundDate: "2026-08-15",
		Currency:     "USD",
		// Type:         int32(serp.OneWay),
	}

	cheapest := search.FlightOptions{
		Option: db.Option{TotalDuration: 15*60 + 30, Price: 842},
		Segments: []db.Segment{
			{
				Airline:            "United",
				FlightNumber:       "UA 123",
				DepartureAirportID: "SNA",
				ArrivalAirportID:   "EWR",
				Duration:           6*60 + 15,
			},
			{
				Airline:            "United",
				FlightNumber:       "UA 7382",
				DepartureAirportID: "EWR",
				ArrivalAirportID:   "LHR",
				Duration:           9 * 60,
			},
		},
		Layovers: []db.Layover{
			{AirportID: "EWR", Duration: 2*60 + 15},
		},
	}

	embed := buildEmbed(it, 999, 842, cheapest)
	return sendDiscordWebhook(ctx, discordPayload{Embeds: []discordEmbed{embed}})
}
