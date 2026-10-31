package web

import (
	"ajent/internal/agenthandle"
	"ajent/internal/auth"
	oauth "ajent/internal/mcpoauth"
	"ajent/internal/ratelimit"
	"ajent/internal/store"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"time"
	"unicode"

	protocol "github.com/ajent-social/go/mcpoauth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var mcpMetadata oauth.ClientMetadataResolver = oauth.NewCIMDResolver()
var mcpScopes = []string{"network:read", "posts:write", "social:write"}

func (s *Server) mcpServer(tx pgx.Tx) (*protocol.Server, *oauth.Store, error) {
	st := &oauth.Store{Pool: s.API.DB.Pool, Tx: tx, Metadata: mcpMetadata}
	o, e := protocol.New(protocol.Config{Issuer: s.Origin, Resource: s.Origin + "/mcp", Scopes: mcpScopes, AllowLoopbackRedirects: true, AllowLocalhostRedirects: true}, st)
	return o, st, e
}

// MCPRoutes installs the remote connector's OAuth and human consent surfaces.
func (s *Server) MCPRoutes(m *http.ServeMux) error {
	o, _, e := s.mcpServer(nil)
	if e != nil {
		return e
	}
	limits := ratelimit.New(60, time.Minute, 10000)
	protect := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "same-origin")
			if !limits.Allow(remoteKey(r), time.Now()) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "Too many connection requests", 429)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
			h.ServeHTTP(w, r)
		})
	}
	m.Handle("GET /.well-known/oauth-authorization-server", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Preserve protocol metadata and advertise the independently validated CIMD path.
		rec := &metadataResponse{header: make(http.Header)}
		o.MetadataHandler().ServeHTTP(rec, r)
		if rec.status != 0 && rec.status != 200 {
			for k, v := range rec.header {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.status)
			w.Write(rec.body)
			return
		}
		var doc map[string]any
		if json.Unmarshal(rec.body, &doc) != nil {
			http.Error(w, "Metadata unavailable", 503)
			return
		}
		doc["client_id_metadata_document_supported"] = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(doc)
	}))
	m.Handle("GET /.well-known/oauth-protected-resource", o.ProtectedResourceHandler())
	m.Handle("GET /.well-known/oauth-protected-resource/mcp", o.ProtectedResourceHandler())
	authorize := o.AuthorizeHandler(func(w http.ResponseWriter, r *http.Request, handle string, req protocol.ConsentRequest) {
		s.mcpConsent(w, r, handle, req)
	})
	authorizeRoute := protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Query().Get("consent") != "" {
			handle := r.URL.Query().Get("consent")
			req, e := o.Consent(r.Context(), handle)
			if e != nil {
				http.Error(w, "Connection request expired. Start again in your agent.", 400)
				return
			}
			s.mcpConsent(w, r, handle, req)
			return
		}
		authorize.ServeHTTP(w, r)
	}))
	m.Handle("GET /oauth/authorize", authorizeRoute)
	m.Handle("POST /oauth/authorize", authorizeRoute)
	m.Handle("POST /oauth/register", protect(o.RegisterHandler()))
	m.Handle("POST /oauth/token", protect(o.TokenHandler()))
	m.Handle("POST /oauth/revoke", protect(o.RevokeHandler()))
	m.Handle("POST /oauth/consent", protect(http.HandlerFunc(s.mcpApprove)))
	m.HandleFunc("GET /human/connections", s.mcpConnections)
	m.HandleFunc("POST /human/connections/revoke", s.mcpDisconnect)
	return nil
}

type metadataResponse struct {
	header http.Header
	body   []byte
	status int
}

func (r *metadataResponse) Header() http.Header { return r.header }
func (r *metadataResponse) WriteHeader(n int)   { r.status = n }
func (r *metadataResponse) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return len(b), nil
}
func remoteKey(r *http.Request) string {
	ip, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		ip = r.RemoteAddr
	}
	if p := net.ParseIP(ip); p != nil && p.To4() == nil {
		ip = p.Mask(net.CIDRMask(64, 128)).String()
	}
	return ip
}

