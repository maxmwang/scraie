package search

import "github.com/maxmwang/scraie/flights/internal/db"

type FlightOptions struct {
	Option   db.Option
	Segments []db.Segment
	Layovers []db.Layover
}

type Result struct {
	Options []FlightOptions
}

type Searcher interface {
	Search(db.Itinerary) (Result, error)
}
