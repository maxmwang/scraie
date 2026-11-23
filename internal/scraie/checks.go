package scraie

import (
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
	"github.com/maxmwang/scraie/flights/internal/util/option"
)

// priceFloat reads an Options.price NUMERIC as a float64, which is what every
// comparison, percentage and chart value in this package works in. A fare is
// far inside float64's exact range, so the only precision this can lose is
// below the cent. An invalid or NaN NUMERIC reads as 0, matching how a missing
// price behaved when the column was an integer.
func priceFloat(p pgtype.Numeric) float64 {
	f, err := p.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

type checkResults struct {
	Near7DayMinimum option.Option[float64]
	PriceMovement   option.Option[float64]
}

func makeChecks(dailyCheapestOptions []db.Option) checkResults {
	return checkResults{
		Near7DayMinimum: checkNear7DayMinimum(dailyCheapestOptions),
		PriceMovement:   checkPriceMovement(dailyCheapestOptions),
	}
}

func (c checkResults) any() bool {
	return c.Near7DayMinimum.IsSome() || c.PriceMovement.IsSome()
}

// near7DayMinimumThreshold is the precentage the newly scraped minimum
// needs to be near the 7 day minimum to pass the checkNear7DayMinimum
// check.
const near7DayMinimumThreshold = 0.01

// checkNear7DayMinimum first calculates the (minimum) prices of the past 7
// days, then returns true if the newly scraped minimum is at or below the
// 7 day minimum, or within 1% above it.
//
// Expects options to be sorted by time and includes only cheapest options.
func checkNear7DayMinimum(options []db.Option) option.Option[float64] {
	if len(options) < 2 {
		return option.None[float64]()
	}

	// includes latest scrape when calculating 7 day minimum
	prev7DayMinimum := priceFloat(options[len(options)-1].Price)
	for i := len(options) - 2; i >= 0; i-- {
		if options[i].SearchedAt.Time.Before(time.Now().AddDate(0, 0, -7).Add(-time.Hour)) {
			break
		}
		if p := priceFloat(options[i].Price); p < prev7DayMinimum {
			prev7DayMinimum = p
		}
	}

	newPrice := priceFloat(options[len(options)-1].Price)

	return option.SomeIf(math.Abs(prev7DayMinimum-newPrice)/prev7DayMinimum <= near7DayMinimumThreshold, prev7DayMinimum)
}

// priceMovementThreshold is the minimum percent change from the previous
// scrape minimum to the newly scraped minimum needed to pass the
// checkPriceMovement check.
const priceMovementThreshold = 0.05

// checkPriceMovement calculates the (minimum) price from the
// previous scrape (len(options) - 2), then returns true if the newly scraped
// minimum moved at least 5% in either direction. Returns false if the input
// slice contains less than 2 entries.
func checkPriceMovement(options []db.Option) option.Option[float64] {
	if len(options) < 2 {
		return option.None[float64]()
	}

	prevMin := priceFloat(options[len(options)-2].Price)
	newMin := priceFloat(options[len(options)-1].Price)

	return option.SomeIf(math.Abs(prevMin-newMin)/prevMin >= priceMovementThreshold, prevMin)
}
