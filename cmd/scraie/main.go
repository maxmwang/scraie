package main

import (
	"context"
	"flag"

	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/maxmwang/scraie/flights/internal/config"
	"github.com/maxmwang/scraie/flights/internal/discord"
	"github.com/maxmwang/scraie/flights/internal/scraie"
)

func main() {
	nosearch := flag.Bool("nosearch", false, "skip the search and only run the Discord notifications")
	readonly := flag.Bool("readonly", false, "search and notify without writing options to the database")
	flag.Parse()

	setupLogger()

	cfg := config.Load()
	ctx := context.WithValue(context.Background(), config.ContextKey, cfg)
	bot, err := discord.New(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("bot setup failed")
	}
	defer bot.Close()
	scraie.Handle(ctx, scraie.JobArgs{
		NoSearch: *nosearch,
		Readonly: *readonly,
		Notifier: bot.Notifier(),
	})
}

func setupLogger() {
	if b, err := strconv.ParseBool(os.Getenv("LOG_JSON")); err == nil && b == true {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			NoColor:    false,
			TimeFormat: time.DateTime,
		})
	}
}
