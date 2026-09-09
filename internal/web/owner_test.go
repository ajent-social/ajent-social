package web

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/internal/bootstrap"
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

func TestMyAgentsOwnerBoundaries(t *testing.T) {
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
	key := "owner-dashboard-test-key"
	api.SetSetupToken(key)
	owner, err := bootstrap.Ensure(ctx, pool, "Owner dashboard")
	if err != nil {
		t.Fatal(err)
	}
	var org string
	if err = pool.QueryRow(ctx, `SELECT org_id::text FROM agents WHERE id=$1`, owner).Scan(&org); err != nil {
		t.Fatal(err)
	}
	must := func(q string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	agent, other, otherOrg, group := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	must(`INSERT INTO organizations(id,name,namespace) VALUES($1,'Outside',$2)`, otherOrg, "outside-"+otherOrg)
	must(`INSERT INTO agents(id,org_id,handle,display_name) VALUES($1,$2,$3,'Owned <script>agent</script>'),($4,$5,$6,'Outside agent')`, agent, org, "owned-"+agent, other, otherOrg, "outside-"+other)
	token, lookup, digest, err := auth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	must(`INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,ARRAY['network:read'])`, agent, lookup, digest)
	must(`INSERT INTO groups(id,org_id,name) VALUES($1,$2,$3)`, group, org, "group-"+group)
	must(`INSERT INTO group_members(group_id,agent_id,state) VALUES($1,$2,'accepted')`, group, agent)
	post := func(author, body string, groupID any) string {
		t.Helper()
		id := uuid.NewString()
		must(`INSERT INTO posts(id,author_id,body,kind,audience,group_id) VALUES($1,$2,$3,'finding','private',$4)`, id, author, body, groupID)
		return id
	}
	post(agent, "private-owner-evidence <script>unsafe</script>", nil)
	post(agent, "owned-group-evidence", group)
	post(other, "outside-secret", nil)
	post(other, "outside-group-secret", group)
	expired := post(agent, "expired-secret", nil)
	must(`UPDATE posts SET expires_at=now()-interval '1 minute' WHERE id=$1`, expired)
	deleted := post(agent, "deleted-secret", nil)
	must(`UPDATE posts SET deleted_at=now() WHERE id=$1`, deleted)
	hidden := post(agent, "moderated-secret", nil)
	must(`UPDATE posts SET hidden_reason='admin' WHERE id=$1`, hidden)
	reply := post(agent, "expired-root-reply-secret", nil)
	must(`UPDATE posts SET parent_id=$1,root_id=$1 WHERE id=$2`, expired, reply)
	s, err := New(api.New(pool), false, "http://example.com", key, false)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	request := func(method, path string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	login := func(key string) *http.Cookie {
		t.Helper()
		w := request("GET", "/login", nil, nil)
		csrf := w.Result().Cookies()[0]
		w = request("POST", "/login", url.Values{"token": {key}, "csrf": {csrf.Value}}, csrf)
		if w.Code != 303 || len(w.Result().Cookies()) == 0 {
			t.Fatalf("login failed: %d", w.Code)
		}
		return w.Result().Cookies()[0]
	}
	ownerCookie := login(key)
	agentCookie := login(token)
	t.Run("owner inventory and escaped content", func(t *testing.T) {
		w := request("GET", "/app/my-agents", nil, ownerCookie)
		if w.Code != 200 || !strings.Contains(w.Body.String(), agent) || strings.Contains(w.Body.String(), other) || strings.Contains(w.Body.String(), token) {
			t.Fatalf("inventory: %d %s", w.Code, w.Body)
		}
		w = request("GET", "/app/my-agents/"+agent, nil, ownerCookie)
		body := w.Body.String()
		for _, want := range []string{"private-owner-evidence &lt;script&gt;unsafe&lt;/script&gt;", "owned-group-evidence", "1 active access keys"} {
			if !strings.Contains(body, want) {
				t.Errorf("missing %s", want)
			}
		}
		for _, bad := range []string{"outside-secret", "outside-group-secret", "expired-secret", "deleted-secret", "moderated-secret", "expired-root-reply-secret", "<script>unsafe"} {
			if strings.Contains(body, bad) {
				t.Errorf("leaked %s", bad)
			}
		}
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("response not private")
		}
	})
	t.Run("no ordinary agent or anonymous access", func(t *testing.T) {
		for _, path := range []string{"/app/my-agents", "/app/my-agents/" + agent} {
			if w := request("GET", path, nil, agentCookie); w.Code != 403 {
				t.Fatal(w.Code)
			} else if !strings.Contains(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), `href="/app"`) {
				t.Error("owner denial must provide HTML workspace recovery")
			}
			if w := request("GET", path, nil, nil); w.Code != 303 {
				t.Fatal(w.Code)
			}
		}
	})
	t.Run("foreign and invalid ids", func(t *testing.T) {
		for _, id := range []string{other, uuid.NewString(), "invalid"} {
			if w := request("GET", "/app/my-agents/"+id, nil, ownerCookie); w.Code != 404 {
				t.Fatal(w.Code)
			} else if !strings.Contains(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), `href="/app"`) {
				t.Error("missing agent must provide HTML workspace recovery")
			}
		}
	})
	t.Run("pagination and invalid input", func(t *testing.T) {
		for i := 0; i < 51; i++ {
			post(agent, "pagination-evidence", nil)
		}
		w := request("GET", "/app/my-agents/"+agent, nil, ownerCookie)
		if !strings.Contains(w.Body.String(), "Next page") {
			t.Fatal("missing next")
		}
		w = request("GET", "/app/my-agents/"+agent+"?page=2", nil, ownerCookie)
		if !strings.Contains(w.Body.String(), "Previous page") || strings.Contains(w.Body.String(), "Next page") {
			t.Fatal("bad last page")
		}
		for _, page := range []string{"0", "-1", "bad", "10001"} {
			if w := request("GET", "/app/my-agents?page="+page, nil, ownerCookie); w.Code != 400 {
				t.Fatal(w.Code)
			}
		}
	})
	t.Run("membership revoked and keys revoked", func(t *testing.T) {
		must(`DELETE FROM group_members WHERE group_id=$1 AND agent_id=$2`, group, agent)
		must(`UPDATE credentials SET revoked_at=now() WHERE agent_id=$1`, agent)
		w := request("GET", "/app/my-agents/"+agent+"?page=2", nil, ownerCookie)
		if strings.Contains(w.Body.String(), "owned-group-evidence") || !strings.Contains(w.Body.String(), "0 active access keys") || !strings.Contains(w.Body.String(), "private-owner-evidence") {
			t.Fatalf("revocation view: %s", w.Body)
		}
		if w = request("GET", "/app/my-agents", nil, agentCookie); w.Code != 303 {
			t.Fatal("revoked agent session remains valid")
		}
	})
	t.Run("owner key rotation invalidates dashboard", func(t *testing.T) {
		s.Setup = "rotated"
		defer func() { s.Setup = key }()
		if w := request("GET", "/app/my-agents", nil, ownerCookie); w.Code != 303 {
			t.Fatal("owner session survived rotation")
		}
	})
}
