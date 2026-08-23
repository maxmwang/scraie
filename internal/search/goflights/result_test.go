package goflights

import (
	"math"
	"testing"
	"time"

	gf "github.com/maxmwang/goflights"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
)

// pacific and eastern stand in for the fixed offsets the booking token carries
// on each segment's timestamps.
var (
	pacific = time.FixedZone("PDT", -7*60*60)
	eastern = time.FixedZone("EDT", -4*60*60)
)

func TestToResult(t *testing.T) {
	it := itinerary()

	// SFO 23:28 -> ORD 05:32 next day, connect, ORD 07:00 -> JFK 10:15.
	option := gf.FlightOption{
		Price:         41100,
		DecimalDigits: 2,
		Currency:      "USD",
		Segments: []gf.FlightSegment{
			{
				FromAirport:   "SFO",
				ToAirport:     "ORD",
				DepartureTime: time.Date(2026, 9, 1, 23, 28, 0, 0, pacific),
				ArrivalTime:   time.Date(2026, 9, 2, 5, 32, 0, 0, eastern),
				Airline:       "UA",
				FlightNumber:  "1234",
				Class:         "Economy",
				Aircraft:      "321",
			},
			{
				FromAirport:   "ORD",
				ToAirport:     "JFK",
				DepartureTime: time.Date(2026, 9, 2, 7, 0, 0, 0, eastern),
				ArrivalTime:   time.Date(2026, 9, 2, 10, 15, 0, 0, eastern),
				Airline:       "UA",
				FlightNumber:  "56",
				Class:         "Economy",
				Aircraft:      "738",
			},
		},
	}

	got := toResult(it, []gf.FlightOption{option})
	if len(got.Options) != 1 {
		t.Fatalf("len(Options) = %d, want 1", len(got.Options))
	}
	fo := got.Options[0]

	wantOption := db.Option{
		ItineraryID: 7,
		// 23:28 PDT to 10:15 EDT the next day.
		TotalDuration: 467,
		Type:          "Round trip",
	}
	// Price holds a *big.Int, so it is checked on its own and cleared before
	// the rest of the struct is compared with ==.
	gotOption := fo.Option
	if got := fareText(t, gotOption.Price); got != "411.00" {
		t.Errorf("Option.Price = %s, want 411.00", got)
	}
	gotOption.Price = pgtype.Numeric{}
	if gotOption != wantOption {
		t.Errorf("Option = %+v, want %+v", gotOption, wantOption)
	}

	wantSegments := []db.Segment{
		{
			DepartureAirportName: "SFO",
			DepartureAirportID:   "SFO",
			DepartureTime:        "2026-09-01 23:28",
			ArrivalAirportName:   "ORD",
			ArrivalAirportID:     "ORD",
			ArrivalTime:          "2026-09-02 05:32",
			Duration:             184,
			Airplane:             "321",
			Airline:              "UA",
			TravelClass:          "Economy",
			FlightNumber:         "UA 1234",
			Overnight:            true,
		},
		{
			DepartureAirportName: "ORD",
			DepartureAirportID:   "ORD",
			DepartureTime:        "2026-09-02 07:00",
			ArrivalAirportName:   "JFK",
			ArrivalAirportID:     "JFK",
			ArrivalTime:          "2026-09-02 10:15",
			Duration:             195,
			Airplane:             "738",
			Airline:              "UA",
			TravelClass:          "Economy",
			FlightNumber:         "UA 56",
		},
	}
	if len(fo.Segments) != len(wantSegments) {
		t.Fatalf("len(Segments) = %d, want %d", len(fo.Segments), len(wantSegments))
	}
	for i, want := range wantSegments {
		if fo.Segments[i] != want {
			t.Errorf("Segments[%d] = %+v, want %+v", i, fo.Segments[i], want)
		}
	}

	// One layover for two segments, the gap between the two at ORD.
	wantLayovers := []db.Layover{
		{Duration: 88, Name: "ORD", AirportID: "ORD"},
	}
	if len(fo.Layovers) != len(wantLayovers) {
		t.Fatalf("len(Layovers) = %d, want %d", len(fo.Layovers), len(wantLayovers))
	}
	for i, want := range wantLayovers {
		if fo.Layovers[i] != want {
			t.Errorf("Layovers[%d] = %+v, want %+v", i, fo.Layovers[i], want)
		}
	}
}

