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

func TestPublicWebBoundaries(t *testing.T) {
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
	org, agent, other := uuid.NewString(), uuid.NewString(), uuid.NewString()
	must(`INSERT INTO organizations(id,name,namespace) VALUES($1,'private organization',$2)`, org, "private-"+org)
	must(`INSERT INTO agents(id,org_id,handle,display_name) VALUES($1,$2,'publisher','Public <script>name</script>'),($3,$2,'other','Other')`, agent, org, other)
	for _, aid := range []string{agent, other} {
		must(`INSERT INTO verified_domains(domain,org_id,verified_until) SELECT id::text||'.example.test',org_id,now()+interval '30 days' FROM agents WHERE id=$1`, aid)
		must(`INSERT INTO agent_domains(agent_id,domain) VALUES($1::uuid,$1::text||'.example.test')`, aid)
		must(`UPDATE agents SET public_posting=true WHERE id=$1`, aid)
	}
	post := func(body string, published bool) string {
		id := uuid.NewString()
		must(`INSERT INTO posts(id,author_id,kind,body,audience,web_published) VALUES($1,$2,'finding',$3,'public',$4)`, id, agent, body, published)
		return id
	}
	visible := post("public-evidence <script>unsafe</script>", true)
	network := post("network-only-secret", false)
	private := post("private-secret", true)
	must(`UPDATE posts SET audience='private' WHERE id=$1`, private)
	followers := post("followers-secret", true)
	must(`UPDATE posts SET audience='followers' WHERE id=$1`, followers)
	expired := post("expired-secret", true)
	must(`UPDATE posts SET expires_at=now()-interval '1 hour' WHERE id=$1`, expired)
	hidden := post("hidden-secret", true)
	must(`UPDATE posts SET hidden_reason='admin' WHERE id=$1`, hidden)
	deleted := post("deleted-secret", true)
	must(`UPDATE posts SET deleted_at=now() WHERE id=$1`, deleted)
	group := uuid.NewString()
	must(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'private group name','public')`, group, org)
	grouped := post("group-secret", true)
	must(`UPDATE posts SET group_id=$1 WHERE id=$2`, group, grouped)
	reply := post("reply-secret", true)
	must(`UPDATE posts SET parent_id=$1,root_id=$1 WHERE id=$2`, network, reply)
	nested := post("nested-secret", true)
	must(`UPDATE posts SET parent_id=$1,root_id=$2 WHERE id=$3`, reply, visible, nested)
	token, lookup, digest, _ := auth.Generate()
	var cred string
	if err = pool.QueryRow(ctx, `INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,ARRAY['network:read','posts:write']) RETURNING id::text`, agent, lookup, digest).Scan(&cred); err != nil {
		t.Fatal(err)
	}
	raw := uuid.NewString()
	must(`INSERT INTO web_sessions(digest,credential_id,agent_id,org_id,csrf) VALUES($1,$2,$3,$4,'csrf')`, auth.Digest(raw), cred, agent, org)
	s, err := New(api.New(pool), false, "http://example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	request := func(method, path string, form url.Values, session bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if session {
			r.AddCookie(&http.Cookie{Name: "ajent_session", Value: raw})
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	t.Run("anonymous projection and escaping", func(t *testing.T) {
		w := request("GET", "/explore?q=public-evidence", nil, false)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
		body := w.Body.String()
		if !strings.Contains(body, "public-evidence &lt;script&gt;unsafe&lt;/script&gt;") {
			t.Fatal(body)
		}
		for _, bad := range []string{"private organization", "private group name", org, token, "<script>unsafe", "network-only-secret", "private-secret", "group-secret", "nested-secret"} {
			if strings.Contains(body, bad) {
				t.Errorf("leaked %s", bad)
			}
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cache")
		}
	})
	t.Run("direct links deny every hidden category", func(t *testing.T) {
		for _, id := range []string{network, private, followers, expired, hidden, deleted, grouped, reply, nested, uuid.NewString(), "invalid"} {
			if w := request("GET", "/explore/posts/"+id, nil, false); w.Code != 404 {
				t.Errorf("%s: %d", id, w.Code)
			}
		}
		if w := request("GET", "/explore/posts/"+visible, nil, false); w.Code != 200 {
			t.Fatal(w.Code)
		}
	})
	t.Run("publication requires session csrf consent and authorship", func(t *testing.T) {
		f := url.Values{"id": {network}, "publication": {"publish"}, "confirm": {"yes"}, "csrf": {"csrf"}}
		if w := request("POST", "/actions/web-publication", f, false); w.Code != 303 {
			t.Fatal(w.Code)
		}
		f.Set("csrf", "bad")
		if w := request("POST", "/actions/web-publication", f, true); w.Code != 403 {
			t.Fatal(w.Code)
		}
		f.Set("csrf", "csrf")
		f.Del("confirm")
		if w := request("POST", "/actions/web-publication", f, true); w.Code != 400 {
			t.Fatal(w.Code)
		}
		f.Set("confirm", "yes")
		foreign := post("foreign-secret", false)
		must(`UPDATE posts SET author_id=$1 WHERE id=$2`, other, foreign)
		f.Set("id", foreign)
		if w := request("POST", "/actions/web-publication", f, true); w.Code != 404 {
			t.Fatal(w.Code)
		}
		for _, id := range []string{private, grouped, expired} {
			f.Set("id", id)
			if w := request("POST", "/actions/web-publication", f, true); w.Code != 404 {
				t.Fatal(w.Code)
			}
		}
		f.Set("id", network)
		if w := request("POST", "/actions/web-publication", f, true); w.Code != 303 {
			t.Fatal(w.Code, w.Body)
		}
		if w := request("GET", "/explore/posts/"+reply, nil, false); w.Code != 200 {
			t.Fatal("published ancestor did not unlock reply", w.Code)
		}
		f.Set("publication", "withdraw")
		if w := request("POST", "/actions/web-publication", f, true); w.Code != 303 {
			t.Fatal(w.Code)
		}
		if w := request("GET", "/explore/posts/"+reply, nil, false); w.Code != 404 {
			t.Fatal("withdrawn ancestor leaked", w.Code)
		}
	})
	t.Run("read-only keys and revoked sessions cannot publish", func(t *testing.T) {
		f := url.Values{"id": {network}, "publication": {"publish"}, "confirm": {"yes"}, "csrf": {"csrf"}}
		must(`UPDATE credentials SET scopes=ARRAY['network:read'] WHERE id=$1`, cred)
		if w := request("POST", "/actions/web-publication", f, true); w.Code != 403 {
			t.Fatal(w.Code)
		}
		must(`UPDATE credentials SET scopes=ARRAY['network:read','posts:write'],revoked_at=now() WHERE id=$1`, cred)
		if w := request("POST", "/actions/web-publication", f, true); w.Code != 303 {
			t.Fatal(w.Code)
		}
		var published bool
		if err := pool.QueryRow(ctx, `SELECT web_published FROM posts WHERE id=$1`, network).Scan(&published); err != nil || published {
			t.Fatal("revoked key published", err)
		}
		must(`UPDATE credentials SET revoked_at=NULL WHERE id=$1`, cred)
	})
	t.Run("owner is limited to its organization and changes are audited", func(t *testing.T) {
		owned := post("owned-publication", false)
		must(`UPDATE posts SET author_id=$1 WHERE id=$2`, other, owned)
		outsideOrg, outsideAgent := uuid.NewString(), uuid.NewString()
		must(`INSERT INTO organizations(id,name,namespace) VALUES($1,'Outside',$2)`, outsideOrg, "outside-"+outsideOrg)
		must(`INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'outside')`, outsideAgent, outsideOrg)
		outside := post("outside-publication", false)
		must(`UPDATE posts SET author_id=$1 WHERE id=$2`, outsideAgent, outside)
		must(`UPDATE web_sessions SET is_owner=true,owner_key_digest=$1 WHERE digest=$2`, auth.Digest(s.Setup), auth.Digest(raw))
		f := url.Values{"id": {owned}, "publication": {"publish"}, "confirm": {"yes"}, "csrf": {"csrf"}}
		if w := request("POST", "/actions/web-publication", f, true); w.Code != 303 {
			t.Fatal(w.Code)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE object_id=$1 AND action='post.web_publication'`, owned).Scan(&count); err != nil || count != 1 {
			t.Fatal("audit", count, err)
		}
		f.Set("id", outside)
		if w := request("POST", "/actions/web-publication", f, true); w.Code != 404 {
			t.Fatal(w.Code)
		}
		must(`UPDATE web_sessions SET is_owner=false WHERE digest=$1`, auth.Digest(raw))
	})
	t.Run("suspensions remove public content", func(t *testing.T) {
		must(`UPDATE agents SET status='suspended' WHERE id=$1`, agent)
		if w := request("GET", "/explore/posts/"+visible, nil, false); w.Code != 404 {
			t.Fatal(w.Code)
		}
		must(`UPDATE agents SET status='active' WHERE id=$1`, agent)
		must(`UPDATE organizations SET status='suspended' WHERE id=$1`, org)
		if w := request("GET", "/explore/posts/"+visible, nil, false); w.Code != 404 {
			t.Fatal(w.Code)
		}
		must(`UPDATE organizations SET status='active' WHERE id=$1`, org)
	})
	t.Run("bounded search pagination and read only", func(t *testing.T) {
		for i := 0; i < 21; i++ {
			post("pagination-"+org, true)
		}
		w := request("GET", "/explore?q=pagination-"+org, nil, false)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Next page") {
			t.Fatal(w.Code, w.Body)
		}
		w = request("GET", "/explore?page=2&q=pagination-"+org, nil, false)
		if !strings.Contains(w.Body.String(), "Previous page") || strings.Contains(w.Body.String(), "Next page") {
			t.Fatal(w.Body)
		}
		for _, path := range []string{"/explore?page=0", "/explore?page=10001", "/explore?q=" + strings.Repeat("a", 201)} {
			if w = request("GET", path, nil, false); w.Code != 400 {
				t.Fatal(w.Code)
			}
		}
		if w = request("POST", "/explore", nil, false); w.Code != 405 {
			t.Fatal(w.Code)
		}
		if w = request("GET", "/app/public-sharing?q=network-only-secret", nil, true); w.Code != 200 || !strings.Contains(w.Body.String(), "I approve publishing") {
			t.Fatal(w.Code, w.Body)
		}
	})
}
