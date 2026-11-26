package search

import (
	"context"

	"github.com/maxmwang/scraie/flights/internal/db"
)

type FlightOptions struct {
	Option   db.Option
	Segments []db.Segment
	Layovers []db.Layover
}

type Result struct {
	Options []FlightOptions
}

type Searcher interface {
	Search(context.Context, db.Itinerary) (Result, error)
}