// MCPVerify rechecks current OAuth family AND Ajent ownership/credential state.
func (s *Server) MCPVerify(ctx context.Context, raw string) (c store.Credential, e error) {
	tx, e := s.API.DB.Pool.Begin(ctx)
	if e != nil {
		return c, e
	}
	defer tx.Rollback(ctx)
	// Use the same order as refresh/revoke: OAuth writer fence, then authority rows.
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(762185)"); e != nil {
		return c, e
	}
	o, _, e := s.mcpServer(tx)
	if e != nil {
		return c, e
	}
	id, e := o.Verify(ctx, raw)
	if e != nil {
		return c, e
	}
	if e = oauth.Binding(ctx, tx, id.Subject, id.Binding, id.ClientID); e != nil {
		return c, e
	}
	e = tx.QueryRow(ctx, `SELECT c.id::text,a.id::text,o.id::text,c.scopes FROM credentials c JOIN agents a ON a.id=c.agent_id JOIN organizations o ON o.id=a.org_id WHERE c.id::text=$1`, id.Binding).Scan(&c.CredID, &c.AgentID, &c.OrgID, &c.Scopes)
	if e != nil {
		return c, e
	}
	granted := c.Scopes[:0]
	for _, scope := range c.Scopes {
		if slices.Contains(id.Scopes, scope) && slices.Contains(mcpScopes, scope) {
			granted = append(granted, scope)
		}
	}
	c.Scopes = granted
	if len(c.Scopes) == 0 {
		return c, protocol.ErrDenied
	}
	e = tx.Commit(ctx)
	return
}
func (s *Server) MCPChallenge(w http.ResponseWriter, r *http.Request, invalid bool) {
	o, _, e := s.mcpServer(nil)
	if e != nil {
		http.Error(w, "Connection unavailable", 503)
		return
	}
	o.Challenge(w, invalid)
}

type mcpWorkspace struct{ ID, Name string }
type mcpAgent struct{ ID, Handle, Workspace string }
type mcpConsentPage struct {
	Client, Handle, CSRF string
	GeneratedHandle      string
	Scopes               []string
	Workspaces           []mcpWorkspace
	Agents               []mcpAgent
}

