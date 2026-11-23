package discord

import (
	"context"
	"crypto/rand"
	"errors"
	"log"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maxmwang/scraie/flights/internal/config"
	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/discord/commands"
	"github.com/maxmwang/scraie/flights/internal/scraie"
)

type Bot struct {
	session   *discordgo.Session
	pool      *pgxpool.Pool
	db        *db.Queries
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

	b := &Bot{session: s, pool: pool, db: db.New(pool)}
	commandHandler := &commands.Handler{
		DB:         b.db,
		EnsureUser: b.getOrCreateUser,
	}
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.ApplicationCommandData().Name {
		case "create":
			commands.HandleCreate(commandHandler, s, i)
		case "list":
			commands.HandleList(commandHandler, s, i)
		case "get":
			commands.HandleGet(commandHandler, s, i)
		case "update":
			commands.HandleUpdate(commandHandler, s, i)
		}
	})

	if err := s.Open(); err != nil {
		pool.Close()
		return nil, err
	}
	registeredCommands := []*discordgo.ApplicationCommand{
		commands.Create,
		commands.List,
		commands.Get,
		commands.Update,
	}
	for _, command := range registeredCommands {
		if _, err := s.ApplicationCommandCreate(s.State.User.ID, "", command); err != nil {
			s.Close()
			pool.Close()
			return nil, err
		}
	}

	log.Println("bot is running")
	return b, nil
}

func discordUserID(i *discordgo.InteractionCreate) string {
	if i.User != nil {
		return i.User.ID
	}
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User.ID
	}
	return ""
}

func (b *Bot) getOrCreateUser(ctx context.Context, i *discordgo.InteractionCreate) (pgtype.UUID, bool) {
	discordID := discordUserID(i)
	if discordID == "" {
		return pgtype.UUID{}, false
	}
	user, err := b.db.GetUserByDiscordID(ctx, discordID)
	if err == nil {
		return user.ID, true
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, false
	}

	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return pgtype.UUID{}, false
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	userID, err := b.db.CreateDiscordUser(ctx, db.CreateDiscordUserParams{
		ID: pgtype.UUID{Bytes: bytes, Valid: true}, DiscordID: discordID,
	})
	if err != nil {
		return pgtype.UUID{}, false
	}
	return userID, true
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
	return &notifier{session: b.session, db: b.db}
}

func Handle(ctx context.Context) error {
	b, err := New(ctx)
	if err != nil {
		return err
	}
	return b.Run(ctx)
}
