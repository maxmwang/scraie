package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maxmwang/scraie/flights/internal/config"
	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/scraie"
)

// ---------- commands ----------

func jsonOption(desc string) *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type:        discordgo.ApplicationCommandOptionString,
		Name:        "json",
		Description: desc,
		Required:    true,
		MaxLength:   6000, // Discord's cap for a string option
	}
}

func idOption() *discordgo.ApplicationCommandOption {
	return &discordgo.ApplicationCommandOption{
		Type: discordgo.ApplicationCommandOptionInteger, Name: "id",
		Description: "Itinerary ID", Required: true,
		MinValue: func() *float64 { v := 1.0; return &v }(),
	}
}

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "create",
		Description: "Track a new itinerary from JSON",
		Options:     []*discordgo.ApplicationCommandOption{jsonOption("Itinerary as single-line JSON")},
	},
	{Name: "list", Description: "List itineraries"},
	{Name: "get", Description: "Get an itinerary", Options: []*discordgo.ApplicationCommandOption{idOption()}},
	{
		Name:        "update",
		Description: "Update an itinerary with JSON (only included keys change)",
		Options: []*discordgo.ApplicationCommandOption{
			idOption(),
			jsonOption("Fields to change as single-line JSON; null clears an optional field"),
		},
	},
}

// ---------- JSON input shape ----------

// Plain fields keep their current/default value when the key is omitted.
// Pointer fields are optional in the DB: omit = unchanged, null = clear.
type ItineraryInput struct {
	DepartureID     string          `json:"departure_id"`
	ArrivalID       string          `json:"arrival_id"`
	Type            int32           `json:"type"`
	OutboundDate    string          `json:"outbound_date"`
	ReturnDate      *string         `json:"return_date"`
	TravelClass     int32           `json:"travel_class"`
	ShowHidden      bool            `json:"show_hidden"`
	MultiCity       json.RawMessage `json:"multi_city"` // nested JSON, stored as text
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

// ---------- conversion helpers ----------

func tx(p *string) pgtype.Text {
	if p == nil || strings.TrimSpace(*p) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *p, Valid: true}
}

func i4(p *int32) pgtype.Int4 {
	if p == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *p, Valid: true}
}

func strPtr(t pgtype.Text) *string {
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
		DepartureID: it.DepartureID, ArrivalID: it.ArrivalID, Type: it.Type,
		OutboundDate: it.OutboundDate, ReturnDate: strPtr(it.ReturnDate),
		TravelClass: it.TravelClass, ShowHidden: it.ShowHidden,
		ExcludeBasic: it.ExcludeBasic, DeepSearch: it.DeepSearch,
		Adults: it.Adults, Children: it.Children,
		InfantsInSeat: it.InfantsInSeat, InfantsOnLap: it.InfantsOnLap,
		SortBy: it.SortBy, Stops: it.Stops,
		ExcludeAirlines: strPtr(it.ExcludeAirlines), IncludeAirlines: strPtr(it.IncludeAirlines),
		Bags: it.Bags, MaxPrice: int32Ptr(it.MaxPrice),
		OutboundTimes: strPtr(it.OutboundTimes), ReturnTimes: strPtr(it.ReturnTimes),
		Emissions: int32Ptr(it.Emissions), LayoverDuration: strPtr(it.LayoverDuration),
		ExcludeConns: strPtr(it.ExcludeConns), MaxDuration: int32Ptr(it.MaxDuration),
		Gl: it.Gl, Hl: it.Hl, Currency: it.Currency,
		Notify: it.Notify, Description: strPtr(it.Description),
	}
	if it.MultiCityJson.Valid && json.Valid([]byte(it.MultiCityJson.String)) {
		in.MultiCity = json.RawMessage(it.MultiCityJson.String)
	}
	return in
}

