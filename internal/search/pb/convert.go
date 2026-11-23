package pb

import (
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/proto"

	"github.com/maxmwang/scraie/flights/internal/db"
)

// FromLink decodes Google's base64url-encoded tfs query parameter and
// converts the search request into an itinerary.
func FromLink(link string) db.Itinerary {
	raw := strings.TrimSpace(link)
	if raw == "" {
		return db.Itinerary{}
	}

	if u, err := url.Parse(raw); err == nil {
		if tfs := u.Query().Get("tfs"); tfs != "" {
			raw = tfs
			it := decodeAndConvert(raw)
			if u.Query().Get("hl") != "" {
				it.Hl = strings.ToLower(u.Query().Get("hl"))
			}
			if u.Query().Get("gl") != "" {
				it.Gl = strings.ToLower(u.Query().Get("gl"))
			}
			if u.Query().Get("curr") != "" {
				it.Currency = strings.ToUpper(u.Query().Get("curr"))
			}
			return it
		}
	}

	return decodeAndConvert(raw)
}

func decodeAndConvert(raw string) db.Itinerary {
	encoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		encoded, err = base64.URLEncoding.DecodeString(raw)
	}
	if err != nil {
		encoded, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil {
		encoded, err = base64.StdEncoding.DecodeString(raw)
	}
	if err != nil {
		return db.Itinerary{}
	}

	var req InternalRequest
	if err := proto.Unmarshal(encoded, &req); err != nil {
		return db.Itinerary{}
	}
	return requestToItinerary(&req)
}

func requestToItinerary(req *InternalRequest) db.Itinerary {
	var it db.Itinerary
	if req == nil {
		return it
	}

	if len(req.Flights) > 0 {
		outbound := req.Flights[0]
		if outbound != nil {
			it.OutboundDate = outbound.Date
			it.DepartureID = airportCodes(outbound.From)
			it.ArrivalID = airportCodes(outbound.To)
			it.OutboundTimes = flightTimes(outbound)
			it.Stops = stopsFromProto(outbound.MaxStops)

			if len(outbound.Airlines) > 0 {
				it.IncludeAirlines = pgtype.Text{
					String: strings.Join(outbound.Airlines, ","),
					Valid:  true,
				}
			}
			if outbound.MaxDurationMinutes != nil {
				it.MaxDuration = pgtype.Int4{
					Int32: *outbound.MaxDurationMinutes,
					Valid: true,
				}
			}
			it.LayoverDuration = layoverDuration(outbound)
			if containsEmission(outbound.Emissions, InternalEmissions_INTERNAL_EMISSIONS_LESS_ONLY) {
				it.Emissions = pgtype.Int4{Int32: 1, Valid: true}
			}
		}
	}

	if len(req.Flights) > 1 && req.Flights[1] != nil {
		it.ReturnDate = pgtype.Text{
			String: req.Flights[1].Date,
			Valid:  req.Flights[1].Date != "",
		}
		it.ReturnTimes = flightTimes(req.Flights[1])
	}

	switch req.TripType {
	case InternalTripType_INTERNAL_TRIP_TYPE_ROUND_TRIP:
		it.Type = int32(req.TripType)
	case InternalTripType_INTERNAL_TRIP_TYPE_ONE_WAY:
		it.Type = int32(req.TripType)
	case InternalTripType_INTERNAL_TRIP_TYPE_MULTI_CITY:
		it.Type = int32(req.TripType)
	default:
		switch len(req.Flights) {
		case 1:
			it.Type = 2
		case 2:
			it.Type = 1
		default:
			if len(req.Flights) > 2 {
				it.Type = 3
			}
		}
	}

	it.TravelClass = int32(req.Class)
	for _, passenger := range req.Passengers {
		switch passenger {
		case InternalPassenger_INTERNAL_PASSENGER_ADULT:
			it.Adults++
		case InternalPassenger_INTERNAL_PASSENGER_CHILD:
			it.Children++
		case InternalPassenger_INTERNAL_PASSENGER_INFANT_IN_SEAT:
			it.InfantsInSeat++
		case InternalPassenger_INTERNAL_PASSENGER_INFANT_ON_LAP:
			it.InfantsOnLap++
		}
	}

	if req.MaxPrice != nil {
		it.MaxPrice = pgtype.Int4{Int32: *req.MaxPrice, Valid: true}
	}
	if req.Baggage != nil && req.Baggage.CarryOnBag != nil {
		it.Bags = *req.Baggage.CarryOnBag
	}
	if req.ExcludeBasicEconomy != nil {
		it.ExcludeBasic = *req.ExcludeBasicEconomy
	}

	return it
}

