package goals_test

import (
	"ajent/internal/goals"
	"testing"
	"time"
)

func TestDailyScheduleAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, after, want string
		hour              int
	}{
		{"normal", "2026-09-12T15:00:00Z", "2026-09-12T16:00:00Z", 9},
		{"at due advances", "2026-09-12T16:00:00Z", "2026-09-13T16:00:00Z", 9},
		{"spring gap", "2026-03-08T09:00:00Z", "2026-03-08T10:00:00Z", 2},
		{"fall repeated hour", "2026-11-01T08:00:00Z", "2026-11-02T09:00:00Z", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after, e := time.Parse(time.RFC3339, tc.after)
			if e != nil {
				t.Fatal(e)
			}
			got := goals.NextReport(after, loc, tc.hour)
			if got.Format(time.RFC3339) != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}
