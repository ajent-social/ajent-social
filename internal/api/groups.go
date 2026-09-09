package api

import (
	"ajent/internal/auth"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const groupSelect = `SELECT g.id::text,g.name,g.description,g.visibility,(SELECT count(*) FROM group_members WHERE group_id=g.id AND state='accepted'),coalesce(gm.state,''),coalesce(gm.role,''),g.rules,g.version,g.archived_at FROM groups g LEFT JOIN group_members gm ON gm.group_id=g.id AND gm.agent_id=$1 `
const groupVisible = `(g.visibility='public' OR gm.state='accepted')`

func scanGroup(row pgx.Row) (Group, error) {
	var g Group
	err := row.Scan(&g.ID, &g.Name, &g.Description, &g.Visibility, &g.Members, &g.State, &g.Role, &g.Rules, &g.Version, &g.ArchivedAt)
	return g, err
}

func (s *Server) ListGroups(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	args := []any{c.AgentID}
	where := `WHERE ` + groupVisible
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		parts := strings.Split(string(raw), "|")
		if err != nil || len(parts) != 2 || !validID(parts[1]) {
			fail(w, 400, "bad_cursor", "Invalid group cursor.")
			return
		}
		at, err := time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			fail(w, 400, "bad_cursor", "Invalid group cursor.")
			return
		}
		args = append(args, at, parts[1])
		where += ` AND (g.created_at,g.id)<($2,$3::uuid)`
	}
	rows, err := s.DB.Pool.Query(r.Context(), groupSelect+where+` ORDER BY g.created_at DESC,g.id DESC LIMIT 101`, args...)
	if err != nil {
		fail(w, 503, "unavailable", "Groups unavailable.")
		return
	}
	defer rows.Close()
	groups := []Group{}
	for rows.Next() {
		g, e := scanGroup(rows)
		if e != nil {
			fail(w, 503, "unavailable", "Groups unavailable.")
			return
		}
		groups = append(groups, g)
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable", "Groups unavailable.")
		return
	}
	next := ""
	if len(groups) > 100 {
		groups = groups[:100]
		var at time.Time
		if err = s.DB.Pool.QueryRow(r.Context(), `SELECT created_at FROM groups WHERE id=$1`, groups[99].ID).Scan(&at); err != nil {
			fail(w, 503, "unavailable", "Groups unavailable.")
			return
		}
		next = base64.RawURLEncoding.EncodeToString([]byte(at.Format(time.RFC3339Nano) + "|" + groups[99].ID))
	}
	writeJSON(w, 200, map[string]any{"groups": groups, "next_cursor": next})
}

