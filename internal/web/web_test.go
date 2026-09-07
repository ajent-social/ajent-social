package web

import (
	"ajent/internal/api"
	"ajent/internal/bootstrap"
	"ajent/migrations"
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestOwnerWebFlowAndCSRF(t *testing.T) {
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if e = migrations.Apply(ctx, pool); e != nil {
		t.Fatal(e)
	}
	key := "test-owner-key-that-is-only-used-in-tests"
	api.SetSetupToken(key)
	owner, e := bootstrap.Ensure(ctx, pool, "Web test")
	if e != nil {
		t.Fatal(e)
	}
	if e = bootstrap.Demo(ctx, pool, owner); e != nil {
		t.Fatal(e)
	}
	a := api.New(pool)
	s, e := New(a, false, "http://example.com", key, true)
	if e != nil {
		t.Fatal(e)
	}
	m := http.NewServeMux()
	s.Routes(m)
	req := func(method, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		m.ServeHTTP(w, r)
		return w
	}
	login := req("GET", "/login", nil)
	if login.Code != 200 {
		t.Fatal(login.Code)
	}
	lc := login.Result().Cookies()[0]
	bad := req("POST", "/login", url.Values{"token": {key}, "csrf": {"bad"}}, lc)
	if bad.Code != 403 {
		t.Fatal("login CSRF accepted")
	}
	good := req("POST", "/login", url.Values{"token": {key}, "csrf": {lc.Value}}, lc)
	if good.Code != 303 {
		t.Fatalf("login %d %s", good.Code, good.Body)
	}
	session := good.Result().Cookies()[0]
	r := httptest.NewRequest("GET", "/app", nil)
	r.AddCookie(session)
	sess := s.session(r)
	if sess == nil || !sess.Owner {
		t.Fatal("session not created")
	}
	for _, path := range []string{"/", "/docs", "/app", "/app/groups", "/app/agents", "/app/notifications", "/app/settings", "/app/agents/" + owner, "/app?q=retries"} {
		w := req("GET", path, nil, session)
		if w.Code != 200 || strings.Contains(w.Body.String(), "Page could not be rendered") {
			t.Fatalf("route %s %d %s", path, w.Code, w.Body)
		}
	}
	bad = req("POST", "/actions/post", url.Values{"kind": {"status"}, "body": {"bad"}}, session)
	if bad.Code != 403 {
		t.Fatal("mutation without CSRF")
	}
	form := url.Values{"csrf": {sess.CSRF}, "kind": {"finding"}, "title": {"Useful <script>alert(1)</script>"}, "body": {"Escaped evidence"}, "audience": {"public"}, "request_key": {"web-post-test"}}
	good = req("POST", "/actions/post", form, session)
	if good.Code != 303 {
		t.Fatalf("web post %d %s", good.Code, good.Body)
	}
	feed := req("GET", "/app", nil, session)
	if strings.Contains(feed.Body.String(), "Useful <script>") || !strings.Contains(feed.Body.String(), "Useful &lt;script&gt;") {
		t.Fatal("HTML escaping failed")
	}
	r = httptest.NewRequest("GET", "/app?q=retries", nil)
	r.AddCookie(session)
	r.Header.Set("HX-Target", "feed-results")
	w := httptest.NewRecorder()
	m.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `id="feed-results"`) || strings.Contains(w.Body.String(), "<!doctype") {
		t.Fatal("HTMX fragment contract")
	}
	good = req("POST", "/actions/group", url.Values{"csrf": {sess.CSRF}, "name": {"web-" + uuid.NewString()[:8]}, "visibility": {"private"}, "description": {"A private test group"}}, session)
	if good.Code != 303 {
		t.Fatalf("group form failed %s", good.Body)
	}
	good = req("POST", "/actions/agent", url.Values{"csrf": {sess.CSRF}, "handle": {"web-" + uuid.NewString()[:8]}, "name": {"Web agent"}}, session)
	if good.Code != 200 || !strings.Contains(good.Body.String(), "Save this access key.") {
		t.Fatalf("credential form failed %s", good.Body)
	}
	s.Setup = "rotated-owner-key-that-is-only-used-in-tests"
	if s.session(r) != nil {
		t.Error("owner session survives owner key rotation")
	}
	s.Setup = key
	logout := req("POST", "/logout", url.Values{"csrf": {sess.CSRF}}, session)
	if logout.Code != 303 {
		t.Fatal("logout failed")
	}
	if req("GET", "/app", nil, session).Code != 303 {
		t.Fatal("logged-out session active")
	}
}

func TestHTMXRedirectDoesNotFollowHTTPRedirect(t *testing.T) {
	r := httptest.NewRequest("POST", "/actions/post", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	redirect(w, r, "/app")
	if w.Code != 204 || w.Header().Get("HX-Redirect") != "/app" {
		t.Fatalf("HTMX redirect: status %d headers %v", w.Code, w.Header())
	}
}
