package main

import (
	"context"
	"flag"

	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/maxmwang/scraie/flights/internal/app"
)

// Local entry point. Both flags default to false, so a plain run searches and
// writes exactly as the deployed function does. The deployed function runs via
// handler.Handle (see project.yml), which passes no flags.
func main() {
	nosearch := flag.Bool("nosearch", false, "skip the search and only run the Discord notifications")
	readonly := flag.Bool("readonly", false, "search and notify without writing options to the database")
	flag.Parse()

	setupLogger()

	app.Handle(context.Background(), app.Args{NoSearch: *nosearch, Readonly: *readonly})
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
