package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/scraie"
	"github.com/maxmwang/scraie/flights/internal/search"
	searchpb "github.com/maxmwang/scraie/flights/internal/search/pb"
	"github.com/maxmwang/scraie/flights/internal/util"
	"github.com/maxmwang/scraie/flights/internal/util/option"
)

type notifier struct {
	session *discordgo.Session
	db      *db.Queries
}

var (
	colorPriceDrop = 0x2ECC71
	colorPriceRise = 0xE74C3C
)

func renderPriceNotification(cur string, previousMin, newMin float64, nearMinimum option.Option[float64], movement option.Option[float64]) string {
	var s strings.Builder
	s.WriteString("```ansi\n")
	nearMinimumValue, nearMinimumPassed := nearMinimum.Value()
	movementValue, movementPassed := movement.Value()
	if nearMinimumPassed {
		fmt.Fprintf(&s, "Near 7 Day Minimum: %s%.2f → %s%.2f\n", cur, nearMinimumValue, cur, newMin)
	}
	if nearMinimumPassed || movementPassed {
		oldMin := previousMin
		if movementPassed {
			oldMin = movementValue
		}
		color := "\x1b[1;31m"
		if newMin <= oldMin {
			color = "\x1b[1;32m"
		}
		diff := newMin - oldMin
		sign := "+"
		if diff < 0 {
			sign = "-"
			diff = -diff
		}
		percent := "0%"
		if oldMin != 0 {
			percent = fmt.Sprintf("%+.1f%%", (newMin-oldMin)/oldMin*100)
		}
		fmt.Fprintf(&s, "Price Movement:\t %s%s%.2f → %s%.2f | %s%s%.2f (%s)\x1b[0m\n",
			color, cur, oldMin, cur, newMin, sign, cur, diff, percent)
	}
	s.WriteString("```")
	return s.String()
}

func renderCheapestOptionSummary(option search.FlightOptions) string {
	if len(option.Segments) == 0 {
		return ""
	}
	times := make([]string, len(option.Segments))
	var airlineWidth, timeWidth int
	for i, segment := range option.Segments {
		times[i] = formatTimeRange(segment.DepartureTime, segment.ArrivalTime, segment.Overnight)
		airlineWidth = max(airlineWidth, len(segment.Airline))
		timeWidth = max(timeWidth, len(times[i]))
	}
	var b strings.Builder
	b.WriteString("```")
	for i, segment := range option.Segments {
		fmt.Fprintf(&b, "\n%-*s · %-*s · %s→%s · %s",
			airlineWidth, segment.Airline,
			timeWidth, times[i],
			segment.DepartureAirportID, segment.ArrivalAirportID,
			formatDuration(segment.Duration))
		if i < len(option.Layovers) {
			layover := option.Layovers[i]
			airport := layover.AirportID
			if airport == "" {
				airport = layover.Name
			}
			if duration := formatDuration(layover.Duration); duration != "" {
				fmt.Fprintf(&b, "\n|\n| (%s in %s)\n|", duration, airport)
			} else {
				fmt.Fprintf(&b, "\n|\n| (Layover in %s)\n|", airport)
			}
		}
	}
	b.WriteString("\n```")
	return b.String()
}

func formatTimeRange(departure, arrival string, overnight bool) string {
	clock := func(value string) string {
		t, err := time.Parse("2006-01-02 15:04", value)
		if err != nil {
			return value
		}
		return t.Format("3:04pm")
	}
	value := fmt.Sprintf("%s - %s", clock(departure), clock(arrival))
	if overnight {
		value += "(+1)"
	}
	return value
}

func formatDuration(minutes int32) string {
	if minutes <= 0 {
		return ""
	}
	hours, remaining := minutes/60, minutes%60
	switch {
	case hours == 0:
		return fmt.Sprintf("%dm", remaining)
	case remaining == 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dh %dm", hours, remaining)
	}
}

func (n *notifier) Notify(ctx context.Context, notification scraie.Notification) error {
	fields := []*discordgo.MessageEmbedField{
		{
			Name:  "Itinerary Summary",
			Value: renderItinerarySummary(notification.Itinerary, itinerarySummaryOptions{}),
		},
		{
			Name: "Price Notification",
			Value: renderPriceNotification(
				util.CurrencySymbol(notification.Itinerary.Currency),
				notification.PreviousPrice,
				notification.NewPrice,
				notification.Near7DayMinimum,
				notification.PriceMovement),
		},
		{
			Name:  "Newest Cheapest Option's First Leg",
			Value: renderCheapestOptionSummary(notification.CheapestOption),
		},
	}

	color := colorPriceDrop
	if notification.NewPrice > notification.PreviousPrice {
		color = colorPriceRise
	}

	embed := &discordgo.MessageEmbed{
		Title:       util.ItineraryToString(notification.Itinerary),
		Description: "[Google Flights Link](" + searchpb.ToLink(notification.Itinerary) + ")",
		Color:       color,
		Fields:      fields,
	}
	if !notification.Timestamp.IsZero() {
		embed.Timestamp = notification.Timestamp.UTC().Format(time.RFC3339)
	}
	if chart, err := util.BuildDailyMinimumPriceChartURL(notification.Itinerary, notification.PriceHistory, scraie.PriceHistoryLength); err == nil && chart != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: chart}
	}

	user, err := n.db.GetUserByID(ctx, notification.Itinerary.UserID)
	if err != nil {
		return fmt.Errorf("get notification recipient: %w", err)
	}
	channel, err := n.session.UserChannelCreate(user.DiscordID)
	if err != nil {
		return fmt.Errorf("create direct message channel: %w", err)
	}
	_, err = n.session.ChannelMessageSendComplex(channel.ID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
	})
	return err
}