func airportCodes(airports []*InternalAirport) string {
	codes := make([]string, 0, len(airports))
	for _, airport := range airports {
		if airport != nil && strings.TrimSpace(airport.Code) != "" {
			codes = append(codes, strings.TrimSpace(airport.Code))
		}
	}
	return strings.Join(codes, ",")
}

func stopsFromProto(maxStops *int32) int32 {
	if maxStops == nil {
		return 0
	}
	switch *maxStops {
	case 0:
		return 1
	case 1:
		return 2
	case 2:
		return 3
	default:
		return 0
	}
}

func flightTimes(flight *InternalFlightInfo) pgtype.Text {
	if flight == nil ||
		flight.EarliestDepartureHour == nil ||
		flight.LatestDepartureHour == nil {
		return pgtype.Text{}
	}

	value := strconv.Itoa(int(*flight.EarliestDepartureHour)) + "," +
		strconv.Itoa(int(*flight.LatestDepartureHour))
	if flight.EarliestArrivalHour != nil && flight.LatestArrivalHour != nil {
		value += "," + strconv.Itoa(int(*flight.EarliestArrivalHour)) +
			"," + strconv.Itoa(int(*flight.LatestArrivalHour))
	}
	return pgtype.Text{String: value, Valid: true}
}

func layoverDuration(flight *InternalFlightInfo) pgtype.Text {
	if flight == nil ||
		(flight.MinLayoverMinutes == nil && flight.MaxLayoverMinutes == nil) {
		return pgtype.Text{}
	}

	minutes := int32(0)
	if flight.MinLayoverMinutes != nil {
		minutes = *flight.MinLayoverMinutes
	}
	maxMinutes := int32(0)
	if flight.MaxLayoverMinutes != nil {
		maxMinutes = *flight.MaxLayoverMinutes
	}
	value := strconv.Itoa(int(minutes)) + "," + strconv.Itoa(int(maxMinutes))
	return pgtype.Text{String: value, Valid: true}
}

func containsEmission(emissions []InternalEmissions, wanted InternalEmissions) bool {
	for _, emission := range emissions {
		if emission == wanted {
			return true
		}
	}
	return false
}

// ToLink encodes an itinerary as Google's base64url-encoded tfs search
// request and returns a Google Flights search URL.
func ToLink(it db.Itinerary) string {
	req := itineraryToRequest(it)
	raw, err := proto.Marshal(req)
	if err != nil {
		return ""
	}

	query := url.Values{}
	query.Set("tfs", base64.RawURLEncoding.EncodeToString(raw))
	if it.Hl != "" {
		query.Set("hl", strings.ToLower(it.Hl))
	}
	if it.Gl != "" {
		query.Set("gl", strings.ToLower(it.Gl))
	}
	if it.Currency != "" {
		query.Set("curr", strings.ToUpper(it.Currency))
	}

	return "https://www.google.com/travel/flights/search?" + query.Encode()
}

