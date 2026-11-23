package commands

import (
	"context"

	"github.com/bwmarrin/discordgo"

	"github.com/maxmwang/scraie/flights/internal/db"
)

var Update = &discordgo.ApplicationCommand{
	Name: "update", Description: "Update an itinerary with JSON (only included keys change)",
	Options: []*discordgo.ApplicationCommandOption{
		{
			Type: discordgo.ApplicationCommandOptionInteger, Name: "id",
			Description: "Itinerary ID", Required: true,
			MinValue: func() *float64 { v := 1.0; return &v }(),
		},
		{
			Type: discordgo.ApplicationCommandOptionString, Name: "json",
			Description: "Fields to change as single-line JSON; null clears an optional field",
			Required:    true, MaxLength: 6000,
		},
	},
}

func HandleUpdate(ctx context.Context, h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, ok := h.EnsureUser(ctx, i)
	if !ok {
		respondError(s, i, "Couldn't identify your Discord account.", true)
		return
	}
	opts := i.ApplicationCommandData().Options
	id := optInt(opts, "id")
	existing, err := ownedItinerary(ctx, h, id, userID)
	if err != nil {
		respondError(s, i, "No itinerary `#"+fmtID(id)+"`.", true)
		return
	}
	in := fromItinerary(existing)
	if err := decodeInto(optString(opts, "json"), &in); err != nil {
		respondError(s, i, "Invalid JSON: `"+err.Error()+"`", true)
		return
	}
	it := toItinerary(in, existing)
	if err := validateRanges(&it); err != nil {
		respondError(s, i, err.Error(), true)
		return
	}
	if _, err := h.DB.UpdateItinerary(ctx, db.ToUpdateParams(it)); err != nil {
		respondError(s, i, "Couldn't save changes.", true)
		return
	}
	respondSuccess(s, i, "Itinerary updated", "`#"+fmtID(it.ID)+"`: "+summary(it), false)
}
