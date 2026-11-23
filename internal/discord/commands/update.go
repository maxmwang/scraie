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

func HandleUpdate(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, ok := h.EnsureUser(context.Background(), i)
	if !ok {
		respond(s, i, "❌ Couldn't identify your Discord account.", true)
		return
	}
	opts := i.ApplicationCommandData().Options
	id := optInt(opts, "id")
	existing, err := ownedItinerary(context.Background(), h, id, userID)
	if err != nil {
		respond(s, i, "❌ No itinerary `#"+fmtID(id)+"`.", true)
		return
	}
	in := fromItinerary(existing)
	if err := decodeInto(optString(opts, "json"), &in); err != nil {
		respond(s, i, "❌ Invalid JSON: `"+err.Error()+"`", true)
		return
	}
	it := toItinerary(in, existing)
	if err := validateRanges(&it); err != nil {
		respond(s, i, "❌ "+err.Error(), true)
		return
	}
	if _, err := h.DB.UpdateItinerary(context.Background(), db.ToUpdateParams(it)); err != nil {
		respond(s, i, "❌ Couldn't save changes.", true)
		return
	}
	respond(s, i, "✅ Updated `#"+fmtID(it.ID)+"`: "+summary(it), false)
}