func itineraryToRequest(it db.Itinerary) *InternalRequest {
	req := &InternalRequest{
		Class:    InternalClass(it.TravelClass),
		TripType: InternalTripType(it.Type),
	}

	for i := int32(0); i < it.Adults; i++ {
		req.Passengers = append(req.Passengers, InternalPassenger_INTERNAL_PASSENGER_ADULT)
	}
	for i := int32(0); i < it.Children; i++ {
		req.Passengers = append(req.Passengers, InternalPassenger_INTERNAL_PASSENGER_CHILD)
	}
	for i := int32(0); i < it.InfantsInSeat; i++ {
		req.Passengers = append(req.Passengers, InternalPassenger_INTERNAL_PASSENGER_INFANT_IN_SEAT)
	}
	for i := int32(0); i < it.InfantsOnLap; i++ {
		req.Passengers = append(req.Passengers, InternalPassenger_INTERNAL_PASSENGER_INFANT_ON_LAP)
	}

	if it.MaxPrice.Valid {
		req.MaxPrice = &it.MaxPrice.Int32
	}
	if it.Bags > 0 {
		req.Baggage = &InternalBaggage{CarryOnBag: &it.Bags}
	}
	if it.ExcludeBasic {
		excludeBasic := true
		req.ExcludeBasicEconomy = &excludeBasic
	}

	outbound := &InternalFlightInfo{
		Date: it.OutboundDate,
		From: airportsFromCodes(it.DepartureID),
		To:   airportsFromCodes(it.ArrivalID),
	}
	applyItineraryFilters(outbound, it, it.OutboundTimes)
	req.Flights = append(req.Flights, outbound)

	if it.Type == 1 {
		inbound := &InternalFlightInfo{
			Date: it.ReturnDate.String,
			From: airportsFromCodes(it.ArrivalID),
			To:   airportsFromCodes(it.DepartureID),
		}
		applyItineraryFilters(inbound, it, it.ReturnTimes)
		req.Flights = append(req.Flights, inbound)
	}

	return req
}

func airportsFromCodes(codes string) []*InternalAirport {
	var airports []*InternalAirport
	for _, code := range strings.Split(codes, ",") {
		code = strings.TrimSpace(code)
		if code != "" {
			airports = append(airports, &InternalAirport{Code: code})
		}
	}
	return airports
}

func applyItineraryFilters(flight *InternalFlightInfo, it db.Itinerary, times pgtype.Text) {
	switch it.Stops {
	case 1:
		maxStops := int32(0)
		flight.MaxStops = &maxStops
	case 2:
		maxStops := int32(1)
		flight.MaxStops = &maxStops
	case 3:
		maxStops := int32(2)
		flight.MaxStops = &maxStops
	}

	if it.IncludeAirlines.Valid {
		for _, airline := range strings.Split(it.IncludeAirlines.String, ",") {
			airline = strings.TrimSpace(airline)
			if airline != "" {
				flight.Airlines = append(flight.Airlines, airline)
			}
		}
	}

	if it.MaxDuration.Valid {
		flight.MaxDurationMinutes = &it.MaxDuration.Int32
	}
	if it.Emissions.Valid && it.Emissions.Int32 == 1 {
		flight.Emissions = []InternalEmissions{InternalEmissions_INTERNAL_EMISSIONS_LESS_ONLY}
	}

	if it.LayoverDuration.Valid {
		values := splitInts(it.LayoverDuration.String)
		if len(values) == 2 {
			if values[0] > 0 {
				flight.MinLayoverMinutes = &values[0]
			}
			if values[1] > 0 {
				flight.MaxLayoverMinutes = &values[1]
			}
		}
	}

	if times.Valid {
		values := splitInts(times.String)
		if len(values) == 2 || len(values) == 4 {
			flight.EarliestDepartureHour = &values[0]
			flight.LatestDepartureHour = &values[1]
			if len(values) == 4 {
				flight.EarliestArrivalHour = &values[2]
				flight.LatestArrivalHour = &values[3]
			}
		}
	}
}

func splitInts(value string) []int32 {
	var values []int32
	for _, field := range strings.Split(value, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			return nil
		}
		n, err := strconv.ParseInt(field, 10, 32)
		if err != nil {
			return nil
		}
		values = append(values, int32(n))
	}
	return values
}
