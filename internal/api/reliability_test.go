package api

import (
	"ajent/internal/events"
	"ajent/internal/jobs"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

type inboxResponse struct {
	Items []struct {
		ID  string `json:"id"`
		Seq int64  `json:"seq"`
	} `json:"items"`
	Through      int64 `json:"acknowledge_through"`
	Acknowledged int64 `json:"acknowledged_through"`
	More         bool  `json:"has_more"`
	Gap          bool  `json:"history_gap"`
}

func TestInboxDeliveryAndAcknowledgment(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for i := 0; i < 23; i++ {
		f.post(f.ta, f.g, "")
	}
	if _, err := events.PublishOnce(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	get := func(token string) inboxResponse {
		t.Helper()
		w := f.request("GET", "/v1/inbox", token, "", "")
		if w.Code != 200 {
			t.Fatalf("inbox %d %s", w.Code, w.Body)
		}
		var x inboxResponse
		if err := json.Unmarshal(w.Body.Bytes(), &x); err != nil {
			t.Fatal(err)
		}
		return x
	}
	first := get(f.ta)
	again := get(f.ta)
	if len(first.Items) != 20 || !first.More || first.Acknowledged != 0 || again.Through != first.Through {
		t.Fatalf("fetch consumed or truncated: %+v %+v", first, again)
	}
	if w := f.request("POST", "/v1/inbox/ack", f.tb, fmt.Sprintf(`{"through":%d}`, first.Through), ""); w.Code != 409 {
		t.Fatalf("foreign delivery acknowledged: %d", w.Code)
	}
	if w := f.request("POST", "/v1/inbox/ack", f.ta, fmt.Sprintf(`{"through":%d}`, first.Through+100000), ""); w.Code != 409 {
		t.Fatalf("future cursor accepted: %d", w.Code)
	}
	for i := 0; i < 2; i++ {
		if w := f.request("POST", "/v1/inbox/ack", f.ta, fmt.Sprintf(`{"through":%d}`, first.Through), ""); w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	next := get(f.ta)
	if len(next.Items) != 3 || next.Acknowledged != first.Through {
		t.Fatalf("next batch %+v", next)
	}
	outsider := get(f.tb)
	if len(outsider.Items) != 0 {
		t.Fatal("private content leaked")
	}
	// Retraction is author-only and idempotent, and removes content from pending previews.
	id := next.Items[0].ID
	if w := f.request("DELETE", "/v1/posts/"+id, f.tb, "", ""); w.Code != 404 {
		t.Fatal("foreign retract accepted")
	}
	for i := 0; i < 2; i++ {
		if w := f.request("DELETE", "/v1/posts/"+id, f.ta, "", ""); w.Code != 200 {
			t.Fatal("retract retry", w.Code)
		}
	}
	if got := get(f.ta); len(got.Items) != 2 {
		t.Fatal("retracted preview retained")
	}
}

func TestFleetObservationIsolation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// A probe must not manufacture evidence of agent activity.
	r := httptest.NewRequest("GET", "/v1/reliability", nil)
	r.Header.Set("Authorization", "Bearer "+f.ta)
	r.Header.Set("X-Ajent-Client", "probe")
	w := httptest.NewRecorder()
	f.m.ServeHTTP(w, r)
	if w.Code != 200 || strings.Contains(w.Body.String(), f.b) {
		t.Fatal("fleet scope", w.Code, w.Body)
	}
	var count int
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM agent_activity WHERE agent_id=$1`, f.a).Scan(&count); err != nil || count != 0 {
		t.Fatal("probe manufactured presence", count, err)
	}
	r = httptest.NewRequest("GET", "/v1/feed", nil)
	r.Header.Set("Authorization", "Bearer "+f.ta)
	r.Header.Set("X-Ajent-Client-Version", ClientVersion)
	w = httptest.NewRecorder()
	f.m.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	health, err := f.s.FleetHealth(ctx, f.org)
	if err != nil || len(health) != 1 {
		t.Fatal(health, err)
	}
	if health[0].LastSeen == nil || health[0].LastFeedRead == nil || health[0].ClientVersion != ClientVersion || health[0].State != "recently_seen" {
		t.Fatalf("missing health %+v", health[0])
	}
	if _, err = f.s.DB.Pool.Exec(ctx, `UPDATE agent_activity SET last_seen=now()-interval '1 hour' WHERE agent_id=$1`, f.a); err != nil {
		t.Fatal(err)
	}
	health, err = f.s.FleetHealth(ctx, f.org)
	if err != nil || health[0].State != "stale" {
		t.Fatal(health, err)
	}
}

func TestReliabilityBacklogSignal(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	id := f.post(f.ta, f.g, "")
	if _, err := f.s.DB.Pool.Exec(ctx, `UPDATE outbox SET created_at=now()-interval '3 minutes' WHERE object_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(old)
	if err := jobs.Reliability(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(logs.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["publication_backlog"] != true || out["oldest_pending_seconds"].(float64) < 120 {
		t.Fatal(out)
	}
	if strings.Contains(logs.String(), f.a) || strings.Contains(logs.String(), id) {
		t.Fatal("identity leaked into aggregate metrics")
	}
}
