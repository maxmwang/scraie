package http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"

	"github.com/maxmwang/scraie/flights/internal/config"
	"github.com/maxmwang/scraie/flights/internal/db"
)

type Args struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

func NewHandler(ctx context.Context, dbClient *db.Queries) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))

	r.Get("/itineraries", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		itineraries, err := dbClient.GetItineraries(ctx)
		if err != nil {
			http.Error(w, "failed to load itineraries", http.StatusInternalServerError)
			return
		}

		if err := json.NewEncoder(w).Encode(itineraries); err != nil {
			log.Printf("encode itineraries: %v", err)
		}
	})

	r.Put("/itinerary", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		var req db.Itinerary
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()

		if err := dec.Decode(&req); err != nil {
			http.Error(w, "invalid itinerary", http.StatusBadRequest)
			return
		}

		// TODO: check user has auth to modify itinerary id
		// TODO: validate new itinerary

		if _, err := dbClient.UpdateItinerary(ctx, db.ToUpdateParams(req)); err != nil {
			http.Error(w, "failed to update itinerary", http.StatusInternalServerError)
		}
	})

	return r
}

func Handle(ctx context.Context, args Args) error {
	if args.Addr == "" {
		args.Addr = ":8080"
	}
	if args.ReadTimeout == 0 {
		args.ReadTimeout = 5 * time.Second
	}
	if args.WriteTimeout == 0 {
		args.WriteTimeout = 10 * time.Second
	}
	if args.IdleTimeout == 0 {
		args.IdleTimeout = 60 * time.Second
	}

	cfg := config.Load()
	ctx = context.WithValue(ctx, config.ContextKey, cfg)

	pool := db.Connect(ctx)
	defer pool.Close()

	srv := &http.Server{
		Addr:         args.Addr,
		Handler:      NewHandler(ctx, db.New(pool)),
		ReadTimeout:  args.ReadTimeout,
		WriteTimeout: args.WriteTimeout,
		IdleTimeout:  args.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on %s", args.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		log.Println("server stopped")
		return nil
	}
}
