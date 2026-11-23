package commands

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/maxmwang/scraie/flights/internal/db"
)

var Create = &discordgo.ApplicationCommand{
	Name:        "create",
	Description: "Track a new itinerary from JSON",
	Options: []*discordgo.ApplicationCommandOption{{
		Type: discordgo.ApplicationCommandOptionString, Name: "json",
		Description: "Itinerary as single-line JSON", Required: true, MaxLength: 6000,
	}},
}

func HandleCreate(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, ok := h.EnsureUser(context.Background(), i)
	if !ok {
		respond(s, i, "❌ Couldn't identify your Discord account.", true)
		return
	}
	in := defaultInput()
	if err := decodeInto(optString(i.ApplicationCommandData().Options, "json"), &in); err != nil {
		respond(s, i, "❌ Invalid JSON: `"+err.Error()+"`", true)
		return
	}
	it := toItinerary(in, db.Itinerary{})
	if err := validateRanges(&it); err != nil {
		respond(s, i, "❌ "+err.Error(), true)
		return
	}
	id, err := h.DB.CreateItinerary(context.Background(), db.CreateItineraryParams{
		UserID: userID, DepartureID: it.DepartureID, ArrivalID: it.ArrivalID, Type: it.Type, OutboundDate: it.OutboundDate,
		ReturnDate: it.ReturnDate, TravelClass: it.TravelClass, ShowHidden: it.ShowHidden, MultiCityJson: it.MultiCityJson,
		ExcludeBasic: it.ExcludeBasic, DeepSearch: it.DeepSearch, Adults: it.Adults, Children: it.Children,
		InfantsInSeat: it.InfantsInSeat, InfantsOnLap: it.InfantsOnLap, SortBy: it.SortBy, Stops: it.Stops,
		ExcludeAirlines: it.ExcludeAirlines, IncludeAirlines: it.IncludeAirlines, Bags: it.Bags, MaxPrice: it.MaxPrice,
		OutboundTimes: it.OutboundTimes, ReturnTimes: it.ReturnTimes, Emissions: it.Emissions,
		LayoverDuration: it.LayoverDuration, ExcludeConns: it.ExcludeConns, MaxDuration: it.MaxDuration,
		Gl: it.Gl, Hl: it.Hl, Currency: it.Currency,
	})
	if err != nil {
		respond(s, i, "❌ Couldn't save that itinerary.", true)
		return
	}
	it.ID = id
	respond(s, i, fmt.Sprintf("✅ Created `#%d`: %s", id, summary(it)), false)
}
