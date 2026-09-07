package web

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type verifiedDomain struct {
	Domain, Token, Notice string
	Until                 time.Time
	Verified              bool
}
type scopedGrant struct{ ID, Domain, Scope string }
type verifiedAgent struct{ ID, Handle, Domain string }
type verificationPage struct {
	Session *Session
	Domains []verifiedDomain
	Agents  []verifiedAgent
	Notice  string
	Grants  []scopedGrant
}

var domainName = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func (s *Server) Verification(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	v := s.session(r)
	if v == nil {
		redirect(w, r, "/login")
		return
	}
	if linked, e := s.linkedInstallation(r, v.Credential); e != nil {
		http.Error(w, "Ownership unavailable.", 503)
		return
	} else if linked && !v.Owner {
		http.Error(w, "Manage domain access from your human account at /human/workspaces.", 403)
		return
	}
	if !s.directoryManager(r, v) {
		http.Error(w, "Use your original installation account to manage domain verification.", 403)
		return
	}
	p := verificationPage{Session: v}
	if r.URL.Query().Get("saved") == "1" {
		p.Notice = "Saved. Only verified, explicitly authorized agents can participate publicly."
	}
	rows, err := s.API.DB.Pool.Query(r.Context(), `SELECT c.domain,c.token,coalesce(d.verified_until,c.expires_at),coalesce(d.org_id=c.org_id AND d.verified_until>now(),false),coalesce(d.last_check_error,'') FROM domain_challenges c LEFT JOIN verified_domains d ON d.domain=c.domain WHERE c.org_id=$1 ORDER BY c.domain`, v.Credential.OrgID)
	if err != nil {
		http.Error(w, "Verification unavailable.", 503)
		return
	}
	for rows.Next() {
		var d verifiedDomain
		if err = rows.Scan(&d.Domain, &d.Token, &d.Until, &d.Verified, &d.Notice); err != nil {
			break
		}
		p.Domains = append(p.Domains, d)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		http.Error(w, "Verification unavailable.", 503)
		return
	}
	rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT id::text,handle,coalesce(verified_agent_domain(id),'') FROM agents WHERE org_id=$1 AND status='active' ORDER BY handle LIMIT 100`, v.Credential.OrgID)
	if err != nil {
		http.Error(w, "Agents unavailable.", 503)
		return
	}
	for rows.Next() {
		var a verifiedAgent
		if err = rows.Scan(&a.ID, &a.Handle, &a.Domain); err != nil {
			break
		}
		p.Agents = append(p.Agents, a)
	}
	rowErr = rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		http.Error(w, "Agents unavailable.", 503)
		return
	}
	rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT g.id::text,g.domain,g.scope_key FROM domain_grants g JOIN credentials c ON c.id=g.parent_credential_id JOIN agents a ON a.id=c.agent_id WHERE a.org_id=$1 ORDER BY g.domain,g.scope_key`, v.Credential.OrgID)
	if err != nil {
		http.Error(w, "Grants unavailable.", 503)
		return
	}
	for rows.Next() {
		var g scopedGrant
		if err = rows.Scan(&g.ID, &g.Domain, &g.Scope); err != nil {
			break
		}
		if g.Scope == "user" {
			g.Scope = "All projects"
		} else {
			g.Scope = "One project"
		}
		p.Grants = append(p.Grants, g)
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		http.Error(w, "Grants unavailable.", 503)
		return
	}
	var b bytes.Buffer
	if err = s.T.ExecuteTemplate(&b, "verification", p); err != nil {
		http.Error(w, "Page unavailable.", 503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b.Bytes())
}

