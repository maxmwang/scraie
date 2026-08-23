package main

import (
	"context"
	"flag"

	"github.com/maxmwang/scraie/flights/internal/app"
)

// Local entry point. Both flags default to false, so a plain run searches and
// writes exactly as the deployed function does. The deployed function runs via
// handler.Handle (see project.yml), which passes no flags.
func main() {
	nosearch := flag.Bool("nosearch", false, "skip the search and only run the Discord notifications")
	readonly := flag.Bool("readonly", false, "search and notify without writing options to the database")
	flag.Parse()

	app.Handle(context.Background(), app.Args{NoSearch: *nosearch, Readonly: *readonly})
}
