package analyze

import (
	"math"
	"time"

	"github.com/maxmwang/scraie/flights/internal/db"
)

type checkResults struct {
	near7DayMinimum near7DayMinimumResult
	priceMovement   priceMovementResult
}

func makeChecks(dailyCheapestOptions []db.Option) checkResults {
	return checkResults{
		near7DayMinimum: checkNear7DayMinimum(dailyCheapestOptions),
		priceMovement:   checkPriceMovement(dailyCheapestOptions),
	}
}

func (c checkResults) any() bool {
	return c.near7DayMinimum.pass || c.priceMovement.pass
}

type near7DayMinimumResult struct {
	pass            bool
	prev7DayMinimum int32
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
func checkNear7DayMinimum(options []db.Option) near7DayMinimumResult {
	if len(options) < 2 {
		return near7DayMinimumResult{}
	}

	// includes latest scrape when calculating 7 day minimum
	prev7DayMinimum := options[len(options)-1].Price
	for i := len(options) - 2; i >= 0; i-- {
		if options[i].SearchedAt.Time.Before(time.Now().AddDate(0, 0, -7).Add(-time.Hour)) {
			break
		}
		if options[i].Price < prev7DayMinimum {
			prev7DayMinimum = options[i].Price
		}
	}

	newPrice := options[len(options)-1].Price

	if math.Abs(float64(prev7DayMinimum-newPrice))/float64(prev7DayMinimum) <= near7DayMinimumThreshold {
		return near7DayMinimumResult{pass: true, prev7DayMinimum: prev7DayMinimum}
	} else {
		return near7DayMinimumResult{}
	}
}

type priceMovementResult struct {
	pass bool
	prev int32
}

// priceMovementThreshold is the minimum percent change from the previous
// scrape minimum to the newly scraped minimum needed to pass the
// checkPriceMovement check.
const priceMovementThreshold = 0.05

// checkPriceMovement calculates the (minimum) price from the
// previous scrape (len(options) - 2), then returns true if the newly scraped
// minimum moved at least 5% in either direction. Returns false if the input
// slice contains less than 2 entries.
func checkPriceMovement(options []db.Option) priceMovementResult {
	if len(options) < 2 {
		return priceMovementResult{}
	}

	prevMin := options[len(options)-2].Price
	newMin := options[len(options)-1].Price

	if math.Abs(float64(prevMin-newMin))/float64(prevMin) >= priceMovementThreshold {
		return priceMovementResult{pass: true, prev: prevMin}
	} else {
		return priceMovementResult{}
	}
}