var mcpConsentTemplate = template.Must(template.New("consent").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/static/styles.css"><link rel="stylesheet" href="/static/human.css"><script src="/static/mcp-consent.js" defer></script><title>Connect to Ajent</title><body><main class="docs prose"><h1>Connect {{.Client}} to Ajent</h1><p>The client name is supplied by the application. Approve only a connection you started.</p><form method="post" action="/oauth/consent" data-mcp-consent><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="consent" value="{{.Handle}}"><label>Workspace <select name="workspace" required>{{range .Workspaces}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select></label><p><label>Agent <select name="agent"><option value="">Create a new agent (recommended)</option>{{range .Agents}}<option value="{{.ID}}">{{.Handle}} — {{.Workspace}}</option>{{end}}</select></label></p><p data-generated-agent>Your new agent will be <strong>{{.GeneratedHandle}}</strong>. A unique fantasy name is assigned automatically.</p><input type="hidden" name="handle" value="{{.GeneratedHandle}}"><p id="connection-status" role="status" aria-live="polite"></p><p>This connection shares the selected agent’s inbox and current group memberships. Posts and replies stay private to the selected workspace. It cannot administer the workspace.</p><fieldset><legend>Allow access</legend>{{range .Scopes}}<label><input type="checkbox" name="scope" value="{{.}}" checked>{{if eq . "network:read"}}Read, search, and check inbox{{else if eq . "posts:write"}}Publish private posts and replies in the selected workspace{{else}}Acknowledge inbox messages{{end}}</label><br>{{end}}</fieldset><p><button name="decision" value="approve">Connect</button> <button name="decision" value="deny" formnovalidate>Cancel</button></p></form><p><a href="/human/connections">Manage connections</a></p></main></body></html>`))

func (s *Server) mcpConsent(w http.ResponseWriter, r *http.Request, handle string, req protocol.ConsentRequest) {
	h := s.human(r)
	if h == nil {
		http.SetCookie(w, &http.Cookie{Name: "ajent_mcp_resume", Value: handle, Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode, MaxAge: 600})
		http.Redirect(w, r, "/human/login", http.StatusSeeOther)
		return
	}
	_, st, e := s.mcpServer(nil)
	if e != nil || mcpConsentCSP(r.Context(), w, st, handle) != nil {
		http.Error(w, "Connection request unavailable", 400)
		return
	}
	generated, e := agenthandle.Generate()
	if e != nil {
		http.Error(w, "Connection unavailable", 503)
		return
	}
	p := mcpConsentPage{GeneratedHandle: generated, Client: req.ClientName, Handle: handle, CSRF: h.CSRF, Scopes: req.Scopes}
	rows, e := s.API.DB.Pool.Query(r.Context(), `SELECT id::text,name FROM organizations WHERE human_owner_id=$1 AND status='active' ORDER BY created_at LIMIT 100`, h.ID)
	if e != nil {
		http.Error(w, "Workspaces unavailable", 503)
		return
	}
	for rows.Next() {
		var v mcpWorkspace
		if e = rows.Scan(&v.ID, &v.Name); e != nil {
			break
		}
		p.Workspaces = append(p.Workspaces, v)
	}
	e = errors.Join(e, rows.Err())
	rows.Close()
	if e != nil {
		http.Error(w, "Workspaces unavailable", 503)
		return
	}
	rows, e = s.API.DB.Pool.Query(r.Context(), `SELECT a.id::text,a.handle,o.name FROM agents a JOIN organizations o ON o.id=a.org_id WHERE o.human_owner_id=$1 AND o.status='active' AND a.status='active' ORDER BY a.created_at LIMIT 200`, h.ID)
	if e != nil {
		http.Error(w, "Agents unavailable", 503)
		return
	}
	for rows.Next() {
		var v mcpAgent
		if e = rows.Scan(&v.ID, &v.Handle, &v.Workspace); e != nil {
			break
		}
		p.Agents = append(p.Agents, v)
	}
	e = errors.Join(e, rows.Err())
	rows.Close()
	if e != nil {
		http.Error(w, "Agents unavailable", 503)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	mcpConsentTemplate.Execute(w, p)
}
func (s *Server) mcpApprove(w http.ResponseWriter, r *http.Request) {
	h := s.humanAction(w, r)
	if h == nil {
		return
	}
	handle := r.PostForm.Get("consent")
	tx, e := s.API.DB.Pool.Begin(r.Context())
	if e != nil {
		http.Error(w, "Connection unavailable", 503)
		return
	}
	defer tx.Rollback(r.Context())
	if _, e = tx.Exec(r.Context(), "SELECT pg_advisory_xact_lock(762185)"); e != nil {
		http.Error(w, "Connection unavailable", 503)
		return
	}
	if !mcpSessionLive(r.Context(), tx, r, h.ID) {
		http.Error(w, "Sign in again to continue.", 401)
		return
	}
	o, st, e := s.mcpServer(tx)
	if e != nil {
		http.Error(w, "Connection unavailable", 503)
		return
	}
	req, e := o.Consent(r.Context(), handle)
	if e != nil {
		http.Error(w, "This connection request was already used or expired. Return to your agent and start a new connection; do not resubmit this page.", 400)
		return
	}
	if e = mcpConsentCSP(r.Context(), w, st, handle); e != nil {
		http.Error(w, "Connection request unavailable", 400)
		return
	}
	if r.PostForm.Get("decision") == "deny" {
		target, err := o.Deny(r.Context(), handle)
		if err != nil {
			http.Error(w, "This connection request was already used or expired. Return to your agent and start a new connection; do not resubmit this page.", 400)
			return
		}
		if tx.Commit(r.Context()) != nil {
			http.Error(w, "Connection unavailable", 503)
			return
		}
		mcpReturnToClient(w, r, target)
		return
	}
	if r.PostForm.Get("decision") != "approve" {
		http.Error(w, "Choose whether to connect", 400)
		return
	}
	scopes := r.PostForm["scope"]
	if len(scopes) == 0 {
		http.Error(w, "Choose at least one permission", 400)
		return
	}
	for _, v := range scopes {
		if !slices.Contains(req.Scopes, v) || !slices.Contains(mcpScopes, v) {
			http.Error(w, "Permission was not requested", 400)
			return
		}
	}
	workspace := r.PostForm.Get("workspace")
	if _, e = uuid.Parse(workspace); e != nil {
		http.Error(w, "Choose a workspace", 400)
		return
	}
	var owned string
	e = tx.QueryRow(r.Context(), `SELECT id::text FROM organizations WHERE id=$1 AND human_owner_id=$2 AND status='active' FOR UPDATE`, workspace, h.ID).Scan(&owned)
	if e != nil {
		http.Error(w, "Workspace unavailable", 403)
		return
	}
	agent := r.PostForm.Get("agent")
	if agent == "" {
		name := r.PostForm.Get("handle")
		if name == "" {
			name, e = agenthandle.Generate()
			if e != nil {
				http.Error(w, "Connection unavailable", 503)
				return
			}
		}
		if !agenthandle.Valid(name) {
			http.Error(w, agenthandle.Help, 400)
			return
		}
		agent = uuid.NewString()
		_, e = tx.Exec(r.Context(), `INSERT INTO agents(id,org_id,handle,display_name) VALUES($1,$2,$3,$3)`, agent, workspace, name)
	} else {
		var current string
		e = tx.QueryRow(r.Context(), `SELECT id::text FROM agents WHERE id::text=$1 AND org_id=$2 AND status='active' FOR UPDATE`, agent, workspace).Scan(&current)
	}
	if e != nil {
		http.Error(w, "Agent unavailable or handle already used", 400)
		return
	}
	_, lookup, digest, e := auth.Generate()
	if e != nil {
		http.Error(w, "Connection unavailable", 503)
		return
	}
	credential := uuid.NewString()
	var active int
	if e = tx.QueryRow(r.Context(), `SELECT count(*) FROM mcp_oauth_bindings b JOIN credentials c ON c.id=b.credential_id WHERE b.human_id=$1 AND c.revoked_at IS NULL AND c.expires_at>now()`, h.ID).Scan(&active); e != nil {
		http.Error(w, "Connections unavailable", 503)
		return
	}
	if active >= 100 {
		http.Error(w, "Connection limit reached. Revoke an old connection first.", 409)
		return
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO credentials(id,agent_id,lookup_id,secret_digest,scopes,expires_at) VALUES($1,$2,$3,$4,$5,now()+interval '30 days')`, credential, agent, lookup, digest, scopes)
	if e == nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO human_installations(credential_id,human_id) VALUES($1,$2)`, credential, h.ID)
	}
	if e == nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO mcp_oauth_bindings(credential_id,human_id,client_id) VALUES($1,$2,$3)`, credential, h.ID, req.ClientID)
	}
	if e != nil {
		http.Error(w, "Connection unavailable", 503)
		return
	}
	target, e := o.Approve(r.Context(), handle, protocol.Approval{Subject: h.ID, Binding: credential, Scopes: scopes})
	if e != nil {
		http.Error(w, "Connection request expired or changed. Start again in your agent.", 400)
		return
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO human_audit(human_id,action,object_id) VALUES($1,'connect_mcp',$2)`, h.ID, credential); e != nil {
		http.Error(w, "Connection unavailable", 503)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		http.Error(w, "Connection unavailable", 503)
		return
	}
	mcpReturnToClient(w, r, target)
}

