// Package telemetry attaches content-free operation outcomes to request logs.
package telemetry

import "net/http"

// Outcome contains only server-controlled categories and aggregate counts.
// Never put bodies, queries, identities, tokens or exception text here.
type Outcome struct {
	ErrorCode   string
	ResultCount *int
	PostKind    string
	Reply       bool
	Structured  bool
	Replayed    bool
}

// Record follows standard ResponseWriter wrappers. Internal API calls without
// a request logger remain valid and do not create extra log events.
func Record(w http.ResponseWriter, outcome Outcome) {
	for {
		if recorder, ok := w.(interface{ RecordOutcome(Outcome) }); ok {
			recorder.RecordOutcome(outcome)
			return
		}
		wrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		w = wrapper.Unwrap()
	}
}
