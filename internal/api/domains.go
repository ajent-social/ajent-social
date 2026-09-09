package api

import (
	"ajent/internal/domains"
	"ajent/internal/store"
	"context"
	"net"
	"net/http"
	"regexp"
	"time"
)

var scopePattern = regexp.MustCompile(`^(user|project:[a-f0-9]{64})$`)

func (s *Server) installation(w http.ResponseWriter, r *http.Request) (*store.Credential, bool) {
	c, ok := s.authenticate(w, r, "groups:manage")
	if !ok {
		return nil, false
	}
	var exists bool
	err := s.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM public_enrollments WHERE credential_id=$1)`, c.CredID).Scan(&exists)
	if err != nil {
		fail(w, 503, "unavailable", "Installation unavailable.")
		return nil, false
	}
	if !exists {
		fail(w, 403, "installation_required", "Use the original installation account to manage domain grants.")
		return nil, false
	}
	return c, true
}

func (s *Server) Domains(w http.ResponseWriter, r *http.Request) {
	c, ok := s.installation(w, r)
	if !ok {
		return
	}
	rows, err := s.DB.Pool.Query(r.Context(), `SELECT domain,verified_until,verified_until>now(),last_check_error FROM verified_domains WHERE org_id=$1 ORDER BY domain`, c.OrgID)
	if err != nil {
		fail(w, 503, "unavailable", "Domains unavailable.")
		return
	}
	out := []map[string]any{}
	for rows.Next() {
		var domain, notice string
		var until time.Time
		var verified bool
		err = rows.Scan(&domain, &until, &verified, &notice)
		if err != nil {
			break
		}
		out = append(out, map[string]any{"domain": domain, "verified": verified, "verified_until": until, "notice": notice})
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		fail(w, 503, "unavailable", "Domains unavailable.")
		return
	}
	grants := []map[string]string{}
	rows, err = s.DB.Pool.Query(r.Context(), `SELECT id::text,domain,scope_key FROM domain_grants WHERE parent_credential_id=$1 ORDER BY domain,scope_key`, c.CredID)
	if err != nil {
		fail(w, 503, "unavailable", "Grants unavailable.")
		return
	}
	for rows.Next() {
		var id, domain, scope string
		err = rows.Scan(&id, &domain, &scope)
		if err != nil {
			break
		}
		grants = append(grants, map[string]string{"id": id, "domain": domain, "scope": scope})
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		fail(w, 503, "unavailable", "Grants unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"domains": out, "grants": grants})
}
func (s *Server) DomainProof(w http.ResponseWriter, r *http.Request) {
	s.domainProof(w, r, net.DefaultResolver.LookupTXT)
}
func (s *Server) domainProof(w http.ResponseWriter, r *http.Request, lookup domains.Lookup) {
	c, ok := s.installation(w, r)
	if !ok {
		return
	}
	var linked bool
	if err := s.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM human_installations WHERE credential_id=$1)`, c.CredID).Scan(&linked); err != nil {
		fail(w, 503, "unavailable", "Ownership unavailable.")
		return
	}
	if linked {
		fail(w, 403, "human_required", "Manage domain verification from your human workspace.")
		return
	}

	var p struct {
		Domain string `json:"domain"`
		Action string `json:"action"`
	}
	if !decode(w, r, &p) {
		return
	}
	domain, err := domains.Normalize(p.Domain)
	if err != nil {
		fail(w, 400, "bad_domain", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	switch p.Action {
	case "challenge":
		token, err := domains.Challenge(ctx, s.DB.Pool, c.OrgID, c.AgentID, domain)
		if err != nil {
			fail(w, 409, "challenge_failed", "Could not create domain challenge.")
			return
		}
		writeJSON(w, 200, map[string]string{"domain": domain, "name": "_ajent-verification." + domain, "value": "ajent-verification=" + token})
	case "check":
		if err = domains.Check(ctx, s.DB.Pool, c.OrgID, c.AgentID, domain, lookup); err != nil {
			fail(w, 409, "verification_failed", "Check the TXT proof and retry. The domain may be held by another account.")
			return
		}
		writeJSON(w, 200, map[string]any{"domain": domain, "verified": true})
	default:
		fail(w, 400, "bad_action", "Choose challenge or check.")
	}
}
func (s *Server) GrantDomain(w http.ResponseWriter, r *http.Request) {
	c, ok := s.installation(w, r)
	if !ok {
		return
	}
	var p struct {
		Domain string `json:"domain"`
		Scope  string `json:"scope"`
	}
	if !decode(w, r, &p) {
		return
	}
	domain, err := domains.Normalize(p.Domain)
	if err != nil || !scopePattern.MatchString(p.Scope) {
		fail(w, 400, "bad_scope", "Choose a domain and user or project scope.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Grant unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	var id string
	// Same lock order as enrollment, then domain checks, serializes grants and revocation.
	err = tx.QueryRow(ctx, `SELECT id::text FROM credentials WHERE id=$1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()) FOR UPDATE`, c.CredID).Scan(&id)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT domain FROM verified_domains WHERE domain=$1 AND org_id=$2 AND verified_until>now() FOR SHARE`, domain, c.OrgID).Scan(&id)
	}
	if err != nil {
		fail(w, 403, "verified_domain_required", "Verify this domain for your account first.")
		return
	}
	var linked, approved bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM human_installations WHERE credential_id=$1),EXISTS(SELECT 1 FROM domain_grants WHERE parent_credential_id=$1 AND domain=$2 AND scope_key=$3)`, c.CredID, domain, p.Scope).Scan(&linked, &approved)
	if err != nil {
		fail(w, 503, "unavailable", "Ownership unavailable.")
		return
	}
	if linked && !approved {
		fail(w, 403, "human_required", "Run client.py link and approve this domain scope with your human account.")
		return
	}
	if linked {
		err = tx.QueryRow(ctx, `SELECT id::text FROM domain_grants WHERE parent_credential_id=$1 AND domain=$2 AND scope_key=$3 FOR SHARE`, c.CredID, domain, p.Scope).Scan(&id)
		if err != nil {
			fail(w, 403, "human_required", "Approve this scope from your human account.")
			return
		}
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO domain_grants(parent_credential_id,domain,scope_key) VALUES($1,$2,$3) ON CONFLICT(parent_credential_id,domain,scope_key) DO UPDATE SET domain=excluded.domain RETURNING id::text`, c.CredID, domain, p.Scope).Scan(&id)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,'domain.grant','domain_grant',$2)`, c.AgentID, id)
	}
	if err != nil || tx.Commit(ctx) != nil {
		fail(w, 503, "unavailable", "Grant could not be saved.")
		return
	}
	writeJSON(w, 200, map[string]string{"id": id, "domain": domain, "scope": p.Scope})
}
func (s *Server) RevokeDomainGrant(w http.ResponseWriter, r *http.Request) {
	c, ok := s.installation(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 400, "bad_grant", "Choose a grant.")
		return
	}
	tx, err := s.DB.Pool.Begin(r.Context())
	if err != nil {
		fail(w, 503, "unavailable", "Grant unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `DELETE FROM domain_grants WHERE id=$1 AND parent_credential_id=$2`, id, c.CredID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,'domain.grant_revoke','domain_grant',$2)`, c.AgentID, id)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		fail(w, 503, "unavailable", "Grant could not be revoked.")
		return
	}
	writeJSON(w, 200, map[string]bool{"revoked": true})
}
