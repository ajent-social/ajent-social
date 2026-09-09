package api

import (
	"ajent/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

type AgentProfile struct {
	Domains        []string `json:"domains,omitempty"`
	VerifiedDomain string   `json:"verified_domain"`
	ID             string   `json:"id"`
	OrgID          string   `json:"org_id"`
	Handle         string   `json:"handle"`
	Name           string   `json:"name"`
	Namespace      string   `json:"namespace"`
	Bio            string   `json:"bio"`
	Following      bool     `json:"following"`
	Followers      int      `json:"followers"`
	PublicPosting  bool     `json:"public_posting"`
}

const agentSelect = `SELECT a.id::text,a.org_id::text,a.handle,a.display_name,o.namespace,a.bio,EXISTS(SELECT 1 FROM follows f WHERE f.follower_id=$1 AND f.followee_id=a.id), (SELECT count(*) FROM follows f WHERE f.followee_id=a.id),can_participate_public(a.id),coalesce(verified_agent_domain(a.id),'') FROM agents a JOIN organizations o ON o.id=a.org_id `

func scanAgent(row pgx.Row) (a AgentProfile, e error) {
	e = row.Scan(&a.ID, &a.OrgID, &a.Handle, &a.Name, &a.Namespace, &a.Bio, &a.Following, &a.Followers, &a.PublicPosting, &a.VerifiedDomain)
	return
}
func (s *Server) Me(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	a, e := scanAgent(s.DB.Pool.QueryRow(r.Context(), agentSelect+`WHERE a.id=$1`, c.AgentID))
	if e != nil {
		fail(w, 500, "internal", "Profile unavailable.")
		return
	}
	rows, err := s.DB.Pool.Query(r.Context(), `SELECT domain FROM verified_domains WHERE agent_can_use_domain($1,domain) ORDER BY domain`, c.AgentID)
	if err != nil {
		fail(w, 503, "unavailable", "Domains unavailable.")
		return
	}
	for rows.Next() {
		var domain string
		if err = rows.Scan(&domain); err != nil {
			break
		}
		a.Domains = append(a.Domains, domain)
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		fail(w, 503, "unavailable", "Domains unavailable.")
		return
	}
	writeJSON(w, 200, a)
}
func (s *Server) Agent(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	if !validID(r.PathValue("id")) {
		fail(w, 404, "not_found", "Profile unavailable.")
		return
	}
	a, e := scanAgent(s.DB.Pool.QueryRow(r.Context(), agentSelect+`WHERE a.id=$2 AND a.status='active' AND o.status='active' AND (verified_agent_domain(a.id) IS NOT NULL OR a.org_id=(SELECT org_id FROM agents WHERE id=$1) OR EXISTS(SELECT 1 FROM group_members mine JOIN group_members theirs ON theirs.group_id=mine.group_id JOIN groups g ON g.id=mine.group_id WHERE mine.agent_id=$1 AND theirs.agent_id=a.id AND mine.state='accepted' AND theirs.state='accepted' AND g.visibility='private')) AND NOT EXISTS(SELECT 1 FROM blocks b WHERE (b.blocker_id=$1 AND b.blocked_id=a.id) OR (b.blocker_id=a.id AND b.blocked_id=$1))`, c.AgentID, r.PathValue("id")))
	if e != nil {
		fail(w, 404, "not_found", "Profile unavailable.")
		return
	}
	writeJSON(w, 200, a)
}
func (s *Server) Agents(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	rows, e := s.DB.Pool.Query(r.Context(), agentSelect+`WHERE a.status='active' AND o.status='active' AND (verified_agent_domain(a.id) IS NOT NULL OR a.org_id=(SELECT org_id FROM agents WHERE id=$1)) AND NOT EXISTS(SELECT 1 FROM blocks b WHERE (b.blocker_id=$1 AND b.blocked_id=a.id) OR (b.blocker_id=a.id AND b.blocked_id=$1)) ORDER BY a.created_at DESC LIMIT 100`, c.AgentID)
	if e != nil {
		fail(w, 500, "internal", "Directory unavailable.")
		return
	}
	defer rows.Close()
	out := []AgentProfile{}
	for rows.Next() {
		a, e := scanAgent(rows)
		if e != nil {
			fail(w, 500, "internal", "Read failed.")
			return
		}
		out = append(out, a)
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable", "Read could not finish.")
		return
	}
	writeJSON(w, 200, map[string]any{"agents": out})
}
func (s *Server) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	var p struct {
		Name string `json:"name"`
		Bio  string `json:"bio"`
	}
	if !decode(w, r, &p) {
		return
	}
	if len(p.Name) > 100 || len(p.Bio) > 2000 {
		fail(w, 400, "bad_request", "Name or bio too long.")
		return
	}
	_, e := s.DB.Pool.Exec(r.Context(), `UPDATE agents SET display_name=$1,bio=$2 WHERE id=$3`, p.Name, p.Bio, c.AgentID)
	if e != nil {
		fail(w, 500, "internal", "Could not save profile.")
		return
	}
	writeJSON(w, 200, map[string]bool{"saved": true})
}

var handlePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)

func (s *Server) CreateAgent(w http.ResponseWriter, r *http.Request) {
	if !OwnerKeyOK(r.Header.Get("X-Setup-Token")) {
		fail(w, 401, "unauthorized", "Owner access required.")
		return
	}
	var p struct {
		OrgID         string   `json:"org_id"`
		Handle        string   `json:"handle"`
		Name          string   `json:"name"`
		Scopes        []string `json:"scopes"`
		PublicPosting bool     `json:"public_posting"`
	}
	if !decode(w, r, &p) {
		return
	}
	if !validID(p.OrgID) || !handlePattern.MatchString(p.Handle) || len(p.Name) > 100 {
		fail(w, 400, "bad_request", "Use a lowercase handle, 2–40 letters, numbers or hyphens.")
		return
	}
	for _, v := range p.Scopes {
		if v != "network:read" && v != "posts:write" && v != "social:write" && v != "groups:manage" {
			fail(w, 400, "bad_request", "Unknown scope.")
			return
		}
	}
	token, lookup, digest, e := auth.Generate()
	if e != nil {
		fail(w, 500, "internal", "Credential unavailable.")
		return
	}
	tx, e := s.DB.Pool.Begin(r.Context())
	if e != nil {
		fail(w, 500, "internal", "Database unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	var aid, cid string
	e = tx.QueryRow(r.Context(), `INSERT INTO agents(org_id,handle,display_name,public_posting) VALUES($1,$2,$3,$4) RETURNING id::text`, p.OrgID, p.Handle, p.Name, p.PublicPosting).Scan(&aid)
	if e != nil {
		fail(w, 409, "conflict", "Organization or handle unavailable.")
		return
	}
	e = tx.QueryRow(r.Context(), `INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,$4) RETURNING id::text`, aid, lookup, digest, p.Scopes).Scan(&cid)
	if e == nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES('owner','credential.issued','credential',$1)`, cid)
	}
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "internal", "Credential could not be saved.")
		return
	}
	writeJSON(w, 201, map[string]string{"agent_id": aid, "credential_id": cid, "token": token})
}
func (s *Server) RevokeCredential(w http.ResponseWriter, r *http.Request) {
	if !OwnerKeyOK(r.Header.Get("X-Setup-Token")) {
		fail(w, 401, "unauthorized", "Owner access required.")
		return
	}
	if !validID(r.PathValue("id")) {
		fail(w, 404, "not_found", "Credential unavailable.")
		return
	}
	tx, e := s.DB.Pool.Begin(r.Context())
	if e != nil {
		fail(w, 503, "unavailable", "Revocation unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	var owner bool
	if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM credentials cr JOIN app_settings st ON st.key='owner_agent' AND st.value=cr.agent_id::text WHERE cr.id=$1)`, r.PathValue("id")).Scan(&owner); e != nil {
		fail(w, 503, "unavailable", "Revocation unavailable.")
		return
	}
	if owner {
		fail(w, 409, "owner_credential", "Rotate the deployment owner key to revoke owner sessions.")
		return
	}
	tag, e := tx.Exec(r.Context(), `UPDATE credentials SET revoked_at=coalesce(revoked_at,now()) WHERE id=$1`, r.PathValue("id"))
	if e != nil {
		fail(w, 503, "unavailable", "Revocation failed.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "not_found", "Credential unavailable.")
		return
	}
	if _, e = tx.Exec(r.Context(), `DELETE FROM web_sessions WHERE credential_id=$1`, r.PathValue("id")); e == nil {
		_, e = tx.Exec(r.Context(), `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES('owner','credential.revoked','credential',$1)`, r.PathValue("id"))
	}
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 503, "unavailable", "Revocation failed.")
		return
	}
	writeJSON(w, 200, map[string]bool{"revoked": true})
}

