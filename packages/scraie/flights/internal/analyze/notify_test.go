package analyze

import (
	"testing"
	"time"

	"github.com/maxmwang/scraie/flights/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

// ts is a fixed base time so the test cases are deterministic.
var ts = time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)

// opt builds a db.Option with just the fields the function under test reads.
func opt(id int64, price int32, searchedAt time.Time) db.Option {
	return db.Option{
		ID:         id,
		Price:      price,
		SearchedAt: pgtype.Timestamptz{Time: searchedAt, Valid: true},
	}
}

func TestFindCheapeastOptionOfPreviousAndLatestSearchTimestamp(t *testing.T) {
	t0 := ts                    // oldest search
	t1 := ts.Add(1 * time.Hour) // previous search
	t2 := ts.Add(2 * time.Hour) // latest search

	tests := []struct {
		name string
		// options are ordered oldest-first, matching GetOptionsSince.
		options    []db.Option
		wantPrevID int64 // cheapest option at the previous (second-latest) timestamp
		wantLateID int64 // cheapest option at the latest timestamp
	}{
		{
			name: "multiple options per search",
			options: []db.Option{
				opt(1, 100, t0),
				opt(2, 90, t0),
				opt(3, 80, t1),
				opt(4, 70, t1), // cheapest at the previous (t1) search
				opt(5, 60, t2),
				opt(6, 55, t2), // cheapest at the latest (t2) search
			},
			wantPrevID: 4,
			wantLateID: 6,
		},
		{
			name: "single option in the previous search",
			options: []db.Option{
				opt(1, 200, t0),
				opt(2, 150, t1), // only option at t1
				opt(3, 120, t2),
				opt(4, 110, t2), // cheapest at t2
			},
			wantPrevID: 2,
			wantLateID: 4,
		},
		{
			name: "cheapest latest is not the most recently inserted",
			options: []db.Option{
				opt(1, 100, t0),
				opt(2, 90, t1),
				opt(3, 80, t1), // cheapest at t1
				opt(4, 75, t2),
				opt(5, 60, t2), // cheapest at t2, not the last row
				opt(6, 70, t2),
			},
			wantPrevID: 3,
			wantLateID: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrev, gotLate := findCheapeastOptionOfPreviousAndLatestSearchTimestamp(tt.options)
			if gotPrev.ID != tt.wantPrevID {
				t.Errorf("previous = option %d (price %d), want option %d", gotPrev.ID, gotPrev.Price, tt.wantPrevID)
			}
			if gotLate.ID != tt.wantLateID {
				t.Errorf("latest = option %d (price %d), want option %d", gotLate.ID, gotLate.Price, tt.wantLateID)
			}
		})
	}
}
