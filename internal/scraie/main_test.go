package scraie

import (
	"testing"
	"time"
)

func TestNextRun(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		now   time.Time
		want  time.Time
		hours []int
	}{
		{
			name:  "before first run",
			now:   time.Date(2026, 11, 22, 5, 30, 0, 0, location),
			want:  time.Date(2026, 11, 22, 6, 0, 0, 0, location),
			hours: []int{6, 18},
		},
		{
			name:  "between runs",
			now:   time.Date(2026, 11, 22, 12, 0, 0, 0, location),
			want:  time.Date(2026, 11, 22, 18, 0, 0, 0, location),
			hours: []int{6, 18},
		},
		{
			name:  "after final run",
			now:   time.Date(2026, 11, 22, 19, 0, 0, 0, location),
			want:  time.Date(2026, 11, 23, 6, 0, 0, 0, location),
			hours: []int{6, 18},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextRun(tt.now, location, tt.hours)
			if !got.Equal(tt.want) {
				t.Fatalf("nextRun() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateRunHours(t *testing.T) {
	hours, err := validateRunHours([]int{18, 6, 18})
	if err != nil {
		t.Fatal(err)
	}
	if len(hours) != 2 || hours[0] != 6 || hours[1] != 18 {
		t.Fatalf("validateRunHours() = %v, want [6 18]", hours)
	}

	if _, err := validateRunHours([]int{24}); err == nil {
		t.Fatal("validateRunHours() accepted an invalid hour")
	}
}