func TestToResultOneWayNonstop(t *testing.T) {
	it := itinerary()
	it.Type = 2
	it.ReturnDate = pgtype.Text{}

	got := toResult(it, []gf.FlightOption{{
		Price:         19920,
		DecimalDigits: 2,
		Segments: []gf.FlightSegment{{
			FromAirport:   "SFO",
			ToAirport:     "JFK",
			DepartureTime: time.Date(2026, 9, 1, 8, 0, 0, 0, pacific),
			ArrivalTime:   time.Date(2026, 9, 1, 16, 30, 0, 0, eastern),
			Airline:       "AS",
			FlightNumber:  "12",
			Class:         "Economy",
		}},
	}})

	if len(got.Options) != 1 {
		t.Fatalf("len(Options) = %d, want 1", len(got.Options))
	}
	fo := got.Options[0]
	if fo.Option.Type != "One way" {
		t.Errorf("Option.Type = %q, want %q", fo.Option.Type, "One way")
	}
	if got := fareText(t, fo.Option.Price); got != "199.20" {
		t.Errorf("Option.Price = %s, want 199.20", got)
	}
	// 8:00 PDT to 16:30 EDT is 5h30m gate to gate.
	if fo.Option.TotalDuration != 330 {
		t.Errorf("Option.TotalDuration = %d, want 330", fo.Option.TotalDuration)
	}
	if len(fo.Layovers) != 0 {
		t.Errorf("len(Layovers) = %d, want 0", len(fo.Layovers))
	}
	if fo.Segments[0].Overnight {
		t.Error("Segments[0].Overnight = true, want false")
	}
}

// TestToResultSkipsEmptyOption covers an option with no segments, which cannot
// be described and would otherwise index out of range.
func TestToResultSkipsEmptyOption(t *testing.T) {
	got := toResult(itinerary(), []gf.FlightOption{{Price: 100, DecimalDigits: 2}})
	if len(got.Options) != 0 {
		t.Errorf("len(Options) = %d, want 0", len(got.Options))
	}
}

func TestToResultOvernightLayover(t *testing.T) {
	got := toResult(itinerary(), []gf.FlightOption{{
		DecimalDigits: 2,
		Segments: []gf.FlightSegment{
			{
				FromAirport:   "SFO",
				ToAirport:     "ORD",
				DepartureTime: time.Date(2026, 9, 1, 18, 0, 0, 0, pacific),
				ArrivalTime:   time.Date(2026, 9, 1, 23, 55, 0, 0, eastern),
			},
			{
				FromAirport:   "ORD",
				ToAirport:     "JFK",
				DepartureTime: time.Date(2026, 9, 2, 6, 30, 0, 0, eastern),
				ArrivalTime:   time.Date(2026, 9, 2, 9, 45, 0, 0, eastern),
			},
		},
	}})

	l := got.Options[0].Layovers[0]
	if !l.Overnight {
		t.Error("Layovers[0].Overnight = false, want true")
	}
	if l.Duration != 395 {
		t.Errorf("Layovers[0].Duration = %d, want 395", l.Duration)
	}
}

// fareText renders a stored fare the way Postgres would write the NUMERIC, so
// both the digits and the scale are asserted at once.
func fareText(t *testing.T, p pgtype.Numeric) string {
	t.Helper()
	v, err := p.Value()
	if err != nil {
		t.Fatalf("Price.Value() error = %v", err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("Price.Value() = %v (%T), want string", v, v)
	}
	return s
}

func TestFare(t *testing.T) {
	tests := []struct {
		name          string
		amount        int64
		decimalDigits int
		want          string
	}{
		{name: "dollars and cents", amount: 41100, decimalDigits: 2, want: "411.00"},
		{name: "cents are kept rather than rounded", amount: 41149, decimalDigits: 2, want: "411.49"},
		{name: "yen has no minor units", amount: 62000, decimalDigits: 0, want: "62000"},
		{name: "four minor units", amount: 4110000, decimalDigits: 4, want: "411.0000"},
		{name: "negative digits are treated as none", amount: 411, decimalDigits: -1, want: "411"},
		{name: "absurd digits are capped", amount: 4110000, decimalDigits: 30, want: "411.0000"},
		{name: "no overflow at the top of int64", amount: math.MaxInt64, decimalDigits: 0, want: "9223372036854775807"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fareText(t, fare(tt.amount, tt.decimalDigits)); got != tt.want {
				t.Errorf("fare(%d, %d) = %s, want %s", tt.amount, tt.decimalDigits, got, tt.want)
			}
		})
	}
}
