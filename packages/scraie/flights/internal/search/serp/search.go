package serp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
	"github.com/serpapi/serpapi-golang"
)

const (
	dateFormat     = "2006-01-02"
	defaultTimeout = 30 * time.Second
	maxRetries     = 3
)

type FlightType int32

const (
	RoundTrip FlightType = 1
	OneWay    FlightType = 2
	MultiCity FlightType = 3
)

type StopCount int32

const (
	anyStops     StopCount = 0
	nonStop      StopCount = 1
	upToOneStop  StopCount = 2
	upToTwoStops StopCount = 3
)

type TravelClass int32

const (
	economy        TravelClass = 1
	premiumEconomy TravelClass = 2
	business       TravelClass = 3
	first          TravelClass = 4
)

type SerpAPI struct {
	Client  serpapi.SerpApiClient
	Timeout time.Duration
}

func New(APIKey string) SerpAPI {
	setting := serpapi.NewSerpApiClientSetting(APIKey)
	client := serpapi.NewClient(setting)

	return SerpAPI{
		Client:  client,
		Timeout: defaultTimeout,
	}
}

func (s SerpAPI) Search(it db.Itinerary) (search.Result, error) {
	if err := validate(it); err != nil {
		return search.Result{}, err
	}

	p := itineraryToParameter(it, false)

	var raw map[string]interface{}
	var err error
	for attempt := range maxRetries {
		raw, err = s.callWithTimeout(p)
		if err == nil {
			break
		}
		if attempt < maxRetries-1 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}
	}
	if err != nil {
		return search.Result{}, fmt.Errorf("serpapi: %w", err)
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return search.Result{}, fmt.Errorf("marshal response: %w", err)
	}

	var result result
	if err := json.Unmarshal(data, &result); err != nil {
		return search.Result{}, fmt.Errorf("unmarshal response: %w", err)
	}

	return result.toDBResult(it.ID), nil
}

func (s SerpAPI) callWithTimeout(p map[string]string) (map[string]interface{}, error) {
	type callResult struct {
		data map[string]interface{}
		err  error
	}
	ch := make(chan callResult, 1)
	go func() {
		data, err := s.Client.Search(p)
		ch <- callResult{data, err}
	}()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-time.After(s.Timeout):
		return nil, fmt.Errorf("timed out after %v", s.Timeout)
	}
}

func validate(it db.Itinerary) error {
	if it.DepartureID == "" || it.ArrivalID == "" {
		return fmt.Errorf("unset departure/arrival airport id is unsupported")
	}

	if it.Adults < 1 {
		return fmt.Errorf("at least one adult passenger is required")
	}

	if it.Type == int32(MultiCity) {
		return fmt.Errorf("multi city itinerary is unsupported")
	}
	if it.Type != int32(RoundTrip) && it.Type != int32(OneWay) {
		return fmt.Errorf("invalid itinerary type")
	}

	if it.OutboundDate == "" {
		return fmt.Errorf("unset outbound date is unsupported")
	}
	if _, err := time.Parse(dateFormat, it.OutboundDate); err != nil {
		return fmt.Errorf("outbound_date %q must be in YYYY-MM-DD format", it.OutboundDate)
	}
	if it.Type == int32(RoundTrip) && (!it.ReturnDate.Valid || it.ReturnDate.String == "") {
		return fmt.Errorf("unset return date is unsupported")
	}
	if it.ReturnDate.Valid && it.ReturnDate.String != "" {
		if _, err := time.Parse(dateFormat, it.ReturnDate.String); err != nil {
			return fmt.Errorf("return_date %q must be in YYYY-MM-DD format", it.ReturnDate.String)
		}
	}

	if it.TravelClass < int32(economy) || it.TravelClass > int32(first) {
		return fmt.Errorf("invalid travel class value")
	}

	if it.IncludeAirlines.Valid && it.ExcludeAirlines.Valid {
		return fmt.Errorf("include_airlines and exclude_airlines cannot be used together")
	}

	if it.Bags > 0 {
		if it.Bags > it.Adults+it.Children+it.InfantsInSeat {
			return fmt.Errorf("bags (%d) exceeds passengers with carry-on allowance (%d)", it.Bags, it.Adults+it.Children+it.InfantsInSeat)
		}
	}

	if it.Stops < int32(anyStops) || it.Stops > int32(upToTwoStops) {
		return fmt.Errorf("invalid stop count")
	}
	if it.Type != int32(RoundTrip) && it.ReturnTimes.Valid {
		return fmt.Errorf("return_times can only be used with roundtrip")
	}
	return nil
}

