package web

import (
	"ajent/internal/auth"
	"ajent/internal/ratelimit"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	wa "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Human struct {
	ID, Name, CSRF string
	Keys           []wa.Credential
}

func (h Human) WebAuthnID() []byte                   { return []byte(h.ID) }
func (h Human) WebAuthnName() string                 { return h.Name }
func (h Human) WebAuthnDisplayName() string          { return h.Name }
func (h Human) WebAuthnCredentials() []wa.Credential { return h.Keys }

// Every ceremony validates the configured RP origin; request Host is never trusted.
func (s *Server) passkeys() (*wa.WebAuthn, error) {
	u, err := url.Parse(s.Origin)
	if err != nil {
		return nil, err
	}
	return wa.New(&wa.Config{RPDisplayName: "Ajent", RPID: u.Hostname(), RPOrigins: []string{s.Origin}, AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}})
}

var humanLimits = ratelimit.New(20, time.Minute, 10000)

func (s *Server) humanRate(w http.ResponseWriter, r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if p := net.ParseIP(ip); p != nil && p.To4() == nil {
		ip = p.Mask(net.CIDRMask(64, 128)).String()
	}
	if !humanLimits.Allow(ip, time.Now()) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Too many attempts. Try again shortly.", 429)
		return false
	}
	return true
}
func (s *Server) human(r *http.Request) *Human {
	cookie, err := r.Cookie("ajent_human")
	if err != nil {
		return nil
	}
	h := &Human{}
	err = s.API.DB.Pool.QueryRow(r.Context(), `SELECT h.id::text,h.name,s.csrf FROM human_sessions s JOIN humans h ON h.id=s.human_id WHERE s.digest=$1 AND s.expires_at>now()`, auth.Digest(cookie.Value)).Scan(&h.ID, &h.Name, &h.CSRF)
	if err != nil {
		return nil
	}
	return h
}
func (s *Server) humanCSRF(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") != s.Origin {
		http.Error(w, "Reload this page to continue.", 403)
		return false
	}
	c, err := r.Cookie("ajent_human_csrf")
	if err != nil || c.Value == "" || !auth.ConstantEq([]byte(c.Value), []byte(r.Header.Get("X-CSRF-Token"))) {
		http.Error(w, "Reload this page to continue.", 403)
		return false
	}
	return true
}
func (s *Server) humanPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	h := s.human(r)
	if r.URL.Path == "/start" && h != nil {
		redirect(w, r, "/join")
		return
	}
	csrf, err := random()
	if err != nil {
		http.Error(w, "Sign-in unavailable.", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "ajent_human_csrf", Value: csrf, Path: "/", Secure: s.Secure, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 600})
	w.Header().Set("Referrer-Policy", "no-referrer")
	data := struct {
		CSRF       string
		Human      *Human
		Onboarding bool
	}{csrf, h, r.URL.Path == "/start"}
	if err = s.T.ExecuteTemplate(w, "human-login", data); err != nil {
		http.Error(w, "Sign-in unavailable.", 503)
	}
}
func (s *Server) humanBegin(w http.ResponseWriter, r *http.Request) {
	if !s.humanRate(w, r) || !s.humanCSRF(w, r) {
		return
	}
	var p struct{ Action, Name string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&p) != nil {
		http.Error(w, "Invalid request.", 400)
		return
	}
	engine, err := s.passkeys()
	if err != nil {
		http.Error(w, "Passkeys unavailable.", 503)
		return
	}
	h := Human{ID: uuid.NewString(), Name: strings.TrimSpace(p.Name)}
	kind := p.Action
	var options any
	var state *wa.SessionData
	switch kind {
	case "register":
		if current := s.human(r); current != nil {
			h = *current
			kind = "add"
		}
		if len(h.Name) == 0 || len(h.Name) > 100 {
			http.Error(w, "Enter your name (up to 100 characters).", 400)
			return
		}
		options, state, err = engine.BeginRegistration(h)
	case "login":
		options, state, err = engine.BeginDiscoverableLogin()
	default:
		http.Error(w, "Choose register or login.", 400)
		return
	}
	if err != nil {
		http.Error(w, "Passkey setup unavailable.", 503)
		return
	}
	raw, err := random()
	if err != nil {
		http.Error(w, "Passkey setup unavailable.", 503)
		return
	}
	data, err := json.Marshal(state)
	if err != nil {
		http.Error(w, "Passkey setup unavailable.", 503)
		return
	}
	_, err = s.API.DB.Pool.Exec(r.Context(), `INSERT INTO human_ceremonies(digest,human_id,name,kind,session_data) VALUES($1,$2,$3,$4,$5)`, auth.Digest(raw), h.ID, h.Name, kind, string(data))
	if err != nil {
		http.Error(w, "Passkey setup unavailable.", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "ajent_ceremony", Value: raw, Path: "/human", Secure: s.Secure, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 300})
	browserJSON(w, 200, options)
}
func (s *Server) humanSession(ctx context.Context, tx pgx.Tx, w http.ResponseWriter, id string) (string, error) {
	raw, err := random()
	if err != nil {
		return "", err
	}
	csrf, err := random()
	if err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO human_sessions(digest,human_id,csrf) VALUES($1,$2,$3)`, auth.Digest(raw), id, csrf)
	return raw, err
}
func (s *Server) setHumanCookie(w http.ResponseWriter, raw string) {
	http.SetCookie(w, &http.Cookie{Name: "ajent_human", Value: raw, Path: "/", Secure: s.Secure, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 43200})
}
func (s *Server) humanFinish(w http.ResponseWriter, r *http.Request) {
	if !s.humanRate(w, r) || !s.humanCSRF(w, r) {
		return
	}
	c, err := r.Cookie("ajent_ceremony")
	if err != nil {
		http.Error(w, "Start sign-in again.", 400)
		return
	}
	var data []byte
	var kind string
	h := Human{}
	// Consume before verification, including invalid responses: a ceremony is single-use.
	err = s.API.DB.Pool.QueryRow(r.Context(), `DELETE FROM human_ceremonies WHERE digest=$1 AND expires_at>now() RETURNING human_id::text,name,kind,session_data`, auth.Digest(c.Value)).Scan(&h.ID, &h.Name, &kind, &data)
	if err != nil {
		http.Error(w, "Sign-in expired or already used. Start again.", 410)
		return
	}
	var state wa.SessionData
	if json.Unmarshal(data, &state) != nil {
		http.Error(w, "Sign-in unavailable.", 503)
		return
	}
	engine, err := s.passkeys()
	if err != nil {
		http.Error(w, "Passkeys unavailable.", 503)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	tx, err := s.API.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "Sign-in unavailable.", 503)
		return
	}
	defer tx.Rollback(r.Context())
	var credential *wa.Credential
	if kind == "register" || kind == "add" {
		if kind == "add" {
			current := s.human(r)
			if current == nil || current.ID != h.ID {
				http.Error(w, "Sign in again before adding a passkey.", 403)
				return
			}
		}
		credential, err = engine.FinishRegistration(h, state, r)
	} else {
		var user wa.User
		user, credential, err = engine.FinishPasskeyLogin(func(rawID, userHandle []byte) (wa.User, error) {
			var encoded []byte
			var existing Human
			e := tx.QueryRow(r.Context(), `SELECT h.id::text,h.name,p.credential FROM human_passkeys p JOIN humans h ON h.id=p.human_id WHERE p.id=$1 AND h.id::text=$2 FOR UPDATE OF p`, rawID, string(userHandle)).Scan(&existing.ID, &existing.Name, &encoded)
			if e != nil {
				return nil, e
			}
			var key wa.Credential
			if e = json.Unmarshal(encoded, &key); e != nil {
				return nil, e
			}
			existing.Keys = []wa.Credential{key}
			return existing, nil
		}, state, r)
		if err == nil {
			h = user.(Human)
		}
	}
	if err != nil || credential == nil || credential.Authenticator.CloneWarning {
		http.Error(w, "Passkey verification failed. Try again.", 401)
		return
	}
	recovery := ""
	if kind == "register" {
		recovery, err = random()
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO humans(id,name,recovery_digest) VALUES($1,$2,$3)`, h.ID, h.Name, auth.Digest(recovery))
		}
	}
	if err == nil {
		encoded, e := json.Marshal(credential)
		err = e
		if err == nil {
			if kind == "login" {
				_, err = tx.Exec(r.Context(), `UPDATE human_passkeys SET credential=$2 WHERE id=$1 AND human_id=$3`, credential.ID, string(encoded), h.ID)
			} else {
				_, err = tx.Exec(r.Context(), `INSERT INTO human_passkeys(id,human_id,credential) VALUES($1,$2,$3)`, credential.ID, h.ID, string(encoded))
			}
		}
	}
	var raw string
	if err == nil {
		raw, err = s.humanSession(r.Context(), tx, w, h.ID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO human_audit(human_id,action,object_id) VALUES($1,$2,$3)`, h.ID, "human."+kind, h.ID)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		http.Error(w, "Sign-in could not finish.", 503)
		return
	}
	s.setHumanCookie(w, raw)
	browserJSON(w, 200, map[string]string{"redirect": "/human/workspaces", "recovery_code": recovery})
}
func (s *Server) humanRecover(w http.ResponseWriter, r *http.Request) {
	if !s.humanRate(w, r) || !s.humanCSRF(w, r) {
		return
	}
	var p struct{ Code string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&p) != nil || len(p.Code) != 43 {
		http.Error(w, "Invalid recovery code.", 401)
		return
	}
	replacement, err := random()
	if err != nil {
		http.Error(w, "Recovery unavailable.", 503)
		return
	}
	tx, err := s.API.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "Recovery unavailable.", 503)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `UPDATE humans SET recovery_digest=$2 WHERE recovery_digest=$1 RETURNING id::text`, auth.Digest(p.Code), auth.Digest(replacement)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "Invalid or used recovery code.", 401)
		return
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `DELETE FROM human_sessions WHERE human_id=$1`, id)
	}
	var raw string
	if err == nil {
		raw, err = s.humanSession(r.Context(), tx, w, id)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO human_audit(human_id,action,object_id) VALUES($1,'human.recovered',$2)`, id, id)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		http.Error(w, "Recovery unavailable.", 503)
		return
	}
	s.setHumanCookie(w, raw)
	browserJSON(w, 200, map[string]string{"redirect": "/human/login", "recovery_code": replacement})
}
func (s *Server) humanAction(w http.ResponseWriter, r *http.Request) *Human {
	h := s.human(r)
	if h == nil {
		http.Error(w, "Sign in as yourself to continue.", 401)
		return nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if r.ParseForm() != nil || r.Header.Get("Origin") != s.Origin || !auth.ConstantEq([]byte(h.CSRF), []byte(r.PostForm.Get("csrf"))) {
		http.Error(w, "Reload the page to continue.", 403)
		return nil
	}
	return h
}
func (s *Server) humanLogout(w http.ResponseWriter, r *http.Request) {
	h := s.humanAction(w, r)
	if h == nil {
		return
	}
	c, err := r.Cookie("ajent_human")
	if err != nil {
		return
	}
	if _, err = s.API.DB.Pool.Exec(r.Context(), `DELETE FROM human_sessions WHERE digest=$1`, auth.Digest(c.Value)); err != nil {
		http.Error(w, "Sign-out unavailable.", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "ajent_human", Value: "", Path: "/", Secure: s.Secure, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	redirect(w, r, "/human/login")
}
