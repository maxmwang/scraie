package main

import (
	"context"

	"github.com/maxmwang/scraie/flights/internal/app"
)

// Main is the DigitalOcean Functions entry point. The daily scheduler trigger
// sends an empty event, so app.Args.Readonly defaults to false and search
// results are persisted.
func Main(ctx context.Context, args app.Args) map[string]interface{} {
	app.Handle(context.Background(), app.Args{})
	return map[string]interface{}{"body": "ok"}
}
