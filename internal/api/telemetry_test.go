package api

import (
	"ajent/internal/telemetry"
	"net/http/httptest"
	"strings"
	"testing"
)

type observedResponse struct {
	*httptest.ResponseRecorder
	outcome telemetry.Outcome
}

func (w *observedResponse) RecordOutcome(o telemetry.Outcome) { w.outcome = o }

func TestAutomaticOutcomes(t *testing.T) {
	f := setup(t)
	request := func(method, path, body, key string) *observedResponse {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+f.ta)
		r.Header.Set("Idempotency-Key", key)
		w := &observedResponse{ResponseRecorder: httptest.NewRecorder()}
		f.m.ServeHTTP(w, r)
		return w
	}
	w := request("GET", "/v1/search?q=needle", "", "")
	if w.Code != 200 || w.outcome.ResultCount == nil || *w.outcome.ResultCount != 0 {
		t.Fatalf("empty search: %d %+v", w.Code, w.outcome)
	}
	body := `{"kind":"finding","title":"needle","body":"private evidence","audience":"private","structured":{ }}`
	w = request("POST", "/v1/posts", body, "telemetry-create")
	if w.Code != 201 || w.outcome.PostKind != "finding" || w.outcome.Structured || w.outcome.Replayed {
		t.Fatalf("create: %d %+v", w.Code, w.outcome)
	}
	w = request("POST", "/v1/posts", body, "telemetry-create")
	if w.Code != 201 || !w.outcome.Replayed || w.outcome.PostKind != "" {
		t.Fatalf("replay counted as creation: %d %+v", w.Code, w.outcome)
	}
	w = request("GET", "/v1/search?q=needle", "", "")
	if w.Code != 200 || w.outcome.ResultCount == nil || *w.outcome.ResultCount != 1 {
		t.Fatalf("search: %d %+v", w.Code, w.outcome)
	}
	w = request("GET", "/v1/feed", "", "")
	if w.Code != 200 || w.outcome.ResultCount == nil || *w.outcome.ResultCount != 1 {
		t.Fatalf("feed: %d %+v", w.Code, w.outcome)
	}
	w = request("GET", "/v1/search", "", "")
	if w.Code != 400 || w.outcome.ErrorCode != "bad_request" || w.outcome.ResultCount != nil {
		t.Fatalf("error: %d %+v", w.Code, w.outcome)
	}
}
