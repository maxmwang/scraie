package scraie

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search/goflights"
	"github.com/maxmwang/scraie/flights/internal/util"
)

type Notifier interface {
	Notify(context.Context, Notification) error
}

type JobArgs struct {
	NoSearch bool
	Readonly bool
	Notifier Notifier
}

func Handle(ctx context.Context, args JobArgs) {
	log.Info().
		Bool("nosearch", args.NoSearch).
		Bool("readonly", args.Readonly).
		Str("notifier", "TODO").
		Msg("starting run")

	pool := db.FromContext(ctx)
	itineraries, err := db.New(pool).GetItineraries(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("error loading itineraries")
		return
	}

	searcher := goflights.New()

	var wg sync.WaitGroup
	for _, it := range itineraries {
		if it.Invalid {
			continue
		}
		wg.Go(func() {
			if !args.NoSearch {
				result, err := searcher.Search(it)
				if err != nil {
					log.Error().
						Err(err).
						Int64("itinerary_id", it.ID).
						Msg("itinerary search error")
					return
				}

				log.Info().
					Int64("itinerary_id", it.ID).
					Str("desc", util.ItineraryToString(it)).
					Msg(fmt.Sprintf("found %d option(s)", len(result.Options)))

				searchedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}

				if !args.Readonly {
					for i, option := range result.Options {
						logError := func(err error, msg string) {
							log.Error().
								Err(err).
								Int64("itinerary_id", it.ID).
								Int("option_i", i).
								Msg(msg)
						}

						tx, err := pool.Begin(ctx)
						if err != nil {
							logError(err, "itinerary option transaction begin error")
							continue
						}
						qtx := db.New(pool).WithTx(tx)

						optionID, err := qtx.CreateOption(ctx, db.CreateOptionParams{
							ItineraryID:   option.Option.ItineraryID,
							TotalDuration: option.Option.TotalDuration,
							Price:         option.Option.Price,
							Type:          option.Option.Type,
							SearchedAt:    searchedAt,
						})
						if err != nil {
							logError(err, "itinerary option save error")
							tx.Rollback(ctx)
							continue
						}

						segmentParams := make([]db.BulkInsertSegmentsParams, 0, len(option.Segments))
						for _, s := range option.Segments {
							segmentParams = append(segmentParams, db.BulkInsertSegmentsParams{
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
							})
						}
						_, err = qtx.BulkInsertSegments(ctx, segmentParams)
						if err != nil {
							logError(err, "itinerary option segments save error")
							tx.Rollback(ctx)
							continue
						}

						layoverParams := make([]db.BulkInsertLayoversParams, len(option.Layovers))
						for k, l := range option.Layovers {
							layoverParams[k] = db.BulkInsertLayoversParams{
								OptionID:  optionID,
								Duration:  l.Duration,
								Name:      l.Name,
								AirportID: l.AirportID,
								Overnight: l.Overnight,
							}
						}
						if _, err := qtx.BulkInsertLayovers(ctx, layoverParams); err != nil {
							logError(err, "itinerary option layovers save error")
							tx.Rollback(ctx)
							continue
						}

						if err := tx.Commit(ctx); err != nil {
							logError(err, "itinerary option transaction commit error")
							continue
						}
					}

					log.Info().
						Int64("itinerary_id", it.ID).
						Str("desc", util.ItineraryToString(it)).
						Msg(fmt.Sprintf("saved %d option(s)", len(result.Options)))
				}
			}

			if it.Notify {
				notification, err := buildNotification(ctx, it)
				if err != nil {
					log.Error().
						Err(err).
						Int64("itinerary_id", it.ID).
						Msg("failed to build notification")
				}

				if notification, ok := notification.Value(); ok {
					err = args.Notifier.Notify(ctx, notification)
					if err != nil {
						log.Error().
							Err(err).
							Int64("itinerary_id", it.ID).
							Msg("itinerary discord notify")
					}
				}
			}
		})
	}
	wg.Wait()
}
