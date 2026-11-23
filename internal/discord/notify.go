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
	"github.com/maxmwang/scraie/flights/internal/util"
	"github.com/maxmwang/scraie/flights/internal/util/option"
)

type notifier struct {
	session   *discordgo.Session
	channelID string
}

var (
	colorPriceDrop = 0x2ECC71
	colorPriceRise = 0xE74C3C

	flightTypeLabels = map[int32]string{
		1: "Round trip",
		2: "One way",
		3: "Multi-city",
	}
	travelClassLabels = map[int32]string{
		1: "Economy",
		2: "Premium economy",
		3: "Business",
		4: "First",
	}
	stopsLabels = map[int32]string{
		0: "Any",
		1: "Nonstop",
		2: "Up to 1 stop",
		3: "Up to 2 stops",
	}
)

func renderItinerarySummary(it db.Itinerary) string {
	lines := []string{}
	field := func(label, value string) {
		lines = append(lines, fmt.Sprintf("__%s__: %s", label, value))
	}
	field("Departure", it.DepartureID)
	field("Arrival", it.ArrivalID)
	field("Outbound", it.OutboundDate)
	if it.ReturnDate.Valid && it.ReturnDate.String != "" {
		field("Return", it.ReturnDate.String)
	}
	if it.Description.Valid && it.Description.String != "" {
		field("Description", it.Description.String)
	}
	lines = append(lines, "")
	if it.Type != 1 {
		field("Trip type", labelOr(flightTypeLabels, it.Type))
	}
	if it.TravelClass != 1 {
		field("Travel class", labelOr(travelClassLabels, it.TravelClass))
	}
	if it.Stops != 0 {
		field("Stops", labelOr(stopsLabels, it.Stops))
	}
	if it.Adults != 1 {
		field("Adults", fmt.Sprintf("%d", it.Adults))
	}
	if it.Children > 0 {
		field("Children", fmt.Sprintf("%d", it.Children))
	}
	if it.InfantsInSeat > 0 {
		field("Infants in seat", fmt.Sprintf("%d", it.InfantsInSeat))
	}
	if it.InfantsOnLap > 0 {
		field("Infants on lap", fmt.Sprintf("%d", it.InfantsOnLap))
	}
	if it.Bags > 0 {
		field("Carry-on bags", fmt.Sprintf("%d", it.Bags))
	}
	if it.ExcludeBasic {
		field("Exclude basic", "true")
	}
	if it.IncludeAirlines.Valid {
		field("Include airlines", it.IncludeAirlines.String)
	}
	if it.ExcludeAirlines.Valid {
		field("Exclude airlines", it.ExcludeAirlines.String)
	}
	if it.MaxPrice.Valid {
		field("Max price", fmt.Sprintf("%d", it.MaxPrice.Int32))
	}
	if it.OutboundTimes.Valid {
		field("Outbound times", it.OutboundTimes.String)
	}
	if it.ReturnTimes.Valid {
		field("Return times", it.ReturnTimes.String)
	}
	if it.LayoverDuration.Valid {
		field("Layover duration", it.LayoverDuration.String)
	}
	if it.ExcludeConns.Valid {
		field("Exclude connections", it.ExcludeConns.String)
	}
	if it.MaxDuration.Valid {
		field("Max duration", fmt.Sprintf("%d", it.MaxDuration.Int32))
	}
	return strings.Join(lines, "\n")
}

func labelOr(labels map[int32]string, value int32) string {
	if label, ok := labels[value]; ok {
		return label
	}
	return fmt.Sprintf("%d", value)
}

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

func currencySymbol(code string) string {
	switch strings.ToUpper(code) {
	case "", "USD":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	case "JPY":
		return "¥"
	default:
		return code
	}
}

func (n *notifier) Notify(ctx context.Context, notification scraie.Notification) error {
	fields := []*discordgo.MessageEmbedField{
		{
			Name:  "Itinerary Summary",
			Value: renderItinerarySummary(notification.Itinerary),
		},
		{
			Name: "Price Notification",
			Value: renderPriceNotification(
				currencySymbol(notification.Itinerary.Currency),
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
		Title:  util.ItineraryToString(notification.Itinerary),
		Color:  color,
		Fields: fields,
	}
	if !notification.Timestamp.IsZero() {
		embed.Timestamp = notification.Timestamp.UTC().Format(time.RFC3339)
	}
	if chart, err := scraie.BuildDailyMinimumPriceChartURL(notification.Itinerary, notification.PriceHistory, scraie.PriceHistoryLength); err == nil && chart != "" {
		embed.Image = &discordgo.MessageEmbedImage{URL: chart}
	}

	_, err := n.session.ChannelMessageSendComplex(n.channelID, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{embed},
	})
	return err
}
