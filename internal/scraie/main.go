package scraie

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/maxmwang/scraie/flights/internal/util/option"
)

type ServerArgs struct {
	LocationName   option.Option[string]
	RunHours       []int
	RunImmediately bool
}

func Run(ctx context.Context, serverArgs ServerArgs, jobArgs JobArgs) error {
	var location *time.Location
	if locationName, ok := serverArgs.LocationName.Value(); ok {
		var err error
		location, err = time.LoadLocation(locationName)
		if err != nil {
			return fmt.Errorf("load timezone %q: %w", locationName, err)
		}
	} else {
		location = time.Local
	}

	runHours, err := validateRunHours(serverArgs.RunHours)
	if err != nil {
		return err
	}

	if serverArgs.RunImmediately {
		Handle(ctx, jobArgs)
	}

	for {
		next := nextRun(time.Now().In(location), location, runHours)
		log.Info().
			Time("next_run", next).
			Str("timezone", location.String()).
			Msg("scheduled next run")

		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
			Handle(ctx, jobArgs)
		}
	}
}

func validateRunHours(hours []int) ([]int, error) {
	if len(hours) == 0 {
		hours = []int{6, 18}
	}

	unique := make(map[int]struct{}, len(hours))
	validated := make([]int, 0, len(hours))
	for _, hour := range hours {
		if hour < 0 || hour > 23 {
			return nil, fmt.Errorf("schedule hour must be between 0 and 23: %d", hour)
		}
		if _, exists := unique[hour]; exists {
			continue
		}
		unique[hour] = struct{}{}
		validated = append(validated, hour)
	}

	slices.Sort(validated)
	return validated, nil
}

func nextRun(now time.Time, location *time.Location, runHours []int) time.Time {
	for _, hour := range runHours {
		next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, location)
		if next.After(now) {
			return next
		}
	}

	tomorrow := now.AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), runHours[0], 0, 0, 0, location)
}
