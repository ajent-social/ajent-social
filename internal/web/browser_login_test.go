package web

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/migrations"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestBrowserLoginHandoff(t *testing.T) {
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
	pool.Exec(ctx, `DELETE FROM signup_buckets`)
	a := api.New(pool)
	s, e := New(a, true, "https://ajent.example", "test-owner-key", false)
	if e != nil {
		t.Fatal(e)
	}
	m := http.NewServeMux()
	a.Routes(m)
	s.Routes(m)
	key, _, _, _ := auth.Generate()
	call := func(path, body, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("Authorization", "Bearer "+key)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		m.ServeHTTP(w, r)
		return w
	}
	if w := call("/v1/signup", fmt.Sprintf(`{"token":%q,"handle":"browser-user"}`, key), "", nil); w.Code != 201 {
		t.Fatalf("signup %d", w.Code)
	}
	page := httptest.NewRecorder()
	m.ServeHTTP(page, httptest.NewRequest("GET", "/connect-login", nil))
	cookie := page.Result().Cookies()[0]
	issue := func() string {
		t.Helper()
		w := call("/v1/browser-login", "{}", "", nil)
		if w.Code != 200 {
			t.Fatalf("link %d", w.Code)
		}
		var v map[string]any
		json.Unmarshal(w.Body.Bytes(), &v)
		if strings.Contains(w.Body.String(), key) {
			t.Fatal("long-lived key leaked")
		}
		return strings.Split(v["url"].(string), "#")[1]
	}
	token := issue()
	body := fmt.Sprintf(`{"token":%q,"csrf":%q}`, token, cookie.Value)
	if w := call("/browser-login/redeem", body, "https://evil.example", cookie); w.Code != 403 {
		t.Fatal("cross-origin redemption")
	}
	if w := call("/browser-login/preview", body, s.Origin, cookie); w.Code != 200 || len(w.Result().Cookies()) != 0 || !strings.Contains(w.Body.String(), "browser-user") {
		t.Fatal("preview must not sign in")
	}
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	sessions := make(chan *http.Cookie, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := call("/browser-login/redeem", body, s.Origin, cookie)
			codes <- w.Code
			if w.Code == 200 {
				sessions <- w.Result().Cookies()[0]
			}
		}()
	}
	wg.Wait()
	close(codes)
	success, gone := 0, 0
	for code := range codes {
		if code == 200 {
			success++
		}
		if code == 410 {
			gone++
		}
	}
	if success != 1 || gone != 1 {
		t.Fatalf("single use: success=%d gone=%d", success, gone)
	}
	sessionCookie := <-sessions
	if !sessionCookie.Secure || !sessionCookie.HttpOnly {
		t.Fatal("insecure session cookie")
	}
	req := httptest.NewRequest("GET", "/app", nil)
	req.AddCookie(sessionCookie)
	session := s.session(req)
	if session == nil || session.Owner {
		t.Fatal("wrong browser authority")
	}
	token = issue()
	pool.Exec(ctx, `UPDATE browser_login_tokens SET expires_at=now()-interval '1 second' WHERE digest=$1`, auth.Digest(token))
	body = fmt.Sprintf(`{"token":%q,"csrf":%q}`, token, cookie.Value)
	if w := call("/browser-login/redeem", body, s.Origin, cookie); w.Code != 410 {
		t.Fatal("expired link accepted")
	}
	token = issue()
	pool.Exec(ctx, `UPDATE credentials SET revoked_at=now() WHERE id=$1`, session.Credential.CredID)
	body = fmt.Sprintf(`{"token":%q,"csrf":%q}`, token, cookie.Value)
	if w := call("/browser-login/redeem", body, s.Origin, cookie); w.Code != 410 {
		t.Fatal("revoked link accepted")
	}
	if s.session(req) != nil {
		t.Fatal("revoked browser session accepted")
	}
}
