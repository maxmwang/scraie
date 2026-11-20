package main

import (
	"context"
	"flag"
	"log"

	"golang.org/x/sync/errgroup"

	"github.com/maxmwang/scraie/flights/internal/discord"
	"github.com/maxmwang/scraie/flights/internal/http"
	"github.com/maxmwang/scraie/flights/internal/scraie"
)

func main() {
	discordMode := flag.Bool("discord", false, "Run with discord bot")
	httpMode := flag.Bool("http", false, "Run with http server")
	scraieMode := flag.Bool("scraie", false, "Run with scheduled jobs")
	scraieNoSearch := flag.Bool("scraie-no-search", false, "Don't execute scraie searches")
	scraieReadOnly := flag.Bool("scraie-read-only", false, "Don't write options to the database")
	flag.Parse()

	if !*discordMode && !*httpMode && !*scraieMode {
		log.Fatal("select at least one mode: -discord, -http, or -scraie")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)

	if *discordMode {
		g.Go(func() error {
			return discord.Handle(ctx)
		})
	}
	if *httpMode {
		g.Go(func() error {
			return http.Handle(ctx, http.Args{})
		})
	}
	if *scraieMode {
		g.Go(func() error {
			scraie.Handle(ctx, scraie.Args{
				NoSearch: *scraieNoSearch,
				Readonly: *scraieReadOnly,
			})
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		log.Fatal(err)
	}
}
