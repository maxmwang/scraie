package db

import (
	"context"
	_ "embed"

	"github.com/maxmwang/scraie/flights/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

//go:embed schema.sql
var schema string

func Connect(ctx context.Context) *pgxpool.Pool {
	pool, err := pgxpool.New(ctx, config.FromContext(ctx).DBURI)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to postgres")
	}

	if _, err := pool.Exec(ctx, schema); err != nil {
		log.Fatal().Err(err).Msg("failed to apply schema")
	}

	return pool
}
