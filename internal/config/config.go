package config

import (
	"context"
	"os"
	"strconv"

	"github.com/rs/zerolog/log"
)

const ContextKey = "scraie:config"

type Config struct {
	SerpAPIKey     string
	DBURI          string
	DiscordWebhook string
	LogJSON        bool
}

func Load() Config {
	cfg := Config{
		SerpAPIKey:     os.Getenv("FLIGHTS_SERPAPI_KEY"),
		DBURI:          os.Getenv("FLIGHTS_DB_URI"),
		DiscordWebhook: os.Getenv("SCRAIE_DISCORD_WEBHOOK"),
		LogJSON:        false,
	}
	if b, err := strconv.ParseBool(os.Getenv("LOG_JSON")); err == nil && b == true {
		cfg.LogJSON = true
	}

	validate(&cfg)

	return cfg
}

func validate(cfg *Config) {
	if cfg.SerpAPIKey == "" {
		log.Fatal().Msg("FLIGHTS_SERPAPI_KEY key not defined")
	}
	if cfg.DBURI == "" {
		log.Fatal().Msg("FLIGHTS_DB_URI key not defined")
	}
	if cfg.DiscordWebhook == "" {
		log.Fatal().Msg("SCRAIE_DISCORD_WEBHOOK key not defined")
	}
}

func FromContext(ctx context.Context) Config {
	cfgRaw := ctx.Value(ContextKey)
	if cfg, ok := cfgRaw.(Config); !ok {
		return Load()
	} else {
		return cfg
	}
}
