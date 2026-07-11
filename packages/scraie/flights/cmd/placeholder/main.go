package main

// For testing: placeholder command — remove cmd/placeholder and
// internal/analyze/placeholder.go when done.
//
// Run with your webhook set:
//   SCRAIE_DISCORD_WEBHOOK=... go run ./cmd/placeholder
//
// Or pass it as the first arg:
//   go run ./cmd/placeholder "https://discord.com/api/webhooks/..."

import (
	"context"
	"fmt"
	"os"

	"github.com/maxmwang/scraie/flights/internal/analyze"
	"github.com/maxmwang/scraie/flights/internal/config"
)

func main() {
	webhook := os.Getenv("SCRAIE_DISCORD_WEBHOOK")
	if len(os.Args) > 1 {
		webhook = os.Args[1]
	}
	if webhook == "" {
		fmt.Fprintln(os.Stderr, "set SCRAIE_DISCORD_WEBHOOK or pass the webhook URL as the first arg")
		os.Exit(1)
	}

	ctx := context.WithValue(context.Background(), config.ContextKey, config.Config{DiscordWebhook: webhook})
	if err := analyze.SendPlaceholder(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "send failed:", err)
		os.Exit(1)
	}
	fmt.Println("sent placeholder notification")
}
