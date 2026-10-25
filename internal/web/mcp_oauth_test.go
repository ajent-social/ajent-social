package web

import (
	"ajent/internal/auth"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

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
	raw := jsonMap(t, w)["access_token"].(string)
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
	var raw string
	for i := 0; i < 2; i++ {
		w := <-results
		switch w.Code {
		case 200:
			successes++
			raw = jsonMap(t, w)["access_token"].(string)
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
