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

func TestActionErrorResponses(t *testing.T) {
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
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	owner, err := bootstrap.Ensure(ctx, pool, "Action error regression")
	if err != nil {
		t.Fatal(err)
	}
	const setup = "synthetic-action-error-owner-key"
	const csrf = "synthetic-action-error-csrf"
	sessions := map[string]string{}
	for _, role := range []string{"owner", "agent"} {
		raw := uuid.NewString()
		sessions[role] = raw
		if _, err := pool.Exec(ctx, `INSERT INTO web_sessions(digest,credential_id,agent_id,org_id,is_owner,csrf,owner_key_digest) SELECT $1,c.id,a.id,a.org_id,$2,$3,$4 FROM agents a JOIN credentials c ON c.agent_id=a.id WHERE a.id=$5 AND c.revoked_at IS NULL LIMIT 1`, auth.Digest(raw), role == "owner", csrf, auth.Digest(setup), owner); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := pool.Exec(ctx, `DELETE FROM web_sessions WHERE digest=$1`, auth.Digest(raw)); err != nil {
				t.Error(err)
			}
		})
	}
	id, member := uuid.NewString(), uuid.NewString()
	for _, role := range []string{"owner", "agent"} {
		for _, tc := range []struct {
			name, action, method, endpoint, back, wantBack string
			status                                         int
		}{
			{"delete_forbidden", "delete", "DELETE", "/v1/posts/" + id, "/app/posts/" + id, "/app/posts/" + id, 403},
			{"delete_conflict", "delete", "DELETE", "/v1/posts/" + id, "/app/posts/" + id, "/app/posts/" + id, 409},
			{"member_forbidden", "member", "POST", "/v1/groups/" + id + "/members/" + member, "/app/groups/" + id, "/app/groups/" + id, 403},
			{"member_conflict", "member", "POST", "/v1/groups/" + id + "/members/" + member, "/app/groups/" + id, "/app/groups/" + id, 409},
			{"member_invalid_action", "member", "POST", "/v1/groups/" + id + "/members/" + member, "/app/groups/" + id, "/app/groups/" + id, 400},
			{"external_back_rejected", "delete", "DELETE", "/v1/posts/" + id, "//outside.example/steal", "/app", 403},
		} {
			t.Run(role+"/"+tc.name, func(t *testing.T) {
				s, err := New(api.New(pool), false, "http://example.com", setup, false)
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				const detail = "This change is unavailable. Return to the page and choose another action."
				s.APIHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case tc.endpoint:
						calls++
						if r.Method != tc.method {
							t.Errorf("downstream method = %s, want %s", r.Method, tc.method)
						}
						browserJSON(w, tc.status, map[string]string{"detail": detail})
					case "/v1/me":
						browserJSON(w, 200, map[string]string{"id": owner, "handle": "owner", "name": "Synthetic action actor"})
					case "/v1/groups":
						browserJSON(w, 200, map[string]any{"groups": []any{}})
					default:
						t.Errorf("unexpected downstream request %s %s", r.Method, r.URL.Path)
						http.Error(w, "unexpected request", 500)
					}
				})
				form := url.Values{"csrf": {csrf}, "id": {id}, "agent_id": {member}, "member_action": {"remove"}, "back": {tc.back}}
				if tc.status == 400 {
					form.Set("member_action", "invalid")
				}
				r := httptest.NewRequest("POST", "/actions/"+tc.action, strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				r.Header.Set("Origin", "http://example.com")
				r.AddCookie(&http.Cookie{Name: "ajent_session", Value: sessions[role]})
				mux := http.NewServeMux()
				s.Routes(mux)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				if calls != 1 {
					t.Fatalf("failed endpoint reached %d times, want exactly one", calls)
				}
				if w.Code != tc.status {
					t.Errorf("action error HTTP %d, want downstream %d", w.Code, tc.status)
				}
				if w.Header().Get("Location") != "" || w.Header().Get("HX-Redirect") != "" {
					t.Error("failed action returned a success redirect")
				}
				body := w.Body.String()
				if !strings.Contains(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(body, detail) {
					t.Error("failed action did not render useful HTML error detail")
				}
				if !strings.Contains(body, `href="`+html.EscapeString(tc.wantBack)+`"`) {
					t.Errorf("missing recovery link to %s", tc.wantBack)
				}
				if strings.Contains(body, `href="//outside.example`) {
					t.Error("recovery link accepted an external destination")
				}
				t.Run("enhanced", func(t *testing.T) {
					calls = 0
					r := httptest.NewRequest("POST", "/actions/"+tc.action, strings.NewReader(form.Encode()))
					r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
					r.Header.Set("Origin", "http://example.com")
					r.Header.Set("HX-Request", "true")
					r.AddCookie(&http.Cookie{Name: "ajent_session", Value: sessions[role]})
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, r)
					if calls != 1 {
						t.Fatalf("enhanced action reached failed endpoint %d times, want one", calls)
					}
					if w.Code != tc.status {
						t.Errorf("enhanced action HTTP %d, want downstream %d", w.Code, tc.status)
					}
					if w.Header().Get("Location") != "" || w.Header().Get("HX-Redirect") != "" {
						t.Error("enhanced failed action returned a success redirect")
					}
					if !strings.Contains(w.Header().Get("Content-Type"), "text/plain") || strings.TrimSpace(w.Body.String()) != detail {
						t.Errorf("enhanced failure must return plain detail, got %q", w.Body.String())
					}
				})
			})
		}
	}
}

func TestAppReturnPathBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"app_root", "/app", "/app"},
		{"thread_with_query", "/app/posts/example?kind=answer&view=saved", "/app/posts/example?kind=answer&view=saved"},
		{"encoded_query", "/app/groups/example?q=one%20two", "/app/groups/example?q=one%20two"},
		{"internal_normalization", "/app/groups/../settings", "/app/settings"},
		{"absolute_external", "https://outside.example/app", "/app"},
		{"protocol_relative", "//outside.example/app", "/app"},
		{"lookalike_prefix", "/application/settings", "/app"},
		{"traversal_outside_app", "/app/../human/workspaces", "/app"},
		{"encoded_traversal", "/app/%2e%2e/human/workspaces", "/app"},
		{"escaped_backslash", "/app/%5coutside.example", "/app"},
		{"literal_backslash", `/app/\outside.example`, "/app"},
		{"malformed_escape", "/app/%zz", "/app"},
		{"empty", "", "/app"},
		{"relative", "app/settings", "/app"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := appReturnPath(tc.input); got != tc.want {
				t.Errorf("appReturnPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
