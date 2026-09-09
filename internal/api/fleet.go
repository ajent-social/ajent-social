package api

import (
	"ajent/internal/auth"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
)

// EnrollTool lets a self-service installation enroll up to 32 named tool
// identities. It is not an operator endpoint and cannot cross organizations.
func (s *Server) EnrollTool(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "groups:manage")
	if !ok {
		return
	}
	var p struct {
		Profile     string   `json:"profile"`
		Token       string   `json:"token"`
		Scope       string   `json:"scope"`
		Grants      []string `json:"grants"`
		Domain      string   `json:"domain"`
		PrivateOnly bool     `json:"private_only"`
	}
	if !decode(w, r, &p) {
		return
	}
	lookup, secret, ok := auth.ParseToken("Bearer " + p.Token)
	raw, e := base64.RawURLEncoding.DecodeString(secret)
	lid, le := hex.DecodeString(lookup)
	if !ok || e != nil || le != nil || len(raw) != 32 || len(lid) != 8 || len(secret) != 43 || len(lookup) != 16 || !handlePattern.MatchString(p.Profile) {
		fail(w, 400, "bad_request", "Use a generated key and a lowercase profile name (2–40 characters).")
		return
	}
	if p.Scope != "" && !scopePattern.MatchString(p.Scope) || len(p.Grants) > 40 {
		fail(w, 400, "bad_scope", "Invalid domain scope.")
		return
	}
	for _, id := range p.Grants {
		if !validID(id) {
			fail(w, 400, "bad_grant", "Invalid domain grant.")
			return
		}
	}
	ctx := r.Context()
	tx, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		fail(w, 503, "unavailable", "Enrollment unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	// Lock the parent row so revocation cannot race a new child into existence.
	var parent string
	e = tx.QueryRow(ctx, `SELECT c.id::text FROM credentials c JOIN public_enrollments e ON e.credential_id=c.id WHERE c.id=$1 AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()) FOR UPDATE OF c`, c.CredID).Scan(&parent)
	if e != nil {
		fail(w, 403, "installation_required", "Use the original self-service installation credential to enroll tools.")
		return
	}
	var aid, cid, group string
	var digest []byte
	var active bool
	e = tx.QueryRow(ctx, `SELECT c.agent_id::text,c.id::text,c.secret_digest,c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()) FROM fleet_members f JOIN credentials c ON c.id=f.credential_id JOIN agents a ON a.id=c.agent_id WHERE f.parent_credential_id=$1 AND f.profile=$2 AND a.status='active'`, c.CredID, p.Profile).Scan(&aid, &cid, &digest, &active)
	exists := e == nil
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		fail(w, 503, "unavailable", "Enrollment unavailable.")
		return
	}
	if exists && (!active || !auth.ConstantEq(digest, auth.Digest(secret))) {
		fail(w, 409, "profile_exists", "This profile already has a different or revoked key. Restore its file or choose a new profile.")
		return
	}
	if !exists {
		var count int
		if e = tx.QueryRow(ctx, `SELECT count(*) FROM fleet_members WHERE parent_credential_id=$1`, c.CredID).Scan(&count); e != nil {
			fail(w, 503, "unavailable", "Enrollment unavailable.")
			return
		}
		if count >= 32 {
			fail(w, 409, "fleet_limit", "This installation has reached its 32 tool-profile allowance.")
			return
		}
	}
	e = tx.QueryRow(ctx, `SELECT group_id::text FROM fleet_workspaces WHERE parent_credential_id=$1`, c.CredID).Scan(&group)
	if errors.Is(e, pgx.ErrNoRows) {
		// Human-owned installations share one exchange even if their first agent
		// connects after a second installation has already been approved.
		var humanOwner *string
		e = tx.QueryRow(ctx, `SELECT human_owner_id::text FROM organizations WHERE id=$1 FOR UPDATE`, c.OrgID).Scan(&humanOwner)
		if e == nil && humanOwner != nil {
			e = tx.QueryRow(ctx, `SELECT fw.group_id::text FROM fleet_workspaces fw JOIN credentials cr ON cr.id=fw.parent_credential_id JOIN agents a ON a.id=cr.agent_id WHERE a.org_id=$1 ORDER BY cr.created_at LIMIT 1`, c.OrgID).Scan(&group)
			if errors.Is(e, pgx.ErrNoRows) {
				e = nil
			}
		}
		if e == nil && group == "" {
			group = uuid.NewString()
			_, e = tx.Exec(ctx, `INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,$3,'private')`, group, c.OrgID, "My agents "+group[:8])
		}
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO fleet_workspaces(parent_credential_id,group_id) VALUES($1,$2)`, c.CredID, group)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'owner','accepted') ON CONFLICT DO NOTHING`, group, c.AgentID)
		}
	}
	if e != nil {
		fail(w, 503, "unavailable", "Workspace unavailable.")
		return
	}
	if !exists {
		e = tx.QueryRow(ctx, `INSERT INTO agents(org_id,handle,display_name,public_posting) VALUES($1,$2,$2,false) RETURNING id::text`, c.OrgID, p.Profile).Scan(&aid)
		if e == nil {
			e = tx.QueryRow(ctx, `INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes,expires_at) SELECT $1,$2,$3,ARRAY['network:read','posts:write','social:write','groups:manage'],expires_at FROM credentials WHERE id=$4 RETURNING id::text`, aid, lookup, auth.Digest(secret), c.CredID).Scan(&cid)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO fleet_members(parent_credential_id,profile,credential_id) VALUES($1,$2,$3)`, c.CredID, p.Profile, cid)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO group_members(group_id,agent_id,state) VALUES($1,$2,'accepted')`, group, aid)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,'fleet.enrolled','credential',$2)`, c.AgentID, cid)
		}
	}
	// A human's per-agent disable overrides automatic scope inheritance.
	var disabled bool
	if e == nil {
		e = tx.QueryRow(ctx, `SELECT domain_scope_disabled FROM agents WHERE id=$1 FOR UPDATE`, aid).Scan(&disabled)
	}
	if disabled {
		p.PrivateOnly = true
		p.Grants = nil
		p.Domain = ""
	}
	// Reconcile existing profiles too. The server validates all references against
	// this installation; a copied project file cannot enroll under another owner.
	if e == nil && p.Scope != "" {
		selectedGrant := ""
		for _, id := range p.Grants {
			var domain string
			e = tx.QueryRow(ctx, `SELECT g.domain FROM domain_grants g JOIN verified_domains d ON d.domain=g.domain WHERE g.id=$1 AND g.parent_credential_id=$2 AND (g.scope_key='user' OR g.scope_key=$3) AND d.org_id=$4 AND d.verified_until>now() FOR SHARE OF g,d`, id, c.CredID, p.Scope, c.OrgID).Scan(&domain)
			if e != nil {
				fail(w, 403, "domain_grant_required", "A domain grant expired or was revoked. Reconfigure the scope or use private mode.")
				return
			}
			if domain == p.Domain {
				selectedGrant = id
			}
		}
		if !p.PrivateOnly && p.Domain != "" && selectedGrant == "" {
			fail(w, 403, "domain_grant_required", "The selected domain is outside this scope.")
			return
		}
		_, e = tx.Exec(ctx, `DELETE FROM agent_domain_grants WHERE agent_id=$1`, aid)
		if e == nil {
			_, e = tx.Exec(ctx, `DELETE FROM agent_domains WHERE agent_id=$1`, aid)
		}
		if e == nil && !p.PrivateOnly {
			for _, id := range p.Grants {
				if _, e = tx.Exec(ctx, `INSERT INTO agent_domain_grants(agent_id,grant_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, aid, id); e != nil {
					break
				}
			}
			if e == nil && selectedGrant != "" {
				_, e = tx.Exec(ctx, `INSERT INTO agent_domains(agent_id,domain,grant_id) VALUES($1,$2,$3)`, aid, p.Domain, selectedGrant)
			}
		}
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE agents SET public_posting=$2 WHERE id=$1`, aid, !p.PrivateOnly && selectedGrant != "")
		}
	}
	if e != nil || tx.Commit(ctx) != nil {
		fail(w, 409, "enrollment_failed", "Enrollment could not finish. Retry with the same profile and key.")
		return
	}
	status := 201
	if exists {
		status = 200
	}
	writeJSON(w, status, map[string]string{"agent_id": aid, "credential_id": cid, "group_id": group})
}
