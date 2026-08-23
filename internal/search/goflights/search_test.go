package goflights

import (
	"testing"
	"time"

	gf "github.com/maxmwang/goflights"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
)

func txt(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func i4(n int32) pgtype.Int4 { return pgtype.Int4{Int32: n, Valid: true} }

// itinerary builds a valid round trip row, which each test then varies.
func itinerary() db.Itinerary {
	return db.Itinerary{
		ID:           7,
		DepartureID:  "SFO",
		ArrivalID:    "JFK",
		Type:         1,
		OutboundDate: "2026-09-01",
		ReturnDate:   txt("2026-09-08"),
		TravelClass:  1,
		Adults:       1,
		Gl:           "us",
		Hl:           "en",
		Currency:     "USD",
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*db.Itinerary)
		wantErr bool
	}{
		{name: "round trip", mutate: func(*db.Itinerary) {}},
		{name: "one way", mutate: func(it *db.Itinerary) {
			it.Type = 2
			it.ReturnDate = pgtype.Text{}
		}},
		{name: "every supported filter", mutate: func(it *db.Itinerary) {
			it.Stops = 2
			it.IncludeAirlines = txt("UA,AS")
			it.Bags = 1
			it.MaxPrice = i4(900)
			it.OutboundTimes = txt("6,18")
			it.ReturnTimes = txt("6,18,4,22")
			it.Emissions = i4(1)
			it.LayoverDuration = txt("60,300")
			it.MaxDuration = i4(900)
			it.ExcludeBasic = true
		}},
		{name: "multiple airports per end", mutate: func(it *db.Itinerary) {
			it.DepartureID = "SFO,OAK"
			it.ArrivalID = "JFK,EWR"
		}},

		{name: "no departure", mutate: func(it *db.Itinerary) { it.DepartureID = "" }, wantErr: true},
		{name: "kgmid location id", mutate: func(it *db.Itinerary) { it.DepartureID = "/m/0vzm" }, wantErr: true},
		{name: "no adults", mutate: func(it *db.Itinerary) { it.Adults = 0 }, wantErr: true},
		{name: "multi city", mutate: func(it *db.Itinerary) { it.Type = 3 }, wantErr: true},
		{name: "unknown type", mutate: func(it *db.Itinerary) { it.Type = 9 }, wantErr: true},
		{name: "no outbound date", mutate: func(it *db.Itinerary) { it.OutboundDate = "" }, wantErr: true},
		{name: "malformed outbound date", mutate: func(it *db.Itinerary) { it.OutboundDate = "09/01/2026" }, wantErr: true},
		{name: "round trip without return date", mutate: func(it *db.Itinerary) { it.ReturnDate = pgtype.Text{} }, wantErr: true},
		{name: "malformed return date", mutate: func(it *db.Itinerary) { it.ReturnDate = txt("09/08/2026") }, wantErr: true},
		{name: "unknown travel class", mutate: func(it *db.Itinerary) { it.TravelClass = 5 }, wantErr: true},
		{name: "more bags than passengers", mutate: func(it *db.Itinerary) { it.Bags = 2 }, wantErr: true},
		{name: "unknown stop count", mutate: func(it *db.Itinerary) { it.Stops = 4 }, wantErr: true},
		{name: "return times on a one way", mutate: func(it *db.Itinerary) {
			it.Type = 2
			it.ReturnDate = pgtype.Text{}
			it.ReturnTimes = txt("6,18")
		}, wantErr: true},
		{name: "unknown emissions value", mutate: func(it *db.Itinerary) { it.Emissions = i4(2) }, wantErr: true},

		// NULL and "" both mean unset, and neither is a filter to reject.
		{name: "empty exclude airlines", mutate: func(it *db.Itinerary) { it.ExcludeAirlines = txt("") }},

		// The exclusions are warned about and dropped, not rejected.
		{name: "exclude airlines", mutate: func(it *db.Itinerary) { it.ExcludeAirlines = txt("UA") }},
		{name: "exclude connections", mutate: func(it *db.Itinerary) { it.ExcludeConns = txt("ORD") }},
		{name: "include and exclude airlines", mutate: func(it *db.Itinerary) {
			it.IncludeAirlines = txt("AS")
			it.ExcludeAirlines = txt("UA")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := itinerary()
			tt.mutate(&it)

			err := validate(it)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			// A row validate accepts has to survive being built, or the
			// failure only shows up against the live endpoint.
			req, err := itineraryToRequest(it)
			if err != nil {
				t.Fatalf("itineraryToRequest() error = %v", err)
			}
			if _, err := req.URL(); err != nil {
				t.Fatalf("URL() error = %v", err)
			}
		})
	}
}

