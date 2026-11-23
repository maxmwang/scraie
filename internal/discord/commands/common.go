package commands

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
)

type Handler struct {
	DB                    *db.Queries
	EnsureUser            func(context.Context, *discordgo.InteractionCreate) (pgtype.UUID, bool)
	RenderDetailedSummary func(db.Itinerary) string
}

type ItineraryInput struct {
	DepartureID     string          `json:"departure_id"`
	ArrivalID       string          `json:"arrival_id"`
	Type            int32           `json:"type"`
	OutboundDate    string          `json:"outbound_date"`
	ReturnDate      *string         `json:"return_date"`
	TravelClass     int32           `json:"travel_class"`
	ShowHidden      bool            `json:"show_hidden"`
	MultiCity       json.RawMessage `json:"multi_city"`
	ExcludeBasic    bool            `json:"exclude_basic"`
	DeepSearch      bool            `json:"deep_search"`
	Adults          int32           `json:"adults"`
	Children        int32           `json:"children"`
	InfantsInSeat   int32           `json:"infants_in_seat"`
	InfantsOnLap    int32           `json:"infants_on_lap"`
	SortBy          int32           `json:"sort_by"`
	Stops           int32           `json:"stops"`
	ExcludeAirlines *string         `json:"exclude_airlines"`
	IncludeAirlines *string         `json:"include_airlines"`
	Bags            int32           `json:"bags"`
	MaxPrice        *int32          `json:"max_price"`
	OutboundTimes   *string         `json:"outbound_times"`
	ReturnTimes     *string         `json:"return_times"`
	Emissions       *int32          `json:"emissions"`
	LayoverDuration *string         `json:"layover_duration"`
	ExcludeConns    *string         `json:"exclude_conns"`
	MaxDuration     *int32          `json:"max_duration"`
	Gl              string          `json:"gl"`
	Hl              string          `json:"hl"`
	Currency        string          `json:"currency"`
	Notify          bool            `json:"notify"`
	Description     *string         `json:"description"`
}

func defaultInput() ItineraryInput {
	return ItineraryInput{Type: 1, TravelClass: 1, Adults: 1, SortBy: 1, Gl: "us", Hl: "en", Currency: "USD"}
}

func text(p *string) pgtype.Text {
	if p == nil || strings.TrimSpace(*p) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *p, Valid: true}
}

func int4(p *int32) pgtype.Int4 {
	if p == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *p, Valid: true}
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

func int32Ptr(t pgtype.Int4) *int32 {
	if !t.Valid {
		return nil
	}
	v := t.Int32
	return &v
}

func fromItinerary(it db.Itinerary) ItineraryInput {
	in := ItineraryInput{
		DepartureID:     it.DepartureID,
		ArrivalID:       it.ArrivalID,
		Type:            it.Type,
		OutboundDate:    it.OutboundDate,
		ReturnDate:      textPtr(it.ReturnDate),
		TravelClass:     it.TravelClass,
		ShowHidden:      it.ShowHidden,
		ExcludeBasic:    it.ExcludeBasic,
		DeepSearch:      it.DeepSearch,
		Adults:          it.Adults,
		Children:        it.Children,
		InfantsInSeat:   it.InfantsInSeat,
		InfantsOnLap:    it.InfantsOnLap,
		SortBy:          it.SortBy,
		Stops:           it.Stops,
		ExcludeAirlines: textPtr(it.ExcludeAirlines),
		IncludeAirlines: textPtr(it.IncludeAirlines),
		Bags:            it.Bags,
		MaxPrice:        int32Ptr(it.MaxPrice),
		OutboundTimes:   textPtr(it.OutboundTimes),
		ReturnTimes:     textPtr(it.ReturnTimes),
		Emissions:       int32Ptr(it.Emissions),
		LayoverDuration: textPtr(it.LayoverDuration),
		ExcludeConns:    textPtr(it.ExcludeConns),
		MaxDuration:     int32Ptr(it.MaxDuration),
		Gl:              it.Gl,
		Hl:              it.Hl,
		Currency:        it.Currency,
		Notify:          it.Notify,
		Description:     textPtr(it.Description),
	}
	if it.MultiCityJson.Valid && json.Valid([]byte(it.MultiCityJson.String)) {
		in.MultiCity = json.RawMessage(it.MultiCityJson.String)
	}
	return in
}

