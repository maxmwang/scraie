package commands

import (
	"context"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var List = &discordgo.ApplicationCommand{
	Name: "list", Description: "List itineraries",
}

func HandleList(h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, ok := h.EnsureUser(context.Background(), i)
	if !ok {
		respond(s, i, "❌ Couldn't identify your Discord account.", true)
		return
	}
	itineraries, err := h.DB.GetItinerariesByUser(context.Background(), userID)
	if err != nil {
		respond(s, i, "❌ Couldn't load itineraries.", true)
		return
	}
	if len(itineraries) == 0 {
		respond(s, i, "No itineraries.", false)
		return
	}
	lines := make([]string, 0, len(itineraries))
	for _, it := range itineraries {
		lines = append(lines, "`#"+fmtID(it.ID)+"` "+summary(it))
	}
	respondEmbed(s, i, "Itineraries", strings.Join(lines, "\n"))
}
