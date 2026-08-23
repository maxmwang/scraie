package util

import (
	"strings"
	"time"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search/serp"
)

// ItineraryToString returns a string concise description of an itinerary as:
//
//	"DEP <arrow> ARR | <dates>"
func ItineraryToString(it db.Itinerary) string {
	var b strings.Builder
	b.WriteString(it.DepartureID)
	if it.Type == int32(serp.OneWay) {
		b.WriteString(" → ")
	} else {
		b.WriteString(" ↔ ")
	}
	b.WriteString(it.ArrivalID)
	b.WriteString(" | ")
	b.WriteString(formatDate(it.OutboundDate))
	if it.Type != int32(serp.OneWay) {
		b.WriteString(" - ")
		b.WriteString(formatDate(it.ReturnDate.String))
	}
	return b.String()
}

// formatDate turns a "2006-01-02" date into "Mon, 2 Jan 2006", falling back to
// the original string if it cannot be parsed.
func formatDate(s string) string {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return s
	}
	return t.Format("Mon, 2 Jan 2006")
}
