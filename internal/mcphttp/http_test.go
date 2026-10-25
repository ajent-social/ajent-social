package mcphttp

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/internal/store"
	"ajent/migrations"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type testAuthority struct {
	mu      sync.Mutex
	revoked bool
	calls   int
	db      *store.Store
}

func (a *testAuthority) MCPVerify(ctx context.Context, token string) (store.Credential, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	if a.revoked {
		return store.Credential{}, errors.New("revoked")
	}
	if a.db == nil {
		if token != "test-token" && token != "read-token" {
			return store.Credential{}, errors.New("invalid")
		}
		if token == "read-token" {
			return store.Credential{Scopes: []string{"network:read"}}, nil
		}
		return store.Credential{Scopes: []string{"network:read", "posts:write", "social:write"}}, nil
	}
	lookup, secret, ok := auth.ParseToken("Bearer " + token)
	if !ok {
		return store.Credential{}, errors.New("invalid")
	}
	return a.db.Authenticate(ctx, lookup, secret)
}
func (a *testAuthority) MCPChallenge(w http.ResponseWriter, r *http.Request, invalid bool) {
	w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://ajent.example/.well-known/oauth-protected-resource"`)
	w.WriteHeader(401)
}
func rpc(t *testing.T, h http.Handler, token, method string, args any) map[string]any {
	t.Helper()
	params := args
	if params == nil {
		params = map[string]any{}
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://ajent.example/mcp", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-11-25")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var out map[string]any
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatalf("not JSON: %s", w.Body.String())
	}
	return out
}
func invoke(t *testing.T, h http.Handler, token, name string, args any) map[string]any {
	return rpc(t, h, token, "tools/call", map[string]any{"name": name, "arguments": args})
}
func resultData(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	result, ok := out["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %#v", out)
	}
	if result["isError"] == true {
		t.Fatalf("tool failed: %#v", out)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("no structured result: %#v", out)
	}
	data, ok := structured["data"].(map[string]any)
	if !ok {
		t.Fatalf("no data: %#v", out)
	}
	return data
}
func TestHTTPBoundaryAndCatalog(t *testing.T) {
	authority := &testAuthority{}
	h := New(api.New(nil), authority, "https://ajent.example")
	for _, tc := range []struct {
		method, token, origin string
		status                int
	}{{"POST", "", "", 401}, {"POST", "bad", "", 401}, {"POST", "test-token", "https://evil.example", 403}, {"GET", "test-token", "", 405}, {"DELETE", "test-token", "", 405}} {
		r := httptest.NewRequest(tc.method, "https://ajent.example/mcp", nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%+v got %d", tc, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("missing no-store")
		}
	}
	init := rpc(t, h, "test-token", "initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}})
	if init["error"] != nil {
		t.Fatalf("initialize: %#v", init)
	}
	list := rpc(t, h, "test-token", "tools/list", nil)
	data := list["result"].(map[string]any)
	tools := data["tools"].([]any)
	if len(tools) != 9 {
		t.Fatalf("catalog has %d tools", len(tools))
	}
	for _, v := range tools {
		name := v.(map[string]any)["name"].(string)
		if strings.Contains(name, "credential") || strings.Contains(name, "admin") {
			t.Fatal("admin tool exposed")
		}
	}
	out := invoke(t, h, "test-token", "ajent_post", map[string]any{"kind": "finding", "body": "text", "key": "one", "approved": false})
	if out["result"].(map[string]any)["isError"] != true {
		t.Fatal("unapproved post accepted")
	}
	out = invoke(t, h, "test-token", "ajent_read", map[string]any{"id": "../../credentials"})
	if out["result"].(map[string]any)["isError"] != true {
		t.Fatal("arbitrary route accepted")
	}
	readonly := rpc(t, h, "read-token", "tools/list", nil)
	if len(readonly["result"].(map[string]any)["tools"].([]any)) != 7 {
		t.Fatal("read-only catalog exposes write tools")
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ajent_post","arguments":{"kind":"finding","title":"No","body":"No","key":"no","approved":true}}}`
	rscope := httptest.NewRequest("POST", "https://ajent.example/mcp", strings.NewReader(body))
	rscope.Header.Set("Authorization", "Bearer read-token")
	wscope := httptest.NewRecorder()
	h.ServeHTTP(wscope, rscope)
	if wscope.Code != 403 || !strings.Contains(wscope.Header().Get("WWW-Authenticate"), `scope="posts:write"`) {
		t.Fatal("missing HTTP insufficient-scope challenge")
	}
	authority.mu.Lock()
	authority.revoked = true
	authority.mu.Unlock()
	r := httptest.NewRequest("POST", "https://ajent.example/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	r.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("revoked token admitted")
	}
}

// This crosses real HTTP MCP tool dispatch and the existing PostgreSQL API.
// OAuth consent/refresh/revocation itself is exercised separately in web tests.
func TestPostReplyVisibilityAndRetryPostgreSQL(t *testing.T) {
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required; disposable PostgreSQL only")
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
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	org, other, agentA, agentB, outsider, group := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO organizations(id,name,namespace) VALUES($1,'MCP workspace',$2),($3,'Outside',$4)`, org, "mcp-"+org, other, "mcp-"+other)
	exec(`INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'grokbot'),($3,$2,'muse'),($4,$5,'outside')`, agentA, org, agentB, outsider, other)
	tokens := []string{}
	for _, agent := range []string{agentA, agentB, outsider} {
		token, lookup, digest, e := auth.Generate()
		if e != nil {
			t.Fatal(e)
		}
		exec(`INSERT INTO credentials(id,agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), agent, lookup, digest, []string{"network:read", "posts:write", "social:write"})
		tokens = append(tokens, token)
	}
	exec(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'MCP private','private')`, group, org)
	exec(`INSERT INTO group_members(group_id,agent_id,state,role) VALUES($1,$2,'accepted','owner'),($1,$3,'accepted','member')`, group, agentA, agentB)
	publicGroup, foreignGroup := uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'Public group','public'),($3,$4,'Foreign private','private')`, publicGroup, org, foreignGroup, other)
	exec(`INSERT INTO group_members(group_id,agent_id,state,role) VALUES($1,$2,'accepted','member'),($3,$2,'accepted','member')`, publicGroup, agentA, foreignGroup)
	authority := &testAuthority{db: &store.Store{Pool: pool}}
	h := New(api.New(pool), authority, "https://ajent.example")
	args := map[string]any{"kind": "finding", "title": "Review question", "body": "Please review this finding", "key": "round-trip", "approved": true, "group_id": group}
	posted := resultData(t, invoke(t, h, tokens[0], "ajent_post", args))
	id, ok := posted["id"].(string)
	if !ok {
		t.Fatalf("post ID absent: %#v", posted)
	}
	retried := resultData(t, invoke(t, h, tokens[0], "ajent_post", args))
	if retried["id"] != id {
		t.Fatal("retry duplicated post")
	}
	read := resultData(t, invoke(t, h, tokens[1], "ajent_read", map[string]any{"id": id}))
	encoded, _ := json.Marshal(read)
	if !strings.Contains(string(encoded), "Please review this finding") {
		t.Fatalf("Muse cannot read: %s", encoded)
	}
	denied := invoke(t, h, tokens[2], "ajent_read", map[string]any{"id": id})
	if denied["result"].(map[string]any)["isError"] != true {
		t.Fatal("outside workspace read private post")
	}
	reply := resultData(t, invoke(t, h, tokens[1], "ajent_post", map[string]any{"kind": "answer", "body": "Reviewed by Muse", "parent_id": id, "group_id": group, "key": "reply-one", "approved": true}))
	if reply["id"] == nil {
		t.Fatal("missing reply")
	}
	var author string
	if err = pool.QueryRow(ctx, `SELECT author_id::text FROM posts WHERE id=$1`, reply["id"]).Scan(&author); err != nil {
		t.Fatal(err)
	}
	if author != agentB {
		t.Fatal("reply has wrong attribution")
	}
	inbox := resultData(t, invoke(t, h, tokens[0], "ajent_inbox", map[string]any{}))
	if inbox["acknowledge_through"] == nil {
		t.Fatal("missing inbox cursor")
	}
	resultData(t, invoke(t, h, tokens[0], "ajent_ack", map[string]any{"through": inbox["acknowledge_through"]}))
	for _, blockedGroup := range []string{publicGroup, foreignGroup} {
		denied := invoke(t, h, tokens[0], "ajent_post", map[string]any{"kind": "finding", "title": "Boundary", "body": "Should not publish", "group_id": blockedGroup, "key": "blocked-" + blockedGroup, "approved": true})
		if denied["result"].(map[string]any)["isError"] != true {
			t.Fatal("connector escaped private workspace publication boundary")
		}
	}
	if authority.calls < 14 {
		t.Fatal("authority not rechecked per request and tool")
	}
}
