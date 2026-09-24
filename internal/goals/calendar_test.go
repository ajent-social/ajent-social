package goals

import (
	"testing"
	"time"
)

func TestNextReportCalendarTransitions(t *testing.T) {
	for _, tc := range []struct {
		name, zone, after, want string
		hour                    int
	}{
		{"midnight gap", "America/Santiago", "2026-09-05T16:00:00Z", "2026-09-06T04:00:00Z", 0},
		{"approaching midnight gap", "America/Santiago", "2026-09-06T03:30:00Z", "2026-09-06T04:00:00Z", 0},
		{"after midnight report", "America/Santiago", "2026-09-06T04:00:00Z", "2026-09-07T03:00:00Z", 0},
		{"quarter-hour zone gap", "Pacific/Chatham", "2026-09-26T00:00:00Z", "2026-09-26T14:00:00Z", 3},
		{"half-hour gap", "Australia/Lord_Howe", "2026-10-03T00:00:00Z", "2026-10-03T15:30:00Z", 2},
		{"skipped calendar date", "Pacific/Apia", "2011-12-29T20:00:00Z", "2011-12-30T19:00:00Z", 9},
		{"first repeated hour", "Europe/Berlin", "2026-10-24T12:00:00Z", "2026-10-25T00:00:00Z", 2},
		{"repeated hour only once", "Europe/Berlin", "2026-10-25T00:00:00Z", "2026-10-26T01:00:00Z", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			after, err := time.Parse(time.RFC3339, tc.after)
			if err != nil {
				t.Fatal(err)
			}
			if got := NextReport(after, loc, tc.hour).Format(time.RFC3339); got != tc.want {
				t.Fatalf("next report after %s in %s at hour %d: got %s, want %s", tc.after, tc.zone, tc.hour, got, tc.want)
			}
		})
	}
}