func (s *Server) VerifyDomain(w http.ResponseWriter, r *http.Request) {
	s.verifyDomain(w, r, net.DefaultResolver.LookupTXT)
}
func (s *Server) verifyDomain(w http.ResponseWriter, r *http.Request, lookup func(context.Context, string) ([]string, error)) {
	w.Header().Set("Cache-Control", "no-store")
	v := s.validAction(w, r)
	if v == nil {
		return
	}
	if linked, e := s.linkedInstallation(r, v.Credential); e != nil {
		http.Error(w, "Ownership unavailable.", 503)
		return
	} else if linked && !v.Owner {
		http.Error(w, "Manage domain access from your human account at /human/workspaces.", 403)
		return
	}
	if !s.directoryManager(r, v) {
		http.Error(w, "Original installation account required.", 403)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.PostForm.Get("domain")), "."))
	action := r.PostForm.Get("action")
	if action != "unassign" && action != "revoke-grant" && (len(domain) > 253 || !domainName.MatchString(domain) || net.ParseIP(domain) != nil) {
		http.Error(w, "Enter a DNS domain, such as example.com, without a URL or port.", 400)
		return
	}
	org := v.Credential.OrgID
	var token string
	if action == "check" {
		if err := s.API.DB.Pool.QueryRow(ctx, `SELECT token FROM domain_challenges WHERE org_id=$1 AND domain=$2 AND expires_at>now()`, org, domain).Scan(&token); err != nil {
			http.Error(w, "Create or renew the DNS challenge first.", 400)
			return
		}
		records, err := lookup(ctx, "_ajent-verification."+domain)
		found := false
		for _, record := range records {
			if record == "ajent-verification="+token {
				found = true
			}
		}
		if err != nil || !found {
			http.Error(w, "DNS proof not found. Add the exact TXT record and retry after DNS propagation. No verification was granted.", 409)
			return
		}
	}
	tx, err := s.API.DB.Pool.Begin(ctx)
	if err != nil {
		http.Error(w, "Verification unavailable.", 503)
		return
	}
	defer tx.Rollback(ctx)
	// Serialize all changes to this organization's authority.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, org).Scan(&locked); err != nil {
		http.Error(w, "Organization unavailable.", 403)
		return
	}
	switch action {
	case "revoke-grant":
		id := r.PostForm.Get("grant")
		if !validUUID(id) {
			http.Error(w, "Choose a grant.", 400)
			return
		}
		_, err = tx.Exec(ctx, `DELETE FROM domain_grants g USING credentials c,agents a WHERE g.id=$1 AND g.parent_credential_id=c.id AND c.agent_id=a.id AND a.org_id=$2`, id, org)
		domain = id
	case "challenge":
		var count int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM domain_challenges WHERE org_id=$1 AND domain<>$2`, org, domain).Scan(&count)
		if err != nil {
			break
		}
		if count >= 20 {
			http.Error(w, "Maximum 20 domain claims per organization.", 409)
			return
		}
		token, err = random()
		if err != nil {
			break
		}
		_, err = tx.Exec(ctx, `INSERT INTO domain_challenges(org_id,domain,token) VALUES($1,$2,$3) ON CONFLICT(org_id,domain) DO UPDATE SET token=$3,expires_at=now()+interval '24 hours'`, org, domain, token)
	case "check":
		// Reject a rotated/expired challenge even if its DNS lookup just succeeded.
		var current string
		err = tx.QueryRow(ctx, `SELECT token FROM domain_challenges WHERE org_id=$1 AND domain=$2 AND token=$3 AND expires_at>now() FOR UPDATE`, org, domain, token).Scan(&current)
		if err != nil {
			break
		}
		tag, e := tx.Exec(ctx, `INSERT INTO verified_domains(domain,org_id,verified_until,proof_token,next_check_at) VALUES($1,$2,now()+interval '30 days',$3,now()+interval '1 day') ON CONFLICT(domain) DO UPDATE SET org_id=$2,verified_until=now()+interval '30 days',proof_token=$3,next_check_at=now()+interval '1 day',last_check_error='' WHERE verified_domains.org_id=$2 OR verified_domains.verified_until<=now()`, domain, org, token)
		err = e
		if err == nil && tag.RowsAffected() == 0 {
			http.Error(w, "This domain is already verified by another organization. Its administrator must release it or its verification must expire.", 409)
			return
		}
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM domain_grants g USING credentials c,agents a WHERE g.parent_credential_id=c.id AND c.agent_id=a.id AND g.domain=$1 AND a.org_id<>$2`, domain, org)
		}
		// Ownership transfer must never revive the previous organization's assignments.
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM agent_domains ad USING agents a WHERE ad.agent_id=a.id AND ad.domain=$1 AND a.org_id<>$2`, domain, org)
		}
	case "assign":
		id := r.PostForm.Get("agent")
		if !validUUID(id) {
			http.Error(w, "Choose an agent.", 400)
			return
		}
		var aid string
		err = tx.QueryRow(ctx, `SELECT a.id::text FROM agents a JOIN verified_domains d ON d.org_id=a.org_id WHERE a.id=$1 AND a.org_id=$2 AND a.status='active' AND d.domain=$3 AND d.verified_until>now() FOR SHARE OF d FOR UPDATE OF a`, id, org, domain).Scan(&aid)
		if err != nil {
			http.Error(w, "Choose your own agent and a currently verified domain.", 403)
			return
		}
		_, err = tx.Exec(ctx, `INSERT INTO agent_domains(agent_id,domain) VALUES($1,$2) ON CONFLICT(agent_id) DO UPDATE SET domain=$2,grant_id=NULL`, id, domain)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE agents SET public_posting=true,domain_scope_disabled=false WHERE id=$1`, id)
		}
	case "unassign":
		id := r.PostForm.Get("agent")
		if !validUUID(id) {
			http.Error(w, "Choose an agent.", 400)
			return
		}
		_, err = tx.Exec(ctx, `DELETE FROM agent_domain_grants ag USING agents a WHERE ag.agent_id=a.id AND a.id=$1 AND a.org_id=$2`, id, org)
		if err != nil {
			break
		}
		_, err = tx.Exec(ctx, `DELETE FROM agent_domains ad USING agents a WHERE ad.agent_id=a.id AND a.id=$1 AND a.org_id=$2`, id, org)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE agents SET public_posting=false,domain_scope_disabled=true WHERE id=$1 AND org_id=$2`, id, org)
		}
	case "revoke":
		_, err = tx.Exec(ctx, `DELETE FROM domain_grants g USING credentials c,agents a WHERE g.parent_credential_id=c.id AND c.agent_id=a.id AND g.domain=$1 AND a.org_id=$2`, domain, org)
		if err != nil {
			break
		}
		_, err = tx.Exec(ctx, `UPDATE verified_domains SET verified_until=now(),proof_token='',next_check_at=NULL WHERE domain=$1 AND org_id=$2`, domain, org)
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM agent_domains ad USING agents a WHERE ad.agent_id=a.id AND ad.domain=$1 AND a.org_id=$2`, domain, org)
		}
	default:
		http.Error(w, "Choose a verification action.", 400)
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,$2,'domain',$3)`, v.Credential.AgentID, "domain."+action, domain)
	}
	if err != nil || tx.Commit(ctx) != nil {
		http.Error(w, "Verification change could not be saved. Please retry.", 503)
		return
	}
	redirect(w, r, "/app/verification?saved=1")
}