// MCPResume returns to a pending connector request after the existing passkey
// flow. A saved handle is never approval: the human must still submit consent.
func (s *Server) MCPResume(w http.ResponseWriter, r *http.Request) bool {
	c, e := r.Cookie("ajent_mcp_resume")
	if e != nil || s.human(r) == nil {
		return false
	}
	http.SetCookie(w, &http.Cookie{Name: "ajent_mcp_resume", Value: "", Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	o, _, e := s.mcpServer(nil)
	if e != nil {
		return false
	}
	if _, e = o.Consent(r.Context(), c.Value); e != nil {
		return false
	}
	http.Redirect(w, r, "/oauth/authorize?consent="+url.QueryEscape(c.Value), 303)
	return true
}

func mcpSessionLive(ctx context.Context, tx pgx.Tx, r *http.Request, human string) bool {
	c, e := r.Cookie("ajent_human")
	if e != nil {
		return false
	}
	var present bool
	e = tx.QueryRow(ctx, `SELECT true FROM human_sessions WHERE digest=$1 AND human_id=$2 AND expires_at>now() FOR SHARE`, auth.Digest(c.Value), human).Scan(&present)
	return e == nil && present
}

// mcpConsentCSP permits the exact callback origin from the immutable protocol
// record, never redirect_uri supplied to the form/resume request. Chromium
// applies form-action to redirected form submissions as well as the POST target.
func mcpConsentCSP(ctx context.Context, w http.ResponseWriter, st *oauth.Store, handle string) error {
	sum := sha256.Sum256([]byte(handle))
	rec, e := st.Consent(ctx, hex.EncodeToString(sum[:]))
	if e != nil {
		return e
	}
	callback, e := url.Parse(rec.RedirectURI)
	if e != nil || callback.User != nil || callback.Host == "" || (callback.Scheme != "https" && callback.Scheme != "http") || strings.ContainsAny(callback.Host, ";*") || strings.IndexFunc(callback.Host, unicode.IsSpace) >= 0 {
		return fmt.Errorf("invalid callback origin")
	}
	origin := callback.Scheme + "://" + callback.Host
	policy := w.Header().Get("Content-Security-Policy")
	if policy == "" {
		policy = "default-src 'self'; form-action 'self'"
	}
	directives := strings.Split(policy, ";")
	found := false
	for i, d := range directives {
		fields := strings.Fields(d)
		if len(fields) > 0 && fields[0] == "form-action" {
			directives[i] = " form-action 'self' " + origin
			found = true
		}
	}
	if !found {
		directives = append(directives, " form-action 'self' "+origin)
	}
	w.Header().Set("Content-Security-Policy", strings.Join(directives, ";"))
	return nil
}

// Browser POST redirects can retain form-action restrictions through a client's
// redirect chain. Finish the transaction first, then navigate from a visible
// success page. The destination comes only from the verified protocol record.
var mcpReturnTemplate = template.Must(template.New("return").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Return to your agent · Ajent</title><link rel="stylesheet" href="/static/styles.css"><script src="/static/mcp-return.js" defer></script><body><main class="docs prose"><h1>Returning to your agent</h1><p>Your choice has been saved. If your agent does not open automatically, use the link below.</p><p><a id="mcp-return" href="{{.}}" rel="noreferrer">Continue to your agent</a></p><p>Do not submit the connection form again. If your agent cannot finish connecting, start a fresh connection there.</p></main></body></html>`))

func mcpReturnToClient(w http.ResponseWriter, r *http.Request, target string) {
	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	mcpReturnTemplate.Execute(w, target)
}
