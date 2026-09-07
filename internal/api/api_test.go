package api

import (
	"ajent/internal/auth"
	"ajent/internal/events"
	"ajent/internal/jobs"
	"ajent/migrations"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixture struct {
	t                    *testing.T
	s                    *Server
	m                    http.Handler
	org, a, b, ta, tb, g string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required; use an isolated disposable PostgreSQL database")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	if e = migrations.Apply(ctx, pool); e != nil {
		t.Fatal(e)
	}
	f := &fixture{t: t, s: New(pool), org: uuid.NewString(), a: uuid.NewString(), b: uuid.NewString(), g: uuid.NewString()}
	must := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	orgB := uuid.NewString()
	must(`INSERT INTO organizations(id,name,namespace) VALUES($1,'Test owner',$2),($3,'Outside owner',$4)`, f.org, "test-"+f.org, orgB, "test-"+orgB)
	must(`INSERT INTO agents(id,org_id,handle,public_posting) VALUES($1,$2,'alice',true),($3,$4,'bob',true)`, f.a, f.org, f.b, orgB)
	for _, aid := range []string{f.a, f.b} {
		must(`INSERT INTO verified_domains(domain,org_id,verified_until) SELECT id::text||'.example.test',org_id,now()+interval '30 days' FROM agents WHERE id=$1`, aid)
		must(`INSERT INTO agent_domains(agent_id,domain) VALUES($1::uuid,$1::text||'.example.test')`, aid)
		must(`UPDATE agents SET public_posting=true WHERE id=$1`, aid)
	}
	for _, id := range []string{f.a, f.b} {
		token, l, d, e := auth.Generate()
		if e != nil {
			t.Fatal(e)
		}
		must(`INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,ARRAY['network:read','posts:write','social:write','groups:manage'])`, id, l, d)
		if id == f.a {
			f.ta = token
		} else {
			f.tb = token
		}
	}
	must(`INSERT INTO groups(id,org_id,name) VALUES($1,$2,$3)`, f.g, f.org, "private-"+f.g)
	must(`INSERT INTO group_members(group_id,agent_id,role) VALUES($1,$2,'owner')`, f.g, f.a)
	m := http.NewServeMux()
	f.s.Routes(m)
	f.m = m
	return f
}
func (f *fixture) request(method, path, token, body, key string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	f.m.ServeHTTP(w, r)
	return w
}
func (f *fixture) post(token, g, parent string) string {
	b, _ := json.Marshal(PostRequest{Kind: "finding", Title: "Needle finding", Body: "needle private evidence <script>alert(1)</script>", SchemaVersion: 1, GroupID: g, ParentID: parent})
	w := f.request("POST", "/v1/posts", token, string(b), uuid.NewString())
	if w.Code != 201 {
		f.t.Fatalf("post: %d %s", w.Code, w.Body)
	}
	var v map[string]any
	json.Unmarshal(w.Body.Bytes(), &v)
	return v["id"].(string)
}
func TestPrivacyAndReplyAudience(t *testing.T) {
	f := setup(t)
	id := f.post(f.ta, f.g, "")
	for _, path := range []string{"/v1/posts/" + id, "/v1/feed", "/v1/search?q=needle", "/v1/events?after=0"} {
		w := f.request("GET", path, f.tb, "", "")
		if strings.Contains(w.Body.String(), id) {
			t.Errorf("private ID leaked on %s: %s", path, w.Body)
		}
		if strings.Contains(path, "/posts/") && w.Code != 404 {
			t.Errorf("expected 404: %d", w.Code)
		}
	}
	w := f.request("POST", "/v1/posts", f.tb, fmt.Sprintf(`{"kind":"answer","body":"leak","parent_id":%q,"audience":"public"}`, id), uuid.NewString())
	if w.Code != 404 {
		t.Fatalf("reply to inaccessible parent allowed: %d %s", w.Code, w.Body)
	}
	f.request("POST", "/v1/follows/"+f.a, f.tb, "", "")
	w = f.request("GET", "/v1/posts/"+id, f.tb, "", "")
	if w.Code != 404 {
		t.Fatal("follow opened private group")
	}
	reply := f.post(f.ta, "", id)
	var gid string
	f.s.DB.Pool.QueryRow(context.Background(), `SELECT group_id::text FROM posts WHERE id=$1`, reply).Scan(&gid)
	if gid != f.g {
		t.Fatal("reply changed audience")
	}
}
func TestAtomicIdempotency(t *testing.T) {
	f := setup(t)
	body := fmt.Sprintf(`{"kind":"status","body":"once","group_id":%q}`, f.g)
	key := uuid.NewString()
	var wg sync.WaitGroup
	out := make(chan *httptest.ResponseRecorder, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); out <- f.request("POST", "/v1/posts", f.ta, body, key) }()
	}
	wg.Wait()
	close(out)
	seen := map[string]bool{}
	for w := range out {
		if w.Code != 201 {
			t.Fatalf("retry %d %s", w.Code, w.Body)
		}
		var result map[string]any
		json.Unmarshal(w.Body.Bytes(), &result)
		seen[result["id"].(string)] = true
	}
	if len(seen) != 1 {
		t.Fatal("duplicate IDs")
	}
	var n int
	f.s.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM posts WHERE author_id=$1 AND body='once'`, f.a).Scan(&n)
	if n != 1 {
		t.Fatalf("%d writes", n)
	}
	w := f.request("POST", "/v1/posts", f.ta, strings.Replace(body, "once", "twice", 1), key)
	if w.Code != 409 {
		t.Fatalf("conflicting retry %d", w.Code)
	}
}
func TestExpiryBlocksAndRevocation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	f.s.DB.Pool.Exec(ctx, `INSERT INTO group_members(group_id,agent_id) VALUES($1,$2)`, f.g, f.b)
	id := f.post(f.ta, f.g, "")
	reply := f.post(f.tb, f.g, id)
	f.request("POST", "/v1/blocks", f.ta, fmt.Sprintf(`{"agent_id":%q}`, f.b), "")
	if w := f.request("GET", "/v1/posts/"+reply, f.ta, "", ""); w.Code != 404 {
		t.Fatal("blocker sees blocked reply")
	}
	if w := f.request("GET", "/v1/posts/"+id, f.tb, "", ""); w.Code != 200 {
		t.Fatal("blocked shared member lost read")
	}
	f.s.DB.Pool.Exec(ctx, `UPDATE posts SET expires_at=now()-interval '1 second' WHERE id=$1`, id)
	if w := f.request("GET", "/v1/posts/"+reply, f.tb, "", ""); w.Code != 404 {
		t.Fatal("reply outlived root")
	}
	if e := jobs.Cleanup(ctx, f.s.DB.Pool); e != nil {
		t.Fatal(e)
	}
	f.s.DB.Pool.Exec(ctx, `UPDATE credentials SET revoked_at=now() WHERE agent_id=$1`, f.b)
	if w := f.request("GET", "/v1/feed", f.tb, "", ""); w.Code != 401 {
		t.Fatal("revoked credential accepted")
	}
}
func TestPublisherAndCursorExpiry(t *testing.T) {
	f := setup(t)
	id := f.post(f.ta, f.g, "")
	if _, e := events.PublishOnce(context.Background(), f.s.DB.Pool); e != nil {
		t.Fatal(e)
	}
	w := f.request("GET", "/v1/events?after=0", f.ta, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), id) {
		t.Fatalf("event missing: %s", w.Body)
	}
	f.s.DB.Pool.Exec(context.Background(), `UPDATE stream_watermark SET purged_through=999`)
	w = f.request("GET", "/v1/events?after=1", f.ta, "", "")
	if w.Code != 410 {
		t.Fatal("expired cursor not rejected")
	}
	f.s.DB.Pool.Exec(context.Background(), `UPDATE stream_watermark SET purged_through=0`)
}
func TestAuthAndSearch(t *testing.T) {
	f := setup(t)
	if w := f.request("GET", "/v1/feed", "", "", ""); w.Code != 401 {
		t.Fatal("anonymous read")
	}
	id := f.post(f.ta, f.g, "")
	w := f.request("GET", "/v1/search?q=needle", f.ta, "", "")
	if !strings.Contains(w.Body.String(), id) {
		t.Fatalf("search failed: %s", w.Body)
	}
	f.s.DB.Pool.Exec(context.Background(), `UPDATE credentials SET scopes=ARRAY['network:read'] WHERE agent_id=$1`, f.b)
	w = f.request("POST", "/v1/posts", f.tb, `{"kind":"status","body":"no"}`, uuid.NewString())
	if w.Code != 403 {
		t.Fatal("write scope not enforced")
	}
	_ = time.Second
}

func TestReplyUsesRootAudience(t *testing.T) {
	f := setup(t)
	outsider := setup(t)
	ctx := context.Background()
	for _, pair := range [][2]string{{f.b, f.a}, {outsider.a, f.b}} {
		if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO follows(follower_id,followee_id) VALUES($1,$2)`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	w := f.request("POST", "/v1/posts", f.ta, `{"kind":"question","body":"restricted root","audience":"followers"}`, uuid.NewString())
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var root map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &root); err != nil {
		t.Fatal(err)
	}
	reply := f.post(f.tb, "", root["id"])
	if w := f.request("GET", "/v1/posts/"+reply, outsider.ta, "", ""); w.Code != 404 {
		t.Fatalf("reply leaked outside root audience: %d %s", w.Code, w.Body)
	}
	if w := f.request("GET", "/v1/posts/"+reply, f.ta, "", ""); w.Code != 200 {
		t.Fatalf("root author cannot read reply: %d %s", w.Code, w.Body)
	}
	if _, err := f.s.DB.Pool.Exec(ctx, `DELETE FROM follows WHERE follower_id=$1 AND followee_id=$2`, f.b, f.a); err != nil {
		t.Fatal(err)
	}
	if w := f.request("GET", "/v1/posts/"+reply, f.tb, "", ""); w.Code != 404 {
		t.Fatalf("former follower retains conversation access: %d", w.Code)
	}
}

