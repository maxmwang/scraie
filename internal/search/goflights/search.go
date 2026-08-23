// Package goflights searches Google Flights directly, through
// github.com/maxmwang/goflights, with no API key. It reads the same
// Itineraries rows as the serp package and produces the same db records, so
// the two are interchangeable as a search.Searcher.
//
// The itinerary columns are SerpAPI's parameter set. Those that only ask for
// more results are ignored (show_hidden, deep_search, sort_by), as are the
// exclusion filters (exclude_airlines, exclude_conns), which validate logs a
// warning for rather than rejecting. Airport display names and airline
// logo URLs are absent from the booking tokens this decodes, so name columns
// hold the IATA code and the logo is empty.
package goflights

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	gf "github.com/maxmwang/goflights"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
	"github.com/maxmwang/scraie/flights/internal/search/serp"
)

const (
	dateFormat     = time.DateOnly
	defaultTimeout = 30 * time.Second
	maxRetries     = 3
)

const (
	stopsAny     int32 = 0
	stopsNonStop int32 = 1
	stopsUpToOne int32 = 2
	stopsUpToTwo int32 = 3
)

const (
	economy int32 = 1
	first   int32 = 4
)

const emissionsLessOnly int32 = 1

// client searches Google Flights with goflights' own HTTP client and headers.
// timeout bounds a single attempt, not the retries around it.
type client struct {
	timeout time.Duration
}

var defaultClient = client{timeout: defaultTimeout}

func New() search.Searcher {
	return defaultClient
}

// Search runs the itinerary against Google Flights and returns its options as
// db records. A round trip returns outbound options, each priced as the whole
// round trip, which is what SerpAPI returns for the same row.
func (c client) Search(it db.Itinerary) (search.Result, error) {
	return c.SearchContext(context.Background(), it)
}

// SearchContext is Search bounded by ctx as well as by Timeout.
func (c client) SearchContext(ctx context.Context, it db.Itinerary) (search.Result, error) {
	if err := validate(it); err != nil {
		return search.Result{}, err
	}

	req, err := itineraryToRequest(it)
	if err != nil {
		return search.Result{}, err
	}
	// Render now: a malformed itinerary should fail here, not three times.
	if _, err := req.URL(); err != nil {
		return search.Result{}, fmt.Errorf("goflights: %w", err)
	}

	var options []gf.FlightOption
	for attempt := range maxRetries {
		options, err = c.execute(ctx, req)
		if errors.Is(err, gf.ErrPartialResults) {
			// Google renders the full list only for parties of four or fewer
			// with no infants; retrying returns the same short list.
			log.Warn().Int64("itinerary_id", it.ID).
				Msg(fmt.Sprintf("google deferred some results, got %d option(s)", len(options)))
			err = nil
		}
		// A search that matched nothing is not worth retrying either.
		if err == nil || errors.Is(err, gf.ErrNoFlights) || ctx.Err() != nil {
			break
		}
		if attempt < maxRetries-1 {
			select {
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			case <-ctx.Done():
			}
		}
	}
	if err != nil {
		return search.Result{}, fmt.Errorf("goflights: %w", err)
	}

	return toResult(it, options), nil
}

func (c client) execute(ctx context.Context, req *gf.Request) ([]gf.FlightOption, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	return req.Execute(ctx)
}

