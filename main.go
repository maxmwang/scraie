package main

import (
	"context"

	"github.com/maxmwang/scraie/flights/internal/config"
	"github.com/maxmwang/scraie/flights/internal/discord"
	"github.com/maxmwang/scraie/flights/internal/scraie"
)

// Main is the DigitalOcean Functions entry point. The daily scheduler trigger
// sends an empty event, so scraie.Args.Readonly defaults to false and search
// results are persisted.
func Main(ctx context.Context, args scraie.JobArgs) map[string]interface{} {
	cfg := config.Load()
	ctx = context.WithValue(context.Background(), config.ContextKey, cfg)
	bot, err := discord.New(ctx)
	if err != nil {
		return map[string]interface{}{"body": err.Error()}
	}
	defer bot.Close()
	scraie.Handle(ctx, scraie.JobArgs{
		Notifier: bot.Notifier(),
	})
	return map[string]interface{}{"body": "ok"}
}
