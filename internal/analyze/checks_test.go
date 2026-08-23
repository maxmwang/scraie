package analyze

import (
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/maxmwang/scraie/flights/internal/db"
)

// opt builds a db.Option with just the fields the function under test reads.
func opt(id int64, price int64, searchedAt time.Time) db.Option {
	return db.Option{
		ID:         id,
		Price:      pgtype.Numeric{Int: big.NewInt(price), Valid: true},
		SearchedAt: pgtype.Timestamptz{Time: searchedAt, Valid: true},
	}
}

// nowMinusDays builds a timestamp relative to time.Now(), which is what
// checkNear7DayMinimum compares its 7 day window against. The fixed ts base
// used by the other tests in this package would fall outside that window.
func nowMinusDays(d int) time.Time {
	return time.Now().AddDate(0, 0, -d)
}

func TestCheckNear7DayMinimum(t *testing.T) {
	tests := []struct {
		name string
		// options are ordered oldest-first and hold one (cheapest) entry per day.
		options []db.Option
		want    bool
	}{
		{
			name:    "no options",
			options: []db.Option{},
			want:    false,
		},
		{
			name: "new price equals the 7 day minimum",
			options: []db.Option{
				opt(1, 120, nowMinusDays(3)),
				opt(2, 100, nowMinusDays(2)),
				opt(3, 110, nowMinusDays(1)),
				opt(4, 100, nowMinusDays(0)),
			},
			want: true,
		},
		{
			name: "new price exactly 1% above the minimum",
			options: []db.Option{
				opt(1, 120, nowMinusDays(2)),
				opt(2, 100, nowMinusDays(1)),
				opt(3, 101, nowMinusDays(0)),
			},
			want: true,
		},
		{
			name: "new price more than 1% above the minimum",
			options: []db.Option{
				opt(1, 120, nowMinusDays(2)),
				opt(2, 100, nowMinusDays(1)),
				opt(3, 102, nowMinusDays(0)),
			},
			want: false,
		},
		{
			name: "new price is a new low well under the minimum",
			options: []db.Option{
				opt(1, 100, nowMinusDays(1)),
				opt(2, 50, nowMinusDays(0)),
			},
			want: true,
		},
		{
			name:    "single option",
			options: []db.Option{opt(1, 100, nowMinusDays(0))},
			want:    false,
		},
		{
			name: "two options, unchanged price",
			options: []db.Option{
				opt(1, 100, nowMinusDays(1)),
				opt(2, 100, nowMinusDays(0)),
			},
			want: true,
		},
		{
			name: "prices older than 7 days are excluded from the minimum",
			options: []db.Option{
				opt(1, 50, nowMinusDays(10)), // outside the window, must be ignored
				opt(2, 100, nowMinusDays(3)),
				opt(3, 100, nowMinusDays(0)),
			},
			want: true,
		},
		{
			name: "minimum is taken from the oldest in-window scrape",
			options: []db.Option{
				opt(1, 80, nowMinusDays(6)),
				opt(2, 120, nowMinusDays(4)),
				opt(3, 120, nowMinusDays(2)),
				opt(4, 80, nowMinusDays(0)),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkNear7DayMinimum(tt.options); got.pass != tt.want {
				t.Errorf("checkNear7DayMinimum() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckPriceMovement(t *testing.T) {
	tests := []struct {
		name string
		// options are ordered oldest-first and hold one (cheapest) entry per day.
		options []db.Option
		want    bool
	}{
		{
			name:    "no options",
			options: []db.Option{},
			want:    false,
		},
		{
			name:    "single option",
			options: []db.Option{opt(1, 100, nowMinusDays(0))},
			want:    false,
		},
		{
			name: "price unchanged",
			options: []db.Option{
				opt(1, 100, nowMinusDays(1)),
				opt(2, 100, nowMinusDays(0)),
			},
			want: false,
		},
		{
			name: "increase within 5%",
			options: []db.Option{
				opt(1, 100, nowMinusDays(1)),
				opt(2, 104, nowMinusDays(0)),
			},
			want: false,
		},
		{
			name: "decrease of exactly 5%",
			options: []db.Option{
				opt(1, 100, nowMinusDays(1)),
				opt(2, 95, nowMinusDays(0)),
			},
			want: true,
		},
		{
			name: "decrease of more than 5%",
			options: []db.Option{
				opt(1, 100, nowMinusDays(1)),
				opt(2, 90, nowMinusDays(0)),
			},
			want: true,
		},
		{
			name: "increase of more than 5%",
			options: []db.Option{
				opt(1, 100, nowMinusDays(1)),
				opt(2, 110, nowMinusDays(0)),
			},
			want: true,
		},
		{
			name: "only the last two scrapes are compared",
			options: []db.Option{
				opt(1, 500, nowMinusDays(2)), // ignored, a 5x move would otherwise pass
				opt(2, 100, nowMinusDays(1)),
				opt(3, 102, nowMinusDays(0)),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkPriceMovement(tt.options); got.pass != tt.want {
				t.Errorf("checkPriceMovement() = %v, want %v", got, tt.want)
			}
		})
	}
}
