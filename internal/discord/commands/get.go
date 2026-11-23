package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
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
	if err != nil {
		return db.Itinerary{}, fmt.Errorf("itinerary #%d not found", id)
	}
	if it.UserID != userID {
		return db.Itinerary{}, fmt.Errorf("itinerary #%d not found", id)
	}
	return it, nil
}

func HandleGet(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, ok := h.EnsureUser(context.Background(), i)
	if !ok {
		respond(s, i, "❌ Couldn't identify your Discord account.", true)
		return
	}
	id := optInt(i.ApplicationCommandData().Options, "id")
	it, err := ownedItinerary(context.Background(), h, id, userID)
	if err != nil {
		respond(s, i, fmt.Sprintf("❌ No itinerary `#%d`.", id), true)
		return
	}
	details, err := json.MarshalIndent(struct {
		ID int64 `json:"id"`
		ItineraryInput
	}{it.ID, fromItinerary(it)}, "", "  ")
	if err != nil {
		respond(s, i, "❌ Couldn't format that itinerary.", true)
		return
	}
	respondEmbed(s, i, fmt.Sprintf("Itinerary #%d", id), "```json\n"+string(details)+"\n```")
}
