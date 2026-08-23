package goflights

import (
	"math/big"
	"strings"
	"time"

	gf "github.com/maxmwang/goflights"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
	"github.com/maxmwang/scraie/flights/internal/search/serp"
)

// dbTimeFormat is how Segments stores a local departure or arrival time. The
// analyze package parses this layout, and SerpAPI wrote the same one.
const dbTimeFormat = "2006-01-02 15:04"

// Option.type as SerpAPI spelled it, which is also how the notification labels
// read the column back.
const (
	typeRoundTrip = "Round trip"
	typeOneWay    = "One way"
)

// maxDecimalDigits caps the scaling exponent when converting a fare. No ISO
// 4217 currency has more than four minor units, so this only ever bounds a
// nonsense value.
const maxDecimalDigits = 4

func toResult(it db.Itinerary, flightOptions []gf.FlightOption) search.Result {
	optionType := typeRoundTrip
	if it.Type == int32(serp.OneWay) {
		optionType = typeOneWay
	}

	options := make([]search.FlightOptions, 0, len(flightOptions))
	for _, fo := range flightOptions {
		if len(fo.Segments) == 0 {
			continue
		}

		segments := make([]db.Segment, len(fo.Segments))
		for i, s := range fo.Segments {
			segments[i] = db.Segment{
				// The booking token carries IATA codes only; the airport names
				// live in the page's JSON, which is not what this decodes. The
				// code stands in so the column is never empty.
				DepartureAirportName: s.FromAirport,
				DepartureAirportID:   s.FromAirport,
				DepartureTime:        s.DepartureTime.Format(dbTimeFormat),
				ArrivalAirportName:   s.ToAirport,
				ArrivalAirportID:     s.ToAirport,
				ArrivalTime:          s.ArrivalTime.Format(dbTimeFormat),

				Duration: minutesBetween(s.DepartureTime, s.ArrivalTime),
				Airplane: s.Aircraft,
				Airline:  s.Airline,
				// Logo URLs come from the page's JSON, not the token.
				AirlineLogo:  "",
				TravelClass:  s.Class,
				FlightNumber: strings.TrimSpace(s.Airline + " " + s.FlightNumber),
				Overnight:    !sameLocalDate(s.DepartureTime, s.ArrivalTime),
			}
		}

		// A layover is the gap between two consecutive segments, so there is
		// one fewer of them than there are segments, and layover i follows
		// segment i — the order the notification embed walks them in.
		layovers := make([]db.Layover, 0, len(fo.Segments)-1)
		for i := 0; i+1 < len(fo.Segments); i++ {
			arrival, departure := fo.Segments[i].ArrivalTime, fo.Segments[i+1].DepartureTime
			layovers = append(layovers, db.Layover{
				Duration:  minutesBetween(arrival, departure),
				Name:      fo.Segments[i].ToAirport,
				AirportID: fo.Segments[i].ToAirport,
				Overnight: !sameLocalDate(arrival, departure),
			})
		}

		options = append(options, search.FlightOptions{
			Option: db.Option{
				ItineraryID: it.ID,
				// Gate to gate across the whole leg, connections included.
				TotalDuration: minutesBetween(fo.Segments[0].DepartureTime,
					fo.Segments[len(fo.Segments)-1].ArrivalTime),
				Price: fare(fo.Price, fo.DecimalDigits),
				Type:  optionType,
			},
			Segments: segments,
			Layovers: layovers,
		})
	}

	return search.Result{Options: options}
}

// fare converts a fare in a currency's minor units to the NUMERIC the Options
// column stores: the token's 41100 with two decimal digits becomes 411.00.
func fare(amount int64, decimalDigits int) pgtype.Numeric {
	return pgtype.Numeric{
		Int:   big.NewInt(amount),
		Exp:   -int32(min(max(decimalDigits, 0), maxDecimalDigits)),
		Valid: true,
	}
}

func minutesBetween(a, b time.Time) int32 {
	return int32(b.Sub(a).Minutes())
}

func sameLocalDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
