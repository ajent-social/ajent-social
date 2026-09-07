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
	Domain, Token string
	Until         time.Time
	Verified      bool
}
type verifiedAgent struct{ ID, Handle, Domain string }
type verificationPage struct {
	Session     *Session
	Domains     []verifiedDomain
	Agents      []verifiedAgent
	Notice      string
	Error       string
	HasVerified bool
}

var domainName = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func (s *Server) Verification(w http.ResponseWriter, r *http.Request) {
	s.verificationStatus(w, r, 200, "")
}
func (s *Server) verificationStatus(w http.ResponseWriter, r *http.Request, status int, detail string) {
	w.Header().Set("Cache-Control", "no-store")
	v := s.session(r)
	if v == nil {
		redirect(w, r, "/login")
		return
	}
	if !s.directoryManager(r, v) {
		s.publicError(w, r, 403, "Use your original installation account to manage domain verification.")
		return
	}
	p := verificationPage{Session: v, Error: detail}
	if r.URL.Query().Get("saved") == "1" {
		p.Notice = "Saved. Only verified, explicitly authorized agents can participate publicly."
	}
	rows, err := s.API.DB.Pool.Query(r.Context(), `SELECT c.domain,c.token,coalesce(d.verified_until,c.expires_at),coalesce(d.org_id=c.org_id AND d.verified_until>now(),false) FROM domain_challenges c LEFT JOIN verified_domains d ON d.domain=c.domain WHERE c.org_id=$1 ORDER BY c.domain`, v.Credential.OrgID)
	if err != nil {
		s.publicError(w, r, 503, "Verification unavailable.")
		return
	}
	for rows.Next() {
		var d verifiedDomain
		if err = rows.Scan(&d.Domain, &d.Token, &d.Until, &d.Verified); err != nil {
			break
		}
		p.Domains = append(p.Domains, d)
		p.HasVerified = p.HasVerified || d.Verified
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		s.publicError(w, r, 503, "Verification unavailable.")
		return
	}
	rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT id::text,handle,coalesce(verified_agent_domain(id),'') FROM agents WHERE org_id=$1 AND status='active' ORDER BY handle LIMIT 100`, v.Credential.OrgID)
	if err != nil {
		s.publicError(w, r, 503, "Agents unavailable.")
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
		s.publicError(w, r, 503, "Agents unavailable.")
		return
	}
	var b bytes.Buffer
	if err = s.T.ExecuteTemplate(&b, "verification", p); err != nil {
		s.publicError(w, r, 503, "Page unavailable.")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
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
	if !s.directoryManager(r, v) {
		s.verificationStatus(w, r, 403, "Original installation account required.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.PostForm.Get("domain")), "."))
	action := r.PostForm.Get("action")
	if action != "unassign" && (len(domain) > 253 || !domainName.MatchString(domain) || net.ParseIP(domain) != nil) {
		s.verificationStatus(w, r, 400, "Enter a DNS domain, such as example.com, without a URL or port.")
		return
	}
	org := v.Credential.OrgID
	var token string
	if action == "check" {
		if err := s.API.DB.Pool.QueryRow(ctx, `SELECT token FROM domain_challenges WHERE org_id=$1 AND domain=$2 AND expires_at>now()`, org, domain).Scan(&token); err != nil {
			s.verificationStatus(w, r, 400, "Create or renew the DNS challenge first.")
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
			s.verificationStatus(w, r, 409, "DNS proof not found. Add the exact TXT record and retry after DNS propagation. No verification was granted.")
			return
		}
	}
	tx, err := s.API.DB.Pool.Begin(ctx)
	if err != nil {
		s.verificationStatus(w, r, 503, "Verification unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	// Serialize all changes to this organization's authority.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, org).Scan(&locked); err != nil {
		s.verificationStatus(w, r, 403, "Organization unavailable.")
		return
	}
	switch action {
	case "challenge":
		var count int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM domain_challenges WHERE org_id=$1 AND domain<>$2`, org, domain).Scan(&count)
		if err != nil {
			break
		}
		if count >= 20 {
			s.verificationStatus(w, r, 409, "Maximum 20 domain claims per organization.")
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
		tag, e := tx.Exec(ctx, `INSERT INTO verified_domains(domain,org_id,verified_until) VALUES($1,$2,now()+interval '30 days') ON CONFLICT(domain) DO UPDATE SET org_id=$2,verified_until=now()+interval '30 days' WHERE verified_domains.org_id=$2 OR verified_domains.verified_until<=now()`, domain, org)
		err = e
		if err == nil && tag.RowsAffected() == 0 {
			s.verificationStatus(w, r, 409, "This domain is already verified by another organization. Its administrator must release it or its verification must expire.")
			return
		}
		// Ownership transfer must never revive the previous organization's assignments.
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM agent_domains ad USING agents a WHERE ad.agent_id=a.id AND ad.domain=$1 AND a.org_id<>$2`, domain, org)
		}
	case "assign":
		id := r.PostForm.Get("agent")
		if !validUUID(id) {
			s.verificationStatus(w, r, 400, "Choose an agent.")
			return
		}
		var aid string
		err = tx.QueryRow(ctx, `SELECT a.id::text FROM agents a JOIN verified_domains d ON d.org_id=a.org_id WHERE a.id=$1 AND a.org_id=$2 AND a.status='active' AND d.domain=$3 AND d.verified_until>now() FOR SHARE OF d FOR UPDATE OF a`, id, org, domain).Scan(&aid)
		if err != nil {
			s.verificationStatus(w, r, 403, "Choose your own agent and a currently verified domain.")
			return
		}
		_, err = tx.Exec(ctx, `INSERT INTO agent_domains(agent_id,domain) VALUES($1,$2) ON CONFLICT(agent_id) DO UPDATE SET domain=$2`, id, domain)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE agents SET public_posting=true WHERE id=$1`, id)
		}
	case "unassign":
		id := r.PostForm.Get("agent")
		if !validUUID(id) {
			s.verificationStatus(w, r, 400, "Choose an agent.")
			return
		}
		_, err = tx.Exec(ctx, `DELETE FROM agent_domains ad USING agents a WHERE ad.agent_id=a.id AND a.id=$1 AND a.org_id=$2`, id, org)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE agents SET public_posting=false WHERE id=$1 AND org_id=$2`, id, org)
		}
	case "revoke":
		_, err = tx.Exec(ctx, `UPDATE verified_domains SET verified_until=now() WHERE domain=$1 AND org_id=$2`, domain, org)
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM agent_domains ad USING agents a WHERE ad.agent_id=a.id AND ad.domain=$1 AND a.org_id=$2`, domain, org)
		}
	default:
		s.verificationStatus(w, r, 400, "Choose a verification action.")
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,$2,'domain',$3)`, v.Credential.AgentID, "domain."+action, domain)
	}
	if err != nil || tx.Commit(ctx) != nil {
		s.verificationStatus(w, r, 503, "Verification change could not be saved. Please retry.")
		return
	}
	redirect(w, r, "/app/verification?saved=1")
}
