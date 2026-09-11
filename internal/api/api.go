package api

import (
	"ajent/internal/auth"
	"ajent/internal/ratelimit"
	"ajent/internal/store"
	"ajent/internal/telemetry"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	DB      *store.Store
	limiter *ratelimit.Limiter
}

func New(pool *pgxpool.Pool) *Server {
	return &Server{DB: &store.Store{Pool: pool}, limiter: ratelimit.New(120, time.Minute, 10000)}
}

var setupToken string
var setupMu sync.RWMutex

func SetSetupToken(t string) { setupMu.Lock(); defer setupMu.Unlock(); setupToken = t }
func OwnerKeyOK(t string) bool {
	setupMu.RLock()
	defer setupMu.RUnlock()
	return setupToken != "" && auth.ConstantEq(auth.Digest(t), auth.Digest(setupToken))
}

type credentialKey struct{}

func WithCredential(ctx context.Context, c store.Credential) context.Context {
	return context.WithValue(ctx, credentialKey{}, c)
}

var validKinds = map[string]bool{"question": true, "answer": true, "finding": true, "validation": true, "status": true, "handoff_request": true}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, k, msg string) {
	telemetry.Record(w, telemetry.Outcome{ErrorCode: k})
	writeJSON(w, code, map[string]string{"error": k, "detail": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "bad_request", "Invalid or oversized request.")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		fail(w, 400, "bad_request", "One JSON object is required.")
		return false
	}
	return true
}
func validID(s string) bool { _, e := uuid.Parse(s); return e == nil }
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request, scope string) (*store.Credential, bool) {
	c, ok := r.Context().Value(credentialKey{}).(store.Credential)
	if !ok {
		l, k, good := auth.ParseToken(r.Header.Get("Authorization"))
		if !good {
			fail(w, 401, "unauthorized", "Agent credential required.")
			return nil, false
		}
		var err error
		c, err = s.DB.Authenticate(r.Context(), l, k)
		if err != nil {
			if errors.Is(err, store.ErrUnauthenticated) {
				fail(w, 401, "unauthorized", "Credential invalid, expired or revoked.")
			} else {
				fail(w, 503, "unavailable", "Authentication temporarily unavailable.")
			}
			return nil, false
		}
	}
	if !s.DB.HasScope(c, scope) {
		fail(w, 403, "insufficient_scope", "Permission required: "+scope)
		return nil, false
	}
	if !s.limiter.Allow(c.CredID, time.Now()) {
		w.Header().Set("Retry-After", "1")
		fail(w, 429, "rate_limited", "Credential request allowance exceeded.")
		return nil, false
	}
	s.observeActivity(r, c.AgentID, false)
	return &c, true
}
func (s *Server) Routes(m *http.ServeMux) {
	m.HandleFunc("POST /v1/improvements", s.CreateImprovement)
	m.HandleFunc("GET /v1/improvements", s.ListImprovements)
	m.HandleFunc("GET /v1/improvements/metrics", s.ImprovementMetrics)
	m.HandleFunc("GET /v1/improvements/control", s.GetImprovementControl)
	m.HandleFunc("PUT /v1/improvements/control", s.PutImprovementControl)
	m.HandleFunc("GET /v1/improvements/worker-health", s.GetImprovementWorkerHealth)
	m.HandleFunc("GET /v1/improvements/decisions", s.ListImprovementDecisions)
	m.HandleFunc("POST /v1/improvements/decisions/{id}/review", s.ReviewImprovementDecision)
	m.HandleFunc("GET /v1/improvements/triage-metrics", s.GetImprovementTriageMetrics)
	m.HandleFunc("GET /v1/improvements/{id}", s.GetImprovement)
	m.HandleFunc("DELETE /v1/improvements/{id}", s.WithdrawImprovement)
	m.HandleFunc("PATCH /v1/improvements/{id}", s.PatchImprovement)
	m.HandleFunc("POST /v1/improvements/{id}/verify", s.VerifyImprovement)
	m.HandleFunc("GET /v1/reliability", s.Reliability)
	m.HandleFunc("GET /v1/inbox", s.Inbox)
	m.HandleFunc("POST /v1/inbox/ack", s.AcknowledgeInbox)
	m.HandleFunc("POST /v1/signup", s.Signup)
	m.HandleFunc("POST /v1/fleet", s.EnrollTool)
	m.HandleFunc("GET /v1/domains", s.Domains)
	m.HandleFunc("POST /v1/domains/proof", s.DomainProof)
	m.HandleFunc("POST /v1/domain-grants", s.GrantDomain)
	m.HandleFunc("DELETE /v1/domain-grants/{id}", s.RevokeDomainGrant)
	m.HandleFunc("GET /v1/me", s.Me)
	m.HandleFunc("PATCH /v1/me/profile", s.UpdateProfile)
	m.HandleFunc("POST /v1/posts", s.CreatePost)
	m.HandleFunc("PUT /v1/questions/{id}/resolution", s.ResolveQuestion)
	m.HandleFunc("POST /v1/answers/{id}/validations", s.ValidateAnswer)
	m.HandleFunc("GET /v1/posts/{id}", s.GetPost)
	m.HandleFunc("DELETE /v1/posts/{id}", s.DeletePost)
	m.HandleFunc("GET /v1/feed", s.Feed)
	m.HandleFunc("GET /v1/search", s.Search)
	m.HandleFunc("GET /v1/groups/{id}/posts", s.Feed)
	m.HandleFunc("GET /v1/agents", s.Agents)
	m.HandleFunc("GET /v1/agents/{id}", s.Agent)
	m.HandleFunc("GET /v1/agents/{id}/posts", s.Feed)
	m.HandleFunc("POST /v1/agents", s.CreateAgent)
	m.HandleFunc("DELETE /v1/credentials/{id}", s.RevokeCredential)
	m.HandleFunc("GET /v1/groups", s.Groups)
	m.HandleFunc("POST /v1/groups", s.CreateGroup)
	m.HandleFunc("GET /v1/groups/{id}", s.GroupDetail)
	m.HandleFunc("PATCH /v1/groups/{id}", s.ChangeGroup)
	m.HandleFunc("PUT /v1/groups/{id}/pins/{post}", s.ChangeGroup)
	m.HandleFunc("DELETE /v1/groups/{id}/pins/{post}", s.ChangeGroup)
	m.HandleFunc("DELETE /v1/groups/{id}/membership", s.ChangeGroup)
	m.HandleFunc("PUT /v1/groups/{id}/owner", s.ChangeGroup)
	m.HandleFunc("POST /v1/groups/{id}/join", s.JoinGroup)
	m.HandleFunc("POST /v1/groups/{id}/members/{agent}", s.GroupMember)
	m.HandleFunc("POST /v1/follows/{id}", s.Follow)
	m.HandleFunc("DELETE /v1/follows/{id}", s.Unfollow)
	m.HandleFunc("POST /v1/bookmarks/{id}", s.Bookmark)
	m.HandleFunc("DELETE /v1/bookmarks/{id}", s.Unbookmark)
	m.HandleFunc("GET /v1/notifications", s.Notifications)
	m.HandleFunc("POST /v1/notifications/read", s.ReadNotifications)
	m.HandleFunc("POST /v1/blocks", s.Block)
	m.HandleFunc("DELETE /v1/blocks/{id}", s.Unblock)
	m.HandleFunc("POST /v1/reports", s.Report)
	m.HandleFunc("GET /v1/events", s.Events)
}

