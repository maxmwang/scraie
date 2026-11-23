package pb

import "testing"

func TestFromGoogleFlightsLink(t *testing.T) {
	link := "https://www.google.com/travel/flights/search?tfs=CBwQAhpIEgoyMDI3LTAxLTAxKAEyCE9ORVdPUkxEQAtIF1AAWAtgoAtqDQgDEgkvbS8wMl8yODZyDAgDEggvbS8wNGpwbJABsAmaAQEBGkgSCjIwMjctMDEtMTAoATIIT05FV09STERAB0gXUABYEWCgC2oMCAMSCC9tLzA0anBscg0IAxIJL20vMDJfMjg2kAGwCZoBAQFAAUgBYMwIagQQARgAcAGCAQsI____________AZgBAQ&tfu=EgYIABAAGAA"

	it := FromLink(link)

	if it.DepartureID != "/m/02_286" || it.ArrivalID != "/m/04jpl" {
		t.Fatalf("airports = %q to %q", it.DepartureID, it.ArrivalID)
	}
	if it.Type != 1 || it.OutboundDate != "2027-01-01" ||
		!it.ReturnDate.Valid || it.ReturnDate.String != "2027-01-10" {
		t.Fatalf("type/dates = %d %q %q", it.Type, it.OutboundDate, it.ReturnDate.String)
	}
	if it.TravelClass != 1 || it.Adults != 1 ||
		it.Children != 0 || it.InfantsInSeat != 0 || it.InfantsOnLap != 0 {
		t.Fatalf("passengers/class = %d adults=%d children=%d infants-seat=%d infants-lap=%d",
			it.TravelClass, it.Adults, it.Children, it.InfantsInSeat, it.InfantsOnLap)
	}
	if it.Stops != 2 || it.IncludeAirlines.String != "ONEWORLD" ||
		it.OutboundTimes.String != "11,23,0,11" ||
		it.ReturnTimes.String != "7,23,0,17" {
		t.Fatalf("filters = stops=%d airlines=%q outbound=%q return=%q",
			it.Stops, it.IncludeAirlines.String, it.OutboundTimes.String, it.ReturnTimes.String)
	}
	if it.Bags != 1 || !it.MaxPrice.Valid || it.MaxPrice.Int32 != 1100 ||
		it.LayoverDuration.String != "0,1200" ||
		!it.MaxDuration.Valid || it.MaxDuration.Int32 != 1440 ||
		!it.Emissions.Valid || it.Emissions.Int32 != 1 {
		t.Fatalf("bag/price/duration/layover/emissions = %d %v %v %q %v",
			it.Bags, it.MaxPrice, it.MaxDuration, it.LayoverDuration.String, it.Emissions)
	}
}

func TestToLinkRoundTrip(t *testing.T) {
	original := FromLink("https://www.google.com/travel/flights/search?tfs=CBwQAhpIEgoyMDI3LTAxLTAxKAEyCE9ORVdPUkxEQAtIF1AAWAtgoAtqDQgDEgkvbS8wMl8yODZyDAgDEggvbS8wNGpwbJABsAmaAQEBGkgSCjIwMjctMDEtMTAoATIIT05FV09STERAB0gXUABYEWCgC2oMCAMSCC9tLzA0anBscg0IAxIJL20vMDJfMjg2kAGwCZoBAQFAAUgBYMwIagQQARgAcAGCAQsI____________AZgBAQ")

	link := ToLink(original)
	roundTrip := FromLink(link)

	if roundTrip.DepartureID != original.DepartureID ||
		roundTrip.ArrivalID != original.ArrivalID ||
		roundTrip.OutboundDate != original.OutboundDate ||
		roundTrip.ReturnDate.String != original.ReturnDate.String ||
		roundTrip.Stops != original.Stops ||
		roundTrip.MaxDuration.Int32 != original.MaxDuration.Int32 ||
		roundTrip.LayoverDuration.String != original.LayoverDuration.String ||
		roundTrip.OutboundTimes.String != original.OutboundTimes.String ||
		roundTrip.ReturnTimes.String != original.ReturnTimes.String ||
		roundTrip.IncludeAirlines.String != original.IncludeAirlines.String ||
		roundTrip.Bags != original.Bags ||
		roundTrip.MaxPrice.Int32 != original.MaxPrice.Int32 ||
		roundTrip.Emissions.Int32 != original.Emissions.Int32 {
		t.Fatalf("round trip mismatch:\noriginal: %+v\nround trip: %+v\nlink: %s", original, roundTrip, link)
	}
}
