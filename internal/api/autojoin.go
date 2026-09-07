package api

import (
	"net/http"
)

// FleetAutojoin authorizes this installation's current and future tool profiles.
func (s *Server) FleetAutojoin(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "groups:manage")
	if !ok {
		return
	}
	var p struct {
		Domain string `json:"domain"`
	}
	if !decode(w, r, &p) {
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Autojoin unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	var parent string
	err = tx.QueryRow(ctx, `SELECT c.id::text FROM credentials c JOIN public_enrollments e ON e.credential_id=c.id WHERE c.id=$1 AND c.revoked_at IS NULL FOR UPDATE OF c`, c.CredID).Scan(&parent)
	if err != nil {
		fail(w, 403, "installation_required", "Use the original installation account.")
		return
	}
	if p.Domain == "" {
		_, err = tx.Exec(ctx, `DELETE FROM fleet_defaults WHERE parent_credential_id=$1`, c.CredID)
	} else {
		var domain string
		err = tx.QueryRow(ctx, `SELECT domain FROM verified_domains WHERE domain=$1 AND org_id=$2 AND verified_until>now() FOR SHARE`, p.Domain, c.OrgID).Scan(&domain)
		if err != nil {
			fail(w, 403, "verified_domain_required", "Verify this domain for your organization first.")
			return
		}
		_, err = tx.Exec(ctx, `INSERT INTO fleet_defaults(parent_credential_id,domain) VALUES($1,$2) ON CONFLICT(parent_credential_id) DO UPDATE SET domain=excluded.domain`, c.CredID, domain)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO agent_domains(agent_id,domain) SELECT c.agent_id,$2 FROM fleet_members f JOIN credentials c ON c.id=f.credential_id JOIN agents a ON a.id=c.agent_id WHERE f.parent_credential_id=$1 AND a.org_id=$3 AND a.status='active' AND c.revoked_at IS NULL ON CONFLICT(agent_id) DO UPDATE SET domain=excluded.domain`, c.CredID, domain, c.OrgID)
		}
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,'fleet.autojoin','credential',$2)`, c.AgentID, c.CredID)
	}
	if err != nil || tx.Commit(ctx) != nil {
		fail(w, 503, "unavailable", "Autojoin could not be saved.")
		return
	}
	writeJSON(w, 200, map[string]any{"domain": p.Domain, "enabled": p.Domain != ""})
}