func TestDatabaseFailureIsNotInvalidCredential(t *testing.T) {
	f := setup(t)
	f.s.DB.Pool.Close()
	if w := f.request("GET", "/v1/feed", f.ta, "", ""); w.Code != 503 {
		t.Fatalf("database outage returned %d, want 503", w.Code)
	}
}

func TestQuotaFailureRollsBack(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO usage_counters(org_id,dimension,bucket,count) VALUES($1,'posts_day',(now() AT TIME ZONE 'UTC')::date,1000)`, f.org); err != nil {
		t.Fatal(err)
	}
	w := f.request("POST", "/v1/posts", f.ta, `{"kind":"status","body":"over quota"}`, uuid.NewString())
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("quota response %d %s", w.Code, w.Body)
	}
	var n int
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM posts WHERE author_id=$1`, f.a).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("over-quota post persisted")
	}
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count FROM usage_counters WHERE org_id=$1`, f.org).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1000 {
		t.Fatalf("quota changed to %d", n)
	}
}

func TestPaginationAndConcurrentPublishers(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	expected := map[string]bool{}
	for i := 0; i < 45; i++ {
		expected[f.post(f.ta, f.g, "")] = true
	}
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 4; page++ {
		w := f.request("GET", "/v1/groups/"+f.g+"/posts?before="+cursor, f.ta, "", "")
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var response struct {
			Posts []Post `json:"posts"`
			Next  string `json:"next_cursor"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		for _, p := range response.Posts {
			if seen[p.ID] {
				t.Fatal("duplicate page entry")
			}
			seen[p.ID] = true
		}
		cursor = response.Next
		if cursor == "" {
			break
		}
	}
	if len(seen) != 45 {
		t.Fatalf("paged %d of 45 posts", len(seen))
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := events.PublishOnce(ctx, f.s.DB.Pool); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for id := range expected {
		var n int
		if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM stream_events WHERE object_id=$1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("post has %d events", n)
		}
	}
}

func TestCleanupBoundsAndEventuallyScrubs(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, err := f.s.DB.Pool.Exec(ctx, `INSERT INTO posts(author_id,kind,title,body,expires_at) SELECT $1,'status','old','expired evidence',now()-interval '1 day' FROM generate_series(1,1005)`, f.a); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Cleanup(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM posts WHERE author_id=$1 AND body<>''`, f.a).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 5 || n >= 1005 {
		t.Fatalf("unbounded or ineffective cleanup: %d remain", n)
	}
	if err := jobs.Cleanup(ctx, f.s.DB.Pool); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM posts WHERE author_id=$1 AND body<>''`, f.a).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("cleanup left %d expired payloads", n)
	}
}
