package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/maxmwang/scraie/flights/internal/analyze"
	"github.com/maxmwang/scraie/flights/internal/config"
	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search/serp"
	"github.com/maxmwang/scraie/flights/internal/util"
)

type Args struct {
	Readonly bool
}

func Handle(ctx context.Context, args Args) {
	setupLogger()

	cfg := config.Load()
	ctx = context.WithValue(ctx, config.ContextKey, cfg)

	pool := db.Connect(ctx)
	defer pool.Close()
	dbClient := db.New(pool)

	itineraries, err := dbClient.GetItineraries(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("error loading itineraries")
		return
	}

	searcher := serp.New(cfg.SerpAPIKey)

	var wg sync.WaitGroup
	for _, it := range itineraries {
		wg.Go(func() {
			result, err := searcher.Search(it)
			if err != nil {
				log.Error().Err(err).Int64("itinerary_id", it.ID).Msg("itinerary search error")
				return
			}
			log.Info().Int64("itinerary_id", it.ID).Str("desc", util.ItineraryToString(it)).
				Msg(fmt.Sprintf("found %d option(s)", len(result.Options)))

			searchedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}

			if !args.Readonly {
				for j, or := range result.Options {
					tx, err := pool.Begin(ctx)
					if err != nil {
						log.Error().Err(err).Int64("itinerary_id", it.ID).Int("option_i", j).Msg("itinerary option transaction begin error")
						continue
					}
					qtx := dbClient.WithTx(tx)

					optionID, err := qtx.CreateOption(ctx, db.CreateOptionParams{
						ItineraryID:   or.Option.ItineraryID,
						TotalDuration: or.Option.TotalDuration,
						Price:         or.Option.Price,
						Type:          or.Option.Type,
						SearchedAt:    searchedAt,
					})
					if err != nil {
						log.Error().Err(err).Int64("itinerary_id", it.ID).Int("option_i", j).Msg("itinerary option save error")
						tx.Rollback(ctx)
						continue
					}

					segmentParams := make([]db.BulkInsertSegmentsParams, len(or.Segments))
					for k, s := range or.Segments {
						segmentParams[k] = db.BulkInsertSegmentsParams{
							OptionID:             optionID,
							DepartureAirportName: s.DepartureAirportName,
							DepartureAirportID:   s.DepartureAirportID,
							DepartureTime:        s.DepartureTime,
							ArrivalAirportName:   s.ArrivalAirportName,
							ArrivalAirportID:     s.ArrivalAirportID,
							ArrivalTime:          s.ArrivalTime,
							Duration:             s.Duration,
							Airplane:             s.Airplane,
							Airline:              s.Airline,
							AirlineLogo:          s.AirlineLogo,
							TravelClass:          s.TravelClass,
							FlightNumber:         s.FlightNumber,
							Overnight:            s.Overnight,
						}
					}
					if _, err := qtx.BulkInsertSegments(ctx, segmentParams); err != nil {
						log.Error().Err(err).Int64("itinerary_id", it.ID).Int("option_i", j).Msg("itinerary option segments save error")
						tx.Rollback(ctx)
						continue
					}

					layoverParams := make([]db.BulkInsertLayoversParams, len(or.Layovers))
					for k, l := range or.Layovers {
						layoverParams[k] = db.BulkInsertLayoversParams{
							OptionID:  optionID,
							Duration:  l.Duration,
							Name:      l.Name,
							AirportID: l.AirportID,
							Overnight: l.Overnight,
						}
					}
					if _, err := qtx.BulkInsertLayovers(ctx, layoverParams); err != nil {
						log.Error().Err(err).Int64("itinerary_id", it.ID).Int("option_i", j).Msg("itinerary option layovers save error")
						tx.Rollback(ctx)
						continue
					}

					if err := tx.Commit(ctx); err != nil {
						log.Error().Err(err).Int64("itinerary_id", it.ID).Int("option_i", j).Msg("itinerary option transaction commit error")
						continue
					}
				}

				log.Info().Int64("itinerary_id", it.ID).Str("desc", util.ItineraryToString(it)).
					Msg(fmt.Sprintf("saved %d option(s)", len(result.Options)))
			}

			if it.Notify {
				err = analyze.NotifyOnPriceChange(ctx, pool, it)
				if err != nil {
					log.Error().Err(err).Int64("itinerary_id", it.ID).Msg("itinerary discord notify")
				}
			}
		})
	}
	wg.Wait()
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
