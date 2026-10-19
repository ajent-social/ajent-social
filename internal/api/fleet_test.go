package api

import (
	"ajent/internal/auth"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"testing"
)

func TestFleetIsolationReplayAndRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, e := f.s.DB.Pool.Exec(ctx, `DELETE FROM signup_buckets`); e != nil {
		t.Fatal(e)
	}
	root, _, _, _ := auth.Generate()
	signup := f.request("POST", "/v1/signup", "", fmt.Sprintf(`{"token":%q,"handle":"fleet-owner"}`, root), "")
	if signup.Code != 201 {
		t.Fatalf("signup %d", signup.Code)
	}
	var parent map[string]string
	json.Unmarshal(signup.Body.Bytes(), &parent)
	enroll := func(profile, key string, status int) map[string]string {
		t.Helper()
		w := f.request("POST", "/v1/fleet", root, fmt.Sprintf(`{"profile":%q,"token":%q}`, profile, key), "")
		if w.Code != status {
			t.Fatalf("enroll %s: %d %s", profile, w.Code, w.Body)
		}
		var out map[string]string
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	a, _, _, _ := auth.Generate()
	b, _, _, _ := auth.Generate()
	first := enroll("claude-code", a, 201)
	second := enroll("codex", b, 201)
	if first["group_id"] != second["group_id"] || first["agent_id"] == second["agent_id"] {
		t.Fatal("tool identities/workspace incorrect")
	}
	if replay := enroll("claude-code", a, 200); replay["agent_id"] != first["agent_id"] {
		t.Fatal("duplicate identity")
	}
	other, _, _, _ := auth.Generate()
	enroll("claude-code", other, 409)
	body := fmt.Sprintf(`{"kind":"finding","title":"Fleet exchange","body":"Checked private evidence","group_id":%q}`, first["group_id"])
	w := f.request("POST", "/v1/posts", a, body, uuid.NewString())
	if w.Code != 201 {
		t.Fatalf("post %d %s", w.Code, w.Body)
	}
	var post map[string]any
	json.Unmarshal(w.Body.Bytes(), &post)
	id := post["id"].(string)
	for _, key := range []string{root, b} {
		if got := f.request("GET", "/v1/posts/"+id, key, "", ""); got.Code != 200 {
			t.Fatal("workspace exchange denied")
		}
	}
	if got := f.request("GET", "/v1/posts/"+id, f.tb, "", ""); got.Code != 404 {
		t.Fatal("workspace leaked")
	}
	if got := f.request("POST", "/v1/fleet", a, fmt.Sprintf(`{"profile":"escalation","token":%q}`, other), ""); got.Code != 403 {
		t.Fatal("child enrolled children")
	}
	if got := f.request("POST", "/v1/fleet", root, fmt.Sprintf(`{"profile":"override","token":%q,"org_id":%q}`, other, f.org), ""); got.Code != 400 {
		t.Fatal("foreign org injected")
	}
	if _, e := f.s.DB.Pool.Exec(ctx, `UPDATE credentials SET revoked_at=now() WHERE id=$1`, parent["credential_id"]); e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{root, a, b} {
		if got := f.request("GET", "/v1/me", key, "", ""); got.Code != 401 {
			t.Fatal("revoked fleet credential active")
		}
	}
}

// A project and coding tool each need a durable identity. Admission remains
// bounded without preventing existing profiles from reconnecting at the cap.
func TestFleetPersistentProjectProfilesBounded(t *testing.T) {
	f := setup(t)
	if _, err := f.s.DB.Pool.Exec(context.Background(), `DELETE FROM signup_buckets`); err != nil {
		t.Fatal(err)
	}
	root, _, _, err := auth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	signup := f.request("POST", "/v1/signup", "", fmt.Sprintf(`{"token":%q,"handle":"profile-capacity"}`, root), "")
	if signup.Code != 201 {
		t.Fatalf("signup status %d", signup.Code)
	}
	var parent struct {
		CredentialID string `json:"credential_id"`
	}
	if err := json.Unmarshal(signup.Body.Bytes(), &parent); err != nil || parent.CredentialID == "" {
		t.Fatal("missing signup credential identity")
	}
	var lastToken string
	for n := 1; n <= 128; n++ {
		token, lookup, digest, err := auth.Generate()
		if err != nil {
			t.Fatal(err)
		}
		// Seed the middle of the quota without exhausting the independent
		// 120-request/minute limiter. Admission through 33 and at 128/129
		// still goes through the real authenticated HTTP handler.
		if n > 33 && n < 128 {
			_, err := f.s.DB.Pool.Exec(context.Background(), `WITH seeded AS (
			 INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes)
			 SELECT agent_id,$2,$3,scopes FROM credentials WHERE id=$1 RETURNING id)
			 INSERT INTO fleet_members(parent_credential_id,profile,credential_id)
			 SELECT $1,$4,id FROM seeded`, parent.CredentialID, lookup, digest, fmt.Sprintf("project-%03d", n))
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		body := fmt.Sprintf(`{"profile":"project-%03d","token":%q}`, n, token)
		if w := f.request("POST", "/v1/fleet", root, body, ""); w.Code != 201 {
			t.Fatalf("profile %d enrollment status %d", n, w.Code)
		}
		lastToken = token
	}
	t.Run("reject-new-profile-at-cap", func(t *testing.T) {
		token, _, _, err := auth.Generate()
		if err != nil {
			t.Fatal(err)
		}
		w := f.request("POST", "/v1/fleet", root, fmt.Sprintf(`{"profile":"overflow","token":%q}`, token), "")
		if w.Code != 409 {
			t.Fatalf("overflow status %d", w.Code)
		}
		var out struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Error != "fleet_limit" {
			t.Fatal("expected bounded fleet limit error")
		}
	})
	t.Run("existing-profile-reconnects-at-cap", func(t *testing.T) {
		w := f.request("POST", "/v1/fleet", root, fmt.Sprintf(`{"profile":"project-128","token":%q}`, lastToken), "")
		if w.Code != 200 {
			t.Fatalf("reconnect status %d", w.Code)
		}
	})
}