// base carries over ID and Invalid (zero value for create).
func toItinerary(in ItineraryInput, base db.Itinerary) db.Itinerary {
	it := base
	it.DepartureID = strings.ToUpper(strings.TrimSpace(in.DepartureID))
	it.ArrivalID = strings.ToUpper(strings.TrimSpace(in.ArrivalID))
	it.Type = in.Type
	it.OutboundDate = strings.TrimSpace(in.OutboundDate)
	it.ReturnDate = tx(in.ReturnDate)
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
	it.ExcludeAirlines = tx(in.ExcludeAirlines)
	it.IncludeAirlines = tx(in.IncludeAirlines)
	it.Bags = in.Bags
	it.MaxPrice = i4(in.MaxPrice)
	it.OutboundTimes = tx(in.OutboundTimes)
	it.ReturnTimes = tx(in.ReturnTimes)
	it.Emissions = i4(in.Emissions)
	it.LayoverDuration = tx(in.LayoverDuration)
	it.ExcludeConns = tx(in.ExcludeConns)
	it.MaxDuration = i4(in.MaxDuration)
	it.Gl = strings.ToLower(strings.TrimSpace(in.Gl))
	it.Hl = strings.ToLower(strings.TrimSpace(in.Hl))
	it.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	it.Notify = in.Notify
	it.Description = tx(in.Description)

	it.MultiCityJson = pgtype.Text{}
	if s := strings.TrimSpace(string(in.MultiCity)); s != "" && s != "null" {
		it.MultiCityJson = pgtype.Text{String: s, Valid: true}
	}
	return it
}

// ---------- decoding + validation ----------

// Decodes over dst, so keys the user omits keep dst's existing values.
func decodeInto(raw string, dst *ItineraryInput) error {
	// Tolerate a pasted ```json fence.
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

// Discord no longer enforces choices/min/max for us, so check here.
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

// ---------- handlers ----------

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

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, msg string, ephemeral bool) {
	var flags discordgo.MessageFlags
	if ephemeral {
		flags = discordgo.MessageFlagsEphemeral
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg, Flags: flags},
	})
	if err != nil {
		log.Printf("respond error: %v", err)
	}
}

func respondEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, title, description string) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{{Title: title, Description: description}},
		},
	})
	if err != nil {
		log.Printf("respond error: %v", err)
	}
}

type bot struct {
	db *db.Queries
}

func (b bot) handleCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	in := defaultInput()
	if err := decodeInto(optString(i.ApplicationCommandData().Options, "json"), &in); err != nil {
		respond(s, i, "❌ Invalid JSON: `"+err.Error()+"`", true)
		return
	}
	it := toItinerary(in, db.Itinerary{})
	if err := validateRanges(&it); err != nil {
		respond(s, i, "❌ "+err.Error(), true)
		return
	}
	id, err := b.db.CreateItinerary(context.Background(), db.CreateItineraryParams{
		DepartureID: it.DepartureID, ArrivalID: it.ArrivalID, Type: it.Type, OutboundDate: it.OutboundDate,
		ReturnDate: it.ReturnDate, TravelClass: it.TravelClass, ShowHidden: it.ShowHidden, MultiCityJson: it.MultiCityJson,
		ExcludeBasic: it.ExcludeBasic, DeepSearch: it.DeepSearch, Adults: it.Adults, Children: it.Children,
		InfantsInSeat: it.InfantsInSeat, InfantsOnLap: it.InfantsOnLap, SortBy: it.SortBy, Stops: it.Stops,
		ExcludeAirlines: it.ExcludeAirlines, IncludeAirlines: it.IncludeAirlines, Bags: it.Bags, MaxPrice: it.MaxPrice,
		OutboundTimes: it.OutboundTimes, ReturnTimes: it.ReturnTimes, Emissions: it.Emissions,
		LayoverDuration: it.LayoverDuration, ExcludeConns: it.ExcludeConns, MaxDuration: it.MaxDuration,
		Gl: it.Gl, Hl: it.Hl, Currency: it.Currency,
	})
	if err != nil {
		respond(s, i, "❌ Couldn't save that itinerary.", true)
		return
	}
	it.ID = id
	respond(s, i, fmt.Sprintf("✅ Created `#%d`: %s", id, summary(it)), false)
}