// validate reports the itinerary fields this searcher cannot honour. The
// exclusion filters are the exception: they are warned about and dropped, so a
// result can hold a flight the itinerary asked to exclude, at a fare that then
// reads as a genuine price drop.
func validate(it db.Itinerary) error {
	if it.DepartureID == "" || it.ArrivalID == "" {
		return errors.New("unset departure/arrival airport id is unsupported")
	}
	for _, code := range append(airports(it.DepartureID), airports(it.ArrivalID)...) {
		if len(code) != 3 {
			return fmt.Errorf("airport %q: only three-letter codes are supported", code)
		}
	}

	if it.Adults < 1 {
		return errors.New("at least one adult passenger is required")
	}

	if it.Type == int32(serp.MultiCity) {
		return errors.New("multi city itinerary is unsupported")
	}
	if it.Type != int32(serp.RoundTrip) && it.Type != int32(serp.OneWay) {
		return errors.New("invalid itinerary type")
	}

	if it.OutboundDate == "" {
		return errors.New("unset outbound date is unsupported")
	}
	if _, err := time.Parse(dateFormat, it.OutboundDate); err != nil {
		return fmt.Errorf("outbound_date %q must be in YYYY-MM-DD format", it.OutboundDate)
	}
	if it.Type == int32(serp.RoundTrip) && !text(it.ReturnDate) {
		return errors.New("unset return date is unsupported")
	}
	if text(it.ReturnDate) {
		if _, err := time.Parse(dateFormat, it.ReturnDate.String); err != nil {
			return fmt.Errorf("return_date %q must be in YYYY-MM-DD format", it.ReturnDate.String)
		}
	}

	if it.TravelClass < economy || it.TravelClass > first {
		return errors.New("invalid travel class value")
	}

	// Google Flights takes allowlists, never blocklists.
	if text(it.ExcludeAirlines) {
		log.Warn().Int64("itinerary_id", it.ID).
			Msg("exclude_airlines is unsupported and was dropped, use include_airlines instead")
	}
	if text(it.ExcludeConns) {
		log.Warn().Int64("itinerary_id", it.ID).
			Msg("exclude_conns is unsupported and was dropped, use layover_duration or stops instead")
	}

	if it.Bags > 0 && it.Bags > it.Adults+it.Children+it.InfantsInSeat {
		return fmt.Errorf("bags (%d) exceeds passengers with carry-on allowance (%d)",
			it.Bags, it.Adults+it.Children+it.InfantsInSeat)
	}

	if it.Stops < stopsAny || it.Stops > stopsUpToTwo {
		return errors.New("invalid stop count")
	}

	if it.Type != int32(serp.RoundTrip) && text(it.ReturnTimes) {
		return errors.New("return_times can only be used with roundtrip")
	}

	if it.Emissions.Valid && it.Emissions.Int32 != emissionsLessOnly {
		return fmt.Errorf("invalid emissions value %d", it.Emissions.Int32)
	}

	return nil
}

// itineraryToRequest builds the search, applying every leg filter to both
// legs of a round trip, as SerpAPI reads the same columns.
func itineraryToRequest(it db.Itinerary) (*gf.Request, error) {
	outbound := gf.NewFlightInfo().
		DepartureDateStr(it.OutboundDate).
		From(airports(it.DepartureID)...).
		To(airports(it.ArrivalID)...)
	if err := applyLegFilters(outbound, it, it.OutboundTimes); err != nil {
		return nil, err
	}
	legs := []*gf.FlightInfo{outbound}

	if it.Type == int32(serp.RoundTrip) {
		inbound := gf.NewFlightInfo().
			DepartureDateStr(it.ReturnDate.String).
			From(airports(it.ArrivalID)...).
			To(airports(it.DepartureID)...)
		if err := applyLegFilters(inbound, it, it.ReturnTimes); err != nil {
			return nil, err
		}
		legs = append(legs, inbound)
	}

	req := gf.NewRequest().
		Flights(legs...).
		Adults(int(it.Adults)).
		Children(int(it.Children)).
		InfantsInSeat(int(it.InfantsInSeat)).
		InfantsOnLap(int(it.InfantsOnLap)).
		TripType(gf.TripType(it.Type)).
		Class(gf.Class(it.TravelClass))

	// An empty locale would be sent as a malformed code, not left off.
	if it.Currency != "" {
		req.Currency(it.Currency)
	}
	if it.Hl != "" {
		req.Language(it.Hl)
	}
	if it.Gl != "" {
		req.Region(it.Gl)
	}

	// SerpAPI's bags counts carry-on bags only.
	if it.Bags > 0 {
		req.CarryOnBag(it.Bags)
	}
	if it.MaxPrice.Valid && it.MaxPrice.Int32 > 0 {
		req.MaxPrice(it.MaxPrice.Int32)
	}
	if it.ExcludeBasic {
		req.ExcludeBasicEconomy()
	}

	return req, nil
}