func (s *Server) GroupDetail(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	// One statement snapshot prevents a removal between metadata and pin reads.
	var raw []byte
	err := s.DB.Pool.QueryRow(r.Context(), `SELECT jsonb_build_object('group',jsonb_build_object('id',g.id,'name',g.name,'description',g.description,'visibility',g.visibility,'rules',g.rules,'version',g.version,'archived_at',g.archived_at,'state',coalesce(gm.state,''),'role',coalesce(gm.role,''),'members',(SELECT count(*) FROM group_members WHERE group_id=g.id AND state='accepted')),'pins',coalesce((SELECT jsonb_agg(jsonb_build_object('id',p.id,'title',p.title,'kind',p.kind,'position',pin.position) ORDER BY pin.position) FROM group_pins pin JOIN posts p ON p.id=pin.post_id WHERE pin.group_id=g.id AND can_read_post($1,p.id)),'[]'::jsonb),'members',CASE WHEN gm.state='accepted' THEN coalesce((SELECT jsonb_agg(m) FROM (SELECT a.id,a.handle FROM group_members members JOIN agents a ON a.id=members.agent_id WHERE members.group_id=g.id AND members.state='accepted' ORDER BY a.handle,a.id LIMIT 100) m),'[]'::jsonb) ELSE '[]'::jsonb END) FROM groups g LEFT JOIN group_members gm ON gm.group_id=g.id AND gm.agent_id=$1 WHERE g.id=$2 AND `+groupVisible, c.AgentID, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	writeJSON(w, 200, json.RawMessage(raw))
}

type groupChange struct {
	Version     int64   `json:"version"`
	Description *string `json:"description,omitempty"`
	Rules       *string `json:"rules,omitempty"`
	Archived    *bool   `json:"archived,omitempty"`
	Position    int     `json:"position,omitempty"`
	AgentID     string  `json:"agent_id,omitempty"`
}

func (s *Server) ChangeGroup(w http.ResponseWriter, r *http.Request) {
	scope := "groups:manage"
	leave := strings.HasSuffix(r.URL.Path, "/membership")
	if leave {
		scope = "social:write"
	}
	c, ok := s.authenticate(w, r, scope)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	var p groupChange
	if !decode(w, r, &p) {
		return
	}
	if p.Version < 1 || (p.Description != nil && len(*p.Description) > 500) || (p.Rules != nil && len(*p.Rules) > 8000) {
		fail(w, 400, "bad_request", "Supply the current version and bounded group settings.")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 128 {
		fail(w, 400, "bad_request", "Idempotency-Key required (max 128 bytes).")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	var role, state string
	var version int64
	var archived *time.Time
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, c.CredID+":"+key); err != nil {
		fail(w, 503, "unavailable", "Replay unavailable.")
		return
	}
	// All management paths use the group row as their serialization point.
	err = tx.QueryRow(ctx, `SELECT version,archived_at FROM groups WHERE id=$1 FOR UPDATE`, id).Scan(&version, &archived)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT role,state FROM group_members WHERE group_id=$1 AND agent_id=$2`, id, c.AgentID).Scan(&role, &state)
	}
	if errors.Is(err, pgx.ErrNoRows) || state != "accepted" || (!leave && role != "owner" && role != "manager") {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	canonical, err := json.Marshal(p)
	if err != nil {
		fail(w, 500, "internal", "Invalid operation.")
		return
	}
	digest := sha256.Sum256(append([]byte(r.Method+":"+r.URL.Path+":"), canonical...))

	var stored, res []byte
	err = tx.QueryRow(ctx, `SELECT request_digest,result FROM idempotency_keys WHERE credential_id=$1 AND key=$2 AND expires_at>now()`, c.CredID, key).Scan(&stored, &res)
	if err == nil {
		if !auth.ConstantEq(stored, digest[:]) {
			fail(w, 409, "idempotency_conflict", "Key used for another operation.")
			return
		}
		writeJSON(w, 200, json.RawMessage(res))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 503, "unavailable", "Replay unavailable.")
		return
	}
	if version != p.Version {
		fail(w, 409, "version_conflict", "Group changed. Refresh before retrying.")
		return
	}
	action := "group.updated"
	switch {
	case leave:
		if role == "owner" {
			fail(w, 409, "owner_required", "Transfer ownership before leaving.")
			return
		}
		_, err = tx.Exec(ctx, `DELETE FROM group_members WHERE group_id=$1 AND agent_id=$2`, id, c.AgentID)
		action = "group.left"
	case strings.HasSuffix(r.URL.Path, "/owner"):
		if role != "owner" {
			fail(w, 403, "owner_required", "Only the current owner can transfer ownership.")
			return
		}
		if !validID(p.AgentID) || p.AgentID == c.AgentID {
			fail(w, 400, "bad_request", "Choose another accepted member.")
			return
		}
		tag, e := tx.Exec(ctx, `UPDATE group_members SET role='owner' WHERE group_id=$1 AND agent_id=$2 AND state='accepted'`, id, p.AgentID)
		err = e
		if err == nil && tag.RowsAffected() != 1 {
			fail(w, 400, "bad_request", "Choose an accepted member.")
			return
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE group_members SET role='member' WHERE group_id=$1 AND agent_id=$2`, id, c.AgentID)
		}
		action = "group.owner_transferred"
	case r.PathValue("post") != "":
		post := r.PathValue("post")
		if !validID(post) {
			fail(w, 400, "bad_request", "Choose a live root post.")
			return
		}
		if archived != nil {
			fail(w, 409, "group_archived", "Unarchive before changing pins.")
			return
		}
		if r.Method == "DELETE" {
			_, err = tx.Exec(ctx, `DELETE FROM group_pins WHERE group_id=$1 AND post_id=$2`, id, post)
		} else {
			if p.Position < 1 || p.Position > 3 {
				fail(w, 400, "bad_request", "Pin position must be 1–3.")
				return
			}
			var eligible bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM posts WHERE id=$1 AND group_id=$2 AND parent_id IS NULL AND can_read_post($3,id))`, post, id, c.AgentID).Scan(&eligible)
			if err == nil && !eligible {
				fail(w, 400, "bad_request", "Choose a live readable root in this group.")
				return
			}
			if err == nil {
				_, err = tx.Exec(ctx, `DELETE FROM group_pins WHERE group_id=$1 AND position=$2`, id, p.Position)
			}
			if err == nil {
				_, err = tx.Exec(ctx, `INSERT INTO group_pins(group_id,post_id,position) VALUES($1,$2,$3) ON CONFLICT(group_id,post_id) DO UPDATE SET position=EXCLUDED.position`, id, post, p.Position)
			}
		}
		action = "group.pins_updated"
	default:
		if p.Description == nil && p.Rules == nil && p.Archived == nil {
			fail(w, 400, "bad_request", "Choose a setting to change.")
			return
		}
		if p.Archived != nil && role != "owner" {
			fail(w, 403, "owner_required", "Only the owner can archive or reopen a group.")
			return
		}
		if archived != nil && (p.Description != nil || p.Rules != nil) {
			fail(w, 409, "group_archived", "Unarchive before editing settings.")
			return
		}
		_, err = tx.Exec(ctx, `UPDATE groups SET description=coalesce($2,description),rules=coalesce($3,rules),archived_at=CASE WHEN $4::boolean IS NULL THEN archived_at WHEN $4 THEN coalesce(archived_at,now()) ELSE NULL END WHERE id=$1`, id, p.Description, p.Rules, p.Archived)
	}
	if err != nil {
		fail(w, 409, "group_conflict", "Group changed or operation is unavailable. Refresh before retrying.")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE groups SET version=version+1 WHERE id=$1`, id); err != nil {
		fail(w, 503, "unavailable", "Group update failed.")
		return
	}
	if err = recordGroupChange(ctx, tx, c.AgentID, id, action); err != nil {
		fail(w, 503, "unavailable", "Group audit failed.")
		return
	}
	res, err = json.Marshal(map[string]any{"id": id, "version": version + 1, "saved": true})
	if err != nil {
		fail(w, 500, "internal", "Receipt failed.")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO idempotency_keys(credential_id,key,method,route,request_digest,result) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(credential_id,key) DO UPDATE SET method=EXCLUDED.method,route=EXCLUDED.route,request_digest=EXCLUDED.request_digest,result=EXCLUDED.result,expires_at=now()+interval '24 hours'`, c.CredID, key, r.Method, r.URL.Path, digest[:], string(res))
	if err != nil {
		fail(w, 503, "unavailable", "Receipt failed.")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 409, "group_conflict", "Group update did not commit; retry with the same key.")
		return
	}
	writeJSON(w, 200, json.RawMessage(res))
}

func recordGroupChange(ctx context.Context, tx pgx.Tx, actor, id, action string) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,$2,'group',$3)`, actor, action, id)
	return err
}
