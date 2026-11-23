package commands

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search/pb"
)

var Create = &discordgo.ApplicationCommand{
	Name:        "create",
	Description: "Track a new itinerary from a Google Flights link",
	Options: []*discordgo.ApplicationCommandOption{{
		Type: discordgo.ApplicationCommandOptionString, Name: "link",
		Description: "Google Flights search link", Required: true, MaxLength: 6000,
	}},
}

func HandleCreate(ctx context.Context, h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, ok := h.EnsureUser(ctx, i)
	if !ok {
		respondError(s, i, "Couldn't identify your Discord account.", true)
		return
	}
	link := optString(i.ApplicationCommandData().Options, "link")
	it := pb.FromLink(link)
	if it.DepartureID == "" || it.ArrivalID == "" || it.OutboundDate == "" {
		respondError(s, i, "Invalid Google Flights search link.", true)
		return
	}
	it.SortBy = 1
	if it.Gl == "" {
		it.Gl = "us"
	}
	if it.Hl == "" {
		it.Hl = "en"
	}
	if it.Currency == "" {
		it.Currency = "USD"
	}
	if err := validateRanges(&it); err != nil {
		respondError(s, i, err.Error(), true)
		return
	}
	id, err := h.DB.CreateItinerary(ctx, db.CreateItineraryParams{
		UserID:          userID,
		DepartureID:     it.DepartureID,
		ArrivalID:       it.ArrivalID,
		Type:            it.Type,
		OutboundDate:    it.OutboundDate,
		ReturnDate:      it.ReturnDate,
		TravelClass:     it.TravelClass,
		ShowHidden:      it.ShowHidden,
		MultiCityJson:   it.MultiCityJson,
		ExcludeBasic:    it.ExcludeBasic,
		DeepSearch:      it.DeepSearch,
		Adults:          it.Adults,
		Children:        it.Children,
		InfantsInSeat:   it.InfantsInSeat,
		InfantsOnLap:    it.InfantsOnLap,
		SortBy:          it.SortBy,
		Stops:           it.Stops,
		ExcludeAirlines: it.ExcludeAirlines,
		IncludeAirlines: it.IncludeAirlines,
		Bags:            it.Bags,
		MaxPrice:        it.MaxPrice,
		OutboundTimes:   it.OutboundTimes,
		ReturnTimes:     it.ReturnTimes,
		Emissions:       it.Emissions,
		LayoverDuration: it.LayoverDuration,
		ExcludeConns:    it.ExcludeConns,
		MaxDuration:     it.MaxDuration,
		Gl:              it.Gl,
		Hl:              it.Hl,
		Currency:        it.Currency,
	})
	if err != nil {
		respondError(s, i, "Couldn't save that itinerary.", true)
		return
	}
	it.ID = id
	respondSuccess(s, i, "Itinerary created", fmt.Sprintf("`#%d`: %s", id, summary(it)), false)
}
