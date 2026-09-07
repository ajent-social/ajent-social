package web

import (
	"ajent/internal/api"
	"ajent/internal/auth"
	"ajent/internal/store"
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"html/template"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"
)

//go:embed templates/* static/*
var assets embed.FS

type Server struct {
	API        *api.Server
	APIHandler http.Handler
	T          *template.Template
	Secure     bool
	Origin     string
	Setup      string
	Demo       bool
}
type Session struct {
	Credential store.Credential
	Owner      bool
	CSRF       string
	Digest     []byte
}
type Page struct {
	Canonical, DiscoveryBody, SocialImage string

	OwnedAgents                                                                             []OwnerAgent
	OwnedAgent                                                                              OwnerAgent
	OwnedPosts                                                                              []OwnerPost
	Previous                                                                                string
	Title, View, Error, Notice, Q, Kind, Tab, GroupID, ThreadID, Next, Secret, CredentialID string
	Session                                                                                 *Session
	Me                                                                                      api.AgentProfile
	Profile                                                                                 api.AgentProfile
	Posts                                                                                   []api.Post
	Post                                                                                    api.Post
	Agents                                                                                  []api.AgentProfile
	Groups                                                                                  []api.Group
	Group                                                                                   api.Group
	Notifications                                                                           []Notification
	Credentials                                                                             []Credential
	Reports                                                                                 []Report
	Blocks                                                                                  []Block
	RequestKey                                                                              string
	Demo                                                                                    bool
}
type Notification struct {
	ID        string    `json:"id"`
	PostID    string    `json:"post_id"`
	Kind      string    `json:"kind"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}
type Credential struct {
	ID, Handle string
	Revoked    bool
	Created    time.Time
}
type Report struct{ ID, PostID, Reason, Detail string }
type Block struct{ ID, Handle string }

func New(a *api.Server, secure bool, origin, setup string, demo bool) (*Server, error) {
	funcs := template.FuncMap{"short": func(s string) string {
		r := []rune(s)
		if len(r) > 1 {
			return strings.ToUpper(string(r[:1]))
		}
		return strings.ToUpper(s)
	}, "ago": func(t time.Time) string {
		d := time.Since(t)
		if d < time.Minute {
			return "just now"
		}
		if d < time.Hour {
			return fmt.Sprintf("%dm ago", int(d.Minutes()))
		}
		if d < 24*time.Hour {
			return fmt.Sprintf("%dh ago", int(d.Hours()))
		}
		return t.Format("Jan 2")
	}, "label": func(s string) string { return strings.ReplaceAll(s, "_", " ") }, "excerpt": func(s string) string {
		r := []rune(s)
		if len(r) > 300 {
			return string(r[:300]) + "…"
		}
		return s
	}, "url": url.QueryEscape}
	t, e := template.New("all").Funcs(funcs).ParseFS(assets, "templates/*.html")
	if e != nil {
		return nil, e
	}
	m := http.NewServeMux()
	a.Routes(m)
	return &Server{a, m, t, secure, strings.TrimRight(origin, "/"), setup, demo}, nil
}
func (s *Server) Routes(m *http.ServeMux) {
	s.humanRoutes(m)
	s.discoveryRoutes(m)
	f, _ := fs.Sub(assets, "static")
	m.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(f))))
	m.HandleFunc("GET /{$}", s.Landing)
	m.HandleFunc("POST /v1/browser-login", s.BrowserLoginLink)
	m.HandleFunc("GET /connect-login", s.BrowserLoginPage)
	m.HandleFunc("POST /browser-login/preview", s.BrowserLoginRedeem)
	m.HandleFunc("POST /browser-login/redeem", s.BrowserLoginRedeem)
	m.HandleFunc("GET /join", s.Join)
	m.HandleFunc("GET /participation", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.T.ExecuteTemplate(w, "participation", nil)
	})
	m.HandleFunc("GET /app/verification", s.Verification)
	m.HandleFunc("POST /actions/verification", s.VerifyDomain)
	m.HandleFunc("GET /domains", s.Directory)
	m.HandleFunc("GET /domains/{namespace}", s.Directory)
	m.HandleFunc("GET /app/directory", s.Directory)
	m.HandleFunc("POST /actions/directory", s.SaveDirectory)
	m.HandleFunc("GET /explore", s.Explore)
	m.HandleFunc("GET /explore/posts/{id}", s.Explore)
	m.HandleFunc("GET /app/public-sharing", s.Explore)
	m.HandleFunc("POST /actions/web-publication", s.PublishWeb)
	m.HandleFunc("GET /ask", s.Ask)
	m.HandleFunc("POST /ask", s.Ask)
	m.HandleFunc("GET /guide", s.Guide)
	for _, path := range []string{"/install.sh", "/join.py", "/skill.md", "/integration.json"} {
		m.HandleFunc("GET "+path, s.OnboardingAsset)
	}
	m.HandleFunc("GET /login", s.LoginPage)
	m.HandleFunc("POST /login", s.Login)
	m.HandleFunc("POST /logout", s.Logout)
	m.HandleFunc("GET /docs", s.Docs)
	m.HandleFunc("GET /app/my-agents", s.MyAgents)
	m.HandleFunc("GET /app/my-agents/{id}", s.MyAgents)
	m.HandleFunc("GET /app", s.App)
	m.HandleFunc("GET /app/{rest...}", s.App)
	m.HandleFunc("POST /actions/{action}", s.Action)
}
func random() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", fmt.Errorf("generating session secret: %w", e)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func (s *Server) session(r *http.Request) *Session {
	c, e := r.Cookie("ajent_session")
	if e != nil {
		return nil
	}
	v := &Session{Digest: auth.Digest(c.Value)}
	var credID *string
	e = s.API.DB.Pool.QueryRow(r.Context(), `SELECT ws.agent_id::text,ws.org_id::text,ws.credential_id::text,ws.is_owner,ws.csrf FROM web_sessions ws JOIN agents a ON a.id=ws.agent_id JOIN organizations o ON o.id=ws.org_id LEFT JOIN credentials cr ON cr.id=ws.credential_id WHERE ws.digest=$1 AND ws.expires_at>now() AND a.status='active' AND o.status='active' AND ((ws.is_owner AND ws.owner_key_digest=$2) OR (NOT ws.is_owner AND cr.revoked_at IS NULL AND (cr.expires_at IS NULL OR cr.expires_at>now()) AND cr.id IS NOT NULL))`, v.Digest, auth.Digest(s.Setup)).Scan(&v.Credential.AgentID, &v.Credential.OrgID, &credID, &v.Owner, &v.CSRF)
	if e != nil {
		return nil
	}
	if credID != nil {
		v.Credential.CredID = *credID
		if err := s.API.DB.Pool.QueryRow(r.Context(), `SELECT scopes FROM credentials WHERE id=$1`, *credID).Scan(&v.Credential.Scopes); err != nil {
			return nil
		}
	}
	return v
}
func (s *Server) render(w http.ResponseWriter, p Page) {
	p.Demo = s.Demo
	p.SocialImage = s.Origin + "/static/juice.png"
	if p.RequestKey == "" {
		p.RequestKey = uuid.NewString()
	}
	w.Header().Set("Cache-Control", "no-store")
	var b bytes.Buffer
	if e := s.T.ExecuteTemplate(&b, "layout", p); e != nil {
		http.Error(w, "Page could not be rendered.", 500)
		fmt.Println("template:", e)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b.Bytes())
}
func (s *Server) call(r *http.Request, session *Session, method, path string, data any) ([]byte, int) {
	var b []byte
	if data != nil {
		b, _ = json.Marshal(data)
	}
	q, err := http.NewRequest(method, path, bytes.NewReader(b))
	if err != nil {
		return []byte(`{"detail":"Invalid request path."}`), 400
	}
	q = q.WithContext(api.WithCredential(r.Context(), session.Credential))
	q.Header.Set("Content-Type", "application/json")
	key := r.FormValue("request_key")
	if key == "" {
		key = uuid.NewString()
	}
	q.Header.Set("Idempotency-Key", key)
	if session.Owner {
		q.Header.Set("X-Setup-Token", s.Setup)
	}
	w := httptest.NewRecorder()
	s.APIHandler.ServeHTTP(w, q)
	return w.Body.Bytes(), w.Code
}
func read[T any](b []byte) T { var v T; json.Unmarshal(b, &v); return v }
func (s *Server) base(r *http.Request, session *Session) Page {
	p := Page{Session: session, Notice: r.URL.Query().Get("notice"), RequestKey: uuid.NewString()}
	b, status := s.call(r, session, "GET", "/v1/me", nil)
	if status != 200 {
		p.Error = "Profile is currently unavailable."
	}
	p.Me = read[api.AgentProfile](b)
	b, status = s.call(r, session, "GET", "/v1/groups", nil)
	if status != 200 {
		p.Error = "Groups are currently unavailable."
	}
	p.Groups = read[struct {
		Groups []api.Group `json:"groups"`
	}](b).Groups
	return p
}
func (s *Server) Landing(w http.ResponseWriter, r *http.Request) {
	if negotiatePublic(w, r, discoveryText) {
		return
	}
	s.render(w, Page{View: "landing", Title: "Ajent.social — shared knowledge for coding agents", Canonical: s.Origin + "/", Session: s.session(r)})
}
func (s *Server) Docs(w http.ResponseWriter, r *http.Request) {
	if negotiatePublic(w, r, discoveryText+docsExamples) {
		return
	}
	s.render(w, Page{View: "docs", Title: "Ajent.social API documentation", Canonical: s.Origin + "/docs", Session: s.session(r)})
}
func (s *Server) LoginPage(w http.ResponseWriter, r *http.Request) {
	csrf, err := random()
	if err != nil {
		http.Error(w, "Sign-in unavailable.", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "ajent_login_csrf", Value: csrf, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.Secure, MaxAge: 600})
	s.render(w, Page{View: "login", Title: "Welcome to ajent", Session: &Session{CSRF: csrf}, Error: r.URL.Query().Get("error")})
}
func (s *Server) originOK(r *http.Request) bool {
	return r.Header.Get("Origin") == "" || r.Header.Get("Origin") == s.Origin
}
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid or oversized form.", 400)
		return
	}
	csrf, e := r.Cookie("ajent_login_csrf")
	if e != nil || !s.originOK(r) || !auth.ConstantEq([]byte(csrf.Value), []byte(r.FormValue("csrf"))) {
		http.Error(w, "Reload the sign-in page and try again.", 403)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	sessionCSRF, err := random()
	if err != nil {
		http.Error(w, "Sign-in unavailable.", 503)
		return
	}
	session := &Session{CSRF: sessionCSRF}
	if api.OwnerKeyOK(token) {
		session.Owner = true
		var cid string
		e = s.API.DB.Pool.QueryRow(r.Context(), `SELECT a.id::text,a.org_id::text,cr.id::text,cr.scopes FROM app_settings st JOIN agents a ON a.id=st.value::uuid JOIN credentials cr ON cr.agent_id=a.id WHERE st.key='owner_agent' AND cr.revoked_at IS NULL ORDER BY cr.created_at DESC LIMIT 1`).Scan(&session.Credential.AgentID, &session.Credential.OrgID, &cid, &session.Credential.Scopes)
		session.Credential.CredID = cid
	} else {
		l, k, ok := auth.ParseToken("Bearer " + token)
		if !ok {
			e = fmt.Errorf("invalid token")
		} else {
			session.Credential, e = s.API.DB.Authenticate(r.Context(), l, k)
		}
	}
	if e != nil || !s.API.DB.HasScope(session.Credential, "network:read") {
		http.Redirect(w, r, "/login?error=Access+key+not+recognized+or+revoked", 303)
		return
	}
	raw, err := random()
	if err != nil {
		http.Error(w, "Sign-in unavailable.", 503)
		return
	}
	_, e = s.API.DB.Pool.Exec(r.Context(), `INSERT INTO web_sessions(digest,credential_id,agent_id,org_id,is_owner,csrf,owner_key_digest) VALUES($1,$2,$3,$4,$5,$6,$7)`, auth.Digest(raw), session.Credential.CredID, session.Credential.AgentID, session.Credential.OrgID, session.Owner, session.CSRF, auth.Digest(s.Setup))
	if e != nil {
		http.Error(w, "Sign-in unavailable.", 500)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "ajent_session", Value: raw, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.Secure, MaxAge: 43200})
	http.Redirect(w, r, "/app", 303)
}
func (s *Server) validAction(w http.ResponseWriter, r *http.Request) *Session {
	v := s.session(r)
	if v == nil {
		redirect(w, r, "/login")
		return nil
	}
	limit := int64(32 << 10)
	if r.URL.Path == "/actions/directory" {
		// Up to 100 explicitly selected profiles, including URL-encoded Unicode.
		limit = 1 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid or oversized form.", 400)
		return nil
	}
	if !s.originOK(r) || !auth.ConstantEq([]byte(v.CSRF), []byte(r.FormValue("csrf"))) {
		http.Error(w, "Form expired. Reload this page.", 403)
		return nil
	}
	return v
}
func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	v := s.validAction(w, r)
	if v == nil {
		return
	}
	if _, err := s.API.DB.Pool.Exec(r.Context(), `DELETE FROM web_sessions WHERE digest=$1`, v.Digest); err != nil {
		http.Error(w, "Sign-out unavailable. Try again.", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "ajent_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", 303)
}
func (s *Server) App(w http.ResponseWriter, r *http.Request) {
	if s.human(r) != nil {
		redirect(w, r, "/human/workspaces")
		return
	}
	v := s.session(r)
	if v == nil {
		redirect(w, r, "/login")
		return
	}
	p := s.base(r, v)
	path := strings.TrimPrefix(r.URL.Path, "/app")
	p.View = "feed"
	p.Title = "Your daily squeeze"
	p.Q = r.URL.Query().Get("q")
	p.Kind = r.URL.Query().Get("kind")
	p.Tab = r.URL.Query().Get("view")
	query := url.Values{"kind": {p.Kind}, "view": {p.Tab}, "before": {r.URL.Query().Get("before")}}
	endpoint := "/v1/feed?" + query.Encode()
	switch {
	case path == "/groups":
		p.View = "groups"
		p.Title = "Find your people. And agents."
	case path == "/agents":
		p.View = "agents"
		p.Title = "Good company."
		b, _ := s.call(r, v, "GET", "/v1/agents", nil)
		p.Agents = read[struct {
			Agents []api.AgentProfile `json:"agents"`
		}](b).Agents
	case path == "/notifications":
		p.View = "notifications"
		p.Title = "You’re in the loop."
		b, _ := s.call(r, v, "GET", "/v1/notifications", nil)
		p.Notifications = read[struct {
			Notifications []Notification `json:"notifications"`
		}](b).Notifications
	case path == "/settings":
		p.View = "settings"
		p.Title = "Make yourself at home."
		if v.Owner {
			rows, e := s.API.DB.Pool.Query(r.Context(), `SELECT cr.id::text,a.handle,cr.revoked_at IS NOT NULL,cr.created_at FROM credentials cr JOIN agents a ON a.id=cr.agent_id WHERE a.org_id=$1 ORDER BY cr.created_at DESC`, v.Credential.OrgID)
			if e == nil {
				for rows.Next() {
					var c Credential
					if rows.Scan(&c.ID, &c.Handle, &c.Revoked, &c.Created) != nil {
						p.Error = "Credentials unavailable."
						break
					}
					p.Credentials = append(p.Credentials, c)
				}
				if rows.Err() != nil {
					p.Error = "Settings could not finish loading."
				}
				rows.Close()
			} else {
				p.Error = "Settings unavailable."
			}
			rows, e = s.API.DB.Pool.Query(r.Context(), `SELECT id::text,post_id::text,reason,detail FROM reports WHERE status='open' ORDER BY created_at LIMIT 50`)
			if e == nil {
				for rows.Next() {
					var q Report
					if rows.Scan(&q.ID, &q.PostID, &q.Reason, &q.Detail) != nil {
						p.Error = "Reports unavailable."
						break
					}
					p.Reports = append(p.Reports, q)
				}
				if rows.Err() != nil {
					p.Error = "Settings could not finish loading."
				}
				rows.Close()
			} else {
				p.Error = "Settings unavailable."
			}
		}
		rows, e := s.API.DB.Pool.Query(r.Context(), `SELECT a.id::text,a.handle FROM blocks b JOIN agents a ON a.id=b.blocked_id WHERE b.blocker_id=$1`, v.Credential.AgentID)
		if e == nil {
			for rows.Next() {
				var b Block
				if rows.Scan(&b.ID, &b.Handle) != nil {
					p.Error = "Blocks unavailable."
					break
				}
				p.Blocks = append(p.Blocks, b)
			}
			rows.Close()
		}
	case strings.HasPrefix(path, "/groups/"):
		p.GroupID = strings.TrimPrefix(path, "/groups/")
		found := false
		for _, g := range p.Groups {
			if g.ID == p.GroupID {
				p.Group = g
				p.Title = g.Name
				found = true
			}
		}
		if !found {
			p.Error = "This group is private or unavailable."
		}
		endpoint = "/v1/groups/" + p.GroupID + "/posts?" + query.Encode()
	case strings.HasPrefix(path, "/agents/"):
		p.View = "profile"
		id := strings.TrimPrefix(path, "/agents/")
		b, status := s.call(r, v, "GET", "/v1/agents/"+id, nil)
		if status != 200 {
			p.Error = "This profile is unavailable."
		}
		p.Profile = read[api.AgentProfile](b)
		p.Title = p.Profile.Name
		endpoint = "/v1/agents/" + id + "/posts?" + query.Encode()
	case strings.HasPrefix(path, "/posts/"):
		p.View = "thread"
		p.Title = "A little shared knowledge."
		id := strings.TrimPrefix(path, "/posts/")
		b, status := s.call(r, v, "GET", "/v1/posts/"+id, nil)
		if status != 200 {
			p.Error = "This conversation has expired or is unavailable."
		}
		p.Post = read[api.Post](b)
		p.ThreadID = id
		if p.Post.RootID != "" {
			p.ThreadID = p.Post.RootID
		}
		endpoint = "/v1/feed?thread=" + p.ThreadID
	case path != "":
		p.View = "error"
		p.Error = "We couldn’t find that page."
		p.Title = "Nothing here yet."
	}
	if p.View == "feed" || p.View == "profile" || p.View == "thread" {
		if p.Q != "" {
			endpoint = "/v1/search?q=" + url.QueryEscape(p.Q) + "&kind=" + url.QueryEscape(p.Kind)
			p.Title = "A fresh perspective."
		}
		b, status := s.call(r, v, "GET", endpoint, nil)
		if status == 200 {
			out := read[struct {
				Posts []api.Post `json:"posts"`
				Next  string     `json:"next_cursor"`
			}](b)
			p.Posts = out.Posts
			if p.View == "thread" {
				filtered := []api.Post{}
				for _, reply := range p.Posts {
					if reply.ID != p.Post.ID {
						filtered = append(filtered, reply)
					}
				}
				p.Posts = filtered
			}
			p.Next = out.Next
		} else if p.Error == "" {
			p.Error = "Content is currently unavailable. Please try again."
		}
	}
	if r.Header.Get("HX-Target") == "feed-results" {
		var fragment bytes.Buffer
		if err := s.T.ExecuteTemplate(&fragment, "feed-results", p); err != nil {
			http.Error(w, "Content could not be rendered.", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(fragment.Bytes())
		return
	}
	s.render(w, p)
}
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", path)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, path, http.StatusSeeOther)
}
func (s *Server) Action(w http.ResponseWriter, r *http.Request) {
	v := s.validAction(w, r)
	if v == nil {
		return
	}
	action := r.PathValue("action")
	id := r.FormValue("id")
	switch action {
	case "delete", "follow", "unfollow", "save", "unsave", "join", "member", "block", "unblock", "report", "revoke", "moderate":
		if !validUUID(id) {
			http.Error(w, "Invalid identifier.", 400)
			return
		}
	}
	if action == "member" && !validUUID(r.FormValue("agent_id")) {
		http.Error(w, "Invalid agent identifier.", 400)
		return
	}
	back := r.FormValue("back")
	if !strings.HasPrefix(back, "/app") || strings.HasPrefix(back, "//") {
		back = "/app"
	}
	method, path := "POST", ""
	var payload any
	switch action {
	case "post":
		aud := r.FormValue("audience")
		gid := r.FormValue("group_id")
		if strings.HasPrefix(aud, "group:") {
			gid = strings.TrimPrefix(aud, "group:")
			aud = "private"
		}
		payload = api.PostRequest{Kind: r.FormValue("kind"), Title: r.FormValue("title"), Body: r.FormValue("body"), SchemaVersion: 1, GroupID: gid, ParentID: r.FormValue("parent_id"), Audience: aud}
		path = "/v1/posts"
	case "delete":
		method = "DELETE"
		path = "/v1/posts/" + id
	case "follow":
		path = "/v1/follows/" + id
	case "unfollow":
		method = "DELETE"
		path = "/v1/follows/" + id
	case "save":
		path = "/v1/bookmarks/" + id
	case "unsave":
		method = "DELETE"
		path = "/v1/bookmarks/" + id
	case "join":
		path = "/v1/groups/" + id + "/join"
	case "group":
		path = "/v1/groups"
		payload = map[string]string{"name": r.FormValue("name"), "description": r.FormValue("description"), "visibility": r.FormValue("visibility")}
		back = "/app/groups"
	case "member":
		path = "/v1/groups/" + id + "/members/" + r.FormValue("agent_id")
		payload = map[string]string{"action": r.FormValue("member_action")}
	case "profile":
		method = "PATCH"
		path = "/v1/me/profile"
		payload = map[string]string{"name": r.FormValue("name"), "bio": r.FormValue("bio")}
	case "read":
		path = "/v1/notifications/read"
	case "block":
		path = "/v1/blocks"
		payload = map[string]string{"agent_id": id}
	case "unblock":
		method = "DELETE"
		path = "/v1/blocks/" + id
	case "report":
		path = "/v1/reports"
		payload = map[string]string{"post_id": id, "reason": r.FormValue("reason"), "detail": r.FormValue("detail")}
	case "agent":
		if !v.Owner {
			http.Error(w, "Owner access required.", 403)
			return
		}
		path = "/v1/agents"
		payload = map[string]any{"org_id": v.Credential.OrgID, "handle": r.FormValue("handle"), "name": r.FormValue("name"), "public_posting": r.FormValue("public_posting") == "on", "scopes": []string{"network:read", "posts:write", "social:write", "groups:manage"}}
	case "revoke":
		if !v.Owner || id == v.Credential.CredID {
			http.Error(w, "Cannot revoke this owner session credential.", 403)
			return
		}
		var owned bool
		s.API.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM credentials cr JOIN agents a ON a.id=cr.agent_id WHERE cr.id::text=$1 AND a.org_id=$2)`, id, v.Credential.OrgID).Scan(&owned)
		if !owned {
			http.NotFound(w, r)
			return
		}
		method = "DELETE"
		path = "/v1/credentials/" + id
	case "moderate":
		if !v.Owner || !validUUID(id) {
			http.Error(w, "Owner access required.", 403)
			return
		}
		tx, e := s.API.DB.Pool.Begin(r.Context())
		if e != nil {
			http.Error(w, "Database unavailable.", 500)
			return
		}
		defer tx.Rollback(r.Context())
		decision := r.FormValue("decision")
		if decision != "hide" && decision != "dismiss" {
			http.Error(w, "Choose hide or dismiss.", 400)
			return
		}
		if decision == "hide" {
			_, e = tx.Exec(r.Context(), `UPDATE posts SET hidden_reason='admin' WHERE id=(SELECT post_id FROM reports WHERE id=$1)`, id)
		}
		if e == nil {
			_, e = tx.Exec(r.Context(), `UPDATE reports SET status=$2,reviewed_at=now() WHERE id=$1`, id, r.FormValue("decision"))
		}
		if e == nil {
			_, e = tx.Exec(r.Context(), `INSERT INTO audit_log(actor,action,object_type,object_id,detail) VALUES($1,'report.reviewed','report',$2,jsonb_build_object('decision',$3::text))`, v.Credential.AgentID, id, decision)
		}
		if e != nil || tx.Commit(r.Context()) != nil {
			http.Error(w, "Could not save review.", 500)
			return
		}
		redirect(w, r, "/app/settings?notice=Review+saved")
		return
	default:
		http.NotFound(w, r)
		return
	}
	b, status := s.call(r, v, method, path, payload)
	if status >= 400 {
		err := read[map[string]string](b)
		if r.Header.Get("HX-Request") == "true" {
			http.Error(w, err["detail"], 422)
			return
		}
		p := s.base(r, v)
		p.View = "error"
		p.Title = "Let’s try that again."
		p.Error = err["detail"]
		s.render(w, p)
		return
	}
	if action == "agent" {
		out := read[map[string]string](b)
		p := s.base(r, v)
		p.View = "secret"
		p.Title = "Your agent is ready."
		p.Secret = out["token"]
		p.CredentialID = out["agent_id"]
		s.render(w, p)
		return
	}
	redirect(w, r, back)
}
func validUUID(s string) bool { _, e := uuid.Parse(s); return e == nil }
