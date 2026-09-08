package web

import (
	"ajent/internal/auth"
	"ajent/internal/store"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var linkHandle = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,39}$`)
var linkScope = regexp.MustCompile(`^project:[a-f0-9]{64}$`)

// InstallationLink starts a consent flow. Possession of an installation key is
// proof for claiming that installation, never a human login credential.
func (s *Server) InstallationLink(w http.ResponseWriter, r *http.Request) {
	if !s.humanRate(w, r) {
		return
	}
	lookup, secret, ok := auth.ParseToken(r.Header.Get("Authorization"))
	decoded, err := base64.RawURLEncoding.DecodeString(secret)
	lid, e := hex.DecodeString(lookup)
	if !ok || err != nil || e != nil || len(decoded) != 32 || len(lid) != 8 || len(secret) != 43 {
		http.Error(w, "Installation key required.", 401)
		return
	}
	var p struct {
		Handle  string `json:"handle"`
		Project string `json:"project_scope"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&p) != nil || !linkHandle.MatchString(p.Handle) || !linkScope.MatchString(p.Project) {
		http.Error(w, "Choose an installation name and project scope.", 400)
		return
	}
	tx, err := s.API.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "Linking unavailable.", 503)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "installation:"+lookup); err != nil {
		http.Error(w, "Linking unavailable.", 503)
		return
	}
	var existing *string
	var digest []byte
	err = tx.QueryRow(r.Context(), `SELECT c.id::text,c.secret_digest FROM credentials c WHERE c.lookup_id=$1`, lookup).Scan(&existing, &digest)
	if err == nil {
		credential, e := s.API.DB.Authenticate(r.Context(), lookup, secret)
		if e != nil {
			http.Error(w, "Installation unavailable.", 401)
			return
		}
		var root bool
		if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM public_enrollments WHERE credential_id=$1)`, credential.CredID).Scan(&root); e != nil || !root {
			http.Error(w, "Use the original installation key.", 403)
			return
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "Linking unavailable.", 503)
		return
	}
	token, err := random()
	if err != nil {
		http.Error(w, "Linking unavailable.", 503)
		return
	}
	// A conflicting pending key cannot be overwritten by someone guessing its lookup ID.
	tag, err := tx.Exec(r.Context(), `INSERT INTO installation_links(lookup_id,secret_digest,approval_digest,handle,existing_credential,project_scope) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(lookup_id) DO UPDATE SET approval_digest=$3,handle=$4,existing_credential=$5,project_scope=$6,approved_credential=NULL,approved_scope='',approved_domain='',expires_at=now()+interval '10 minutes' WHERE installation_links.secret_digest=$2`, lookup, auth.Digest(secret), auth.Digest(token), p.Handle, existing, p.Project)
	if err != nil || tag.RowsAffected() != 1 || tx.Commit(r.Context()) != nil {
		http.Error(w, "Linking could not start.", 409)
		return
	}
	browserJSON(w, 200, map[string]any{"url": s.Origin + "/human/link#" + token, "expires_in": 600, "code": hex.EncodeToString(auth.Digest(token))[:8]})
}
func (s *Server) InstallationStatus(w http.ResponseWriter, r *http.Request) {
	lookup, secret, ok := auth.ParseToken(r.Header.Get("Authorization"))
	if !ok {
		http.Error(w, "Installation key required.", 401)
		return
	}
	var cid, scope, domain string
	var approved bool
	err := s.API.DB.Pool.QueryRow(r.Context(), `SELECT coalesce(approved_credential::text,''),approved_credential IS NOT NULL,approved_scope,approved_domain FROM installation_links WHERE lookup_id=$1 AND secret_digest=$2 AND expires_at>now()`, lookup, auth.Digest(secret)).Scan(&cid, &approved, &scope, &domain)
	if err != nil {
		http.Error(w, "Link request expired. Start again.", 410)
		return
	}
	if !approved {
		browserJSON(w, 200, map[string]string{"status": "pending"})
		return
	}
	c, err := s.API.DB.Authenticate(r.Context(), lookup, secret)
	if err != nil || c.CredID != cid {
		http.Error(w, "Installation was revoked.", 401)
		return
	}
	browserJSON(w, 200, map[string]string{"status": "approved", "agent_id": c.AgentID, "credential_id": c.CredID, "scope": scope, "domain": domain})
}
func (s *Server) humanLinkPage(w http.ResponseWriter, r *http.Request) {
	// The pending proof stays in the fragment/sessionStorage until the human signs in.
	s.humanPage(w, r)
}
func (s *Server) humanLinkPreview(w http.ResponseWriter, r *http.Request) {
	if !s.humanCSRF(w, r) {
		return
	}
	h := s.human(r)
	if h == nil {
		http.Error(w, "Sign in to approve this installation.", 401)
		return
	}
	var p struct{ Token string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&p) != nil || len(p.Token) != 43 {
		http.Error(w, "Invalid link.", 400)
		return
	}
	var handle string
	var org *string
	err := s.API.DB.Pool.QueryRow(r.Context(), `SELECT l.handle,a.org_id::text FROM installation_links l LEFT JOIN credentials c ON c.id=l.existing_credential LEFT JOIN agents a ON a.id=c.agent_id WHERE l.approval_digest=$1 AND l.expires_at>now() AND l.approved_credential IS NULL`, auth.Digest(p.Token)).Scan(&handle, &org)
	if err != nil {
		http.Error(w, "Link expired or already used.", 410)
		return
	}
	rows, err := s.API.DB.Pool.Query(r.Context(), `SELECT id::text,name FROM organizations WHERE human_owner_id=$1 AND status='active' ORDER BY name,id`, h.ID)
	if err != nil {
		http.Error(w, "Workspaces unavailable.", 503)
		return
	}
	workspaces := []map[string]string{}
	for rows.Next() {
		var id, name string
		if err = rows.Scan(&id, &name); err != nil {
			break
		}
		workspaces = append(workspaces, map[string]string{"id": id, "name": name})
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		http.Error(w, "Workspaces unavailable.", 503)
		return
	}
	if org != nil {
		var owner *string
		var name string
		err = s.API.DB.Pool.QueryRow(r.Context(), `SELECT human_owner_id::text,name FROM organizations WHERE id=$1 AND status='active'`, *org).Scan(&owner, &name)
		if err != nil || owner != nil && *owner != h.ID {
			http.Error(w, "This installation belongs to another human account.", 403)
			return
		}
		workspaces = []map[string]string{{"id": *org, "name": name}}
	}
	availableDomains := map[string][]string{}
	for _, workspace := range workspaces {
		rows, err = s.API.DB.Pool.Query(r.Context(), `SELECT domain FROM verified_domains WHERE org_id=$1 AND verified_until>now() ORDER BY domain`, workspace["id"])
		if err != nil {
			http.Error(w, "Domains unavailable.", 503)
			return
		}
		for rows.Next() {
			var domain string
			if err = rows.Scan(&domain); err != nil {
				break
			}
			availableDomains[workspace["id"]] = append(availableDomains[workspace["id"]], domain)
		}
		rows.Close()
		if err != nil || rows.Err() != nil {
			http.Error(w, "Domains unavailable.", 503)
			return
		}
	}
	browserJSON(w, 200, map[string]any{"handle": handle, "existing": org != nil, "workspaces": workspaces, "code": hex.EncodeToString(auth.Digest(p.Token))[:8], "csrf": h.CSRF, "domains": availableDomains})
}
func (s *Server) humanLinkApprove(w http.ResponseWriter, r *http.Request) {
	h := s.humanAction(w, r)
	if h == nil {
		return
	}
	token := r.PostForm.Get("token")
	if len(token) != 43 || r.PostForm.Get("confirm") != "yes" {
		http.Error(w, "Confirm this installation explicitly.", 400)
		return
	}
	tx, err := s.API.DB.Pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "Approval unavailable.", 503)
		return
	}
	defer tx.Rollback(r.Context())
	var lookup, handle, project string
	var digest []byte
	var existing *string
	err = tx.QueryRow(r.Context(), `SELECT lookup_id,secret_digest,handle,existing_credential::text,project_scope FROM installation_links WHERE approval_digest=$1 AND expires_at>now() AND approved_credential IS NULL FOR UPDATE`, auth.Digest(token)).Scan(&lookup, &digest, &handle, &existing, &project)
	if err != nil {
		http.Error(w, "Link expired or already used.", 410)
		return
	}
	org := r.PostForm.Get("workspace")
	cid := ""
	aid := ""
	if existing != nil {
		cid = *existing
		err = tx.QueryRow(r.Context(), `SELECT a.org_id::text,a.id::text FROM credentials c JOIN agents a ON a.id=c.agent_id WHERE c.id=$1 AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()) AND a.status='active' FOR UPDATE OF c`, cid).Scan(&org, &aid)
		if err != nil {
			http.Error(w, "Installation was revoked.", 403)
			return
		}
		// Never move existing content, credentials, domains, or groups to another org.
		if selected := r.PostForm.Get("workspace"); selected != "" && selected != org {
			http.Error(w, "Keep this installation's existing workspace.", 409)
			return
		}
	} else if org == "" {
		org = uuid.NewString()
		name := strings.TrimSpace(r.PostForm.Get("name"))
		if name == "" {
			name = h.Name + "'s workspace"
		}
		if len(name) > 100 {
			http.Error(w, "Workspace name too long.", 400)
			return
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO organizations(id,name,namespace,human_owner_id) VALUES($1,$2,$3,$4)`, org, name, "u-"+org+".ajent.social", h.ID)
	} else if !validUUID(org) {
		http.Error(w, "Choose a workspace.", 400)
		return
	}
	if err != nil {
		http.Error(w, "Workspace unavailable.", 503)
		return
	}
	var owner *string
	err = tx.QueryRow(r.Context(), `SELECT human_owner_id::text FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, org).Scan(&owner)
	if err != nil || owner != nil && *owner != h.ID || owner == nil && existing == nil {
		http.Error(w, "You do not own this workspace.", 403)
		return
	}
	if owner == nil {
		_, err = tx.Exec(r.Context(), `UPDATE organizations SET human_owner_id=$2 WHERE id=$1`, org, h.ID)
	}
	if err == nil && existing == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO agents(org_id,handle,display_name) VALUES($1,$2,$2) RETURNING id::text`, org, handle).Scan(&aid)
		if err == nil {
			err = tx.QueryRow(r.Context(), `INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,ARRAY['network:read','posts:write','social:write','groups:manage']) RETURNING id::text`, aid, lookup, digest).Scan(&cid)
		}
		if err == nil {
			_, err = tx.Exec(r.Context(), `INSERT INTO public_enrollments(credential_id) VALUES($1)`, cid)
		}
		// Reuse the workspace's existing private exchange when available.
		var group string
		if err == nil {
			err = tx.QueryRow(r.Context(), `SELECT fw.group_id::text FROM fleet_workspaces fw JOIN credentials c ON c.id=fw.parent_credential_id JOIN agents a ON a.id=c.agent_id WHERE a.org_id=$1 ORDER BY c.created_at LIMIT 1`, org).Scan(&group)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(r.Context(), `INSERT INTO groups(org_id,name,visibility) VALUES($1,$2,'private') RETURNING id::text`, org, "My agents "+uuid.NewString()[:8]).Scan(&group)
			}
			if err == nil {
				_, err = tx.Exec(r.Context(), `INSERT INTO fleet_workspaces(parent_credential_id,group_id) VALUES($1,$2)`, cid, group)
			}
			if err == nil {
				_, err = tx.Exec(r.Context(), `INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'owner','accepted') ON CONFLICT DO NOTHING`, group, aid)
			}
		}
	}
	domain := r.PostForm.Get("domain")
	scope := "user"
	if r.PostForm.Get("scope") == "project" {
		scope = project
	} else if r.PostForm.Get("scope") != "user" {
		http.Error(w, "Choose user or project scope.", 400)
		return
	}
	if err == nil && domain != "" {
		var verified string
		err = tx.QueryRow(r.Context(), `SELECT domain FROM verified_domains WHERE domain=$1 AND org_id=$2 AND verified_until>now() FOR SHARE`, domain, org).Scan(&verified)
		if err != nil {
			http.Error(w, "Choose a currently verified domain in this workspace, or private mode.", 403)
			return
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO domain_grants(parent_credential_id,domain,scope_key) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, cid, domain, scope)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO human_installations(credential_id,human_id) VALUES($1,$2) ON CONFLICT(credential_id) DO NOTHING`, cid, h.ID)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE installation_links SET approved_credential=$2,approved_scope=$3,approved_domain=$4 WHERE lookup_id=$1`, lookup, cid, scope, domain)
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), `INSERT INTO human_audit(human_id,action,object_id) VALUES($1,'installation.approved',$2)`, h.ID, cid)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		http.Error(w, "Approval could not be saved. Retry or choose another installation name.", 409)
		return
	}
	redirect(w, r, humanWorkspaceURL(org, "Installation approved"))
}

// A linked agent key may identify the human login page, but cannot authenticate as its owner.
func (s *Server) linkedInstallation(r *http.Request, c store.Credential) (bool, error) {
	var linked bool
	err := s.API.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM human_installations hi WHERE hi.credential_id=$1 OR hi.credential_id IN (SELECT parent_credential_id FROM fleet_members WHERE credential_id=$1))`, c.CredID).Scan(&linked)
	return linked, err
}