// TestItineraryToRequest checks the mapping against the same search written
// out by hand with the goflights builders. Both are compared as rendered URLs,
// the tfs parameter being the request itself.
func TestItineraryToRequest(t *testing.T) {
	tests := []struct {
		name string
		it   db.Itinerary
		want *gf.Request
	}{
		{
			name: "bare round trip",
			it:   itinerary(),
			want: gf.NewRequest().
				Flights(
					gf.NewFlightInfo().DepartureDateStr("2026-09-01").From("SFO").To("JFK"),
					gf.NewFlightInfo().DepartureDateStr("2026-09-08").From("JFK").To("SFO"),
				).
				Adults(1).
				TripType(gf.TripTypeRoundTrip).
				Class(gf.ClassEconomy).
				Currency("USD").Language("en").Region("us"),
		},
		{
			name: "one way, no return leg",
			it: func() db.Itinerary {
				it := itinerary()
				it.Type = 2
				it.ReturnDate = pgtype.Text{}
				it.TravelClass = 3
				it.Children = 2
				return it
			}(),
			want: gf.NewRequest().
				Flights(gf.NewFlightInfo().DepartureDateStr("2026-09-01").From("SFO").To("JFK")).
				Adults(1).Children(2).
				TripType(gf.TripTypeOneWay).
				Class(gf.ClassBusiness).
				Currency("USD").Language("en").Region("us"),
		},
		{
			name: "leg filters reach both legs",
			it: func() db.Itinerary {
				it := itinerary()
				it.Stops = 2 // up to one stop
				it.IncludeAirlines = txt("UA, AS")
				it.LayoverDuration = txt("60,300")
				it.MaxDuration = i4(900)
				it.Emissions = i4(1)
				it.OutboundTimes = txt("6,18")
				it.ReturnTimes = txt("7,19,4,22")
				return it
			}(),
			want: gf.NewRequest().
				Flights(
					gf.NewFlightInfo().DepartureDateStr("2026-09-01").From("SFO").To("JFK").
						MaxStops(1).Airlines("UA", "AS").
						MinLayover(60*time.Minute).MaxLayover(300*time.Minute).
						MaxDuration(900*time.Minute).LessEmissions().
						EarliestDepartureHour(6).LatestDepartureHour(18),
					gf.NewFlightInfo().DepartureDateStr("2026-09-08").From("JFK").To("SFO").
						MaxStops(1).Airlines("UA", "AS").
						MinLayover(60*time.Minute).MaxLayover(300*time.Minute).
						MaxDuration(900*time.Minute).LessEmissions().
						EarliestDepartureHour(7).LatestDepartureHour(19).
						EarliestArrivalHour(4).LatestArrivalHour(22),
				).
				Adults(1).
				TripType(gf.TripTypeRoundTrip).
				Class(gf.ClassEconomy).
				Currency("USD").Language("en").Region("us"),
		},
		{
			name: "trip wide filters",
			it: func() db.Itinerary {
				it := itinerary()
				it.Adults = 2
				it.InfantsOnLap = 1
				it.Bags = 2
				it.MaxPrice = i4(900)
				it.ExcludeBasic = true
				return it
			}(),
			want: gf.NewRequest().
				Flights(
					gf.NewFlightInfo().DepartureDateStr("2026-09-01").From("SFO").To("JFK"),
					gf.NewFlightInfo().DepartureDateStr("2026-09-08").From("JFK").To("SFO"),
				).
				Adults(2).InfantsOnLap(1).
				TripType(gf.TripTypeRoundTrip).
				Class(gf.ClassEconomy).
				Currency("USD").Language("en").Region("us").
				CarryOnBag(2).MaxPrice(900).ExcludeBasicEconomy(),
		},
		{
			name: "nonstop only",
			it: func() db.Itinerary {
				it := itinerary()
				it.Stops = 1
				return it
			}(),
			want: gf.NewRequest().
				Flights(
					gf.NewFlightInfo().DepartureDateStr("2026-09-01").From("SFO").To("JFK").MaxStops(0),
					gf.NewFlightInfo().DepartureDateStr("2026-09-08").From("JFK").To("SFO").MaxStops(0),
				).
				Adults(1).
				TripType(gf.TripTypeRoundTrip).
				Class(gf.ClassEconomy).
				Currency("USD").Language("en").Region("us"),
		},
		{
			name: "unset locale is left off the search",
			it: func() db.Itinerary {
				it := itinerary()
				it.Currency, it.Hl, it.Gl = "", "", ""
				return it
			}(),
			want: gf.NewRequest().
				Flights(
					gf.NewFlightInfo().DepartureDateStr("2026-09-01").From("SFO").To("JFK"),
					gf.NewFlightInfo().DepartureDateStr("2026-09-08").From("JFK").To("SFO"),
				).
				Adults(1).
				TripType(gf.TripTypeRoundTrip).
				Class(gf.ClassEconomy),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := itineraryToRequest(tt.it)
			if err != nil {
				t.Fatalf("itineraryToRequest() error = %v", err)
			}
			gotURL, err := got.URL()
			if err != nil {
				t.Fatalf("URL() error = %v", err)
			}
			wantURL, err := tt.want.URL()
			if err != nil {
				t.Fatalf("want URL() error = %v", err)
			}
			if gotURL.String() != wantURL.String() {
				t.Errorf("URL()\n got %s\nwant %s", gotURL, wantURL)
			}
		})
	}
}

func TestItineraryToRequestErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*db.Itinerary)
	}{
		{name: "malformed outbound times", mutate: func(it *db.Itinerary) { it.OutboundTimes = txt("6") }},
		{name: "non numeric outbound times", mutate: func(it *db.Itinerary) { it.OutboundTimes = txt("morning,evening") }},
		{name: "malformed layover duration", mutate: func(it *db.Itinerary) { it.LayoverDuration = txt("60,300,900") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := itinerary()
			tt.mutate(&it)
			if _, err := itineraryToRequest(it); err == nil {
				t.Error("itineraryToRequest() error = nil, want an error")
			}
		})
	}
}

// TestApplyLayoverDurationUnbounded covers a bound left at zero, which means
// unbounded to SerpAPI but is rejected outright by the goflights builder.
func TestApplyLayoverDurationUnbounded(t *testing.T) {
	it := itinerary()
	it.LayoverDuration = txt("0,300")

	req, err := itineraryToRequest(it)
	if err != nil {
		t.Fatalf("itineraryToRequest() error = %v", err)
	}
	got, err := req.URL()
	if err != nil {
		t.Fatalf("URL() error = %v", err)
	}

	want, err := gf.NewRequest().
		Flights(
			gf.NewFlightInfo().DepartureDateStr("2026-09-01").From("SFO").To("JFK").
				MaxLayover(300*time.Minute),
			gf.NewFlightInfo().DepartureDateStr("2026-09-08").From("JFK").To("SFO").
				MaxLayover(300*time.Minute),
		).
		Adults(1).
		TripType(gf.TripTypeRoundTrip).
		Class(gf.ClassEconomy).
		Currency("USD").Language("en").Region("us").
		URL()
	if err != nil {
		t.Fatalf("want URL() error = %v", err)
	}
	if got.String() != want.String() {
		t.Errorf("URL()\n got %s\nwant %s", got, want)
	}
}