func (b bot) handleList(s *discordgo.Session, i *discordgo.InteractionCreate) {
	itineraries, err := b.db.GetItineraries(context.Background())
	if err != nil {
		respond(s, i, "❌ Couldn't load itineraries.", true)
		return
	}
	if len(itineraries) == 0 {
		respond(s, i, "No itineraries.", false)
		return
	}
	var lines []string
	for _, it := range itineraries {
		lines = append(lines, fmt.Sprintf("`#%d` %s", it.ID, summary(it)))
	}
	respondEmbed(s, i, "Itineraries", strings.Join(lines, "\n"))
}

func (b bot) itinerary(id int64) (db.Itinerary, error) {
	itineraries, err := b.db.GetItineraries(context.Background())
	if err != nil {
		return db.Itinerary{}, err
	}
	for _, it := range itineraries {
		if it.ID == id {
			return it, nil
		}
	}
	return db.Itinerary{}, fmt.Errorf("itinerary #%d not found", id)
}

func (b bot) handleGet(s *discordgo.Session, i *discordgo.InteractionCreate) {
	id := optInt(i.ApplicationCommandData().Options, "id")
	it, err := b.itinerary(id)
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

func (b bot) handleUpdate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := i.ApplicationCommandData().Options
	id := optInt(opts, "id")

	existing, err := b.itinerary(id)
	if err != nil {
		respond(s, i, fmt.Sprintf("❌ No itinerary `#%d`.", id), true)
		return
	}

	in := fromItinerary(existing)                                    // start from current values...
	if err := decodeInto(optString(opts, "json"), &in); err != nil { // ...and overlay the user's keys
		respond(s, i, "❌ Invalid JSON: `"+err.Error()+"`", true)
		return
	}
	it := toItinerary(in, existing)
	if err := validateRanges(&it); err != nil {
		respond(s, i, "❌ "+err.Error(), true)
		return
	}
	if _, err := b.db.UpdateItinerary(context.Background(), db.ToUpdateParams(it)); err != nil {
		respond(s, i, "❌ Couldn't save changes.", true)
		return
	}
	respond(s, i, fmt.Sprintf("✅ Updated `#%d`: %s", it.ID, summary(it)), false)
}

func summary(it db.Itinerary) string {
	s := fmt.Sprintf("%s → %s, %s", it.DepartureID, it.ArrivalID, it.OutboundDate)
	if it.ReturnDate.Valid {
		s += "–" + it.ReturnDate.String
	}
	if it.Description.Valid && it.Description.String != "" {
		s += " — " + it.Description.String
	}
	return s
}

type Bot struct {
	session   *discordgo.Session
	channelID string
	pool      *pgxpool.Pool
	closeOnce sync.Once
}

func New(ctx context.Context) (*Bot, error) {
	cfg := config.FromContext(ctx)
	ctx = context.WithValue(ctx, config.ContextKey, cfg)
	pool := db.Connect(ctx)

	s, err := discordgo.New("Bot " + cfg.DiscordBotToken)
	if err != nil {
		pool.Close()
		return nil, err
	}
	b := bot{db: db.New(pool)}
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.ApplicationCommandData().Name {
		case "create":
			b.handleCreate(s, i)
		case "list":
			b.handleList(s, i)
		case "get":
			b.handleGet(s, i)
		case "update":
			b.handleUpdate(s, i)
		}
	})
	if err := s.Open(); err != nil {
		pool.Close()
		return nil, err
	}
	for _, command := range commands {
		if _, err := s.ApplicationCommandCreate(s.State.User.ID, "", command); err != nil {
			s.Close()
			pool.Close()
			return nil, err
		}
	}

	log.Println("bot is running")
	return &Bot{
		session:   s,
		channelID: cfg.DiscordChannelID,
		pool:      pool,
	}, nil
}

func (b *Bot) Run(ctx context.Context) error {
	defer b.Close()
	<-ctx.Done()
	return ctx.Err()
}

func (b *Bot) Close() {
	b.closeOnce.Do(func() {
		b.session.Close()
		b.pool.Close()
	})
}

func (b *Bot) Notifier() scraie.Notifier {
	return &notifier{session: b.session, channelID: b.channelID}
}

func Handle(ctx context.Context) error {
	b, err := New(ctx)
	if err != nil {
		return err
	}
	return b.Run(ctx)
}
