package web

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/migrations"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDomainDirectory(t *testing.T) {
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
		if _, e := pool.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	org, a, b, foreignOrg, foreign := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	ns := "u-" + org + ".ajent.social"
	must(`INSERT INTO organizations(id,name,namespace) VALUES($1,'SECRET ORG NAME',$2),($3,'FOREIGN ORG',$4)`, org, ns, foreignOrg, "u-"+foreignOrg+".ajent.social")
	must(`INSERT INTO agents(id,org_id,handle,display_name,bio) VALUES($1,$2,'visible-handle','SECRET DISPLAY NAME','SECRET BIO'),($3,$2,'SECRET-HANDLE','Hidden','Hidden'),($4,$5,'foreign','Foreign','Foreign')`, a, org, b, foreign, foreignOrg)
	for _, aid := range []string{a, b, foreign} {
		must(`INSERT INTO verified_domains(domain,org_id,verified_until) SELECT id::text||'.example.test',org_id,now()+interval '30 days' FROM agents WHERE id=$1`, aid)
		must(`INSERT INTO agent_domains(agent_id,domain) VALUES($1::uuid,$1::text||'.example.test')`, aid)
		must(`UPDATE agents SET public_posting=true WHERE id=$1`, aid)
	}
	_, lookup, digest, _ := auth.Generate()
	var cred string
	if err = pool.QueryRow(ctx, `INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,ARRAY['network:read']) RETURNING id::text`, a, lookup, digest).Scan(&cred); err != nil {
		t.Fatal(err)
	}
	must(`INSERT INTO public_enrollments(credential_id) VALUES($1)`, cred)
	raw := uuid.NewString()
	must(`INSERT INTO web_sessions(digest,credential_id,agent_id,org_id,csrf) VALUES($1,$2,$3,$4,'csrf')`, auth.Digest(raw), cred, a, org)
	s, err := New(api.New(pool), false, "http://example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	req := func(method, path string, f url.Values, session bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(f.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if session {
			r.AddCookie(&http.Cookie{Name: "ajent_session", Value: raw})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	form := func() url.Values {
		return url.Values{"csrf": {"csrf"}, "action": {"publish"}, "confirm": {"yes"}, "name": {"Public project <script>"}, "description": {"Building useful directory-proof-" + org}, "website": {"https://example.com/"}, "topics": {"Tools, Research, tools"}, "agent": {a}, "name_" + a: {"Chosen public name"}, "purpose_" + a: {"Checks production deployments."}}
	}
	t.Run("default private and permission boundaries", func(t *testing.T) {
		if w := req("GET", "/domains/"+ns, nil, false); w.Code != 404 {
			t.Fatal(w.Code)
		}
		if w := req("GET", "/app/directory", nil, false); w.Code != 303 {
			t.Fatal(w.Code)
		}
		f := form()
		f.Set("csrf", "bad")
		if w := req("POST", "/actions/directory", f, true); w.Code != 403 {
			t.Fatal(w.Code)
		}
		must(`DELETE FROM public_enrollments WHERE credential_id=$1`, cred)
		if w := req("GET", "/app/directory", nil, true); w.Code != 403 {
			t.Fatal(w.Code)
		}
		if w := req("POST", "/actions/directory", form(), true); w.Code != 403 {
			t.Fatal(w.Code)
		}
		must(`INSERT INTO public_enrollments(credential_id) VALUES($1)`, cred)
	})
	t.Run("unverified organizations cannot publish", func(t *testing.T) {
		must(`UPDATE verified_domains SET verified_until=now()-interval '1 second' WHERE org_id=$1`, org)
		if w := req("POST", "/actions/directory", form(), true); w.Code != 403 {
			t.Fatal("unverified listing accepted", w.Code, w.Body)
		}
		must(`UPDATE verified_domains SET verified_until=now()+interval '30 days' WHERE org_id=$1`, org)
	})
	t.Run("publish selected fields only", func(t *testing.T) {
		w := req("POST", "/actions/directory", form(), true)
		if w.Code != 303 {
			t.Fatal(w.Code, w.Body)
		}
		w = req("GET", "/domains/"+ns, nil, false)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		body := w.Body.String()
		for _, want := range []string{"Public project &lt;script&gt;", "Chosen public name", "Checks production deployments.", "Ajent-issued namespace", "Connect your agent"} {
			if !strings.Contains(body, want) {
				t.Errorf("missing %s", want)
			}
		}
		for _, bad := range []string{"SECRET ORG NAME", "SECRET DISPLAY NAME", "SECRET BIO", "SECRET-HANDLE", foreign, cred, "<script>"} {
			if strings.Contains(body, bad) {
				t.Errorf("leaked %s", bad)
			}
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cache")
		}
	})
	t.Run("invalid edits preserve published listing", func(t *testing.T) {
		for _, change := range []func(url.Values){func(f url.Values) { f.Set("agent", foreign) }, func(f url.Values) { f.Set("website", "javascript:alert(1)") }, func(f url.Values) { f.Set("website", "https://user:pass@example.com") }, func(f url.Values) { f.Set("name", strings.Repeat("x", 101)) }, func(f url.Values) { f.Set("topics", "1,2,3,4,5,6") }, func(f url.Values) { f.Del("confirm") }} {
			f := form()
			change(f)
			f.Set("name_"+foreign, "Foreign")
			f.Set("purpose_"+foreign, "Not mine")
			if w := req("POST", "/actions/directory", f, true); w.Code != 400 {
				t.Fatal(w.Code, w.Body)
			}
		}
		w := req("GET", "/domains/"+ns, nil, false)
		if !strings.Contains(w.Body.String(), "Chosen public name") {
			t.Fatal("invalid update damaged listing")
		}
	})
	t.Run("only selected agents and web-approved discoveries", func(t *testing.T) {
		post := func(author, body string, pub bool) string {
			id := uuid.NewString()
			must(`INSERT INTO posts(id,author_id,kind,body,audience,web_published) VALUES($1,$2,'finding',$3,'public',$4)`, id, author, body, pub)
			return id
		}
		shown := post(a, "shown-discovery", true)
		post(a, "NETWORK-SECRET", false)
		post(b, "UNLISTED-DISCOVERY", true)
		expired := post(a, "EXPIRED-DISCOVERY", true)
		must(`UPDATE posts SET expires_at=now()-interval '1 hour' WHERE id=$1`, expired)
		w := req("GET", "/domains/"+ns, nil, false)
		if !strings.Contains(w.Body.String(), shown) {
			t.Fatal("missing discovery")
		}
		for _, bad := range []string{"NETWORK-SECRET", "UNLISTED-DISCOVERY", "EXPIRED-DISCOVERY"} {
			if strings.Contains(w.Body.String(), bad) {
				t.Fatal("leaked", bad)
			}
		}
		must(`UPDATE posts SET web_published=false WHERE id=$1`, shown)
		if w = req("GET", "/domains/"+ns, nil, false); strings.Contains(w.Body.String(), shown) {
			t.Fatal("withdrawn post leaked")
		}
		must(`UPDATE agents SET status='suspended' WHERE id=$1`, a)
		if w = req("GET", "/domains/"+ns, nil, false); strings.Contains(w.Body.String(), "Chosen public name") {
			t.Fatal("suspended agent leaked")
		}
		must(`UPDATE agents SET status='active' WHERE id=$1`, a)
	})
	t.Run("search filters and bounded pagination", func(t *testing.T) {
		w := req("GET", "/domains?q=directory-proof-"+org+"&topic=tools", nil, false)
		if w.Code != 200 || !strings.Contains(w.Body.String(), ns) {
			t.Fatal(w.Code, w.Body)
		}
		w = req("GET", "/domains?q=directory-proof-"+org+"&topic=other", nil, false)
		if strings.Contains(w.Body.String(), ns) {
			t.Fatal("topic ignored")
		}
		for _, path := range []string{"/domains?page=0", "/domains?page=10001", "/domains?q=" + strings.Repeat("x", 201)} {
			if w = req("GET", path, nil, false); w.Code != 400 {
				t.Fatal(w.Code)
			}
		}
		for i := 0; i < 21; i++ {
			id := uuid.NewString()
			must(`INSERT INTO organizations(id,name,namespace) VALUES($1,'Pagination',$2)`, id, id+".ajent.social")
			must(`INSERT INTO verified_domains(domain,org_id,verified_until) VALUES($1::text||'.example.test',$1::uuid,now()+interval '30 days')`, id)
			must(`INSERT INTO directory_listings(org_id,name,description,published) VALUES($1,'Pagination',$2,true)`, id, "page-marker-"+org)
		}
		w = req("GET", "/domains?q=page-marker-"+org, nil, false)
		if !strings.Contains(w.Body.String(), "Next page") {
			t.Fatal("missing pagination")
		}
		w = req("GET", "/domains?q=page-marker-"+org+"&page=2", nil, false)
		if !strings.Contains(w.Body.String(), "Previous page") || strings.Contains(w.Body.String(), "Next page") {
			t.Fatal("bad last page")
		}
	})
	t.Run("withdrawal suspension and revoked session", func(t *testing.T) {
		must(`UPDATE organizations SET status='suspended' WHERE id=$1`, org)
		if w := req("GET", "/domains/"+ns, nil, false); w.Code != 404 {
			t.Fatal(w.Code)
		}
		must(`UPDATE organizations SET status='active' WHERE id=$1`, org)
		w := req("POST", "/actions/directory", url.Values{"csrf": {"csrf"}, "action": {"withdraw"}}, true)
		if w.Code != 303 {
			t.Fatal(w.Code, w.Body)
		}
		if w = req("GET", "/domains/"+ns, nil, false); w.Code != 404 {
			t.Fatal(w.Code)
		}
		must(`UPDATE credentials SET revoked_at=now() WHERE id=$1`, cred)
		if w = req("POST", "/actions/directory", form(), true); w.Code != 303 {
			t.Fatal(w.Code)
		}
		if w = req("GET", "/domains/"+ns, nil, false); w.Code != 404 {
			t.Fatal("revoked session republished")
		}
	})
}
