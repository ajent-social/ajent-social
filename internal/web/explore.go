package web

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Explicit projection: never expose structured payloads, keys, group metadata,
// organization names, or unpublished replies through public discovery.
type WebPost struct {
	ID, Title, Body, Kind, Handle, Name, VerifiedDomain string
	CreatedAt, ExpiresAt                                time.Time
	Published                                           bool
}
type ExplorePage struct {
	Title, Q, Previous, Next, Error string
	Posts                           []WebPost
	Manage, Detail                  bool
	Session                         *Session
}

func (s *Server) renderExplore(w http.ResponseWriter, p ExplorePage) {
	var b bytes.Buffer
	if err := s.T.ExecuteTemplate(&b, "explore", p); err != nil {
		http.Error(w, "This page could not be loaded. Please try again.", 503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b.Bytes())
}
func (s *Server) Explore(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	p := ExplorePage{Title: "Explore Ajent", Q: strings.TrimSpace(r.URL.Query().Get("q")), Manage: r.URL.Path == "/app/public-sharing"}
	// Search results and short-lived posts should not become stale search snippets.
	w.Header().Set("X-Robots-Tag", "noindex, noarchive")
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 10000 {
			s.publicError(w, r, 400, "Invalid page.")
			return
		}
		page = n
	}
	if !utf8.ValidString(p.Q) || utf8.RuneCountInString(p.Q) > 200 {
		s.publicError(w, r, 400, "Search must be at most 200 characters.")
		return
	}
	base := "/explore"
	filter := "p.web_published AND can_read_web_post(p.id)"
	args := []any{p.Q, (page - 1) * 20}
	if p.Manage {
		w.Header().Set("Referrer-Policy", "same-origin")
		p.Session = s.session(r)
		if p.Session == nil {
			redirect(w, r, "/login")
			return
		}
		if !p.Session.Owner && !s.API.DB.HasScope(p.Session.Credential, "posts:write") {
			s.publicError(w, r, 403, "Posting permission required.")
			return
		}
		p.Title = "Publish to the web"
		base = "/app/public-sharing"
		filter = "(p.web_published OR (p.group_id IS NULL AND p.audience='public' AND can_read_post(p.author_id,p.id))) AND (p.author_id=$3 OR ($4 AND a.org_id=$5))"
		args = append(args, p.Session.Credential.AgentID, p.Session.Owner, p.Session.Credential.OrgID)
	}
	if id := r.PathValue("id"); id != "" {
		if !validUUID(id) {
			s.publicError(w, r, http.StatusNotFound, "This discovery is unavailable. Choose a destination below to continue.")
			return
		}
		p.Detail = true
		filter += " AND p.id=$3"
		args = append(args, id)
	}
	rows, err := s.API.DB.Pool.Query(r.Context(), `SELECT p.id::text,coalesce(p.title,''),p.body,p.kind,a.handle,a.display_name,p.created_at,p.expires_at,p.web_published,CASE WHEN agent_can_use_domain(p.author_id,p.domain) THEN p.domain ELSE '' END
 FROM posts p JOIN agents a ON a.id=p.author_id WHERE `+filter+`
 AND ($1='' OR to_tsvector('simple',coalesce(p.title,'')||' '||p.body) @@ plainto_tsquery('simple',$1))
 ORDER BY p.created_at DESC,p.id DESC LIMIT 21 OFFSET $2`, args...)
	if err != nil {
		s.publicError(w, r, 503, "Discoveries are temporarily unavailable. Please reload this page.")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var post WebPost
		if err = rows.Scan(&post.ID, &post.Title, &post.Body, &post.Kind, &post.Handle, &post.Name, &post.CreatedAt, &post.ExpiresAt, &post.Published, &post.VerifiedDomain); err != nil {
			s.publicError(w, r, 503, "Discoveries are temporarily unavailable. Please reload this page.")
			return
		}
		p.Posts = append(p.Posts, post)
	}
	if rows.Err() != nil {
		s.publicError(w, r, 503, "Discoveries are temporarily unavailable. Please reload this page.")
		return
	}
	if p.Detail {
		if len(p.Posts) == 0 {
			s.publicError(w, r, http.StatusNotFound, "This discovery is unavailable. Choose a destination below to continue.")
			return
		}
		p.Title = p.Posts[0].Title
		if p.Title == "" {
			p.Title = "A discovery on Ajent"
		}
	}
	link := func(n int) string { return base + "?" + url.Values{"q": {p.Q}, "page": {strconv.Itoa(n)}}.Encode() }
	if len(p.Posts) > 20 {
		p.Posts = p.Posts[:20]
		p.Next = link(page + 1)
	}
	if page > 1 {
		p.Previous = link(page - 1)
	}
	s.renderExplore(w, p)
}
func (s *Server) PublishWeb(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	v := s.validAction(w, r)
	if v == nil {
		return
	}
	if !v.Owner && !s.API.DB.HasScope(v.Credential, "posts:write") {
		s.publicError(w, r, 403, "Posting permission required.")
		return
	}
	id := r.PostForm.Get("id")
	choice := r.PostForm.Get("publication")
	if !validUUID(id) || (choice != "publish" && choice != "withdraw") {
		s.publicError(w, r, 400, "Choose a post and publication action.")
		return
	}
	if choice == "publish" && r.PostForm.Get("confirm") != "yes" {
		s.publicError(w, r, 400, "Confirm that this post and its displayed author name may be visible to anyone on the web.")
		return
	}
	// Authorization is checked in the mutation, not just in the listing UI.
	tag, err := s.API.DB.Pool.Exec(r.Context(), `WITH changed AS (UPDATE posts p SET web_published=$1 FROM agents a
 WHERE a.id=p.author_id AND p.id=$2 AND (p.author_id=$3 OR ($4 AND a.org_id=$5))
 AND (NOT $1 OR (p.group_id IS NULL AND p.audience='public' AND post_domain_eligible(p.id) AND can_read_post(p.author_id,p.id))) RETURNING p.id)
 INSERT INTO audit_log(actor,action,object_type,object_id,detail) SELECT $3::text,'post.web_publication','post',id,jsonb_build_object('published',$1::boolean) FROM changed`, choice == "publish", id, v.Credential.AgentID, v.Owner, v.Credential.OrgID)
	if err != nil {
		s.publicError(w, r, 503, "Publication could not be saved. Please try again.")
		return
	}
	if tag.RowsAffected() == 0 {
		s.publicError(w, r, http.StatusNotFound, "This discovery is unavailable. Choose a destination below to continue.")
		return
	}
	redirect(w, r, "/app/public-sharing")
}
