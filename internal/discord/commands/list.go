package commands

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/maxmwang/scraie/flights/internal/db"
)

var List = &discordgo.ApplicationCommand{
	Name: "list", Description: "List itineraries",
}

const listPageSize = 10

func HandleList(ctx context.Context, h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userID, ok := h.EnsureUser(ctx, i)
	if !ok {
		respondError(s, i, "Couldn't identify your Discord account.", true)
		return
	}
	itineraries, err := h.DB.GetItinerariesByUser(ctx, userID)
	if err != nil {
		respondError(s, i, "Couldn't load itineraries.", true)
		return
	}
	if len(itineraries) == 0 {
		respondSuccess(s, i, "Itineraries", "No itineraries.", false)
		return
	}
	sort.Slice(itineraries, func(a, b int) bool {
		return itineraries[a].ID < itineraries[b].ID
	})
	respondListPage(s, i, itineraries, 0, false)
}

func HandleListComponent(ctx context.Context, h *Handler, s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.MessageComponentData()
	parts := strings.Split(data.CustomID, ":")
	if len(parts) != 3 || parts[0] != "list" {
		return
	}
	page, err := strconv.Atoi(parts[2])
	if err != nil {
		return
	}

	userID, ok := h.EnsureUser(ctx, i)
	if !ok {
		respondError(s, i, "Couldn't identify your Discord account.", true)
		return
	}
	itineraries, err := h.DB.GetItinerariesByUser(ctx, userID)
	if err != nil {
		respondError(s, i, "Couldn't load itineraries.", true)
		return
	}
	sort.Slice(itineraries, func(a, b int) bool {
		return itineraries[a].ID < itineraries[b].ID
	})

	totalPages := (len(itineraries) + listPageSize - 1) / listPageSize
	if parts[1] == "prev" {
		page--
	} else if parts[1] == "next" {
		page++
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	respondListPage(s, i, itineraries, page, true)
}

func respondListPage(s *discordgo.Session, i *discordgo.InteractionCreate, itineraries []db.Itinerary, page int, update bool) {
	totalPages := (len(itineraries) + listPageSize - 1) / listPageSize
	start := page * listPageSize
	end := start + listPageSize
	if end > len(itineraries) {
		end = len(itineraries)
	}

	lines := make([]string, 0, end-start)
	for _, it := range itineraries[start:end] {
		lines = append(lines, "`#"+fmtID(it.ID)+"` "+summary(it))
	}

	responseType := discordgo.InteractionResponseChannelMessageWithSource
	if update {
		responseType = discordgo.InteractionResponseUpdateMessage
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: responseType,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{{
				Title:       "Itineraries",
				Description: strings.Join(lines, "\n"),
				Color:       successColor,
				Footer:      &discordgo.MessageEmbedFooter{Text: "Page " + strconv.Itoa(page+1) + " of " + strconv.Itoa(totalPages)},
			}},
			Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				&discordgo.Button{
					Label: "Previous", Style: discordgo.PrimaryButton,
					CustomID: "list:prev:" + strconv.Itoa(page), Disabled: page == 0,
				},
				&discordgo.Button{
					Label: "Next", Style: discordgo.PrimaryButton,
					CustomID: "list:next:" + strconv.Itoa(page), Disabled: page >= totalPages-1,
				},
			}}},
		},
	}); err != nil {
		log.Printf("respond error: %v", err)
	}
}
