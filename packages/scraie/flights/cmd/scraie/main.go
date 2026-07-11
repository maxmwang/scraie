package main

import (
	"context"

	"github.com/maxmwang/scraie/flights/internal/app"
)

// Local entry point. Runs in read-only mode so a local run searches and reports
// without writing to the database. The deployed function runs via handler.Handle
// (see project.yml) with writes enabled.
func main() {
	app.Handle(context.Background(), app.Args{Readonly: true})
}
