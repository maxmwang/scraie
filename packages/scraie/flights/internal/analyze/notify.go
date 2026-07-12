package analyze

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maxmwang/scraie/flights/internal/config"
	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
	"github.com/maxmwang/scraie/flights/internal/util"
)

// priceChangeThreshold is the relative change in the cheapest price that triggers
// a Discord notification.
const priceChangeThreshold = 0.05

const (
	colorPriceDrop = 0x2ECC71 // green
	colorPriceRise = 0xE74C3C // red
)

// ANSI escape codes for Discord's "```ansi" code blocks, the only way to color
// text inside an embed.
const (
	ansiReset = "[0m"
	ansiGreen = "[1;32m"
	ansiRed   = "[1;31m"
)

// LoadPriceHistory returns every option saved for the itinerary at or after
// since, reassembled with their segments so each can be matched to a flight by
// signature. Layovers are not loaded since the price history only needs the
// price, timestamp, and signature. Options are ordered oldest first.
func LoadPriceHistory(ctx context.Context, dbClient *db.Queries, itineraryID int64, since pgtype.Timestamptz) ([]search.FlightOptions, error) {
	options, err := dbClient.GetOptionsSince(ctx, db.GetOptionsSinceParams{
		ItineraryID: itineraryID,
		SearchedAt:  since,
	})
	if err != nil {
		return nil, fmt.Errorf("get options since: %w", err)
	}
	if len(options) == 0 {
		return nil, nil
	}

	ids := make([]int64, len(options))
	for i, o := range options {
		ids[i] = o.ID
	}

	segments, err := dbClient.GetSegmentsByOptionIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get history segments: %w", err)
	}

	segmentsByOption := make(map[int64][]db.Segment, len(options))
	for _, s := range segments {
		segmentsByOption[s.OptionID] = append(segmentsByOption[s.OptionID], s)
	}

	flights := make([]search.FlightOptions, len(options))
	for i, o := range options {
		flights[i] = search.FlightOptions{
			Option:   o,
			Segments: segmentsByOption[o.ID],
		}
	}
	return flights, nil
}

// findCheapeastOptionOfPreviousAndLatestSearchTimestamp finds the cheapest options with
// the second to latest and the latest SearchedAt timestamp
func findCheapeastOptionOfPreviousAndLatestSearchTimestamp(options []db.Option) (db.Option, db.Option) {
	latestSearchTimestamp := options[len(options)-1].SearchedAt.Time
	previousSearchTimestamp := time.Time{}
	i := len(options) - 1

	minLatestI := i
	for ; i >= 0; i-- {
		if options[i].SearchedAt.Time.Before(latestSearchTimestamp) {
			previousSearchTimestamp = options[i].SearchedAt.Time
			break
		} else if options[i].Price <= options[minLatestI].Price {
			minLatestI = i
		}
	}

	minPreviousI := i
	for ; i >= 0 && options[i].SearchedAt.Time.Equal(previousSearchTimestamp); i-- {
		if options[i].Price <= options[minPreviousI].Price {
			minPreviousI = i
		}
	}

	if previousSearchTimestamp.IsZero() {
		return options[minLatestI], options[minLatestI]
	}
	return options[minPreviousI], options[minLatestI]
}

const NumDaysToPlot = 30

// NotifyOnPriceChange compares the cheapest fare of the latest search against the
// cheapest fare of the previous search and, if it moved by more than
// priceChangeThreshold in either direction, posts a summary of the itinerary and
// its recent daily-minimum price chart to Discord.
//
// The Discord webhook is read from the config on the context
// (config.Config.DiscordWebhook). If it is empty, or there is nothing meaningful
// to compare, no message is sent.
func NotifyOnPriceChange(ctx context.Context, pool *pgxpool.Pool, it db.Itinerary) error {
	dbc := db.New(pool)

	options, err := dbc.GetOptionsSince(ctx, db.GetOptionsSinceParams{
		ItineraryID: it.ID,
		SearchedAt:  pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -2), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("GetOptionsSince: %w", err)
	}
	if len(options) == 0 {
		// Nothing has been searched recently: nothing to compare or report.
		return nil
	}

	previousCheapestOption, latestCheapestOption := findCheapeastOptionOfPreviousAndLatestSearchTimestamp(options)
	prevPrice := previousCheapestOption.Price
	latestPrice := latestCheapestOption.Price
	if prevPrice == 0 {
		// No previous search to compare against, or a malformed baseline price.
		return nil
	}
	change := math.Abs(float64(latestPrice-prevPrice)) / float64(prevPrice)
	if change <= priceChangeThreshold {
		return nil
	}

	// Reassemble the latest cheapest option with its segments and layovers so the
	// embed can describe the flight (airline, flight numbers, duration, stops).
	segments, err := dbc.GetSegmentsByOptionIDs(ctx, []int64{latestCheapestOption.ID})
	if err != nil {
		return fmt.Errorf("GetSegmentsByOptionIDs: %w", err)
	}
	layovers, err := dbc.GetLayoversByOptionIDs(ctx, []int64{latestCheapestOption.ID})
	if err != nil {
		return fmt.Errorf("GetLayoversByOptionIDs: %w", err)
	}
	cheapest := search.FlightOptions{
		Option:   latestCheapestOption,
		Segments: segments,
		Layovers: layovers,
	}

	embed := buildEmbed(it, prevPrice, latestPrice, cheapest)

	// Add a chart of the recent daily-minimum price history to the embed. Any
	// failure here is non-fatal: the embed is still worth sending on its own.
	since := pgtype.Timestamptz{Time: time.Now().AddDate(0, 0, -NumDaysToPlot-1), Valid: true}
	history, err := LoadPriceHistory(ctx, dbc, it.ID, since)
	if err != nil {
		return fmt.Errorf("LoadPriceHistory: %w", err)
	}

	chartURL, err := buildDailyMinimumPriceChartURL(it, history, NumDaysToPlot)
	if err != nil {
		return err
	}
	if chartURL != "" {
		embed.Image = &discordImage{URL: chartURL}
	}

	return sendDiscordWebhook(ctx, discordPayload{Embeds: []discordEmbed{embed}})
}

