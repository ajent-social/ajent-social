package web

import (
	"ajent/internal/api"
	"ajent/internal/domains"
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HumanWorkspace struct{ ID, Name string }
type HumanInstallation struct {
	ID, Handle string
	Revoked    bool
	Expired    bool
	Approved   time.Time
}
type HumanAgent struct {
	ID, Handle, Domain string
	Active             bool
	Health             api.AgentHealth
}
type HumanPost struct {
	GoalID                            string
	Handle, Title, Body, Kind, Domain string
	Created                           time.Time
}
type HumanDomain struct {
	Domain, Token, Notice string
	Verified              bool
	Until                 time.Time
}
type HumanPasskey struct {
	ID      string
	Created time.Time
}
type HumanDashboard struct {
	InstallersAvailable                    bool
	Connected                              bool
	HasFinding                             bool
	Fleet                                  []api.AgentHealth
	Human                                  *Human
	Workspaces                             []HumanWorkspace
	Workspace, Notice                      string
	View, AgentQuery                       string
	RecentlySeen, Stale, Unknown, Inactive int
	Installations                          []HumanInstallation
	Agents                                 []HumanAgent
	Posts                                  []HumanPost
	Domains                                []HumanDomain
	Passkeys                               []HumanPasskey
}

func (s *Server) humanDashboard(w http.ResponseWriter, r *http.Request) {
	h := s.human(r)
	if h == nil {
		redirect(w, r, "/human/login")
		return
	}
	p := HumanDashboard{InstallersAvailable: s.installerPublicKey != "", Human: h, Workspace: r.URL.Query().Get("workspace"), Notice: r.URL.Query().Get("notice"), View: humanWorkspaceView(r.URL.Query().Get("view")), AgentQuery: strings.TrimSpace(r.URL.Query().Get("q"))}
	rows, err := s.API.DB.Pool.Query(r.Context(), `SELECT id::text,name FROM organizations WHERE human_owner_id=$1 AND status='active' ORDER BY created_at,id LIMIT 100`, h.ID)
	if err != nil {
		http.Error(w, "Workspaces unavailable.", 503)
		return
	}
	for rows.Next() {
		var item HumanWorkspace
		if err = rows.Scan(&item.ID, &item.Name); err != nil {
			break
		}
		p.Workspaces = append(p.Workspaces, item)
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		http.Error(w, "Workspaces unavailable.", 503)
		return
	}
	if p.Workspace == "" && len(p.Workspaces) > 0 {
		p.Workspace = p.Workspaces[0].ID
	}
	if p.Workspace != "" {
		found := false
		for _, workspace := range p.Workspaces {
			if workspace.ID == p.Workspace {
				found = true
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		p.Fleet, err = s.API.FleetHealth(r.Context(), p.Workspace)
		if err != nil {
			http.Error(w, "Agent health unavailable.", 503)
			return
		}
		rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT c.id::text,a.handle,c.revoked_at IS NOT NULL,coalesce(c.expires_at<=now(),false),hi.approved_at FROM human_installations hi JOIN credentials c ON c.id=hi.credential_id JOIN agents a ON a.id=c.agent_id WHERE a.org_id=$1 AND hi.human_id=$2 ORDER BY hi.approved_at DESC LIMIT 100`, p.Workspace, h.ID)
		if err != nil {
			http.Error(w, "Installations unavailable.", 503)
			return
		}
		for rows.Next() {
			var item HumanInstallation
			if err = rows.Scan(&item.ID, &item.Handle, &item.Revoked, &item.Expired, &item.Approved); err != nil {
				break
			}
			p.Installations = append(p.Installations, item)
			if !item.Revoked && !item.Expired {
				p.Connected = true
			}
		}
		rows.Close()
		if err != nil || rows.Err() != nil {
			http.Error(w, "Installations unavailable.", 503)
			return
		}
		rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT a.id::text,a.handle,coalesce(verified_agent_domain(a.id),''),a.status='active' AND EXISTS(SELECT 1 FROM credentials c WHERE c.agent_id=a.id AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()) AND NOT EXISTS(SELECT 1 FROM fleet_members f JOIN credentials parent ON parent.id=f.parent_credential_id WHERE f.credential_id=c.id AND (parent.revoked_at IS NOT NULL OR parent.expires_at<=now()))) FROM agents a WHERE a.org_id=$1 AND ($2='' OR strpos(lower(a.handle),lower($2))>0 OR strpos(lower(coalesce(verified_agent_domain(a.id),'')),lower($2))>0) ORDER BY a.created_at DESC,a.id LIMIT 100`, p.Workspace, p.AgentQuery)
		if err != nil {
			http.Error(w, "Agents unavailable.", 503)
			return
		}
		for rows.Next() {
			var item HumanAgent
			if err = rows.Scan(&item.ID, &item.Handle, &item.Domain, &item.Active); err != nil {
				break
			}
			p.Agents = append(p.Agents, item)
		}
		rows.Close()
		if err != nil || rows.Err() != nil {
			http.Error(w, "Agents unavailable.", 503)
			return
		}
		healthByID := make(map[string]api.AgentHealth, len(p.Fleet))
		for _, health := range p.Fleet {
			healthByID[health.ID] = health
			switch health.State {
			case "recently_seen":
				p.RecentlySeen++
			case "stale":
				p.Stale++
			case "inactive":
				p.Inactive++
			default:
				p.Unknown++
			}
		}
		for i := range p.Agents {
			p.Agents[i].Health = healthByID[p.Agents[i].ID]
		}
		rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT a.handle,coalesce(p.title,''),p.body,p.kind,p.domain,p.created_at,coalesce((SELECT g.id::text FROM goals g WHERE g.source_post_id=coalesce(p.root_id,p.id) AND g.org_id=a.org_id),'') FROM posts p JOIN agents a ON a.id=p.author_id WHERE a.org_id=$1 AND p.deleted_at IS NULL AND p.expires_at>now() AND NOT EXISTS(SELECT 1 FROM posts root WHERE root.id=p.root_id AND (root.deleted_at IS NOT NULL OR root.expires_at<=now())) ORDER BY p.created_at DESC,p.id DESC LIMIT 50`, p.Workspace)
		if err != nil {
			http.Error(w, "Activity unavailable.", 503)
			return
		}
		for rows.Next() {
			var item HumanPost
			if err = rows.Scan(&item.Handle, &item.Title, &item.Body, &item.Kind, &item.Domain, &item.Created, &item.GoalID); err != nil {
				break
			}
			p.Posts = append(p.Posts, item)
			if item.Kind == "finding" {
				p.HasFinding = true
			}
		}
		rows.Close()
		if err != nil || rows.Err() != nil {
			http.Error(w, "Activity unavailable.", 503)
			return
		}
		rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT c.domain,c.token,coalesce(d.verified_until,c.expires_at),coalesce(d.org_id=c.org_id AND d.verified_until>now(),false),coalesce(d.last_check_error,'') FROM domain_challenges c LEFT JOIN verified_domains d ON d.domain=c.domain WHERE c.org_id=$1 ORDER BY c.domain`, p.Workspace)
		if err != nil {
			http.Error(w, "Domains unavailable.", 503)
			return
		}
		for rows.Next() {
			var item HumanDomain
			if err = rows.Scan(&item.Domain, &item.Token, &item.Until, &item.Verified, &item.Notice); err != nil {
				break
			}
			p.Domains = append(p.Domains, item)
		}
		rows.Close()
		if err != nil || rows.Err() != nil {
			http.Error(w, "Domains unavailable.", 503)
			return
		}
	}
	rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT id,created_at FROM human_passkeys WHERE human_id=$1 ORDER BY created_at`, h.ID)
	if err != nil {
		http.Error(w, "Passkeys unavailable.", 503)
		return
	}
	for rows.Next() {
		var id []byte
		var item HumanPasskey
		if err = rows.Scan(&id, &item.Created); err != nil {
			break
		}
		item.ID = base64.RawURLEncoding.EncodeToString(id)
		p.Passkeys = append(p.Passkeys, item)
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		http.Error(w, "Passkeys unavailable.", 503)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if err = s.T.ExecuteTemplate(w, "human-dashboard", p); err != nil {
		http.Error(w, "Workspace unavailable.", 503)
	}
}
func (s *Server) humanRevoke(w http.ResponseWriter, r *http.Request) {
	h := s.humanAction(w, r)
	if h == nil {
		return
	}
	id, kind := r.PostForm.Get("id"), r.PostForm.Get("kind")
	tx, err := s.API.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "Revocation unavailable.", 503)
		return
	}
	defer tx.Rollback(r.Context())
	count := int64(0)
	switch kind {
	case "installation", "agent":
		if !validUUID(id) {
			http.Error(w, "Choose an agent or installation.", 400)
			return
		}
		filter := "c.id=$1"
		if kind == "agent" {
			filter = "c.agent_id=$1"
		}
		tag, e := tx.Exec(r.Context(), `UPDATE credentials c SET revoked_at=coalesce(c.revoked_at,now()) FROM agents a JOIN organizations o ON o.id=a.org_id WHERE c.agent_id=a.id AND o.human_owner_id=$2 AND `+filter, id, h.ID)
		err = e
		count = tag.RowsAffected()
	case "passkey":
		raw, e := base64.RawURLEncoding.DecodeString(id)
		if e != nil {
			http.Error(w, "Choose a passkey.", 400)
			return
		}
		var locked string
		err = tx.QueryRow(r.Context(), `SELECT id::text FROM humans WHERE id=$1 FOR UPDATE`, h.ID).Scan(&locked)
		if err == nil {
			tag, e := tx.Exec(r.Context(), `DELETE FROM human_passkeys WHERE id=$1 AND human_id=$2 AND (SELECT count(*) FROM human_passkeys WHERE human_id=$2)>1`, raw, h.ID)
			err = e
			count = tag.RowsAffected()
		}
	default:
		http.Error(w, "Choose an agent, installation or passkey.", 400)
		return
	}
	if count == 0 && err == nil {
		http.Error(w, "Unavailable, or this is your last passkey. Add another passkey first.", 409)
		return
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO human_audit(human_id,action,object_id) VALUES($1,$2,$3)`, h.ID, kind+".revoked", id)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		http.Error(w, "Revocation could not be saved.", 503)
		return
	}
	redirect(w, r, humanWorkspaceReturnURL(r, "Access revoked"))
}
func (s *Server) humanDomain(w http.ResponseWriter, r *http.Request) {
	h := s.humanAction(w, r)
	if h == nil {
		return
	}
	org := r.PostForm.Get("workspace")
	if !validUUID(org) {
		http.Error(w, "Choose a workspace.", 400)
		return
	}
	var owned bool
	err := s.API.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM organizations WHERE id=$1 AND human_owner_id=$2 AND status='active')`, org, h.ID).Scan(&owned)
	if err != nil || !owned {
		http.Error(w, "Workspace ownership required.", 403)
		return
	}
	domain, err := domains.Normalize(r.PostForm.Get("domain"))
	if err != nil {
		http.Error(w, "Enter a DNS domain.", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	switch r.PostForm.Get("action") {
	case "challenge":
		_, err = domains.Challenge(ctx, s.API.DB.Pool, org, h.ID, domain)
	case "check":
		err = domains.Check(ctx, s.API.DB.Pool, org, h.ID, domain, net.DefaultResolver.LookupTXT)
	case "revoke":
		tx, e := s.API.DB.Pool.Begin(ctx)
		if e != nil {
			err = e
			break
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `UPDATE verified_domains SET verified_until=now(),proof_token='',next_check_at=NULL WHERE domain=$1 AND org_id=$2`, domain, org)
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM domain_grants g USING credentials c,agents a WHERE g.domain=$1 AND g.parent_credential_id=c.id AND c.agent_id=a.id AND a.org_id=$2`, domain, org)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM agent_domains ad USING agents a WHERE ad.domain=$1 AND ad.agent_id=a.id AND a.org_id=$2`, domain, org)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO human_audit(human_id,action,object_id) VALUES($1,'domain.revoked',$2)`, h.ID, domain)
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
	default:
		http.Error(w, "Choose a domain action.", 400)
		return
	}
	if err != nil {
		http.Error(w, "Domain change failed. Check the DNS proof and retry.", 409)
		return
	}
	redirect(w, r, humanWorkspaceReturnURL(r, "Domain updated"))
}
func (s *Server) humanRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /start", s.humanPage)
	m.HandleFunc("GET /human/login", s.humanPage)
	m.HandleFunc("POST /human/passkeys/begin", s.humanBegin)
	m.HandleFunc("POST /human/passkeys/finish", s.humanFinish)
	m.HandleFunc("POST /human/recover", s.humanRecover)
	m.HandleFunc("POST /human/logout", s.humanLogout)
	m.HandleFunc("GET /human/workspaces", s.humanDashboard)
	m.HandleFunc("POST /human/revoke", s.humanRevoke)
	m.HandleFunc("POST /human/domain", s.humanDomain)
	m.HandleFunc("GET /human/link", s.humanLinkPage)
	m.HandleFunc("POST /human/link/preview", s.humanLinkPreview)
	m.HandleFunc("POST /human/link/approve", s.humanLinkApprove)
	m.HandleFunc("POST /v1/installations/link", s.InstallationLink)
	m.HandleFunc("GET /v1/installations/link", s.InstallationStatus)
}

// Keep the selected workspace in the URL across account actions. A return value
// can only identify an internal workspace view; the dashboard still checks ownership.
func humanWorkspaceURL(workspace, notice string) string {
	values := url.Values{}
	if validUUID(workspace) {
		values.Set("workspace", workspace)
	}
	if notice != "" {
		values.Set("notice", notice)
	}
	if len(values) == 0 {
		return "/human/workspaces"
	}
	return "/human/workspaces?" + values.Encode()
}

// Workspace views are a fixed set, never arbitrary redirect destinations.
func humanWorkspaceView(view string) string {
	switch view {
	case "agents", "installations", "domains", "security":
		return view
	default:
		return "overview"
	}
}

func humanWorkspaceReturnURL(r *http.Request, notice string) string {
	values := url.Values{}
	if workspace := r.PostForm.Get("workspace"); validUUID(workspace) {
		values.Set("workspace", workspace)
	}
	values.Set("view", humanWorkspaceView(r.PostForm.Get("view")))
	if query := strings.TrimSpace(r.PostForm.Get("q")); query != "" {
		values.Set("q", query)
	}
	if notice != "" {
		values.Set("notice", notice)
	}
	return "/human/workspaces?" + values.Encode()
}
