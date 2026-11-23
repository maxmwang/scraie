package discord

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/maxmwang/scraie/flights/internal/db"
)

type itinerarySummaryOptions struct {
	includeID       bool
	includeDefaults bool
	formatDate      func(string) string
}

var flightTypeLabels = map[int32]string{
	1: "Round trip",
	2: "One way",
	3: "Multi-city",
}

var travelClassLabels = map[int32]string{
	1: "Economy",
	2: "Premium economy",
	3: "Business",
	4: "First",
}

var stopsLabels = map[int32]string{
	0: "Any",
	1: "Nonstop",
	2: "Up to 1 stop",
	3: "Up to 2 stops",
}

func formatItineraryDate(raw string) string {
	date, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return raw
	}
	return date.Format("Jan 2, 2006")
}

func renderItinerarySummary(it db.Itinerary, options itinerarySummaryOptions) string {
	lines := []string{}
	field := func(label, value string) {
		lines = append(lines, fmt.Sprintf("__%s__: %s", label, value))
	}

	if options.includeID {
		field("ID", "#"+strconv.FormatInt(it.ID, 10))
	}
	field("Departure", it.DepartureID)
	field("Arrival", it.ArrivalID)
	field("Outbound", formatSummaryDate(it.OutboundDate, options.formatDate))
	if it.ReturnDate.Valid && it.ReturnDate.String != "" {
		field("Return", formatSummaryDate(it.ReturnDate.String, options.formatDate))
	}
	if it.Description.Valid && it.Description.String != "" {
		field("Description", it.Description.String)
	}

	lines = append(lines, "")
	if options.includeDefaults || it.Type != 1 {
		field("Trip type", itineraryLabel(flightTypeLabels, it.Type))
	}
	if options.includeDefaults || it.TravelClass != 1 {
		field("Travel class", itineraryLabel(travelClassLabels, it.TravelClass))
	}
	if options.includeDefaults || it.Stops != 0 {
		field("Stops", itineraryLabel(stopsLabels, it.Stops))
	}
	if options.includeDefaults || it.Adults != 1 {
		field("Adults", strconv.FormatInt(int64(it.Adults), 10))
	}
	if it.Children > 0 {
		field("Children", strconv.FormatInt(int64(it.Children), 10))
	}
	if it.InfantsInSeat > 0 {
		field("Infants in seat", strconv.FormatInt(int64(it.InfantsInSeat), 10))
	}
	if it.InfantsOnLap > 0 {
		field("Infants on lap", strconv.FormatInt(int64(it.InfantsOnLap), 10))
	}
	if it.Bags > 0 {
		field("Carry-on bags", strconv.FormatInt(int64(it.Bags), 10))
	}
	if it.ExcludeBasic {
		field("Exclude basic", "true")
	}
	if it.IncludeAirlines.Valid {
		field("Include airlines", it.IncludeAirlines.String)
	}
	if it.ExcludeAirlines.Valid {
		field("Exclude airlines", it.ExcludeAirlines.String)
	}
	if it.MaxPrice.Valid {
		field("Max price", strconv.FormatInt(int64(it.MaxPrice.Int32), 10))
	}
	if it.OutboundTimes.Valid {
		field("Outbound times", it.OutboundTimes.String)
	}
	if it.ReturnTimes.Valid {
		field("Return times", it.ReturnTimes.String)
	}
	if it.LayoverDuration.Valid {
		field("Layover duration", it.LayoverDuration.String)
	}
	if it.ExcludeConns.Valid {
		field("Exclude connections", it.ExcludeConns.String)
	}
	if it.MaxDuration.Valid {
		field("Max duration", strconv.FormatInt(int64(it.MaxDuration.Int32), 10))
	}
	return strings.Join(lines, "\n")
}

func formatSummaryDate(raw string, formatter func(string) string) string {
	if formatter != nil {
		return formatter(raw)
	}
	return raw
}

func itineraryLabel(labels map[int32]string, value int32) string {
	if label, ok := labels[value]; ok {
		return label
	}
	return strconv.FormatInt(int64(value), 10)
}