// itineraryToParameter converts an db.Itinerary object to a map[string]string
// representing a parameter for SerpAPI. Setting filter to false will ignore
// all fields except:
//
//	 	DepartureID
//		ArrivalID
//		Type
//		OutboundDate
//		ReturnDate
//		TravelClass
//		ExcludeBasic
//
// Data should be filtered when viewed later.
func itineraryToParameter(it db.Itinerary, filter bool) map[string]string {
	if !filter {
		p := map[string]string{
			"departure_id": it.DepartureID,
			"arrival_id":   it.ArrivalID,

			"gl":       it.Gl,
			"hl":       it.Hl,
			"currency": it.Currency,

			"type":          fmt.Sprintf("%d", it.Type),
			"outbound_date": it.OutboundDate,
			"travel_class":  fmt.Sprintf("%d", it.TravelClass),
			"show_hidden":   "true",
			"exclude_basic": fmt.Sprintf("%t", it.ExcludeBasic),
			"deep_search":   "true",

			"sort_by": "1",

			"engine": "google_flights",
		}

		if it.ReturnDate.Valid {
			p["return_date"] = it.ReturnDate.String
		}
	}
	p := map[string]string{
		"departure_id": it.DepartureID,
		"arrival_id":   it.ArrivalID,

		"gl":       it.Gl,
		"hl":       it.Hl,
		"currency": it.Currency,

		"type":          fmt.Sprintf("%d", it.Type),
		"outbound_date": it.OutboundDate,
		"travel_class":  fmt.Sprintf("%d", it.TravelClass),
		"show_hidden":   "true",
		"exclude_basic": fmt.Sprintf("%t", it.ExcludeBasic),
		"deep_search":   "true",

		"adults":          fmt.Sprintf("%d", it.Adults),
		"children":        fmt.Sprintf("%d", it.Children),
		"infants_in_seat": fmt.Sprintf("%d", it.InfantsInSeat),
		"infants_on_lap":  fmt.Sprintf("%d", it.InfantsOnLap),

		"sort_by": "1",

		"stops": fmt.Sprintf("%d", it.Stops),
		"bags":  fmt.Sprintf("%d", it.Bags),

		"engine": "google_flights",
	}

	if it.ReturnDate.Valid {
		p["return_date"] = it.ReturnDate.String
	}
	if it.ExcludeAirlines.Valid {
		p["exclude_airlines"] = it.ExcludeAirlines.String
	}
	if it.IncludeAirlines.Valid {
		p["include_airlines"] = it.IncludeAirlines.String
	}
	if it.MaxPrice.Valid {
		p["max_price"] = fmt.Sprintf("%d", it.MaxPrice.Int32)
	}
	if it.OutboundTimes.Valid {
		p["outbound_times"] = it.OutboundTimes.String
	}
	if it.ReturnTimes.Valid {
		p["return_times"] = it.ReturnTimes.String
	}
	if it.Emissions.Valid {
		p["emissions"] = fmt.Sprintf("%d", it.Emissions.Int32)
	}
	if it.LayoverDuration.Valid {
		p["layover_duration"] = it.LayoverDuration.String
	}
	if it.ExcludeConns.Valid {
		p["exclude_conns"] = it.ExcludeConns.String
	}
	if it.MaxDuration.Valid {
		p["max_duration"] = fmt.Sprintf("%d", it.MaxDuration.Int32)
	}

	return p
}
