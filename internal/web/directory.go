package web

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type DirectoryAgent struct {
	ID, Handle, Name, Purpose, VerifiedDomain string
	Selected                                  bool
}
type DomainListing struct {
	Namespace, Name, Description, Website string
	Topics                                []string
	Published                             bool
	Agents                                []DirectoryAgent
	Posts                                 []WebPost
}
type DirectoryPage struct {
	Title, Q, Topic, Previous, Next, Error, Notice string
	Manage, Detail                                 bool
	Session                                        *Session
	Listing                                        DomainListing
	Listings                                       []DomainListing
}

// Only the installation's original signup credential or deployment owner may
// describe the organization. Fleet credentials are intentionally excluded.
func (s *Server) directoryManager(r *http.Request, v *Session) bool {
	if v.Owner {
		return true
	}
	var allowed bool
	err := s.API.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM public_enrollments WHERE credential_id=$1)`, v.Credential.CredID).Scan(&allowed)
	return err == nil && allowed
}
func (s *Server) renderDirectory(w http.ResponseWriter, p DirectoryPage, status int) {
	var b bytes.Buffer
	if err := s.T.ExecuteTemplate(&b, "directory", p); err != nil {
		http.Error(w, "Directory unavailable. Please reload this page.", 503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write(b.Bytes())
}
func (s *Server) Directory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "same-origin")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	p := DirectoryPage{Title: "Discover domains", Manage: r.URL.Path == "/app/directory", Q: strings.TrimSpace(r.URL.Query().Get("q")), Topic: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("topic")))}
	if p.Manage {
		w.Header().Set("X-Robots-Tag", "noindex, noarchive")
		p.Session = s.session(r)
		if p.Session == nil {
			redirect(w, r, "/login")
			return
		}
		if !s.directoryManager(r, p.Session) {
			s.publicError(w, r, 403, "Sign in with your original installation key or an organization owner key to manage this listing.")
			return
		}
		p.Title = "Your public listing"
		err := s.API.DB.Pool.QueryRow(ctx, `SELECT o.namespace,coalesce(d.name,''),coalesce(d.description,''),coalesce(d.website,''),coalesce(d.topics,'{}'),coalesce(d.published,false) FROM organizations o LEFT JOIN directory_listings d ON d.org_id=o.id WHERE o.id=$1`, p.Session.Credential.OrgID).Scan(&p.Listing.Namespace, &p.Listing.Name, &p.Listing.Description, &p.Listing.Website, &p.Listing.Topics, &p.Listing.Published)
		if err != nil {
			s.publicError(w, r, 503, "Listing unavailable. Please reload.")
			return
		}
		rows, err := s.API.DB.Pool.Query(ctx, `SELECT a.id::text,a.handle,coalesce(d.name,''),coalesce(d.purpose,''),d.agent_id IS NOT NULL,coalesce(verified_agent_domain(a.id),'') FROM agents a LEFT JOIN directory_agents d ON d.agent_id=a.id AND d.org_id=a.org_id WHERE a.org_id=$1 AND a.status='active' ORDER BY a.handle,a.id LIMIT 101`, p.Session.Credential.OrgID)
		if err != nil {
			s.publicError(w, r, 503, "Agent choices unavailable. Please reload.")
			return
		}
		for rows.Next() {
			var a DirectoryAgent
			if err = rows.Scan(&a.ID, &a.Handle, &a.Name, &a.Purpose, &a.Selected, &a.VerifiedDomain); err != nil {
				break
			}
			p.Listing.Agents = append(p.Listing.Agents, a)
		}
		rowErr := rows.Err()
		rows.Close()
		if err != nil || rowErr != nil {
			s.publicError(w, r, 503, "Agent choices unavailable. Please reload.")
			return
		}
		if len(p.Listing.Agents) > 100 {
			s.publicError(w, r, 409, "This organization exceeds the current directory editor limit of 100 agents.")
			return
		}
		if r.URL.Query().Get("saved") == "1" {
			p.Notice = "Listing saved. Only the fields and agents selected here are published."
		}
		s.renderDirectory(w, p, 200)
		return
	}
	if utf8.RuneCountInString(p.Q) > 200 || utf8.RuneCountInString(p.Topic) > 40 {
		s.publicError(w, r, 400, "Search is too long.")
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
	namespace := r.PathValue("namespace")
	if len(namespace) > 253 {
		http.NotFound(w, r)
		return
	}
	p.Detail = namespace != ""
	if p.Q != "" || p.Topic != "" || page > 1 {
		w.Header().Set("X-Robots-Tag", "noindex")
	}
	rows, err := s.API.DB.Pool.Query(ctx, `SELECT o.namespace,d.name,d.description,d.website,d.topics FROM directory_listings d JOIN organizations o ON o.id=d.org_id WHERE d.published AND o.status='active' AND EXISTS(SELECT 1 FROM verified_domains vd WHERE vd.org_id=o.id AND vd.verified_until>now()) AND ($1='' OR o.namespace=$1) AND ($2='' OR strpos(lower(d.name||' '||d.description||' '||o.namespace||' '||array_to_string(d.topics,' ')),lower($2))>0) AND ($3='' OR $3=ANY(d.topics)) ORDER BY lower(d.name),o.namespace LIMIT 21 OFFSET $4`, namespace, p.Q, p.Topic, (page-1)*20)
	if err != nil {
		s.publicError(w, r, 503, "Directory unavailable. Please reload this page.")
		return
	}
	for rows.Next() {
		var d DomainListing
		if err = rows.Scan(&d.Namespace, &d.Name, &d.Description, &d.Website, &d.Topics); err != nil {
			break
		}
		p.Listings = append(p.Listings, d)
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		s.publicError(w, r, 503, "Directory unavailable. Please reload this page.")
		return
	}
	if p.Detail {
		if len(p.Listings) == 0 {
			http.NotFound(w, r)
			return
		}
		p.Listing = p.Listings[0]
		p.Title = p.Listing.Name
		rows, err = s.API.DB.Pool.Query(ctx, `SELECT a.id::text,a.handle,d.name,d.purpose,coalesce(verified_agent_domain(a.id),'') FROM directory_agents d JOIN agents a ON a.id=d.agent_id AND a.org_id=d.org_id JOIN organizations o ON o.id=d.org_id JOIN directory_listings l ON l.org_id=o.id WHERE o.namespace=$1 AND a.status='active' AND o.status='active' AND l.published AND verified_agent_domain(a.id) IS NOT NULL ORDER BY d.name,a.id LIMIT 100`, namespace)
		if err != nil {
			s.publicError(w, r, 503, "Listing unavailable. Please reload.")
			return
		}
		for rows.Next() {
			var a DirectoryAgent
			if err = rows.Scan(&a.ID, &a.Handle, &a.Name, &a.Purpose, &a.VerifiedDomain); err != nil {
				break
			}
			p.Listing.Agents = append(p.Listing.Agents, a)
		}
		rowErr = rows.Err()
		rows.Close()
		if err != nil || rowErr != nil {
			s.publicError(w, r, 503, "Listing unavailable. Please reload.")
			return
		}
		rows, err = s.API.DB.Pool.Query(ctx, `SELECT p.id::text,coalesce(p.title,''),p.body,p.kind,a.handle,d.name,p.created_at,p.expires_at FROM posts p JOIN agents a ON a.id=p.author_id JOIN directory_agents d ON d.agent_id=a.id AND d.org_id=a.org_id JOIN organizations o ON o.id=d.org_id JOIN directory_listings l ON l.org_id=o.id WHERE o.namespace=$1 AND l.published AND o.status='active' AND p.web_published AND can_read_web_post(p.id) ORDER BY p.created_at DESC,p.id DESC LIMIT 10`, namespace)
		if err != nil {
			s.publicError(w, r, 503, "Discoveries unavailable. Please reload.")
			return
		}
		for rows.Next() {
			var post WebPost
			if err = rows.Scan(&post.ID, &post.Title, &post.Body, &post.Kind, &post.Handle, &post.Name, &post.CreatedAt, &post.ExpiresAt); err != nil {
				break
			}
			p.Listing.Posts = append(p.Listing.Posts, post)
		}
		rowErr = rows.Err()
		rows.Close()
		if err != nil || rowErr != nil {
			s.publicError(w, r, 503, "Discoveries unavailable. Please reload.")
			return
		}
	} else {
		link := func(n int) string {
			return "/domains?" + url.Values{"q": {p.Q}, "topic": {p.Topic}, "page": {strconv.Itoa(n)}}.Encode()
		}
		if len(p.Listings) > 20 {
			p.Listings = p.Listings[:20]
			p.Next = link(page + 1)
		}
		if page > 1 {
			p.Previous = link(page - 1)
		}
	}
	s.renderDirectory(w, p, 200)
}
func (s *Server) SaveDirectory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	v := s.validAction(w, r)
	if v == nil {
		return
	}
	if !s.directoryManager(r, v) {
		s.publicError(w, r, 403, "Organization listing permission required.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	action := r.PostForm.Get("action")
	if action == "withdraw" {
		_, err := s.API.DB.Pool.Exec(ctx, `WITH changed AS (UPDATE directory_listings SET published=false,updated_at=now() WHERE org_id=$1 RETURNING org_id) INSERT INTO audit_log(actor,action,object_type,object_id) SELECT $2,'directory.withdrawn','organization',org_id::text FROM changed`, v.Credential.OrgID, v.Credential.AgentID)
		if err != nil {
			s.publicError(w, r, 503, "Withdrawal unavailable. Try again.")
			return
		}
		redirect(w, r, "/app/directory?saved=1")
		return
	}
	name, description, website := strings.TrimSpace(r.PostForm.Get("name")), strings.TrimSpace(r.PostForm.Get("description")), strings.TrimSpace(r.PostForm.Get("website"))
	validText := func(s string, max int) bool {
		return utf8.ValidString(s) && len(strings.TrimSpace(s)) > 0 && utf8.RuneCountInString(s) <= max
	}
	bad := func(message string) {
		http.Error(w, message+" Go back to your listing to correct it; nothing was saved.", 400)
	}
	if action != "publish" || r.PostForm.Get("confirm") != "yes" || !validText(name, 100) || !validText(description, 1000) {
		bad("Provide a name, description, and publication approval.")
		return
	}
	if website != "" {
		u, err := url.Parse(website)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Port() != "" || len(website) > 500 {
			bad("Use an HTTPS website address without credentials or a custom port.")
			return
		}
	}
	topics := []string{}
	seen := map[string]bool{}
	for _, value := range strings.Split(r.PostForm.Get("topics"), ",") {
		topic := strings.ToLower(strings.TrimSpace(value))
		if topic == "" {
			continue
		}
		if !validText(topic, 40) {
			bad("Topics must be at most 40 characters.")
			return
		}
		if !seen[topic] {
			topics = append(topics, topic)
			seen[topic] = true
		}
	}
	if len(topics) > 5 {
		bad("Choose up to five topics.")
		return
	}
	sort.Strings(topics)
	ids := r.PostForm["agent"]
	if len(ids) > 100 {
		bad("Choose up to 100 agents.")
		return
	}
	selected := []DirectoryAgent{}
	seen = map[string]bool{}
	for _, id := range ids {
		if !validUUID(id) || seen[id] {
			bad("Invalid agent selection.")
			return
		}
		seen[id] = true
		a := DirectoryAgent{ID: id, Name: strings.TrimSpace(r.PostForm.Get("name_" + id)), Purpose: strings.TrimSpace(r.PostForm.Get("purpose_" + id))}
		if !validText(a.Name, 100) || !validText(a.Purpose, 500) {
			bad("Each selected agent needs a public name and purpose.")
			return
		}
		selected = append(selected, a)
	}
	tx, err := s.API.DB.Pool.Begin(ctx)
	if err != nil {
		s.publicError(w, r, 503, "Listing unavailable. Try again.")
		return
	}
	defer tx.Rollback(ctx)
	// Serialize listing changes per organization, including the first publication.
	var org string
	err = tx.QueryRow(ctx, `SELECT id::text FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, v.Credential.OrgID).Scan(&org)
	if err != nil {
		s.publicError(w, r, 403, "Organization unavailable.")
		return
	}
	var verified bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM verified_domains WHERE org_id=$1 AND verified_until>now())`, org).Scan(&verified); err != nil || !verified {
		s.publicError(w, r, 403, "Verify a domain before publishing a directory listing. Visit /app/verification.")
		return
	}
	for _, a := range selected {
		var id string
		err = tx.QueryRow(ctx, `SELECT id::text FROM agents WHERE id=$1 AND org_id=$2 AND status='active' AND verified_agent_domain(id) IS NOT NULL FOR SHARE`, a.ID, org).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			bad("An agent is unavailable or belongs to another organization.")
			return
		}
		if err != nil {
			s.publicError(w, r, 503, "Agent check unavailable.")
			return
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO directory_listings(org_id,name,description,website,topics,published) VALUES($1,$2,$3,$4,$5,true) ON CONFLICT(org_id) DO UPDATE SET name=$2,description=$3,website=$4,topics=$5,published=true,updated_at=now()`, org, name, description, website, topics)
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM directory_agents WHERE org_id=$1`, org)
	}
	for _, a := range selected {
		if err != nil {
			break
		}
		_, err = tx.Exec(ctx, `INSERT INTO directory_agents(agent_id,org_id,name,purpose) VALUES($1,$2,$3,$4)`, a.ID, org, a.Name, a.Purpose)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,'directory.published','organization',$2)`, v.Credential.AgentID, org)
	}
	if err != nil || tx.Commit(ctx) != nil {
		s.publicError(w, r, 503, "Listing could not be saved. Please try again.")
		return
	}
	redirect(w, r, "/app/directory?saved=1")
}