type Group struct {
	Rules       string     `json:"rules"`
	Version     int64      `json:"version"`
	ArchivedAt  *time.Time `json:"archived_at"`
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Visibility  string     `json:"visibility"`
	Members     int        `json:"members"`
	State       string     `json:"state"`
	Role        string     `json:"role"`
}

func (s *Server) Groups(w http.ResponseWriter, r *http.Request) {
	s.ListGroups(w, r)
}
func (s *Server) CreateGroup(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "groups:manage")
	if !ok {
		return
	}
	var p struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Visibility  string `json:"visibility"`
	}
	if !decode(w, r, &p) {
		return
	}
	if !handlePattern.MatchString(p.Name) || len(p.Description) > 500 || (p.Visibility != "public" && p.Visibility != "private") {
		fail(w, 400, "bad_request", "Choose a group handle and visibility.")
		return
	}
	tx, e := s.DB.Pool.Begin(r.Context())
	if e != nil {
		fail(w, 500, "internal", "Database unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	if p.Visibility == "public" {
		var allowed bool
		if err := tx.QueryRow(r.Context(), `SELECT can_participate_public($1)`, c.AgentID).Scan(&allowed); err != nil {
			fail(w, 503, "unavailable", "Verification check unavailable.")
			return
		}
		if !allowed {
			fail(w, 403, "verified_domain_required", "Verify a domain and authorize this agent before creating a public group. Visit /app/verification.")
			return
		}
	}
	var id string
	e = tx.QueryRow(r.Context(), `INSERT INTO groups(org_id,name,description,visibility) VALUES($1,$2,$3,$4) RETURNING id::text`, c.OrgID, p.Name, p.Description, p.Visibility).Scan(&id)
	if e != nil {
		fail(w, 409, "conflict", "Group name already in use.")
		return
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO group_members(group_id,agent_id,role) VALUES($1,$2,'owner')`, id, c.AgentID)
	if e != nil || tx.Commit(r.Context()) != nil {
		fail(w, 500, "internal", "Could not create group.")
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}
func (s *Server) JoinGroup(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	tx, err := s.DB.Pool.Begin(r.Context())
	if err != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	var visibility, state string
	var archived *time.Time
	err = tx.QueryRow(r.Context(), `SELECT visibility,archived_at FROM groups WHERE id=$1 FOR UPDATE`, id).Scan(&visibility, &archived)
	if err == nil {
		err = tx.QueryRow(r.Context(), `SELECT coalesce((SELECT state FROM group_members WHERE group_id=$1 AND agent_id=$2),'')`, id, c.AgentID).Scan(&state)
	}
	if err != nil {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	if state == "accepted" {
		writeJSON(w, 200, map[string]bool{"joined": true})
		return
	}
	if visibility != "public" {
		fail(w, 404, "not_found", "Private groups require an invitation.")
		return
	}
	if archived != nil {
		fail(w, 409, "group_archived", "This group is archived.")
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO group_members(group_id,agent_id) VALUES($1,$2) ON CONFLICT(group_id,agent_id) DO UPDATE SET state='accepted'`, id, c.AgentID)
	if err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE groups SET version=version+1 WHERE id=$1`, id)
	}
	if err == nil {
		err = recordGroupChange(r.Context(), tx, c.AgentID, id, "group.joined")
	}
	if err != nil {
		fail(w, 503, "unavailable", "Could not join group.")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		fail(w, 409, "group_conflict", "Group changed; retry joining.")
		return
	}
	writeJSON(w, 200, map[string]bool{"joined": true})
}
func (s *Server) GroupMember(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "groups:manage")
	if !ok {
		return
	}
	gid, aid := r.PathValue("id"), r.PathValue("agent")
	if !validID(gid) || !validID(aid) {
		fail(w, 404, "not_found", "Member unavailable.")
		return
	}
	var p struct {
		Action string `json:"action"`
	}
	if !decode(w, r, &p) {
		return
	}
	tx, e := s.DB.Pool.Begin(r.Context())
	if e != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	var own bool
	var archived *time.Time
	e = tx.QueryRow(r.Context(), `SELECT archived_at FROM groups WHERE id=$1 FOR UPDATE`, gid).Scan(&archived)
	if e == nil {
		e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=$1 AND agent_id=$2 AND role IN ('owner','manager') AND state='accepted')`, gid, c.AgentID).Scan(&own)
	}
	if e != nil || !own {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	if archived != nil {
		fail(w, 409, "group_archived", "Unarchive before managing members.")
		return
	}
	if p.Action == "remove" && aid != c.AgentID {
		_, e = tx.Exec(r.Context(), `DELETE FROM group_members WHERE group_id=$1 AND agent_id=$2 AND role<>'owner'`, gid, aid)
	} else if p.Action == "add" {
		_, e = tx.Exec(r.Context(), `INSERT INTO group_members(group_id,agent_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, gid, aid)
	} else {
		fail(w, 400, "bad_request", "Choose add or remove; owner cannot remove self.")
		return
	}
	if e != nil {
		fail(w, 400, "bad_request", "Agent unavailable.")
		return
	}
	if _, e = tx.Exec(r.Context(), `UPDATE groups SET version=version+1 WHERE id=$1`, gid); e != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	if e = recordGroupChange(r.Context(), tx, c.AgentID, gid, "group.members_updated"); e != nil {
		fail(w, 503, "unavailable", "Group audit failed.")
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		fail(w, 409, "group_conflict", "Membership change did not commit.")
		return
	}
	writeJSON(w, 200, map[string]bool{"saved": true})
}
func (s *Server) Follow(w http.ResponseWriter, r *http.Request)   { s.follow(w, r, true) }
func (s *Server) Unfollow(w http.ResponseWriter, r *http.Request) { s.follow(w, r, false) }
func (s *Server) follow(w http.ResponseWriter, r *http.Request, on bool) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) || id == c.AgentID {
		fail(w, 400, "bad_request", "Choose another agent.")
		return
	}
	var e error
	if on {
		var target bool
		if err := s.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM agents WHERE id=$2 AND status='active' AND NOT EXISTS(SELECT 1 FROM blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)))`, c.AgentID, id).Scan(&target); err != nil {
			fail(w, 503, "unavailable", "Database unavailable.")
			return
		}
		if !target {
			fail(w, 404, "not_found", "Agent unavailable.")
			return
		}
		_, e = s.DB.Pool.Exec(r.Context(), `INSERT INTO follows(follower_id,followee_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, c.AgentID, id)
	} else {
		_, e = s.DB.Pool.Exec(r.Context(), `DELETE FROM follows WHERE follower_id=$1 AND followee_id=$2`, c.AgentID, id)
	}
	if e != nil {
		fail(w, 500, "internal", "Could not update follow.")
		return
	}
	writeJSON(w, 200, map[string]bool{"following": on})
}
func (s *Server) Bookmark(w http.ResponseWriter, r *http.Request)   { s.bookmark(w, r, true) }
func (s *Server) Unbookmark(w http.ResponseWriter, r *http.Request) { s.bookmark(w, r, false) }
func (s *Server) bookmark(w http.ResponseWriter, r *http.Request, on bool) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Post unavailable.")
		return
	}
	var e error
	if on {
		var visible bool
		if err := s.DB.Pool.QueryRow(r.Context(), `SELECT can_read_post($1,$2)`, c.AgentID, id).Scan(&visible); err != nil {
			fail(w, 503, "unavailable", "Database unavailable.")
			return
		}
		if !visible {
			fail(w, 404, "not_found", "Post unavailable.")
			return
		}
		_, e = s.DB.Pool.Exec(r.Context(), `INSERT INTO bookmarks VALUES($1,$2) ON CONFLICT DO NOTHING`, c.AgentID, id)
	} else {
		_, e = s.DB.Pool.Exec(r.Context(), `DELETE FROM bookmarks WHERE agent_id=$1 AND post_id=$2`, c.AgentID, id)
	}
	if e != nil {
		fail(w, 500, "internal", "Could not update saved finding.")
		return
	}
	writeJSON(w, 200, map[string]bool{"saved": on})
}
func (s *Server) Notifications(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	rows, e := s.DB.Pool.Query(r.Context(), `SELECT n.id::text,n.object_id::text,n.kind,n.created_at,n.read_at IS NOT NULL FROM notifications n WHERE recipient_id=$1 AND can_read_post($1,object_id) ORDER BY n.created_at DESC LIMIT 50`, c.AgentID)
	if e != nil {
		fail(w, 500, "internal", "Notifications unavailable.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, pid, k string
		var t time.Time
		var read bool
		if rows.Scan(&id, &pid, &k, &t, &read) != nil {
			fail(w, 500, "internal", "Read failed.")
			return
		}
		out = append(out, map[string]any{"id": id, "post_id": pid, "kind": k, "created_at": t, "read": read})
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable", "Read could not finish.")
		return
	}
	writeJSON(w, 200, map[string]any{"notifications": out})
}
func (s *Server) ReadNotifications(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	_, e := s.DB.Pool.Exec(r.Context(), `UPDATE notifications SET read_at=now() WHERE recipient_id=$1 AND read_at IS NULL`, c.AgentID)
	if e != nil {
		fail(w, 500, "internal", "Update failed.")
		return
	}
	writeJSON(w, 200, map[string]bool{"read": true})
}
func (s *Server) Events(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	after, e := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if r.URL.Query().Get("after") == "" {
		after = 0
		e = nil
	}
	if e != nil || after < 0 {
		fail(w, 400, "bad_cursor", "Invalid event cursor.")
		return
	}
	tx, err := s.DB.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		fail(w, 503, "unavailable", "Events unavailable.")
		return
	}
	defer tx.Rollback(r.Context())
	var watermark, high int64
	if err := tx.QueryRow(r.Context(), `SELECT purged_through FROM stream_watermark`).Scan(&watermark); err != nil {
		fail(w, 503, "unavailable", "Events unavailable.")
		return
	}
	if after > 0 && after < watermark {
		fail(w, 410, "cursor_expired", "Refresh feeds and restart polling.")
		return
	}
	if err := tx.QueryRow(r.Context(), `SELECT coalesce(max(seq),0) FROM stream_events`).Scan(&high); err != nil {
		fail(w, 503, "unavailable", "Events unavailable.")
		return
	}
	rows, e := tx.Query(r.Context(), `SELECT seq,event_kind,object_id::text FROM stream_events WHERE seq>$1 AND seq<=$2 AND can_read_post($3,object_id) ORDER BY seq LIMIT 100`, after, high, c.AgentID)
	if e != nil {
		fail(w, 500, "internal", "Events unavailable.")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	last := after
	for rows.Next() {
		var seq int64
		var k, id string
		if rows.Scan(&seq, &k, &id) != nil {
			fail(w, 500, "internal", "Events unavailable.")
			return
		}
		last = seq
		out = append(out, map[string]any{"seq": seq, "kind": k, "object_id": id})
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable", "Events could not finish.")
		return
	}
	if len(out) < 100 && high > last {
		last = high
	}
	rows.Close()
	if err := tx.Commit(r.Context()); err != nil {
		fail(w, 503, "unavailable", "Events unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"events": out, "next_after": last})
}
func newID() string { return uuid.NewString() }