// applyLegFilters applies the per-leg columns to one leg; times is that leg's
// own outbound_times or return_times.
func applyLegFilters(fi *gf.FlightInfo, it db.Itinerary, times pgtype.Text) error {
	// stops counts from one where MaxStops counts from zero; "any" is unset.
	switch it.Stops {
	case stopsNonStop:
		fi.MaxStops(0)
	case stopsUpToOne:
		fi.MaxStops(1)
	case stopsUpToTwo:
		fi.MaxStops(2)
	}

	if text(it.IncludeAirlines) {
		fi.Airlines(splitList(it.IncludeAirlines.String)...)
	}
	if it.MaxDuration.Valid && it.MaxDuration.Int32 > 0 {
		fi.MaxDuration(time.Duration(it.MaxDuration.Int32) * time.Minute)
	}
	if it.Emissions.Valid && it.Emissions.Int32 == emissionsLessOnly {
		fi.LessEmissions()
	}

	if err := applyLayoverDuration(fi, it.LayoverDuration); err != nil {
		return err
	}
	return applyTimes(fi, times)
}

// applyTimes applies a SerpAPI times filter: earliest and latest departure
// hour, optionally followed by arrival hours, as in "4,18" or "4,18,3,19".
func applyTimes(fi *gf.FlightInfo, v pgtype.Text) error {
	if !text(v) {
		return nil
	}
	hours, err := hourList(v.String, 2, 4)
	if err != nil {
		return fmt.Errorf("times %q: %w", v.String, err)
	}

	fi.EarliestDepartureHour(hours[0]).LatestDepartureHour(hours[1])
	if len(hours) == 4 {
		fi.EarliestArrivalHour(hours[2]).LatestArrivalHour(hours[3])
	}
	return nil
}

// applyLayoverDuration applies a SerpAPI layover_duration filter, "90,330":
// the shortest and longest acceptable connection in minutes.
func applyLayoverDuration(fi *gf.FlightInfo, v pgtype.Text) error {
	if !text(v) {
		return nil
	}
	bounds, err := hourList(v.String, 2, 2)
	if err != nil {
		return fmt.Errorf("layover_duration %q: %w", v.String, err)
	}

	// Zero means unbounded, which goflights will not accept as a duration.
	if bounds[0] > 0 {
		fi.MinLayover(time.Duration(bounds[0]) * time.Minute)
	}
	if bounds[1] > 0 {
		fi.MaxLayover(time.Duration(bounds[1]) * time.Minute)
	}
	return nil
}

// hourList parses a comma separated list of whole numbers, accepting either
// short or long many. Range checking is left to the builders, which name the
// field an impossible hour came from.
func hourList(s string, short, long int) ([]int32, error) {
	fields := strings.Split(s, ",")
	if len(fields) != short && len(fields) != long {
		return nil, fmt.Errorf("want %d or %d comma separated values, got %d", short, long, len(fields))
	}

	out := make([]int32, len(fields))
	for i, f := range fields {
		n, err := strconv.ParseInt(strings.TrimSpace(f), 10, 32)
		if err != nil {
			return nil, fmt.Errorf("value %d: %w", i, err)
		}
		out[i] = int32(n)
	}
	return out, nil
}

// airports splits a SerpAPI airport column: one IATA code, or several
// separated by commas, as in "CDG,ORY".
func airports(s string) []string {
	codes := splitList(s)
	for i, c := range codes {
		codes[i] = strings.ToUpper(c)
	}
	return codes
}

func splitList(s string) []string {
	out := make([]string, 0, 1)
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// text reports whether an optional text column is set; NULL and "" are alike.
func text(v pgtype.Text) bool {
	return v.Valid && v.String != ""
}
