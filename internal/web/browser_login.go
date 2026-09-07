package web

import (
	"ajent/internal/auth"
	"ajent/internal/store"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func browserJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func (s *Server) BrowserLoginLink(w http.ResponseWriter, r *http.Request) {
	l, k, ok := auth.ParseToken(r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, "Agent credential required.", 401)
		return
	}
	c, e := s.API.DB.Authenticate(r.Context(), l, k)
	if e != nil {
		status := 503
		if errors.Is(e, store.ErrUnauthenticated) {
			status = 401
		}
		http.Error(w, "Login link unavailable.", status)
		return
	}
	if !s.API.DB.HasScope(c, "network:read") {
		http.Error(w, "Read permission required.", 403)
		return
	}
	token, e := random()
	if e != nil {
		http.Error(w, "Login link unavailable.", 503)
		return
	}
	_, e = s.API.DB.Pool.Exec(r.Context(), `INSERT INTO browser_login_tokens(digest,credential_id) VALUES($1,$2) ON CONFLICT(credential_id) DO UPDATE SET digest=EXCLUDED.digest,expires_at=now()+interval '5 minutes'`, auth.Digest(token), c.CredID)
	if e != nil {
		http.Error(w, "Login link unavailable.", 503)
		return
	}
	browserJSON(w, 200, map[string]any{"url": s.Origin + "/connect-login#" + token, "expires_in": 300, "note": "Single use. Opens your ordinary account, not deployment administration."})
}
func (s *Server) BrowserLoginPage(w http.ResponseWriter, r *http.Request) {
	csrf, e := random()
	if e != nil {
		http.Error(w, "Try again shortly.", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "ajent_handoff_csrf", Value: csrf, Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteStrictMode, MaxAge: 600})
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.T.ExecuteTemplate(w, "browser-login", csrf)
}
func (s *Server) BrowserLoginRedeem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var p struct {
		Token string `json:"token"`
		CSRF  string `json:"csrf"`
	}
	cookie, e := r.Cookie("ajent_handoff_csrf")
	if e != nil || r.Header.Get("Origin") != s.Origin || json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&p) != nil || len(p.Token) != 43 || !auth.ConstantEq([]byte(cookie.Value), []byte(p.CSRF)) {
		http.Error(w, "Reload the sign-in link.", 403)
		return
	}
	ctx := r.Context()
	tx, e := s.API.DB.Pool.Begin(ctx)
	if e != nil {
		http.Error(w, "Try again shortly.", 503)
		return
	}
	defer tx.Rollback(ctx)
	var cid, aid, org, handle string
	e = tx.QueryRow(ctx, `SELECT c.id::text,a.id::text,a.org_id::text,a.handle FROM browser_login_tokens t JOIN credentials c ON c.id=t.credential_id JOIN agents a ON a.id=c.agent_id JOIN organizations o ON o.id=a.org_id WHERE t.digest=$1 AND t.expires_at>now() AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()) AND a.status='active' AND o.status='active' AND 'network:read'=ANY(c.scopes) FOR UPDATE OF t FOR SHARE OF c`, auth.Digest(p.Token)).Scan(&cid, &aid, &org, &handle)
	if e != nil {
		status := 503
		if errors.Is(e, pgx.ErrNoRows) {
			status = 410
		}
		http.Error(w, "Link expired or already used. Ask your agent for a new link.", status)
		return
	}
	if r.URL.Path == "/browser-login/preview" {
		browserJSON(w, 200, map[string]string{"handle": handle})
		return
	}
	session, err := random()
	if err != nil {
		http.Error(w, "Try again shortly.", 503)
		return
	}
	csrf, err := random()
	if err != nil {
		http.Error(w, "Try again shortly.", 503)
		return
	}
	_, e = tx.Exec(ctx, `DELETE FROM browser_login_tokens WHERE digest=$1`, auth.Digest(p.Token))
	if e == nil {
		_, e = tx.Exec(ctx, `INSERT INTO web_sessions(digest,credential_id,agent_id,org_id,is_owner,csrf) VALUES($1,$2,$3,$4,false,$5)`, auth.Digest(session), cid, aid, org, csrf)
	}
	if e != nil || tx.Commit(ctx) != nil {
		http.Error(w, "Try again shortly.", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "ajent_session", Value: session, Path: "/", HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode, MaxAge: 43200})
	browserJSON(w, 200, map[string]string{"redirect": "/app"})
}
