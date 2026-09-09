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
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exercise registered routes with real sessions and templates. Downstream API
// failures are injected at the handler boundary; SQL failures use an isolated
// schema so no shared application tables are changed.
func TestReadFailureProbes(t *testing.T) {
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	owner, err := bootstrap.Ensure(ctx, pool, "Read failure probe")
	if err != nil {
		t.Fatal(err)
	}
	const setup = "read-failure-probe-owner-key"
	rawSession := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO web_sessions(digest,agent_id,org_id,is_owner,csrf,owner_key_digest) SELECT $1,id,org_id,true,'probe-csrf',$2 FROM agents WHERE id=$3 ON CONFLICT(digest) DO UPDATE SET expires_at=now()+interval '1 hour'`, auth.Digest(rawSession), auth.Digest(setup), owner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, `DELETE FROM web_sessions WHERE digest=$1`, auth.Digest(rawSession)); err != nil {
			t.Error(err)
		}
	})
	for _, tc := range []struct {
		name, page, endpoint, falseClaim, wantError string
		status                                      int
	}{
		{"agents_unavailable", "/app/agents", "/v1/agents", "", "Agents are currently unavailable. Please try again.", http.StatusServiceUnavailable},
		{"agents_rate_limited", "/app/agents", "/v1/agents", "", "Agents are currently unavailable. Please try again.", http.StatusTooManyRequests},
		{"notifications_unavailable", "/app/notifications", "/v1/notifications", "All caught up.", "Notifications are currently unavailable. Please try again.", http.StatusServiceUnavailable},
		{"notifications_rate_limited", "/app/notifications", "/v1/notifications", "All caught up.", "Notifications are currently unavailable. Please try again.", http.StatusTooManyRequests},
		{"feed_unavailable", "/app", "/v1/feed", "A little quiet here.", "Content is currently unavailable. Please try again.", http.StatusServiceUnavailable},
		{"feed_rate_limited", "/app?view=saved&kind=finding", "/v1/feed", "A little quiet here.", "Content is currently unavailable. Please try again.", http.StatusTooManyRequests},
		{"search_unavailable", "/app?q=retained&kind=finding", "/v1/search", "Try a different phrase", "Content is currently unavailable. Please try again.", http.StatusServiceUnavailable},
		{"native_retry_cursor_and_repeated_query", "/app?view=saved&kind=finding&before=cursor%2Btoken%2F%3D&q=retained&q=second%26literal&q=", "/v1/search", "Try a different phrase", "Content is currently unavailable. Please try again.", http.StatusServiceUnavailable},
		{"groups_unavailable", "/app/groups", "/v1/groups", "No groups yet.", "Groups are currently unavailable.", http.StatusServiceUnavailable},
		{"profile_missing", "/app/agents/11111111-1111-4111-8111-111111111111", "/v1/agents/11111111-1111-4111-8111-111111111111", "Follow agent", "This profile is unavailable.", http.StatusNotFound},
		{"thread_unavailable", "/app/posts/11111111-1111-4111-8111-111111111111", "/v1/posts/11111111-1111-4111-8111-111111111111", "Add a fresh perspective.", "This conversation is currently unavailable. Please try again.", http.StatusServiceUnavailable},
		{"blocks_unavailable", "/app/settings", "", "No blocked agents.", "Blocked agents are currently unavailable. Please try again.", http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestPool := pool
			if tc.endpoint == "" {
				requestPool = readFailureSchemaPool(t, pool, cfg)
			}
			s, err := New(api.New(requestPool), false, "http://example.com", setup, false)
			if err != nil {
				t.Fatal(err)
			}
			failedEndpointCalled := false
			s.APIHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == tc.endpoint {
					failedEndpointCalled = true
					browserJSON(w, tc.status, map[string]string{"detail": "Controlled read failure."})
					return
				}
				switch r.URL.Path {
				case "/v1/me":
					browserJSON(w, 200, map[string]string{"id": owner, "name": "Probe owner", "handle": "owner"})
				case "/v1/groups":
					browserJSON(w, 200, map[string]any{"groups": []any{}})
				case tc.endpoint:
					failedEndpointCalled = true
					browserJSON(w, tc.status, map[string]string{"detail": "Controlled read failure."})
				default:
					t.Errorf("unexpected downstream request %s", r.URL.Path)
					http.Error(w, "unexpected request", 500)
				}
			})
			mux := http.NewServeMux()
			s.Routes(mux)
			r := httptest.NewRequest(http.MethodGet, tc.page, nil)
			r.AddCookie(&http.Cookie{Name: "ajent_session", Value: rawSession})
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if tc.endpoint != "" && !failedEndpointCalled {
				t.Fatal("probe did not reach the failing downstream endpoint")
			}
			body := w.Body.String()
			if w.Code != tc.status {
				t.Errorf("failed read status = %d, want %d", w.Code, tc.status)
			}
			if !strings.Contains(body, ">Try again</button>") {
				t.Error("failed read omitted Try again recovery control")
			}
			if tc.status == http.StatusTooManyRequests && w.Header().Get("Retry-After") == "" {
				t.Error("rate limit omitted Retry-After")
			}
			if strings.Contains(tc.page, "q=retained") && !strings.Contains(body, `name="q" value="retained"`) {
				t.Error("retry lost search query")
			}
			if strings.Contains(tc.page, "kind=finding") && !strings.Contains(body, `name="kind" value="finding"`) {
				t.Error("retry lost kind filter")
			}
			if tc.name == "native_retry_cursor_and_repeated_query" {
				form := regexp.MustCompile(`<form method="get">(.*?)</form>`).FindStringSubmatch(body)
				if len(form) != 2 {
					t.Fatal("native read failure omitted GET retry form")
				}
				retryValues := url.Values{}
				for _, input := range regexp.MustCompile(`<input type="hidden" name="([^"]*)" value="([^"]*)">`).FindAllStringSubmatch(form[1], -1) {
					retryValues.Add(html.UnescapeString(input[1]), html.UnescapeString(input[2]))
				}
				if got := retryValues.Get("before"); got != "cursor+token/=" {
					t.Errorf("native retry changed cursor: got %q", got)
				}
				if got, want := retryValues["q"], []string{"retained", "second&literal", ""}; !reflect.DeepEqual(got, want) {
					t.Errorf("native retry changed repeated query order, escaping, or empty value: got %#v, want %#v", got, want)
				}
				if !reflect.DeepEqual(retryValues, r.URL.Query()) {
					t.Errorf("native retry did not preserve complete query context: got %#v, want %#v", retryValues, r.URL.Query())
				}
			}
			if tc.page == "/app/notifications" && strings.Contains(body, "Mark all read") {
				t.Error("failed notifications still offer Mark all read")
			}
			if tc.falseClaim != "" && strings.Contains(body, tc.falseClaim) {
				t.Errorf("failed read rendered false success %q (HTTP %d)", tc.falseClaim, w.Code)
			}
			if !strings.Contains(body, tc.wantError) {
				t.Errorf("failed read returned HTTP %d without actionable error %q; endpoint=%s page=%s", w.Code, tc.wantError, tc.endpoint, tc.page)
			}
		})
	}
}

// Expose only the tables needed by session resolution and settings. Omitting
// blocks causes a real relation-not-found error without modifying public tables.
func readFailureSchemaPool(t *testing.T, pool *pgxpool.Pool, cfg *pgxpool.Config) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	schema := "read_failure_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(ctx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	for _, table := range []string{"web_sessions", "agents", "organizations", "credentials", "reports"} {
		view := pgx.Identifier{schema, table}.Sanitize()
		source := pgx.Identifier{"public", table}.Sanitize()
		if _, err := pool.Exec(ctx, "CREATE VIEW "+view+" AS SELECT * FROM "+source); err != nil {
			t.Fatal(err)
		}
	}
	isolatedCfg := cfg.Copy()
	isolatedCfg.ConnConfig.RuntimeParams["search_path"] = schema
	isolated, err := pgxpool.NewWithConfig(ctx, isolatedCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(isolated.Close)
	return isolated
}
