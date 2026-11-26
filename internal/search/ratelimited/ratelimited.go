package ratelimited

import (
	"context"
	"math/rand"
	"time"

	"golang.org/x/time/rate"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/search"
)

type ratelimited struct {
	searcher search.Searcher
	limiter  *rate.Limiter

	// jitter is the range, in milliseconds, of randomization of delay applied in
	// addition to the limiter.
	jitter int
}

func New(searcher search.Searcher, limiter *rate.Limiter, jitter int) search.Searcher {
	return ratelimited{searcher, limiter, jitter}
}

func NewDefault(searcher search.Searcher) search.Searcher {
	return New(
		searcher,
		rate.NewLimiter(rate.Every(5*time.Second), 2),
		5000,
	)
}

func (r ratelimited) Search(ctx context.Context, it db.Itinerary) (search.Result, error) {
	err := r.limiter.Wait(ctx)
	if err != nil {
		return search.Result{}, err
	}

	jitter := time.NewTimer(time.Duration(rand.Intn(r.jitter) * int(time.Millisecond)))
	defer jitter.Stop()
	select {
	case <-jitter.C:
	case <-ctx.Done():
		return search.Result{}, ctx.Err()
	}

	results, err := r.searcher.Search(ctx, it)
	if err != nil {
		return search.Result{}, err
	}
	return results, nil
}