func toItinerary(in ItineraryInput, base db.Itinerary) db.Itinerary {
	it := base
	it.DepartureID = strings.ToUpper(strings.TrimSpace(in.DepartureID))
	it.ArrivalID = strings.ToUpper(strings.TrimSpace(in.ArrivalID))
	it.Type = in.Type
	it.OutboundDate = strings.TrimSpace(in.OutboundDate)
	it.ReturnDate = text(in.ReturnDate)
	it.TravelClass = in.TravelClass
	it.ShowHidden = in.ShowHidden
	it.ExcludeBasic = in.ExcludeBasic
	it.DeepSearch = in.DeepSearch
	it.Adults = in.Adults
	it.Children = in.Children
	it.InfantsInSeat = in.InfantsInSeat
	it.InfantsOnLap = in.InfantsOnLap
	it.SortBy = in.SortBy
	it.Stops = in.Stops
	it.ExcludeAirlines = text(in.ExcludeAirlines)
	it.IncludeAirlines = text(in.IncludeAirlines)
	it.Bags = in.Bags
	it.MaxPrice = int4(in.MaxPrice)
	it.OutboundTimes = text(in.OutboundTimes)
	it.ReturnTimes = text(in.ReturnTimes)
	it.Emissions = int4(in.Emissions)
	it.LayoverDuration = text(in.LayoverDuration)
	it.ExcludeConns = text(in.ExcludeConns)
	it.MaxDuration = int4(in.MaxDuration)
	it.Gl = strings.ToLower(strings.TrimSpace(in.Gl))
	it.Hl = strings.ToLower(strings.TrimSpace(in.Hl))
	it.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	it.Notify = in.Notify
	it.Description = text(in.Description)
	it.MultiCityJson = pgtype.Text{}
	if s := strings.TrimSpace(string(in.MultiCity)); s != "" && s != "null" {
		it.MultiCityJson = pgtype.Text{String: s, Valid: true}
	}
	return it
}

func decodeInto(raw string, dst *ItineraryInput) error {
	raw = strings.TrimSpace(strings.Trim(strings.TrimSpace(raw), "`"))
	raw = strings.TrimSpace(strings.TrimPrefix(raw, "json"))
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected data after the JSON object")
	}
	return nil
}

func validateRanges(it *db.Itinerary) error {
	switch {
	case it.DepartureID == "" || it.ArrivalID == "" || it.OutboundDate == "":
		return errors.New("departure_id, arrival_id and outbound_date are required")
	case it.Type < 1 || it.Type > 3:
		return errors.New("type must be 1 (round trip), 2 (one way) or 3 (multi-city)")
	case it.TravelClass < 1 || it.TravelClass > 4:
		return errors.New("travel_class must be 1-4 (economy, premium economy, business, first)")
	case it.SortBy < 1 || it.SortBy > 6:
		return errors.New("sort_by must be 1-6")
	case it.Stops < 0 || it.Stops > 3:
		return errors.New("stops must be 0-3")
	case it.Adults < 1 || it.Adults > 9:
		return errors.New("adults must be 1-9")
	case it.Children < 0 || it.InfantsInSeat < 0 || it.InfantsOnLap < 0 || it.Bags < 0:
		return errors.New("passenger and bag counts can't be negative")
	}
	return nil
}

func optString(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, o := range opts {
		if o.Name == name {
			return o.StringValue()
		}
	}
	return ""
}

func optInt(opts []*discordgo.ApplicationCommandInteractionDataOption, name string) int64 {
	for _, o := range opts {
		if o.Name == name {
			return o.IntValue()
		}
	}
	return 0
}

const (
	successColor = 0x57F287
	errorColor   = 0xED4245
)

func formatDate(raw string) string {
	date, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return raw
	}
	return date.Format("Jan 2, 2006")
}

func fmtID(id int64) string {
	return strconv.FormatInt(id, 10)
}

func summary(it db.Itinerary) string {
	value := it.DepartureID + " → " + it.ArrivalID + ", " + formatDate(it.OutboundDate)
	if it.ReturnDate.Valid {
		value += " - " + formatDate(it.ReturnDate.String)
	}
	if it.Description.Valid && it.Description.String != "" {
		value += " — " + it.Description.String
	}
	return value
}

func respondError(s *discordgo.Session, i *discordgo.InteractionCreate, description string, ephemeral bool) {
	var flags discordgo.MessageFlags
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{{
				Title:       "Error",
				Description: description,
				Color:       errorColor,
			}},
			Flags: flags,
		},
	}); err != nil {
		log.Printf("respond error: %v", err)
	}
}

func respondSuccess(s *discordgo.Session, i *discordgo.InteractionCreate, title, description string, ephemeral bool) {
	var flags discordgo.MessageFlags
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{{
				Title:       title,
				Description: description,
				Color:       successColor,
			}},
			Flags: flags,
		},
	}); err != nil {
		log.Printf("respond error: %v", err)
	}
}
