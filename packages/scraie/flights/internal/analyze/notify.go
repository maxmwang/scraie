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

// ansiBlock wraps text in a Discord ansi code block tinted with colorCode.
func ansiBlock(colorCode, text string) string {
	return fmt.Sprintf("```ansi\n%s%s%s\n```", colorCode, text, ansiReset)
}

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

	cfg := config.FromContext(ctx)

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

	return sendDiscordWebhook(cfg, discordPayload{Embeds: []discordEmbed{embed}})
}

// buildEmbed renders a summary embed for the itinerary whose cheapest fare moved
// from oldMin (previous search) to newMin (latest search). The title mirrors the
// chart's route-and-dates header, and the body summarizes the price move and
// describes the new cheapest option; the chart is attached separately.
func buildEmbed(it db.Itinerary, oldMin, newMin int32, cheapest search.FlightOptions) discordEmbed {
	dropped := newMin < oldMin
	color := colorPriceRise
	if dropped {
		color = colorPriceDrop
	}

	cur := currencySymbol(it.Currency)

	// Colored summary of how the cheapest fare moved overall, e.g.
	// "$999 → $842 | -$157 (-15.7%)".
	overallColor := ansiRed
	if dropped {
		overallColor = ansiGreen
	}
	diff := newMin - oldMin
	summary := ansiBlock(overallColor, fmt.Sprintf("%s%d → %s%d | %s%s%d (%s)",
		cur, oldMin, cur, newMin,
		signPrefix(diff), cur, abs32(diff), signedPercent(oldMin, newMin)))

	description := summary
	if desc := describeCheapestOption(cheapest); desc != "" {
		description = fmt.Sprintf("%s\n%s", summary, desc)
	}

	return discordEmbed{
		Title:       util.ItineraryToString(it),
		Description: description,
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

// describeCheapestOption renders a multi-line description of the option: one
// line per segment showing its airline, flight number, route, and duration, with
// each layover called out between the segments it connects. The airline and
// flight-number columns are padded so the "·" separators line up across
// segments. It returns "" when the option has no segments to describe.
func describeCheapestOption(fo search.FlightOptions) string {
	if len(fo.Segments) == 0 {
		return ""
	}

	// Widths of the airline and flight-number columns, so the separators align.
	var airlineWidth, flightNumberWidth int
	for _, s := range fo.Segments {
		airlineWidth = max(airlineWidth, len(s.Airline))
		flightNumberWidth = max(flightNumberWidth, len(s.FlightNumber))
	}

	var b strings.Builder
	b.WriteString("**Newest Cheapest First Leg Option:**\n```")
	for i, s := range fo.Segments {
		fmt.Fprintf(&b, "\n%-*s · %-*s · %s→%s · %s",
			airlineWidth, s.Airline,
			flightNumberWidth, s.FlightNumber,
			s.DepartureAirportID, s.ArrivalAirportID,
			formatDuration(s.Duration))

		// The layover between this segment and the next, if any.
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

// signedPercent formats the relative change from old to new as a signed
// percentage, e.g. "+12.5%" or "-9.1%". It returns "0%" when old is zero.
func signedPercent(old, new int32) string {
	if old == 0 {
		return "0%"
	}
	return fmt.Sprintf("%+.1f%%", float64(new-old)/float64(old)*100)
}

// signPrefix returns the sign that precedes n's magnitude ("-" for negative,
// "+" otherwise), so a signed amount can be rendered as e.g. "-$157".
func signPrefix(n int32) string {
	if n < 0 {
		return "-"
	}
	return "+"
}

// abs32 returns the absolute value of n.
func abs32(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
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

func sendDiscordWebhook(cfg config.Config, payload discordPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, cfg.DiscordWebhook, bytes.NewReader(body))
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
