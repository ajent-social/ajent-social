package web

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/internal/bootstrap"
	"ajent/migrations"
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFormValidationRecovery(t *testing.T) {
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	owner, err := bootstrap.Ensure(ctx, pool, "Synthetic form recovery owner")
	if err != nil {
		t.Fatal(err)
	}
	must := func(q string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	org, agent, credential := uuid.NewString(), uuid.NewString(), uuid.NewString()
	_, lookup, digest, err := auth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	must("INSERT INTO organizations(id,name,namespace) VALUES($1::uuid,'Synthetic form recovery',$1::text)", org)
	must("INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'synthetic-recovery-agent')", agent, org)
	must("INSERT INTO credentials(id,agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,$4,ARRAY['network:read','posts:write','groups:manage'])", credential, agent, lookup, digest)
	const setup = "synthetic-form-recovery-owner-key"
	const csrf = "synthetic-valid-form-recovery-csrf"
	validID := uuid.NewString()
	for _, role := range []struct {
		name, actor string
		owner       bool
	}{{"owner", owner, true}, {"ordinary", agent, false}} {
		raw := uuid.NewString()
		var cred string
		if err = pool.QueryRow(ctx, "SELECT id::text FROM credentials WHERE agent_id=$1 AND revoked_at IS NULL LIMIT 1", role.actor).Scan(&cred); err != nil {
			t.Fatal(err)
		}
		must("INSERT INTO web_sessions(digest,credential_id,agent_id,org_id,is_owner,csrf,owner_key_digest) SELECT $1,$2,id,org_id,$3,$4,$5 FROM agents WHERE id=$6", auth.Digest(raw), cred, role.owner, csrf, auth.Digest(setup), role.actor)
		cases := []struct {
			name, action, id, member, detail string
			status                           int
		}{
			{"malformed_id", "save", "malformed", validID, "Invalid identifier.", 400},
			{"malformed_member", "member", validID, "malformed", "Invalid agent identifier.", 400},
			{"unknown_action", "not-an-action", validID, validID, "", 404},
		}
		if role.owner {
			cases = append(cases, struct {
				name, action, id, member, detail string
				status                           int
			}{"revoke_self", "revoke", cred, validID, "Cannot revoke this owner session credential.", 403}, struct {
				name, action, id, member, detail string
				status                           int
			}{"revoke_foreign", "revoke", credential, validID, "", 404})
		} else {
			for _, action := range []string{"agent", "moderate", "revoke"} {
				detail := "Owner access required."
				if action == "revoke" {
					detail = "Cannot revoke this owner session credential."
				}
				cases = append(cases, struct {
					name, action, id, member, detail string
					status                           int
				}{"forbidden_" + action, action, validID, validID, detail, 403})
			}
		}
		for _, tc := range cases {
			for _, enhanced := range []bool{false, true} {
				mode := "html"
				if enhanced {
					mode = "enhanced"
				}
				t.Run(role.name+"/"+tc.name+"/"+mode, func(t *testing.T) {
					s, e := New(api.New(pool), false, "http://example.com", setup, false)
					if e != nil {
						t.Fatal(e)
					}
					mutations := 0
					s.APIHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != "GET" {
							mutations++
							t.Errorf("early denial reached downstream mutation: %s %s", r.Method, r.URL.Path)
							http.Error(w, "unexpected mutation", 500)
							return
						}
						switch r.URL.Path {
						case "/v1/me":
							browserJSON(w, 200, map[string]string{"id": role.actor, "handle": "synthetic-recovery-agent"})
						case "/v1/groups":
							browserJSON(w, 200, map[string]any{"groups": []any{}})
						default:
							t.Errorf("unexpected downstream read %s", r.URL.Path)
							http.Error(w, "unexpected read", 500)
						}
					})
					back := "/app/settings?tab=access&notice=retry"
					form := url.Values{"csrf": {csrf}, "id": {tc.id}, "agent_id": {tc.member}, "back": {back}, "decision": {"hide"}}
					r := httptest.NewRequest("POST", "/actions/"+tc.action, strings.NewReader(form.Encode()))
					r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					r.Header.Set("Origin", "http://example.com")
					r.AddCookie(&http.Cookie{Name: "ajent_session", Value: raw})
					if enhanced {
						r.Header.Set("HX-Request", "true")
					}
					// Independently establish that this is the intended valid role/session,
					// so a CSRF/authentication rejection cannot satisfy these assertions.
					session := s.session(r)
					if session == nil || session.Owner != role.owner || session.CSRF != csrf || session.Credential.AgentID != role.actor {
						t.Fatal("fixture did not establish intended authenticated role and CSRF")
					}
					mux := http.NewServeMux()
					s.Routes(mux)
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, r)
					if w.Code != tc.status {
						t.Errorf("status=%d want=%d", w.Code, tc.status)
					}
					if w.Header().Get("Location") != "" || w.Header().Get("HX-Redirect") != "" {
						t.Error("denial must not redirect as success")
					}
					body := w.Body.String()
					if tc.detail != "" && !strings.Contains(body, tc.detail) {
						t.Errorf("missing specific validation/permission detail: %q", body)
					}
					if enhanced {
						if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") || strings.TrimSpace(body) == "" || strings.Contains(body, "<html") {
							t.Error("enhanced denial needs plain nonempty detail")
						}
					} else {
						if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(body, "<title>") || !strings.Contains(body, "<main") {
							t.Error("plain form denial needs titled HTML main")
						}
						if !strings.Contains(body, `href="`+html.EscapeString(back)+`"`) {
							t.Error("plain form denial needs usable contextual recovery link")
						}
					}
					if mutations != 0 {
						t.Errorf("downstream mutation count=%d", mutations)
					}
					var intact bool
					if e = pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM credentials WHERE id=$1 AND revoked_at IS NULL) AND EXISTS(SELECT 1 FROM credentials WHERE id=$2 AND revoked_at IS NULL) AND NOT EXISTS(SELECT 1 FROM audit_log WHERE action='report.reviewed' AND object_id=$3)", cred, credential, validID).Scan(&intact); e != nil || !intact {
						t.Errorf("denied action changed persisted credentials/review: intact=%v error=%v", intact, e)
					}
				})
			}
		}
	}
}
