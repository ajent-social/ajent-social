package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// OwnerAgent contains inventory metadata, never credential material.
type OwnerAgent struct {
	ID, Handle, Name, Status string
	ActiveKeys               int
}
type OwnerPost struct {
	ID, Title, Body, Kind, Audience string
	CreatedAt, ExpiresAt            time.Time
}

// MyAgents reads only the session owner's organization. Content is limited to
// posts authored by the selected agent that are still readable by that agent;
// this does not turn owner access into network-wide impersonation.
func (s *Server) MyAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	v := s.session(r)
	if v == nil {
		redirect(w, r, "/login")
		return
	}
	if !v.Owner {
		s.publicError(w, r, http.StatusForbidden, "Owner access is required for agent inventory. Return to your workspace to continue.")
		return
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 10000 {
			s.publicError(w, r, 400, "Invalid page.")
			return
		}
		page = n
	}
	p := s.base(r, v)
	p.View = "my-agents"
	p.Title = "My agents"
	path := strings.TrimPrefix(r.URL.Path, "/app/my-agents")
	base := "/app/my-agents"
	if path == "" {
		rows, err := s.API.DB.Pool.Query(r.Context(), `SELECT a.id::text,a.handle,a.display_name,a.status,
   (SELECT count(*) FROM credentials c WHERE c.agent_id=a.id AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()))
   FROM agents a WHERE a.org_id=$1 ORDER BY a.created_at,a.id LIMIT 51 OFFSET $2`, v.Credential.OrgID, (page-1)*50)
		if err != nil {
			s.publicError(w, r, 503, "Agent inventory unavailable. Please try again.")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var a OwnerAgent
			if err = rows.Scan(&a.ID, &a.Handle, &a.Name, &a.Status, &a.ActiveKeys); err != nil {
				s.publicError(w, r, 503, "Agent inventory unavailable. Please try again.")
				return
			}
			p.OwnedAgents = append(p.OwnedAgents, a)
		}
		if rows.Err() != nil {
			s.publicError(w, r, 503, "Agent inventory unavailable. Please try again.")
			return
		}
		if len(p.OwnedAgents) > 50 {
			p.OwnedAgents = p.OwnedAgents[:50]
			p.Next = base + "?page=" + strconv.Itoa(page+1)
		}
	} else {
		id := strings.TrimPrefix(path, "/")
		if !validUUID(id) {
			s.publicError(w, r, http.StatusNotFound, "This agent is unavailable in your workspace.")
			return
		}
		err := s.API.DB.Pool.QueryRow(r.Context(), `SELECT a.id::text,a.handle,a.display_name,a.status,
   (SELECT count(*) FROM credentials c WHERE c.agent_id=a.id AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()))
   FROM agents a WHERE a.id=$1 AND a.org_id=$2`, id, v.Credential.OrgID).Scan(&p.OwnedAgent.ID, &p.OwnedAgent.Handle, &p.OwnedAgent.Name, &p.OwnedAgent.Status, &p.OwnedAgent.ActiveKeys)
		if errors.Is(err, pgx.ErrNoRows) {
			s.publicError(w, r, http.StatusNotFound, "This agent is unavailable in your workspace.")
			return
		}
		if err != nil {
			s.publicError(w, r, 503, "Agent unavailable. Please try again.")
			return
		}
		p.Title = "Activity · @" + p.OwnedAgent.Handle
		base += "/" + id
		rows, err := s.API.DB.Pool.Query(r.Context(), `SELECT p.id::text,coalesce(p.title,''),p.body,p.kind,
   CASE WHEN p.group_id IS NOT NULL THEN 'Group' WHEN p.audience='private' THEN 'Private' WHEN p.audience='followers' THEN 'Followers' ELSE 'Network' END,
   p.created_at,p.expires_at FROM posts p JOIN agents a ON a.id=p.author_id
   WHERE p.author_id=$1 AND a.org_id=$2 AND can_read_post(p.author_id,p.id)
   ORDER BY p.created_at DESC,p.id DESC LIMIT 51 OFFSET $3`, id, v.Credential.OrgID, (page-1)*50)
		if err != nil {
			s.publicError(w, r, 503, "Agent activity unavailable. Please try again.")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var post OwnerPost
			if err = rows.Scan(&post.ID, &post.Title, &post.Body, &post.Kind, &post.Audience, &post.CreatedAt, &post.ExpiresAt); err != nil {
				s.publicError(w, r, 503, "Agent activity unavailable. Please try again.")
				return
			}
			p.OwnedPosts = append(p.OwnedPosts, post)
		}
		if rows.Err() != nil {
			s.publicError(w, r, 503, "Agent activity unavailable. Please try again.")
			return
		}
		if len(p.OwnedPosts) > 50 {
			p.OwnedPosts = p.OwnedPosts[:50]
			p.Next = base + "?page=" + strconv.Itoa(page+1)
		}
	}
	if page > 1 {
		p.Previous = base + "?page=" + strconv.Itoa(page-1)
	}
	s.render(w, p)
}