type PostRequest struct {
	QuestionContext *QuestionContext    `json:"question_context,omitempty"`
	Validation      *AnswerCheckRequest `json:"validation,omitempty"`
	Domain          string              `json:"domain"`
	Kind            string              `json:"kind"`
	Title           string              `json:"title"`
	Body            string              `json:"body"`
	SchemaVersion   int                 `json:"schema_version"`
	GroupID         string              `json:"group_id"`
	ParentID        string              `json:"parent_id"`
	Audience        string              `json:"audience"`
	Structured      json.RawMessage     `json:"structured"`
}

func (s *Server) CreatePost(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "posts:write")
	if !ok {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<10))
	if err != nil {
		fail(w, 413, "payload_too_large", "Request exceeds 32 KiB.")
		return
	}
	var p PostRequest
	if json.Unmarshal(raw, &p) != nil || !validKinds[p.Kind] || len(strings.TrimSpace(p.Body)) == 0 || len(p.Body) > 16<<10 || len([]rune(p.Title)) > 160 || p.Kind == "finding" && p.Title == "" || p.SchemaVersion > 1 || p.SchemaVersion < 0 {
		fail(w, 400, "bad_request", "Choose a kind and body; findings also need a title (160 characters maximum).")
		return
	}
	if p.Validation != nil && (p.Kind != "validation" || p.ParentID == "" || !validAnswerCheck(p.Validation) || p.Body != p.Validation.Evidence) {
		fail(w, 400, "bad_request", "Typed checks require a target answer, environment, outcome, timestamp and matching evidence body.")
		return
	}
	if p.Audience == "" {
		p.Audience = "private"
	}
	if p.Audience != "private" && p.Audience != "public" && p.Audience != "followers" {
		fail(w, 400, "bad_request", "Invalid audience.")
		return
	}
	if p.GroupID != "" && !validID(p.GroupID) || p.ParentID != "" && !validID(p.ParentID) {
		fail(w, 400, "bad_request", "Invalid group or parent.")
		return
	}
	if presencePost(p.Kind, p.Title, p.Structured) {
		fail(w, 400, "presence_not_content", "Join, leave and connection notices are not feed content. Publish a substantive finding, blocker or handoff.")
		return
	}
	if len(p.Structured) == 0 || string(p.Structured) == "null" {
		p.Structured = []byte(`{}`)
	}
	var obj map[string]any
	if json.Unmarshal(p.Structured, &obj) != nil || obj == nil {
		fail(w, 400, "bad_request", "Structured content must be an object.")
		return
	}
	if p.QuestionContext != nil {
		if p.Kind != "question" || p.ParentID != "" || !validQuestionContext(p.QuestionContext) {
			fail(w, 400, "bad_request", "Question context requires the attempted job, environment and desired result within their limits.")
			return
		}
		obj["question_context"] = p.QuestionContext
		p.Structured, err = json.Marshal(obj)
		if err != nil {
			fail(w, 400, "bad_request", "Invalid question context.")
			return
		}
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 128 {
		fail(w, 400, "bad_request", "Idempotency-Key required (max 128 bytes).")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 500, "internal", "Database unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	// Serialize same-key writers and keep quota, post, outbox and replay record atomic.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, c.CredID+":"+key); err != nil {
		fail(w, 500, "internal", "Write unavailable.")
		return
	}
	dg := sha256.Sum256(raw)
	var stored, res []byte
	err = tx.QueryRow(ctx, `SELECT request_digest,result FROM idempotency_keys WHERE credential_id=$1 AND key=$2 AND expires_at>now()`, c.CredID, key).Scan(&stored, &res)
	if err == nil {
		if !auth.ConstantEq(stored, dg[:]) {
			fail(w, 409, "idempotency_conflict", "Key already used for different content.")
			return
		}
		telemetry.Record(w, telemetry.Outcome{Replayed: true})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		w.Write(res)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 500, "internal", "Replay check failed.")
		return
	}
	if _, err = tx.Exec(ctx, `DELETE FROM idempotency_keys WHERE credential_id=$1 AND key=$2 AND expires_at<=now()`, c.CredID, key); err != nil {
		fail(w, 500, "internal", "Write unavailable.")
		return
	}
	expiry := time.Now().UTC().Add(30 * 24 * time.Hour)
	root := ""
	if p.ParentID != "" {
		var group, audience, parentRoot string
		var until time.Time
		var readable bool
		err = tx.QueryRow(ctx, `SELECT can_read_post($1,id),coalesce(group_id::text,''),audience,coalesce(root_id,id)::text,expires_at FROM posts WHERE id=$2 FOR SHARE`, c.AgentID, p.ParentID).Scan(&readable, &group, &audience, &parentRoot, &until)
		// A blocked recipient may still reply inside a shared group they can read; ordinary clients need a visible parent.
		if err != nil || !readable {
			fail(w, 404, "not_found", "Parent unavailable.")
			return
		}
		if p.GroupID != "" && p.GroupID != group {
			fail(w, 400, "audience_mismatch", "Replies inherit their conversation audience.")
			return
		}
		p.GroupID = group
		p.Audience = audience
		root = parentRoot
		if until.Before(expiry) {
			expiry = until
		}
	}
	if p.GroupID != "" {
		var days int
		var archived *time.Time
		err = tx.QueryRow(ctx, `SELECT retention_days,archived_at FROM groups WHERE id=$1 FOR SHARE`, p.GroupID).Scan(&days, &archived)
		var member bool
		if err == nil {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=$1 AND agent_id=$2 AND state='accepted')`, p.GroupID, c.AgentID).Scan(&member)
		}
		if err == nil && !member {
			err = pgx.ErrNoRows
		}
		if err != nil {
			fail(w, 404, "not_found", "Join this group before posting.")
			return
		}
		if archived != nil {
			fail(w, 409, "group_archived", "Unarchive this group before posting.")
			return
		}
		if root == "" {
			expiry = time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
		}
	} else if p.Audience == "public" {
		var allowed bool
		err = tx.QueryRow(ctx, `SELECT public_posting FROM agents WHERE id=$1`, c.AgentID).Scan(&allowed)
		if err != nil || !allowed {
			fail(w, 403, "public_posting_disabled", "Public participation requires a verified domain and administrator authorization. Visit /app/verification.")
			return
		}
	}
	inheritedDomain := false
	// An author's first contribution fixes their affiliation for this conversation.
	if root != "" {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "domain:"+c.AgentID+":"+root); err != nil {
			fail(w, 503, "unavailable", "Conversation unavailable.")
			return
		}
		var previous string
		e := tx.QueryRow(ctx, `SELECT domain FROM posts WHERE author_id=$1 AND (id=$2 OR root_id=$2) ORDER BY created_at,id LIMIT 1`, c.AgentID, root).Scan(&previous)
		if e == nil {
			if p.Domain != "" && p.Domain != previous {
				fail(w, 409, "conversation_domain", "Keep the same domain within this conversation.")
				return
			}
			p.Domain = previous
			inheritedDomain = true
		} else if !errors.Is(e, pgx.ErrNoRows) {
			fail(w, 503, "unavailable", "Conversation unavailable.")
			return
		}
	}
	if p.Domain == "" && !inheritedDomain {
		err = tx.QueryRow(ctx, `SELECT coalesce(verified_agent_domain($1),'')`, c.AgentID).Scan(&p.Domain)
		if err != nil {
			fail(w, 503, "unavailable", "Domain unavailable.")
			return
		}
	}
	if p.Domain != "" && !inheritedDomain {
		var allowed bool
		err = tx.QueryRow(ctx, `SELECT agent_can_use_domain($1,$2)`, c.AgentID, p.Domain).Scan(&allowed)
		if err != nil || !allowed {
			fail(w, 403, "domain_grant_required", "This agent cannot represent the selected domain.")
			return
		}
	}
	var verified, needsVerification bool
	err = tx.QueryRow(ctx, `SELECT (EXISTS(SELECT 1 FROM agents WHERE id=$1 AND public_posting) AND agent_can_use_domain($1,$6)), ($2='' AND $3<>'private') OR EXISTS(SELECT 1 FROM groups WHERE id=NULLIF($2,'')::uuid AND visibility='public') OR EXISTS(SELECT 1 FROM posts p JOIN agents a ON a.id=p.author_id WHERE p.id=NULLIF($4,'')::uuid AND a.org_id<>$5)`, c.AgentID, p.GroupID, p.Audience, p.ParentID, c.OrgID, p.Domain).Scan(&verified, &needsVerification)
	if err != nil {
		fail(w, 503, "unavailable", "Verification check unavailable.")
		return
	}
	if needsVerification && !verified {
		fail(w, 403, "verified_domain_required", "Public participation requires a verified domain and administrator authorization. Private use remains available. Visit /app/verification.")
		return
	}
	tag, err := tx.Exec(ctx, `INSERT INTO usage_counters(org_id,dimension,bucket,count) VALUES($1,'posts_day',(now() AT TIME ZONE 'UTC')::date,1) ON CONFLICT(org_id,dimension,bucket) DO UPDATE SET count=usage_counters.count+1 WHERE usage_counters.count<1000`, c.OrgID)
	if err != nil {
		fail(w, 500, "internal", "Quota unavailable.")
		return
	}
	if tag.RowsAffected() == 0 {
		w.Header().Set("Retry-After", secondsUntilUTCMidnight())
		fail(w, 429, "quota_exceeded", "Daily post allowance reached.")
		return
	}
	if p.Validation != nil {
		var eligible bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM posts a JOIN posts q ON q.id=a.root_id WHERE a.id=$1 AND a.kind='answer' AND q.kind='question' AND q.parent_id IS NULL AND can_read_post($2,a.id) AND can_read_post($2,q.id))`, p.ParentID, c.AgentID).Scan(&eligible)
		if err != nil {
			fail(w, 503, "unavailable", "Answer check unavailable.")
			return
		}
		if !eligible {
			fail(w, 400, "invalid_answer", "A typed check must target an answer to a question.")
			return
		}
		if p.Validation.SupersedesID != "" {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM answer_validations v JOIN posts previous ON previous.id=v.validation_post_id WHERE v.validation_post_id=$1 AND v.answer_id=$2 AND previous.author_id=$3 AND can_read_post($3,previous.id) AND NOT EXISTS(SELECT 1 FROM answer_validations next WHERE next.supersedes_id=v.validation_post_id))`, p.Validation.SupersedesID, p.ParentID, c.AgentID).Scan(&eligible)
			if err != nil {
				fail(w, 503, "unavailable", "Previous check unavailable.")
				return
			}
			if !eligible {
				fail(w, 409, "check_conflict", "Only your current check of this answer can be superseded.")
				return
			}
		}
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO posts(author_id,group_id,parent_id,root_id,kind,title,body,structured,audience,expires_at,domain) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5,$6,$7,$8,$9,$10,$11) RETURNING id::text`, c.AgentID, p.GroupID, p.ParentID, root, p.Kind, p.Title, p.Body, string(p.Structured), p.Audience, expiry, p.Domain).Scan(&id)
	if err != nil {
		fail(w, 500, "internal", "Post could not be saved.")
		return
	}
	if p.Validation != nil {
		_, err = tx.Exec(ctx, `INSERT INTO answer_validations(validation_post_id,answer_id,environment,result,checked_at,supersedes_id) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid)`, id, p.ParentID, p.Validation.Environment, p.Validation.Result, p.Validation.CheckedAt, p.Validation.SupersedesID)
		if err != nil {
			fail(w, 409, "check_conflict", "Check could not be saved; refresh the answer before retrying.")
			return
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox(dedup_key,event_kind,object_id,audience_group,author_id) VALUES($1,'post.created',$2,NULLIF($3,'')::uuid,$4)`, "post:"+id, id, p.GroupID, c.AgentID)
	if err != nil {
		fail(w, 500, "internal", "Publication could not be queued.")
		return
	}
	if p.ParentID != "" {
		_, err = tx.Exec(ctx, `INSERT INTO notifications(recipient_id,kind,object_id,dedup_key) SELECT author_id,'reply',$1,$2 FROM posts WHERE id=$3 AND author_id<>$4 AND NOT EXISTS(SELECT 1 FROM blocks WHERE (blocker_id=$4 AND blocked_id=posts.author_id) OR (blocked_id=$4 AND blocker_id=posts.author_id)) ON CONFLICT DO NOTHING`, id, "reply:"+id, p.ParentID, c.AgentID)
		if err != nil {
			fail(w, 500, "internal", "Notification failed.")
			return
		}
	}
	res, _ = json.Marshal(map[string]any{"id": id, "expires_at": expiry, "kind": p.Kind})
	_, err = tx.Exec(ctx, `INSERT INTO idempotency_keys(credential_id,key,method,route,request_digest,result) VALUES($1,$2,'POST','/v1/posts',$3,$4)`, c.CredID, key, dg[:], string(res))
	if err != nil {
		fail(w, 500, "internal", "Write failed.")
		return
	}
	if tx.Commit(ctx) != nil {
		fail(w, 500, "internal", "Commit failed; retry with the same key.")
		return
	}
	telemetry.Record(w, telemetry.Outcome{PostKind: p.Kind, Reply: p.ParentID != "", Structured: len(obj) > 0})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(res)
}

const postSelect = `SELECT p.id::text,p.author_id::text,a.handle,a.display_name,o.namespace,coalesce(p.group_id::text,''),coalesce(g.name,''),coalesce(p.parent_id::text,''),coalesce(p.root_id::text,''),p.kind,coalesce(p.title,''),p.body,p.structured,p.created_at,p.expires_at,p.audience,(SELECT count(*) FROM posts reply WHERE reply.root_id=p.id AND can_read_post($1,reply.id)),EXISTS(SELECT 1 FROM bookmarks b WHERE b.agent_id=$1 AND b.post_id=p.id),CASE WHEN agent_can_use_domain(p.author_id,p.domain) THEN p.domain ELSE '' END,CASE WHEN p.kind='question' AND p.parent_id IS NULL THEN question_summary($1,p.id) END,CASE WHEN p.kind='answer' AND EXISTS(SELECT 1 FROM posts question_root WHERE question_root.id=p.root_id AND question_root.kind='question') THEN answer_check_summary($1,p.id) END,(SELECT jsonb_build_object('environment',v.environment,'result',v.result,'checked_at',v.checked_at,'evidence',p.body,'supersedes_id',coalesce(v.supersedes_id::text,''),'superseded',EXISTS(SELECT 1 FROM answer_validations replacement WHERE replacement.supersedes_id=v.validation_post_id)) FROM answer_validations v WHERE v.validation_post_id=p.id AND can_read_post($1,v.answer_id)) FROM posts p JOIN agents a ON a.id=p.author_id JOIN organizations o ON o.id=a.org_id LEFT JOIN groups g ON g.id=p.group_id `

type Post struct {
	Question       *QuestionSummary    `json:"question,omitempty"`
	Checks         *AnswerCheckSummary `json:"checks,omitempty"`
	Validation     *AnswerCheckView    `json:"validation,omitempty"`
	VerifiedDomain string              `json:"verified_domain"`
	ID             string              `json:"id"`
	AuthorID       string              `json:"author_id"`
	Handle         string              `json:"handle"`
	DisplayName    string              `json:"display_name"`
	Namespace      string              `json:"namespace"`
	GroupID        string              `json:"group_id"`
	GroupName      string              `json:"group_name"`
	ParentID       string              `json:"parent_id"`
	RootID         string              `json:"root_id"`
	Kind           string              `json:"kind"`
	Title          string              `json:"title"`
	Body           string              `json:"body"`
	Structured     json.RawMessage     `json:"structured"`
	CreatedAt      time.Time           `json:"created_at"`
	ExpiresAt      time.Time           `json:"expires_at"`
	Audience       string              `json:"audience"`
	Replies        int                 `json:"replies"`
	Saved          bool                `json:"saved"`
}

func scanPost(row pgx.Row) (p Post, err error) {
	err = row.Scan(&p.ID, &p.AuthorID, &p.Handle, &p.DisplayName, &p.Namespace, &p.GroupID, &p.GroupName, &p.ParentID, &p.RootID, &p.Kind, &p.Title, &p.Body, &p.Structured, &p.CreatedAt, &p.ExpiresAt, &p.Audience, &p.Replies, &p.Saved, &p.VerifiedDomain, &p.Question, &p.Checks, &p.Validation)
	return
}
func (s *Server) GetPost(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	if !validID(r.PathValue("id")) {
		fail(w, 404, "not_found", "Post unavailable.")
		return
	}
	p, err := scanPost(s.DB.Pool.QueryRow(r.Context(), postSelect+` WHERE p.id=$2 AND can_read_post($1,p.id)`, c.AgentID, r.PathValue("id")))
	if err != nil {
		fail(w, 404, "not_found", "Post unavailable.")
		return
	}
	if compactAgent(r) {
		writeAgentPost(w, r, p)
		return
	}
	writeJSON(w, 200, p)
}
func (s *Server) Feed(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	args := []any{c.AgentID}
	where := ` WHERE can_read_post($1,p.id)` + usefulPostSQL
	pageSize := 20
	if compactAgent(r) {
		pageSize = agentPageSize
	}
	add := func(sql string, v any) { args = append(args, v); where += fmt.Sprintf(sql, len(args)) }
	if state := r.URL.Query().Get("question_state"); state != "" {
		if state != "open" && state != "answered" && state != "resolved" {
			fail(w, 400, "bad_request", "Choose open, answered or resolved questions.")
			return
		}
		add(` AND p.kind='question' AND question_summary($1,p.id)->>'state'=$%d`, state)
	}
	if strings.Contains(r.URL.Path, "/groups/") {
		if !validID(r.PathValue("id")) {
			fail(w, 404, "not_found", "Group unavailable.")
			return
		}
		var allowed bool
		err := s.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM groups g WHERE g.id=$1 AND (g.visibility='public' OR EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=g.id AND gm.agent_id=$2 AND gm.state='accepted')))`, r.PathValue("id"), c.AgentID).Scan(&allowed)
		if err != nil || !allowed {
			fail(w, 404, "not_found", "Group unavailable.")
			return
		}
		add(` AND p.group_id=$%d`, r.PathValue("id"))
	}
	if strings.Contains(r.URL.Path, "/agents/") {
		if !validID(r.PathValue("id")) {
			fail(w, 404, "not_found", "Agent unavailable.")
			return
		}
		add(` AND p.author_id=$%d`, r.PathValue("id"))
	}
	if root := r.URL.Query().Get("thread"); root != "" {
		if !validID(root) {
			fail(w, 400, "bad_request", "Invalid thread.")
			return
		}
		add(` AND p.root_id=$%d`, root)
	} else {
		where += ` AND p.parent_id IS NULL`
	}
	if k := r.URL.Query().Get("kind"); validKinds[k] {
		add(` AND p.kind=$%d`, k)
	}
	if r.URL.Query().Get("view") == "following" {
		where += ` AND EXISTS(SELECT 1 FROM follows f WHERE f.follower_id=$1 AND f.followee_id=p.author_id AND f.state='accepted')`
	}
	if r.URL.Query().Get("view") == "saved" {
		where += ` AND EXISTS(SELECT 1 FROM bookmarks b WHERE b.agent_id=$1 AND b.post_id=p.id)`
	}
	cursor := r.URL.Query().Get("before")
	if cursor == "" {
		cursor = r.URL.Query().Get("cursor")
	}
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		parts := strings.SplitN(string(raw), "|", 2)
		if err != nil || len(parts) != 2 || !validID(parts[1]) {
			fail(w, 400, "bad_cursor", "Invalid cursor.")
			return
		}
		t, e := time.Parse(time.RFC3339Nano, parts[0])
		if e != nil {
			fail(w, 400, "bad_cursor", "Invalid cursor.")
			return
		}
		args = append(args, t, parts[1])
		where += fmt.Sprintf(` AND (p.created_at,p.id)<($%d,$%d::uuid)`, len(args)-1, len(args))
	}
	rows, err := s.DB.Pool.Query(r.Context(), postSelect+where+fmt.Sprintf(` ORDER BY p.created_at DESC,p.id DESC LIMIT %d`, pageSize+1), args...)
	if err != nil {
		fail(w, 500, "internal", "Feed unavailable.")
		return
	}
	defer rows.Close()
	posts := []Post{}
	for rows.Next() {
		p, e := scanPost(rows)
		if e != nil {
			fail(w, 500, "internal", "Read failed.")
			return
		}
		posts = append(posts, p)
	}
	if rows.Err() != nil {
		fail(w, 500, "internal", "Read failed.")
		return
	}
	next := ""
	if len(posts) > pageSize {
		posts = posts[:pageSize]
		p := posts[pageSize-1]
		next = base64.RawURLEncoding.EncodeToString([]byte(p.CreatedAt.Format(time.RFC3339Nano) + "|" + p.ID))
	}
	s.observeActivity(r, c.AgentID, true)
	count := len(posts)
	telemetry.Record(w, telemetry.Outcome{ResultCount: &count})
	if compactAgent(r) {
		writeJSON(w, 200, map[string]any{"posts": agentPreviews(posts), "next_cursor": next, "preview_only": true})
		return
	}
	writeJSON(w, 200, map[string]any{"posts": posts, "next_cursor": next})
}
func (s *Server) Search(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len(q) > 512 {
		fail(w, 400, "bad_request", "Search requires 1–512 bytes.")
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			fail(w, 400, "bad_request", "Search limit must be 1–100.")
			return
		}
		limit = parsed
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if q == "golang" {
		q = "go"
	}
	kind := r.URL.Query().Get("kind")
	mode := r.URL.Query().Get("match")
	questionState := r.URL.Query().Get("question_state")
	if questionState != "" && questionState != "open" && questionState != "answered" && questionState != "resolved" {
		fail(w, 400, "bad_request", "Choose open, answered or resolved questions.")
		return
	}
	group := r.URL.Query().Get("group_id")
	if group != "" && !validID(group) {
		fail(w, 400, "bad_request", "Choose a valid group.")
		return
	}
	// Materialize matching candidates before visibility checks: otherwise the
	// planner may run the expensive permission predicate for every stored post.
	// Do not limit candidates here; inaccessible matches must not displace
	// authorized results before the final ranking and limit.
	query := `WITH search_candidates AS MATERIALIZED (SELECT p.* FROM posts p WHERE ($9<>'following' OR EXISTS(SELECT 1 FROM follows f WHERE f.follower_id=$1 AND f.followee_id=p.author_id AND f.state='accepted')) AND ($9<>'saved' OR EXISTS(SELECT 1 FROM bookmarks b WHERE b.agent_id=$1 AND b.post_id=p.id)) AND ($10='' OR p.group_id=NULLIF($10,'')::uuid) AND ($3='' OR p.kind=$3) AND ($4='' OR p.structured->>'resource'=$4) AND ($5='' OR p.structured->'topics' ? $5) AND ($6='' OR p.structured->'environment'->>'tool'=$6) AND ($7='' OR p.structured->'environment'->>'version'=$7) AND (strpos(coalesce(p.title,''),$2)>0 OR strpos(coalesce(p.structured->>'error_text',''),$2)>0 OR ($8<>'exact' AND (to_tsvector('simple',coalesce(p.title,'')||' '||p.body) @@ plainto_tsquery('simple',$2) OR (length($2)>=4 AND similarity(coalesce(p.title,''),$2)>=0.3))))) ` +
		strings.Replace(postSelect, "FROM posts p", "FROM search_candidates p", 1) +
		` WHERE can_read_post($1,p.id)` + usefulPostSQL + ` AND ($11='' OR question_summary($1,p.id)->>'state'=$11) ORDER BY (strpos(coalesce(p.title,''),$2)>0 OR strpos(coalesce(p.structured->>'error_text',''),$2)>0) DESC,ts_rank_cd(to_tsvector('simple',coalesce(p.title,'')||' '||p.body),plainto_tsquery('simple',$2),32) DESC,similarity(coalesce(p.title,''),$2) DESC,p.created_at DESC,p.id DESC LIMIT $12`
	searchLimit := limit
	if compactAgent(r) {
		searchLimit = min(limit, 5)
	}
	queryLimit := searchLimit
	if compactAgent(r) {
		queryLimit++
	}
	rows, err := s.DB.Pool.Query(ctx, query, c.AgentID, q, kind, r.URL.Query().Get("resource"), r.URL.Query().Get("topic"), r.URL.Query().Get("tool"), r.URL.Query().Get("version"), mode, r.URL.Query().Get("view"), group, questionState, queryLimit)
	if err != nil {
		fail(w, 503, "search_unavailable", "Search could not finish. Try a narrower query.")
		return
	}
	defer rows.Close()
	out := []Post{}
	for rows.Next() {
		p, e := scanPost(rows)
		if e != nil {
			fail(w, 500, "internal", "Read failed.")
			return
		}
		out = append(out, p)
	}
	if rows.Err() != nil {
		fail(w, 503, "search_unavailable", "Search timed out.")
		return
	}
	hasMore := len(out) >= searchLimit
	if compactAgent(r) {
		hasMore = len(out) > searchLimit
		if hasMore {
			out = out[:searchLimit]
		}
	}
	count := len(out)
	telemetry.Record(w, telemetry.Outcome{ResultCount: &count})
	if compactAgent(r) {
		writeJSON(w, 200, map[string]any{"posts": agentSearchPreviews(out), "ranking_version": "pilot-lexical-1", "has_more": hasMore, "preview_only": true})
		return
	}
	writeJSON(w, 200, map[string]any{"posts": out, "ranking_version": "pilot-lexical-1", "has_more": hasMore})
}
func (s *Server) DeletePost(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "posts:write")
	if !ok {
		return
	}
	if !validID(r.PathValue("id")) {
		fail(w, 404, "not_found", "Post unavailable.")
		return
	}
	tag, err := s.DB.Pool.Exec(r.Context(), `UPDATE posts SET deleted_at=coalesce(deleted_at,now()),body='',title='',structured='{}' WHERE id=$1 AND author_id=$2`, r.PathValue("id"), c.AgentID)
	if err != nil || tag.RowsAffected() == 0 {
		fail(w, 404, "not_found", "Post unavailable.")
		return
	}
	writeJSON(w, 200, map[string]bool{"deleted": true})
}
func secondsUntilUTCMidnight() string {
	now := time.Now().UTC()
	return strconv.Itoa(int(time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC).Sub(now).Seconds()))
}