// buildEmbed renders a summary embed for the itinerary whose cheapest fare moved
// from oldMin (previous search) to newMin (latest search). The title mirrors the
// chart's route-and-dates header, and the body summarizes the price move and
// describes the new cheapest option; the chart is attached separately.
func buildEmbed(it db.Itinerary, oldMin, newMin int32, cheapest search.FlightOptions) discordEmbed {
	color := colorPriceRise
	if newMin < oldMin {
		color = colorPriceDrop
	}

	itinerarySummary := renderItinerarySummary(it)

	priceChange := renderPriceChange(currencySymbol(it.Currency), oldMin, newMin)

	cheapestOptionSummary := renderCheapestOptionSummary(cheapest)

	fields := []discordField{
		{Name: "Itinerary Summary", Value: itinerarySummary},
		{Name: "Price Change", Value: priceChange},
		{Name: "Newest Cheapest Option's First Leg", Value: cheapestOptionSummary},
	}

	return discordEmbed{
		Title:     util.ItineraryToString(it),
		Color:     color,
		Fields:    fields,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// Labels for the itinerary's enum filters, mirroring the SerpAPI enums defined in
// the serp package. renderItinerarySummary uses these to surface only non-default
// filters. The zero-index defaults are: type 1 (round trip), travel class 1
// (economy), and stops 0 (any).
var (
	flightTypeLabels  = map[int32]string{1: "Round trip", 2: "One way", 3: "Multi-city"}
	travelClassLabels = map[int32]string{1: "Economy", 2: "Premium economy", 3: "Business", 4: "First"}
	stopsLabels       = map[int32]string{0: "Any", 1: "Nonstop", 2: "Up to 1 stop", 3: "Up to 2 stops"}
)

// renderItinerarySummary renders the itinerary's description (if any) followed by
// its route, dates, and every filter set to a non-default value, one
// "_Field_: Value" line each.
func renderItinerarySummary(it db.Itinerary) string {
	var lines []string
	if it.Description.Valid && it.Description.String != "" {
		lines = append(lines, it.Description.String)
	}

	field := func(label, value string) {
		lines = append(lines, fmt.Sprintf("__%s__: %s", label, value))
	}

	field("Departure", it.DepartureID)
	field("Arrival", it.ArrivalID)
	field("Outbound", it.OutboundDate)
	if it.ReturnDate.Valid && it.ReturnDate.String != "" {
		field("Return", it.ReturnDate.String)
	}

	if it.Type != 1 {
		field("Trip type", labelOr(flightTypeLabels, it.Type))
	}
	if it.TravelClass != 1 {
		field("Travel class", labelOr(travelClassLabels, it.TravelClass))
	}
	if it.Stops != 0 {
		field("Stops", labelOr(stopsLabels, it.Stops))
	}
	if it.Adults != 1 {
		field("Adults", fmt.Sprintf("%d", it.Adults))
	}
	if it.Children > 0 {
		field("Children", fmt.Sprintf("%d", it.Children))
	}
	if it.InfantsInSeat > 0 {
		field("Infants in seat", fmt.Sprintf("%d", it.InfantsInSeat))
	}
	if it.InfantsOnLap > 0 {
		field("Infants on lap", fmt.Sprintf("%d", it.InfantsOnLap))
	}
	if it.Bags > 0 {
		field("Carry-on bags", fmt.Sprintf("%d", it.Bags))
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
		field("Max price", fmt.Sprintf("%d", it.MaxPrice.Int32))
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
		field("Max duration", fmt.Sprintf("%d", it.MaxDuration.Int32))
	}

	return strings.Join(lines, "\n")
}

// labelOr returns m[k], falling back to k's decimal string when k is not a known
// enum value.
func labelOr(m map[int32]string, k int32) string {
	if v, ok := m[k]; ok {
		return v
	}
	return fmt.Sprintf("%d", k)
}

// renderPriceChange renders a colored summary of how the cheapest fare moved
// from oldMin to newMin, e.g. "$999 → $842 | -$157 (-15.7%)". The block is
// tinted green when the fare dropped and red when it rose. cur is the currency
// symbol to prefix each amount with.
func renderPriceChange(cur string, oldMin, newMin int32) string {
	color := ansiRed
	if newMin < oldMin {
		color = ansiGreen
	}

	diff := newMin - oldMin
	sign := "+"
	if diff < 0 {
		sign = "-"
		diff = -diff
	}

	percent := "0%"
	if oldMin != 0 {
		percent = fmt.Sprintf("%+.1f%%", float64(newMin-oldMin)/float64(oldMin)*100)
	}

	// Wrapped in a Discord ansi code block, the only way to color text in an embed.
	return fmt.Sprintf("```ansi\n%s%s%d → %s%d | %s%s%d (%s)%s\n```",
		color, cur, oldMin, cur, newMin, sign, cur, diff, percent, ansiReset)
}

// renderCheapestOptionSummary renders a multi-line description of the option: one
// line per segment showing its airline, departure/arrival times, route, and
// duration, with each layover called out between the segments it connects. The
// airline and time columns are padded so the "·" separators line up across
// segments. It returns "" when the option has no segments to describe.
func renderCheapestOptionSummary(fo search.FlightOptions) string {
	if len(fo.Segments) == 0 {
		return ""
	}

	// Per-segment "6:32pm - 11:32pm(+1)" time ranges and the widths of the
	// airline and time columns, so the separators align.
	times := make([]string, len(fo.Segments))
	var airlineWidth, timeWidth int
	for i, s := range fo.Segments {
		times[i] = formatTimeRange(s.DepartureTime, s.ArrivalTime, s.Overnight)
		airlineWidth = max(airlineWidth, len(s.Airline))
		timeWidth = max(timeWidth, len(times[i]))
	}

	var b strings.Builder
	b.WriteString("```")
	for i, s := range fo.Segments {
		fmt.Fprintf(&b, "\n%-*s · %-*s · %s→%s · %s",
			airlineWidth, s.Airline,
			timeWidth, times[i],
			s.DepartureAirportID, s.ArrivalAirportID,
			formatDuration(s.Duration))

		if i < len(fo.Layovers) {
			l := fo.Layovers[i]
			airport := l.AirportID
			if airport == "" {
				airport = l.Name
			}
			if d := formatDuration(l.Duration); d != "" {
				fmt.Fprintf(&b, "\n|\n| (%s in %s)\n|", d, airport)
			} else {
				fmt.Fprintf(&b, "\n|\n| (Layover in %s)\n|", airport)
			}
		}
	}
	b.WriteString("\n```")
	return b.String()
}

// formatTimeRange renders a segment's departure and arrival times as
// "6:32pm - 11:32pm", appending "(+1)" when the flight is overnight. Each time
// falls back to its raw string if it cannot be parsed.
func formatTimeRange(departure, arrival string, overnight bool) string {
	clock := func(s string) string {
		t, err := time.Parse("2006-01-02 15:04", s)
		if err != nil {
			return s
		}
		return t.Format("3:04pm")
	}

	s := fmt.Sprintf("%s - %s", clock(departure), clock(arrival))
	if overnight {
		s += "(+1)"
	}
	return s
}

// formatDuration renders a count of minutes as "5h 30m", "3h", or "45m",
// returning "" for a non-positive duration.
func formatDuration(minutes int32) string {
	if minutes <= 0 {
		return ""
	}
	h, m := minutes/60, minutes%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

// currencySymbol maps a currency code to its display symbol, falling back to the
// code itself (or "$" when unset).
func currencySymbol(code string) string {
	switch strings.ToUpper(code) {
	case "", "USD":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	case "JPY":
		return "¥"
	default:
		return code
	}
}

type discordPayload struct {
	Embeds []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color,omitempty"`
	Thumbnail   *discordImage  `json:"thumbnail,omitempty"`
	Image       *discordImage  `json:"image,omitempty"`
	Fields      []discordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

type discordImage struct {
	URL string `json:"url"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

func sendDiscordWebhook(ctx context.Context, payload discordPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.FromContext(ctx).DiscordWebhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}
	return nil
}
