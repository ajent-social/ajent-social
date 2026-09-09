package web

import (
	"ajent/internal/auth"
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestPublicSessionStatus(t *testing.T) {
	f := newHumanFixture(t)
	check := func(t *testing.T, human, agent bool) {
		t.Helper()
		w := f.request("GET", "/session-status", nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("session state must not be cached")
		}
		var state map[string]bool
		if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		_, hasHuman := state["human"]
		_, hasAgent := state["agent"]
		if len(state) != 2 || !hasHuman || !hasAgent || state["human"] != human || state["agent"] != agent {
			t.Fatalf("unexpected state: %v", state)
		}
	}
	t.Run("anonymous", func(t *testing.T) { check(t, false, false) })
	f.register()
	t.Run("human account", func(t *testing.T) { check(t, true, false) })
	if _, err := f.s.API.DB.Pool.Exec(context.Background(), `UPDATE human_sessions SET expires_at=now()-interval '1 minute' WHERE digest=$1`, auth.Digest(f.cookies["ajent_human"].Value)); err != nil {
		t.Fatal(err)
	}
	t.Run("expired human session", func(t *testing.T) { check(t, false, false) })
	f.cookies["ajent_human"] = &http.Cookie{Name: "ajent_human", Value: "invalid"}
	f.cookies["ajent_session"] = &http.Cookie{Name: "ajent_session", Value: "invalid"}
	t.Run("invalid sessions", func(t *testing.T) { check(t, false, false) })
}
