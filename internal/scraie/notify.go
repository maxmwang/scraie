package scraie

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
	"github.com/maxmwang/scraie/flights/internal/util/option"
)

const (
	PriceHistoryDays   = 30
	PriceHistoryLength = PriceHistoryDays
)

type Notification struct {
	Itinerary db.Itinerary

	// PriceHistory are the NumPointsToPlot cheapest options grouped and sorted by date
	PriceHistory []db.Option

	CheapestOption  search.FlightOptions
	Near7DayMinimum option.Option[float64]
	PriceMovement   option.Option[float64]
	PreviousPrice   float64
	NewPrice        float64
	Timestamp       time.Time
}

// TODO(ai): clean up below comment

// buildNotification compares the cheapest fare of the latest search against the
// cheapest fare of the previous search and, if it moved by more than
// priceChangeThreshold in either direction, posts a summary of the itinerary and
// its recent daily-minimum price chart.
//
// newNotification renders a summary notification for the itinerary whose cheapest fare moved
// from oldMin (previous search) to newMin (latest search). The title mirrors the
// chart's route-and-dates header, and the body summarizes the price move and
// describes the new cheapest option; the chart is attached separately.
//
// Messages are sent through the notifier supplied in Args. If the notifier is
// nil, or there is nothing meaningful to compare, no message is sent.
func buildNotification(ctx context.Context, it db.Itinerary) (option.Option[Notification], error) {
	// -1hour in case of small function run scheduling variance
	since := time.Now().AddDate(0, 0, -PriceHistoryDays).Add(-time.Hour)
	options, err := getOptionsSince(ctx, it, since)
	if err != nil {
		return option.None[Notification](), fmt.Errorf("db.GetOptionsSince: %w", err)
	}
	if len(options) < 2 {
		// Nothing has been searched recently: nothing to compare or report.
		return option.None[Notification](), nil
	}

	cheapestOptions := constructTimestampCheapestOptions(options)

	checks := makeChecks(cheapestOptions)
	if !checks.any() {
		return option.None[Notification](), nil
	}

	q := db.New(db.FromContext(ctx))

	// Reassemble the latest cheapest option with its segments and layovers so the
	// embed can describe the flight (airline, flight numbers, duration, stops).
	latestCheapestOption := cheapestOptions[len(cheapestOptions)-1]
	segments, err := q.GetSegmentsByOptionIDs(ctx, []int64{latestCheapestOption.ID})
	if err != nil {
		return option.None[Notification](), fmt.Errorf("db.GetSegmentsByOptionIDs: %w", err)
	}
	layovers, err := q.GetLayoversByOptionIDs(ctx, []int64{latestCheapestOption.ID})
	if err != nil {
		return option.None[Notification](), fmt.Errorf("db.GetLayoversByOptionIDs: %w", err)
	}
	cheapest := search.FlightOptions{
		Option:   latestCheapestOption,
		Segments: segments,
		Layovers: layovers,
	}
	previousPrice := priceFloat(cheapestOptions[len(cheapestOptions)-2].Price)
	newPrice := priceFloat(latestCheapestOption.Price)

	notification := Notification{
		Itinerary:       it,
		PriceHistory:    cheapestOptions,
		CheapestOption:  cheapest,
		Near7DayMinimum: checks.Near7DayMinimum,
		PriceMovement:   checks.PriceMovement,
		PreviousPrice:   previousPrice,
		NewPrice:        newPrice,
		Timestamp:       time.Now().UTC(),
	}
	return option.Some(notification), nil
}

func getOptionsSince(ctx context.Context, it db.Itinerary, since time.Time) ([]db.Option, error) {
	pool := db.FromContext(ctx)

	options, err := db.New(pool).GetOptionsSince(ctx, db.GetOptionsSinceParams{
		ItineraryID: it.ID,
		SearchedAt: pgtype.Timestamptz{
			Time:  since,
			Valid: true,
		},
	})
	if err != nil {
		return nil, err
	}
	return options, nil
}

// constructTimestampCheapestOptions returns a sorted list containing the cheapest
// option from each distinct timestamp. Since a single scheduled function run
// will reuse a single timestamp for all scraped options, we expect each
// timestamp to have many options.
func constructTimestampCheapestOptions(options []db.Option) []db.Option {
	m := make(map[pgtype.Timestamptz]db.Option)

	for _, opt := range options {
		minOpt, ok := m[opt.SearchedAt]
		if !ok || priceFloat(opt.Price) < priceFloat(minOpt.Price) {
			m[opt.SearchedAt] = opt
		}
	}

	s := make([]db.Option, 0, len(m))
	for _, opt := range m {
		s = append(s, opt)
	}
	slices.SortFunc(s, func(a, b db.Option) int {
		return a.SearchedAt.Time.Compare(b.SearchedAt.Time)
	})

	return s
}
