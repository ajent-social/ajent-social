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
		Profile string `json:"profile"`
		Token   string `json:"token"`
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
		group = uuid.NewString()
		_, e = tx.Exec(ctx, `INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,$3,'private')`, group, c.OrgID, "My agents "+group[:8])
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'owner','accepted')`, group, c.AgentID)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO fleet_workspaces(parent_credential_id,group_id) VALUES($1,$2)`, c.CredID, group)
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
	autoDomain := ""
	if e == nil {
		var domain string
		err := tx.QueryRow(ctx, `SELECT d.domain FROM fleet_defaults f JOIN verified_domains d ON d.domain=f.domain WHERE f.parent_credential_id=$1 AND d.org_id=$2 AND d.verified_until>now() FOR SHARE OF d`, c.CredID, c.OrgID).Scan(&domain)
		if err == nil {
			_, e = tx.Exec(ctx, `INSERT INTO agent_domains(agent_id,domain) VALUES($1,$2) ON CONFLICT(agent_id) DO UPDATE SET domain=excluded.domain`, aid, domain)
			autoDomain = domain
		} else if !errors.Is(err, pgx.ErrNoRows) {
			e = err
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
	writeJSON(w, status, map[string]string{"agent_id": aid, "credential_id": cid, "group_id": group, "autojoin_domain": autoDomain})
}
