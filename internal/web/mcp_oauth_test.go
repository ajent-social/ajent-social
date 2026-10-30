package web

import (
	"ajent/internal/agenthandle"
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/internal/mcphttp"
	oauth "ajent/internal/mcpoauth"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	protocol "github.com/ajent-social/go/mcpoauth"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMCPOAuthHandlePolicyAndCasePreservation(t *testing.T) {
	p := newMCPFixture(t)
	for _, name := range []string{"GROK.Bot_9-X", strings.Repeat("Z", agenthandle.MaxLength)} {
		h := p.begin()
		form := url.Values{"csrf": {p.csrf}, "consent": {h}, "decision": {"approve"}, "workspace": {p.workspace}, "agent": {""}, "handle": {name}, "scope": {"network:read"}}
		// A client bypassing JavaScript still cannot submit invalid names.
		for _, invalid := range []string{"A", "Grok Bot", "Grok/Bot", "Ågent", strings.Repeat("Z", agenthandle.MaxLength+1)} {
			form.Set("handle", invalid)
			bad := p.request("POST", "/oauth/consent", form, "")
			p.want(bad, 400)
			if !strings.Contains(bad.Body.String(), agenthandle.Help) {
				t.Fatal("handle rejection did not explain the shared rule")
			}
		}
		// The invalid attempts must not consume the consent request.
		form.Set("handle", name)
		tokens := p.exchange(p.request("POST", "/oauth/consent", form, ""))
		credential, err := p.s.MCPVerify(context.Background(), tokens["access_token"].(string))
		if err != nil {
			t.Fatal(err)
		}
		var saved string
		if err := p.s.API.DB.Pool.QueryRow(context.Background(), `SELECT handle FROM agents WHERE id=$1`, credential.AgentID).Scan(&saved); err != nil || saved != name {
			t.Fatal("accepted handle lost its case or length", err)
		}
	}
}

type mcpFixture struct {
	*humanFixture
	human, workspace, client, verifier, csrf string
}

func newMCPFixture(t *testing.T) *mcpFixture {
	f := newHumanFixture(t)
	if e := f.s.MCPRoutes(f.m.(*http.ServeMux)); e != nil {
		t.Fatal(e)
	}
	p := &mcpFixture{humanFixture: f, human: uuid.NewString(), workspace: uuid.NewString(), verifier: strings.Repeat("a", 43), csrf: uuid.NewString()}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO humans(id,name) VALUES($1,'Connector owner')`, []any{p.human}},
		{`INSERT INTO organizations(id,name,namespace,human_owner_id) VALUES($1,'Connector workspace',$2,$3)`, []any{p.workspace, "mcp-" + p.workspace, p.human}},
		{`INSERT INTO human_sessions(digest,human_id,csrf) VALUES($1,$2,$3)`, []any{auth.Digest("session-" + p.human), p.human, p.csrf}},
	} {
		if _, e := f.s.API.DB.Pool.Exec(context.Background(), q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	f.cookies["ajent_human"] = &http.Cookie{Name: "ajent_human", Value: "session-" + p.human}
	w := f.request("POST", "/oauth/register", map[string]any{"client_name": "Test connector <script>alert(1)</script>", "redirect_uris": []string{"https://client.example/callback"}, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}}, "")
	f.want(w, 201)
	p.client = jsonMap(t, w)["client_id"].(string)
	return p
}
func (p *mcpFixture) begin() string {
	h := sha256.Sum256([]byte(p.verifier))
	q := url.Values{"client_id": {p.client}, "redirect_uri": {"https://client.example/callback"}, "response_type": {"code"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(h[:])}, "resource": {p.s.Origin + "/mcp"}, "scope": {"network:read posts:write social:write"}, "state": {"example-state"}}
	w := p.request("GET", "/oauth/authorize?"+q.Encode(), nil, "")
	p.want(w, 200)
	if strings.Contains(w.Body.String(), "<script>alert(1)</script>") {
		p.t.Fatal("unescaped client name")
	}
	m := regexp.MustCompile(`name="consent" value="([A-Za-z0-9_-]+)"`).FindStringSubmatch(w.Body.String())
	if len(m) != 2 {
		p.t.Fatal("missing consent form")
	}
	return m[1]
}
func (p *mcpFixture) approve(handle, existing string, scopes ...string) *httptest.ResponseRecorder {
	return p.request("POST", "/oauth/consent", url.Values{"csrf": {p.csrf}, "consent": {handle}, "decision": {"approve"}, "workspace": {p.workspace}, "agent": {existing}, "handle": {"muse-" + uuid.NewString()[:8]}, "scope": scopes}, "")
}
func (p *mcpFixture) exchange(w *httptest.ResponseRecorder) map[string]any {
	p.want(w, 303)
	u, e := url.Parse(w.Header().Get("Location"))
	if e != nil {
		p.t.Fatal(e)
	}
	if u.Query().Get("state") != "example-state" {
		p.t.Fatal("lost state")
	}
	token := p.request("POST", "/oauth/token", url.Values{"grant_type": {"authorization_code"}, "client_id": {p.client}, "redirect_uri": {"https://client.example/callback"}, "code": {u.Query().Get("code")}, "code_verifier": {p.verifier}, "resource": {p.s.Origin + "/mcp"}}, "")
	p.want(token, 200)
	return jsonMap(p.t, token)
}
func TestMCPOAuthConsentTokenRefreshRevoke(t *testing.T) {
	p := newMCPFixture(t)
	h := p.begin()
	tokens := p.exchange(p.approve(h, "", "network:read", "posts:write", "social:write"))
	raw := tokens["access_token"].(string)
	c, e := p.s.MCPVerify(context.Background(), raw)
	if e != nil {
		t.Fatal(e)
	}
	if c.OrgID != p.workspace || !slices.Equal(c.Scopes, []string{"network:read", "posts:write", "social:write"}) {
		t.Fatalf("incorrect binding: %+v", c)
	}
	// A consent can neither be replayed nor create another agent on retry.
	p.want(p.approve(h, "", "network:read"), 400)
	refresh := tokens["refresh_token"].(string)
	w := p.request("POST", "/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {p.client}, "resource": {p.s.Origin + "/mcp"}}, "")
	p.want(w, 200)
	next := jsonMap(t, w)
	if _, e = p.s.MCPVerify(context.Background(), next["access_token"].(string)); e != nil {
		t.Fatal(e)
	}
	p.want(p.request("GET", "/human/connections", nil, ""), 200)
	p.want(p.request("POST", "/human/connections/revoke", url.Values{"csrf": {p.csrf}, "connection": {c.CredID}}, ""), 303)
	if _, e = p.s.MCPVerify(context.Background(), raw); e == nil {
		t.Fatal("revoked access accepted")
	}
	if _, e = p.s.MCPVerify(context.Background(), next["access_token"].(string)); e == nil {
		t.Fatal("revoked refreshed access accepted")
	}
	p.want(p.request("POST", "/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {next["refresh_token"].(string)}, "client_id": {p.client}, "resource": {p.s.Origin + "/mcp"}}, ""), 400)
	var status string
	if e = p.s.API.DB.Pool.QueryRow(context.Background(), `SELECT status FROM agents WHERE id=$1`, c.AgentID).Scan(&status); e != nil || status != "active" {
		t.Fatal("revocation damaged agent", e)
	}
}
func TestMCPOAuthExistingAgentNarrowScopesAndLiveOwnership(t *testing.T) {
	p := newMCPFixture(t)
	agent := uuid.NewString()
	if _, e := p.s.API.DB.Pool.Exec(context.Background(), `INSERT INTO agents(id,org_id,handle) VALUES($1,$2,$3)`, agent, p.workspace, "existing-"+agent[:8]); e != nil {
		t.Fatal(e)
	}
	tokens := p.exchange(p.approve(p.begin(), agent, "network:read"))
	raw := tokens["access_token"].(string)
	c, e := p.s.MCPVerify(context.Background(), raw)
	if e != nil || c.AgentID != agent || !slices.Equal(c.Scopes, []string{"network:read"}) {
		t.Fatal("existing agent binding failed", e)
	}
	if _, e = p.s.API.DB.Pool.Exec(context.Background(), `UPDATE organizations SET human_owner_id=NULL WHERE id=$1`, p.workspace); e != nil {
		t.Fatal(e)
	}
	if _, e = p.s.MCPVerify(context.Background(), raw); e == nil {
		t.Fatal("ownership removal accepted")
	}
}
func TestMCPOAuthConsentCSRFWorkspaceAndScopeDenials(t *testing.T) {
	p := newMCPFixture(t)
	h := p.begin()
	p.want(p.request("POST", "/oauth/consent", url.Values{"csrf": {"wrong"}, "consent": {h}, "decision": {"approve"}}, ""), 403)
	p.want(p.approve(h, "", "groups:manage"), 400)
	own := p.workspace
	p.workspace = uuid.NewString()
	p.want(p.approve(h, "", "network:read"), 403)
	p.workspace = own
	// Failed approval leaves consent available for corrected human submission.
	tokens := p.exchange(p.approve(h, "", "network:read"))
	if _, e := p.s.MCPVerify(context.Background(), tokens["access_token"].(string)); e != nil {
		t.Fatal(e)
	}
}
func TestMCPOAuthRefreshReuseRevokesFamily(t *testing.T) {
	p := newMCPFixture(t)
	tokens := p.exchange(p.approve(p.begin(), "", "network:read"))
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tokens["refresh_token"].(string)}, "client_id": {p.client}, "resource": {p.s.Origin + "/mcp"}}
	w := p.request("POST", "/oauth/token", form, "")
	p.want(w, 200)
	next := jsonMap(t, w)
	raw := next["access_token"].(string)
	p.want(p.request("POST", "/oauth/token", form, ""), 400)
	form.Set("refresh_token", next["refresh_token"].(string))
	p.want(p.request("POST", "/oauth/token", form, ""), 400)
	if _, e := p.s.MCPVerify(context.Background(), raw); e == nil {
		t.Fatal("reuse did not revoke family")
	}
}
func TestMCPOAuthMetadata(t *testing.T) {
	p := newMCPFixture(t)
	w := p.request("GET", "/.well-known/oauth-authorization-server", nil, "")
	p.want(w, 200)
	var doc map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &doc); e != nil {
		t.Fatal(e)
	}
	if doc["client_id_metadata_document_supported"] != true {
		t.Fatal("CIMD not advertised")
	}
	p.want(p.request("GET", "/.well-known/oauth-protected-resource/mcp", nil, ""), 200)
}

func TestMCPOAuthConcurrentRefreshHasOneWinnerAndRevokesReuse(t *testing.T) {
	p := newMCPFixture(t)
	tokens := p.exchange(p.approve(p.begin(), "", "network:read"))
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tokens["refresh_token"].(string)}, "client_id": {p.client}, "resource": {p.s.Origin + "/mcp"}}
	results := make(chan *httptest.ResponseRecorder, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			r := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.RemoteAddr = p.ip
			w := httptest.NewRecorder()
			p.m.ServeHTTP(w, r)
			results <- w
		}()
	}
	close(start)
	successes := 0
	var raw, newRefresh string
	for i := 0; i < 2; i++ {
		w := <-results
		switch w.Code {
		case 200:
			successes++
			next := jsonMap(t, w)
			raw = next["access_token"].(string)
			newRefresh = next["refresh_token"].(string)
		case 400:
		default:
			t.Fatalf("unexpected refresh status %d: %s", w.Code, w.Body)
		}
	}
	if successes != 1 {
		t.Fatalf("refresh winners=%d", successes)
	}
	if _, e := p.s.MCPVerify(context.Background(), raw); e == nil {
		t.Fatal("concurrent reuse did not revoke winner family")
	}
	form.Set("refresh_token", newRefresh)
	p.want(p.request("POST", "/oauth/token", form, ""), 400)
}

func TestMCPOAuthLoginHandoff(t *testing.T) {
	p := newMCPFixture(t)
	session := p.cookies["ajent_human"]
	delete(p.cookies, "ajent_human")
	sum := sha256.Sum256([]byte(p.verifier))
	q := url.Values{"client_id": {p.client}, "redirect_uri": {"https://client.example/callback"}, "response_type": {"code"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "resource": {p.s.Origin + "/mcp"}, "scope": {"network:read"}}
	w := p.request("GET", "/oauth/authorize?"+q.Encode(), nil, "")
	p.want(w, 303)
	if w.Header().Get("Location") != "/human/login" {
		t.Fatal("missing login handoff")
	}
	pending := p.cookies["ajent_mcp_resume"]
	if pending == nil || !pending.HttpOnly || !pending.Secure || pending.SameSite != http.SameSiteLaxMode {
		t.Fatal("unsafe handoff cookie")
	}
	p.cookies["ajent_human"] = session
	w = p.request("GET", "/human/workspaces", nil, "")
	p.want(w, 303)
	if !strings.HasPrefix(w.Header().Get("Location"), "/oauth/authorize?consent=") {
		t.Fatal("lost pending consent")
	}
	p.want(p.request("GET", w.Header().Get("Location"), nil, ""), 200)
	if p.cookies["ajent_mcp_resume"] != nil {
		t.Fatal("pending cookie not cleared")
	}
}

// Register the actual web routes first, including method-qualified catch-all GET /.
// A methodless specific OAuth route overlaps that pattern and panics at startup.
func TestMCPOAuthRoutesCoexistWithRoot(t *testing.T) {
	s, e := New(api.New(nil), true, "https://ajent.example", "unused", false)
	if e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	if e = s.MCPRoutes(mux); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/oauth/authorize", nil))
	if w.Code != 405 {
		t.Fatalf("POST authorize: %d", w.Code)
	}
	for _, path := range []string{"/oauth/token", "/oauth/register", "/oauth/revoke", "/oauth/consent", "/human/connections/revoke"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("PUT", path, nil))
		if w.Code != 405 {
			t.Fatalf("PUT %s: %d", path, w.Code)
		}
	}
}

type fixtureCIMDResolver struct{ id string }

func (r fixtureCIMDResolver) Resolve(_ context.Context, id string) (protocol.Client, error) {
	if id != r.id {
		return protocol.Client{}, protocol.ErrNotFound
	}
	return protocol.Client{ID: id, Name: "CIMD connector", RedirectURIs: []string{"https://client.example/callback"}, CreatedAt: time.Now()}, nil
}
func TestMCPOAuthCIMDHTTPRoundTripAndRevocation(t *testing.T) {
	previous := mcpMetadata
	mcpMetadata = fixtureCIMDResolver{id: "https://client.example/oauth.json"}
	t.Cleanup(func() { mcpMetadata = previous })
	p := newMCPFixture(t)
	p.client = "https://client.example/oauth.json"
	p.m.(*http.ServeMux).Handle("POST /mcp", mcphttp.New(p.s.API, p.s, p.s.Origin))
	wrong := url.Values{"client_id": {p.client}, "redirect_uri": {"https://wrong.example/callback"}}
	p.want(p.request("GET", "/oauth/authorize?"+wrong.Encode(), nil, ""), 400)
	tokens := p.exchange(p.approve(p.begin(), "", "network:read"))
	raw := tokens["access_token"].(string)
	c, e := p.s.MCPVerify(context.Background(), raw)
	if e != nil {
		t.Fatal(e)
	}
	invoke := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ajent_me","arguments":{}}}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("MCP-Protocol-Version", "2025-11-25")
		w := httptest.NewRecorder()
		p.m.ServeHTTP(w, r)
		return w
	}
	w := invoke(raw)
	p.want(w, 200)
	if !strings.Contains(w.Body.String(), c.AgentID) {
		t.Fatalf("HTTP MCP lost selected identity: %s", w.Body)
	}
	refresh := p.request("POST", "/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {tokens["refresh_token"].(string)}, "client_id": {p.client}, "resource": {p.s.Origin + "/mcp"}}, "")
	p.want(refresh, 200)
	next := jsonMap(t, refresh)
	p.want(invoke(next["access_token"].(string)), 200)
	var registered int
	if e = p.s.API.DB.Pool.QueryRow(context.Background(), `SELECT count(*) FROM mcp_oauth_records WHERE kind='clients' AND id=$1`, p.client).Scan(&registered); e != nil || registered != 0 {
		t.Fatal("CIMD created DCR state", e)
	}
	p.want(p.request("POST", "/human/connections/revoke", url.Values{"csrf": {p.csrf}, "connection": {c.CredID}}, ""), 303)
	// The resolver still returns this same trusted client: metadata is never
	// a substitute for the current human/workspace/connection grant.
	p.want(invoke(raw), 401)
	p.want(invoke(next["access_token"].(string)), 401)
	p.want(p.request("POST", "/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {next["refresh_token"].(string)}, "client_id": {p.client}, "resource": {p.s.Origin + "/mcp"}}, ""), 400)
}

func TestMCPOAuthAgedDCRClientsSurviveLiveReferences(t *testing.T) {
	p := newMCPFixture(t)
	h := p.begin()
	ctx := context.Background()
	age := func() {
		if _, e := p.s.API.DB.Pool.Exec(ctx, `UPDATE mcp_oauth_records SET expires_at=now()-interval '1 day' WHERE kind='clients' AND id=$1`, p.client); e != nil {
			t.Fatal(e)
		}
	}
	assertRegistered := func() {
		var n int
		if e := p.s.API.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM mcp_oauth_records WHERE kind='clients' AND id=$1`, p.client).Scan(&n); e != nil || n != 1 {
			t.Fatal("live referenced client removed", e)
		}
	}
	_, st, e := p.s.mcpServer(nil)
	if e != nil {
		t.Fatal(e)
	}
	cleanup := func() {
		if e := st.CreateClient(ctx, protocol.Client{ID: uuid.NewString(), Name: "unrelated", RedirectURIs: []string{"https://client.example/other"}, CreatedAt: time.Now()}); e != nil {
			t.Fatal(e)
		}
	}
	// An unrelated registration runs cleanup while this client has live consent.
	age()
	cleanup()
	assertRegistered()
	tokens := p.exchange(p.approve(h, "", "network:read"))
	age()
	cleanup()
	assertRegistered()
	if _, e = p.s.MCPVerify(ctx, tokens["access_token"].(string)); e != nil {
		t.Fatal(e)
	}
	// Even an aged unreferenced registration is renewed on resolution before
	// CreateConsent's cleanup; a fresh authorization remains usable.
	other := newMCPFixture(t)
	if _, e = other.s.API.DB.Pool.Exec(ctx, `UPDATE mcp_oauth_records SET expires_at=now()-interval '1 day' WHERE kind='clients' AND id=$1`, other.client); e != nil {
		t.Fatal(e)
	}
	other.begin()
	var count int
	if e = other.s.API.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM mcp_oauth_records WHERE kind='clients' AND id=$1`, other.client).Scan(&count); e != nil || count != 1 {
		t.Fatal("fresh consent retired aged client", e)
	}
}

var _ oauth.ClientMetadataResolver = fixtureCIMDResolver{}

func TestMCPOAuthConsentCallbackCSP(t *testing.T) {
	p := newMCPFixture(t)
	baseline := "default-src 'self'; script-src 'self'; object-src 'none'; frame-ancestors 'none'; form-action 'self'"
	original := p.m
	p.m = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", baseline)
		original.ServeHTTP(w, r)
	})
	h := p.begin()
	// Resume request and submitted form cannot inject an alternate callback.
	get := p.request("GET", "/oauth/authorize?consent="+url.QueryEscape(h)+"&redirect_uri=https%3A%2F%2Fevil.example%2Fcallback", nil, "")
	p.want(get, 200)
	check := func(w *httptest.ResponseRecorder) {
		policy := w.Header().Get("Content-Security-Policy")
		for _, want := range []string{"default-src 'self'", "script-src 'self'", "object-src 'none'", "frame-ancestors 'none'", "form-action 'self' https://client.example"} {
			if !strings.Contains(policy, want) {
				t.Fatalf("CSP lost %s: %s", want, policy)
			}
		}
		if strings.Contains(policy, "evil.example") {
			t.Fatal("unvalidated callback in CSP")
		}
	}
	check(get)
	form := url.Values{"csrf": {p.csrf}, "consent": {h}, "decision": {"approve"}, "workspace": {p.workspace}, "agent": {""}, "handle": {"muse-" + uuid.NewString()[:8]}, "scope": {"network:read"}, "redirect_uri": {"https://evil.example/callback"}}
	approved := p.request("POST", "/oauth/consent", form, "")
	p.want(approved, 303)
	check(approved)
	cancelHandle := p.begin()
	cancel := p.request("POST", "/oauth/consent", url.Values{"csrf": {p.csrf}, "consent": {cancelHandle}, "decision": {"deny"}, "redirect_uri": {"https://evil.example/callback"}}, "")
	p.want(cancel, 303)
	check(cancel)
}

// Every credential counted by consent admission must remain discoverable and
// revocable, even if code exchange never finishes or its grant is later ended.
func TestMCPOAuthInactiveConnectionsRemainRevocable(t *testing.T) {
	for _, state := range []string{"unexchanged", "protocol_revoked", "inactive_agent", "transferred_workspace", "unowned_workspace"} {
		t.Run(state, func(t *testing.T) {
			p := newMCPFixture(t)
			approved := p.approve(p.begin(), "", "network:read")
			p.want(approved, http.StatusSeeOther)
			ctx := context.Background()
			var credential, agent string
			if err := p.s.API.DB.Pool.QueryRow(ctx, `SELECT b.credential_id::text,c.agent_id::text FROM mcp_oauth_bindings b JOIN credentials c ON c.id=b.credential_id WHERE b.human_id=$1`, p.human).Scan(&credential, &agent); err != nil {
				t.Fatal(err)
			}
			if state != "unexchanged" {
				tokens := p.exchange(approved)
				if state == "protocol_revoked" {
					p.want(p.request("POST", "/oauth/revoke", url.Values{"client_id": {p.client}, "token": {tokens["access_token"].(string)}}, ""), http.StatusOK)
				} else if state == "inactive_agent" {
					if _, err := p.s.API.DB.Pool.Exec(ctx, `UPDATE agents SET status='suspended' WHERE id=$1`, agent); err != nil {
						t.Fatal(err)
					}
				}
			}
			ownershipLost := state == "transferred_workspace" || state == "unowned_workspace"
			const renamedWorkspace = "New owner private workspace"
			const renamedAgent = "new-owner-private-agent"
			if ownershipLost {
				var owner any
				if state == "transferred_workspace" {
					owner = uuid.NewString()
					if _, err := p.s.API.DB.Pool.Exec(ctx, `INSERT INTO humans(id,name) VALUES($1,'New synthetic owner')`, owner); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := p.s.API.DB.Pool.Exec(ctx, `UPDATE organizations SET human_owner_id=$1 WHERE id=$2`, owner, p.workspace); err != nil {
					t.Fatal(err)
				}
				if _, err := p.s.API.DB.Pool.Exec(ctx, `UPDATE organizations SET name=$1 WHERE id=$2`, renamedWorkspace, p.workspace); err != nil {
					t.Fatal(err)
				}
				if _, err := p.s.API.DB.Pool.Exec(ctx, `UPDATE agents SET handle=$1 WHERE id=$2`, renamedAgent, agent); err != nil {
					t.Fatal(err)
				}
			}
			var counted int
			count := func() int {
				t.Helper()
				if err := p.s.API.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM mcp_oauth_bindings b JOIN credentials c ON c.id=b.credential_id WHERE b.human_id=$1 AND c.revoked_at IS NULL AND c.expires_at>now()`, p.human).Scan(&counted); err != nil {
					t.Fatal(err)
				}
				return counted
			}
			if count() != 1 {
				t.Fatal("connection was not counted by consent admission")
			}
			page := p.request("GET", "/human/connections", nil, "")
			p.want(page, http.StatusOK)
			if !strings.Contains(page.Body.String(), credential) || !strings.Contains(page.Body.String(), "Not connected") {
				t.Fatal("counted connection is missing from management page or mislabeled")
			}
			if ownershipLost {
				body := page.Body.String()
				if strings.Contains(body, renamedWorkspace) || strings.Contains(body, renamedAgent) {
					t.Fatal("connection management disclosed post-transfer tenant names")
				}
				if !strings.Contains(body, "Unavailable agent") || !strings.Contains(body, "Unavailable workspace") {
					t.Fatal("connection management omitted neutral unavailable labels")
				}
			}
			p.want(p.request("POST", "/human/connections/revoke", url.Values{"csrf": {p.csrf}, "connection": {credential}}, ""), http.StatusSeeOther)
			if count() != 0 {
				t.Fatal("disconnect did not reclaim connection quota")
			}
			var agents int
			if err := p.s.API.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM agents WHERE id=$1`, agent).Scan(&agents); err != nil || agents != 1 {
				t.Fatalf("disconnect did not preserve agent: count=%d err=%v", agents, err)
			}
		})
	}
}
