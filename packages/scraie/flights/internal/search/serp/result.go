package serp

import (
	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
)

type result struct {
	BestFlights  []flightOption `json:"best_flights"`
	OtherFlights []flightOption `json:"other_flights"`
}

type flightOption struct {
	Flights        []flightSegment `json:"flights"`
	Layovers       []layover       `json:"layovers"`
	TotalDuration  int             `json:"total_duration"`
	Price          int             `json:"price"`
	Type           string          `json:"type"`
	DepartureToken string          `json:"departure_token"`
}

type layover struct {
	Duration  int    `json:"duration"`
	Name      string `json:"name"`
	ID        string `json:"id"`
	Overnight bool   `json:"overnight"`
}

type flightSegment struct {
	DepartureAirport airportInfo `json:"departure_airport"`
	ArrivalAirport   airportInfo `json:"arrival_airport"`
	Duration         int         `json:"duration"`
	Airplane         string      `json:"airplane"`
	Airline          string      `json:"airline"`
	AirlineLogo      string      `json:"airline_logo"`
	TravelClass      string      `json:"travel_class"`
	FlightNumber     string      `json:"flight_number"`
	Overnight        bool        `json:"overnight"`
}

type airportInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Time string `json:"time"`
}

func (r result) toDBResult(itineraryID int64) search.Result {
	all := append(r.BestFlights, r.OtherFlights...)
	options := make([]search.FlightOptions, len(all))
	for i, fo := range all {
		segments := make([]db.Segment, len(fo.Flights))
		for j, fs := range fo.Flights {
			segments[j] = db.Segment{
				DepartureAirportName: fs.DepartureAirport.Name,
				DepartureAirportID:   fs.DepartureAirport.ID,
				DepartureTime:        fs.DepartureAirport.Time,
				ArrivalAirportName:   fs.ArrivalAirport.Name,
				ArrivalAirportID:     fs.ArrivalAirport.ID,
				ArrivalTime:          fs.ArrivalAirport.Time,
				Duration:             int32(fs.Duration),
				Airplane:             fs.Airplane,
				Airline:              fs.Airline,
				AirlineLogo:          fs.AirlineLogo,
				TravelClass:          fs.TravelClass,
				FlightNumber:         fs.FlightNumber,
				Overnight:            fs.Overnight,
			}
		}

		layovers := make([]db.Layover, len(fo.Layovers))
		for j, l := range fo.Layovers {
			layovers[j] = db.Layover{
				Duration:  int32(l.Duration),
				Name:      l.Name,
				AirportID: l.ID,
				Overnight: l.Overnight,
			}
		}

		options[i] = search.FlightOptions{
			Option: db.Option{
				ItineraryID:   itineraryID,
				TotalDuration: int32(fo.TotalDuration),
				Price:         int32(fo.Price),
				Type:          fo.Type,
			},
			Segments: segments,
			Layovers: layovers,
		}
	}

	return search.Result{Options: options}
}
