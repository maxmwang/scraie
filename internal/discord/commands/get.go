package commands

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/scraie"
	searchpb "github.com/maxmwang/scraie/flights/internal/search/pb"
	"github.com/maxmwang/scraie/flights/internal/util"
)

var Get = &discordgo.ApplicationCommand{
	Name: "get", Description: "Get an itinerary",
	Options: []*discordgo.ApplicationCommandOption{{
		Type: discordgo.ApplicationCommandOptionInteger, Name: "id",
		Description: "Itinerary ID", Required: true,
		MinValue: func() *float64 { v := 1.0; return &v }(),
	}},
}

func ownedItinerary(ctx context.Context, h *Handler, id int64, userID pgtype.UUID) (db.Itinerary, error) {
	it, err := h.DB.GetItinerary(ctx, id)
	if err != nil || it.UserID != userID {
		return db.Itinerary{}, fmt.Errorf("itinerary #%d not found", id)
	}
	return it, nil
}

func HandleGet(ctx context.Context, h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, ok := h.EnsureUser(ctx, i)
	if !ok {
		respondError(s, i, "Couldn't identify your Discord account.", true)
		return
	}
	id := optInt(i.ApplicationCommandData().Options, "id")
	it, err := ownedItinerary(ctx, h, id, userID)
	if err != nil {
		respondError(s, i, fmt.Sprintf("No itinerary `#%d`.", id), true)
		return
	}

	detailedSummary := ""
	if h.RenderDetailedSummary != nil {
		detailedSummary = h.RenderDetailedSummary(it)
	} else {
		detailedSummary = summary(it)
	}
	embed := &discordgo.MessageEmbed{
		Title:       util.ItineraryToString(it),
		Description: "[Google Flights Link](" + searchpb.ToLink(it) + ")",
		Color:       successColor,
		Fields: []*discordgo.MessageEmbedField{{
			Name:  "Itinerary Summary",
			Value: detailedSummary,
		}},
	}

	since := time.Now().AddDate(0, 0, -scraie.PriceHistoryDays).Add(-time.Hour)
	history, err := h.DB.GetOptionsSince(ctx, db.GetOptionsSinceParams{
		ItineraryID: it.ID,
		SearchedAt:  pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		history = nil
	}
	chart, err := util.BuildDailyMinimumPriceChartURL(it, history, scraie.PriceHistoryLength)
	if err == nil && chart != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: chart}
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	}); err != nil {
		log.Printf("respond error: %v", err)
	}
}
