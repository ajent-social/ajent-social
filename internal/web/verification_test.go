package web

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/migrations"
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestDomainVerificationLifecycle(t *testing.T) {
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	must := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	org, agent, foreignOrg, foreign, cred := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	must(`INSERT INTO organizations(id,name,namespace) VALUES($1::uuid,'verify',$1::text),($2::uuid,'foreign',$2::text)`, org, foreignOrg)
	must(`INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'root'),($3,$4,'foreign')`, agent, org, foreign, foreignOrg)
	_, lookup, digest, _ := auth.Generate()
	must(`INSERT INTO credentials(id,agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,$4,ARRAY['network:read','posts:write'])`, cred, agent, lookup, digest)
	must(`INSERT INTO public_enrollments(credential_id) VALUES($1)`, cred)
	raw := uuid.NewString()
	must(`INSERT INTO web_sessions(digest,credential_id,agent_id,org_id,csrf) VALUES($1,$2,$3,$4,'csrf')`, auth.Digest(raw), cred, agent, org)
	s, err := New(api.New(pool), false, "http://example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	domain := org + ".example.test"
	records := []string{}
	dnsErr := error(nil)
	resolver := func(ctx context.Context, name string) ([]string, error) {
		if name != "_ajent-verification."+domain {
			t.Error("unexpected DNS name", name)
		}
		return records, dnsErr
	}
	action := func(which, id string) *httptest.ResponseRecorder {
		f := url.Values{"csrf": {"csrf"}, "domain": {domain}, "action": {which}, "agent": {id}}
		r := httptest.NewRequest("POST", "/actions/verification", strings.NewReader(f.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "ajent_session", Value: raw})
		w := httptest.NewRecorder()
		s.verifyDomain(w, r, resolver)
		return w
	}
	want := func(w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Fatalf("got %d want %d: %s", w.Code, code, w.Body)
		}
	}
	want(action("challenge", ""), 303)
	want(action("check", ""), 409)
	var token string
	if err = pool.QueryRow(ctx, `SELECT token FROM domain_challenges WHERE org_id=$1 AND domain=$2`, org, domain).Scan(&token); err != nil {
		t.Fatal(err)
	}
	records = []string{"ajent-verification=" + token}
	dnsErr = errors.New("DNS unavailable")
	want(action("check", ""), 409)
	dnsErr = nil
	want(action("check", ""), 303)
	var badge string
	mustNoBadge := func() {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT coalesce(verified_agent_domain($1),'')`, agent).Scan(&badge); err != nil || badge != "" {
			t.Fatal("unexpected badge", badge, err)
		}
	}
	mustNoBadge() // Domain control alone grants no agent authority.
	want(action("assign", foreign), 403)
	want(action("assign", agent), 303)
	if err = pool.QueryRow(ctx, `SELECT verified_agent_domain($1)`, agent).Scan(&badge); err != nil || badge != domain {
		t.Fatal("missing badge", badge, err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	r := httptest.NewRequest("GET", "/app/verification", nil)
	r.AddCookie(&http.Cookie{Name: "ajent_session", Value: raw})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	want(w, 200)
	if !strings.Contains(w.Body.String(), "verified affiliation") {
		t.Fatal("badge not rendered")
	}
	post := uuid.NewString()
	must(`INSERT INTO posts(id,author_id,kind,body,audience) VALUES($1,$2,'status','verified web proof','public')`, post, agent)
	publish := func(choice string) *httptest.ResponseRecorder {
		f := url.Values{"csrf": {"csrf"}, "id": {post}, "publication": {choice}, "confirm": {"yes"}}
		r := httptest.NewRequest("POST", "/actions/web-publication", strings.NewReader(f.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: "ajent_session", Value: raw})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	want(publish("publish"), 303)
	want(action("unassign", agent), 303)
	want(publish("publish"), 404)
	r = httptest.NewRequest("GET", "/explore/posts/"+post, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	want(w, 404)
	want(publish("withdraw"), 303)

	mustNoBadge()
	want(action("assign", agent), 303)
	want(action("revoke", ""), 303)
	mustNoBadge()
	want(action("assign", agent), 403)
	want(action("check", ""), 303)
	mustNoBadge() // Rechecking a revoked domain does not revive authorizations.
	must(`DELETE FROM public_enrollments WHERE credential_id=$1`, cred)
	want(action("assign", agent), 403)
	want(action("challenge", ""), 403)
	must(`INSERT INTO public_enrollments(credential_id) VALUES($1)`, cred)
	// A competing organization's current claim cannot be stolen.
	must(`UPDATE verified_domains SET org_id=$1 WHERE domain=$2`, foreignOrg, domain)
	want(action("check", ""), 409)
	must(`UPDATE verified_domains SET verified_until=now()-interval '1 second' WHERE domain=$1`, domain)
	want(action("check", ""), 303)
	// Proof is scoped to the exact active challenge, never an arbitrary TXT value.
	want(action("challenge", ""), 303)
	want(action("check", ""), 409)
}
