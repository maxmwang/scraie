package analyze

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/maxmwang/scraie/flights/internal/config"
	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
	"github.com/maxmwang/scraie/flights/internal/util"
)

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

const NumDaysToPlot = 60

// constructDailyCheapestOptions returns a sorted list containing the cheapest
// option from each distinct timestamp. Since a single scheduled function run
// will reuse a single timestamp for all scraped options, we expect about 1
// timestamp per day shared with many options.
func constructDailyCheapestOptions(options []db.Option) []db.Option {
	m := make(map[pgtype.Timestamptz]db.Option)

	for _, opt := range options {
		if minOpt, ok := m[opt.SearchedAt]; !ok || opt.Price < minOpt.Price {
			m[opt.SearchedAt] = opt
		}
	}

	s := make([]db.Option, 0, len(m))
	for _, opt := range m {
		s = append(s, opt)
	}
	slices.SortFunc(s, func(a db.Option, b db.Option) int {
		return a.SearchedAt.Time.Compare(b.SearchedAt.Time)
	})

	return s
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

	since30Days := pgtype.Timestamptz{
		// +1hour in case of small function run scheduling variance
		Time:  time.Now().AddDate(0, 0, -NumDaysToPlot).Add(-time.Hour),
		Valid: true,
	}
	options, err := dbc.GetOptionsSince(ctx, db.GetOptionsSinceParams{
		ItineraryID: it.ID,
		SearchedAt:  since30Days,
	})
	if err != nil {
		return fmt.Errorf("GetOptionsSince: %w", err)
	}
	if len(options) < 2 {
		// Nothing has been searched recently: nothing to compare or report.
		return nil
	}
	cheapestOptions := constructDailyCheapestOptions(options)

	checks := makeChecks(cheapestOptions)
	if !checks.any() {
		return nil
	}

	// Reassemble the latest cheapest option with its segments and layovers so the
	// embed can describe the flight (airline, flight numbers, duration, stops).
	latestCheapestOption := cheapestOptions[len(cheapestOptions)-1]
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

	embed, err := buildEmbed(it, cheapestOptions, cheapest, checks)
	if err != nil {
		return fmt.Errorf("buildEmbed: failed to build embed message")
	}

	return sendDiscordWebhook(ctx, discordPayload{Embeds: []discordEmbed{embed}})
}

// buildEmbed renders a summary embed for the itinerary whose cheapest fare moved
// from oldMin (previous search) to newMin (latest search). The title mirrors the
// chart's route-and-dates header, and the body summarizes the price move and
// describes the new cheapest option; the chart is attached separately.
func buildEmbed(it db.Itinerary, options []db.Option, cheapest search.FlightOptions, checks checkResults) (discordEmbed, error) {
	if len(options) < 2 {
		return discordEmbed{}, fmt.Errorf("unexpected len(opions) < 2")
	}

	oldMin := options[len(options)-2].Price
	newMin := options[len(options)-1].Price
	color := colorPriceRise
	if newMin < oldMin {
		color = colorPriceDrop
	}

	itinerarySummary := renderItinerarySummary(it)

	priceNotification := renderPriceNotification(currencySymbol(it.Currency), newMin, checks)

	cheapestOptionSummary := renderCheapestOptionSummary(cheapest)

	fields := []discordField{
		{Name: "Itinerary Summary", Value: itinerarySummary},
		{Name: "Price Notification", Value: priceNotification},
		{Name: "Newest Cheapest Option's First Leg", Value: cheapestOptionSummary},
	}

	embedMsg := discordEmbed{
		Title:     util.ItineraryToString(it),
		Color:     color,
		Fields:    fields,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	chart, err := buildDailyMinimumPriceChartURL(it, options, NumDaysToPlot)
	if err != nil || chart == "" {
		// error is non-fatal: log error and continue sending message
		log.Error().Err(err).Msg("failed to build buildDailyMinimumPriceChartURL chart")
	} else {
		embedMsg.Image = &discordImage{URL: chart}
	}

	return embedMsg, nil
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

	field := func(label, value string) {
		lines = append(lines, fmt.Sprintf("__%s__: %s", label, value))
	}

	field("Departure", it.DepartureID)
	field("Arrival", it.ArrivalID)
	field("Outbound", it.OutboundDate)
	if it.ReturnDate.Valid && it.ReturnDate.String != "" {
		field("Return", it.ReturnDate.String)
	}
	if it.Description.Valid && it.Description.String != "" {
		field("Description", it.Description.String)
	}

	lines = append(lines, "")

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

// renderPriceNotification renders a colored summary of how the cheapest fare moved
// from oldMin to newMin, e.g. "$999 → $842 | -$157 (-15.7%)". The block is
// tinted green when the fare dropped and red when it rose. cur is the currency
// symbol to prefix each amount with.
func renderPriceNotification(cur string, newMin int32, checks checkResults) string {
	s := strings.Builder{}

	s.WriteString("```ansi\n")

	if checks.near7DayMinimum.pass {
		fmt.Fprintf(&s, "Near 7 Day Minimum: %s%d → %s%d\n",
			cur, checks.near7DayMinimum.prev7DayMinimum, cur, newMin)
	}

	if checks.priceMovement.pass {
		oldMin := checks.priceMovement.prev
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

		fmt.Fprintf(&s, "Price Movement: %s%s%d → %s%d | %s%s%d (%s)%s\n",
			color, cur, oldMin, cur, newMin, sign, cur, diff, percent, ansiReset)
	}

	s.WriteString("```")

	return s.String()
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
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord webhook returned status %d, %s", resp.StatusCode, body)
	}
	return nil
}
