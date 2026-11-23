package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/maxmwang/scraie/flights/internal/config"
	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/discord"
	"github.com/maxmwang/scraie/flights/internal/http"
	"github.com/maxmwang/scraie/flights/internal/scraie"
	"github.com/maxmwang/scraie/flights/internal/util/option"
)

func main() {
	httpMode := flag.Bool("http", false, "Run with http server")
	noSearch := flag.Bool("no-search", false, "Don't execute scraie searches")
	readOnly := flag.Bool("read-only", false, "Don't write options to the database")
	timezone := flag.String("timezone", "", "Timezone used for scheduled runs")
	scheduleHours := flag.String("schedule-hours", "6,18", "Comma-separated hours to run, from 0 to 23")
	runImmediately := flag.Bool("run-on-start", false, "Run once immediately when the server starts")
	flag.Parse()

	runHours, err := parseScheduleHours(*scheduleHours)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	ctx = context.WithValue(ctx, config.ContextKey, cfg)

	pool := db.Connect(ctx)
	defer pool.Close()
	ctx = context.WithValue(ctx, db.ContextKey, pool)

	g, ctx := errgroup.WithContext(ctx)

	bot, err := discord.New(ctx)
	if err != nil {
		log.Fatal(err)
	}
	notifier := bot.Notifier()
	defer bot.Close()
	g.Go(func() error {
		return bot.Run(ctx)
	})

	g.Go(func() error {
		return scraie.Run(
			ctx,
			scraie.ServerArgs{
				LocationName:   option.SomeIfPtr(timezone != nil, timezone),
				RunHours:       runHours,
				RunImmediately: *runImmediately,
			}, scraie.JobArgs{
				NoSearch: *noSearch,
				Readonly: *readOnly,
				Notifier: notifier,
			})
	})

	if *httpMode {
		g.Go(func() error {
			return http.Handle(ctx, http.Args{})
		})
	}

	err = g.Wait()
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func parseScheduleHours(value string) ([]int, error) {
	parts := strings.Split(value, ",")
	hours := make([]int, 0, len(parts))
	for _, part := range parts {
		hour, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("invalid schedule hour %q", part)
		}
		hours = append(hours, hour)
	}
	return hours, nil
}
