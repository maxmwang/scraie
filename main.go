package main

import (
	"context"

	"github.com/maxmwang/scraie/flights/internal/scraie"
)

// Main is the DigitalOcean Functions entry point. The daily scheduler trigger
// sends an empty event, so scraie.Args.Readonly defaults to false and search
// results are persisted.
func Main(ctx context.Context, args scraie.Args) map[string]interface{} {
	scraie.Handle(context.Background(), scraie.Args{})
	return map[string]interface{}{"body": "ok"}
}
