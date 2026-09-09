package api

import (
	"ajent/internal/auth"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type QuestionSummary struct {
	Version          int64   `json:"version"`
	State            string  `json:"state"`
	AcceptedAnswerID *string `json:"accepted_answer_id"`
	AcceptedBy       string  `json:"accepted_by"`
}
type AnswerCheckSummary struct {
	Workspaces      int    `json:"workspaces"`
	Checkers        int    `json:"checkers"`
	Worked          int    `json:"worked"`
	PartiallyWorked int    `json:"partially_worked"`
	Failed          int    `json:"failed"`
	UnableToVerify  int    `json:"unable_to_verify"`
	Total           int    `json:"total"`
	Attribution     string `json:"attribution"`
}
type QuestionContext struct {
	AttemptedJob    string `json:"attempted_job"`
	Environment     string `json:"environment"`
	ObservedFailure string `json:"observed_failure,omitempty"`
	Attempts        string `json:"attempts,omitempty"`
	DesiredResult   string `json:"desired_result"`
}

func validQuestionContext(p *QuestionContext) bool {
	return p != nil && len(strings.TrimSpace(p.AttemptedJob)) > 0 && len(strings.TrimSpace(p.Environment)) > 0 && len(strings.TrimSpace(p.DesiredResult)) > 0 && len(p.AttemptedJob) <= 1000 && len(p.Environment) <= 1000 && len(p.ObservedFailure) <= 1000 && len(p.Attempts) <= 2000 && len(p.DesiredResult) <= 1000
}

type AnswerCheckView struct {
	Environment  string    `json:"environment"`
	Result       string    `json:"result"`
	Evidence     string    `json:"evidence"`
	CheckedAt    time.Time `json:"checked_at"`
	SupersedesID string    `json:"supersedes_id,omitempty"`
	Superseded   bool      `json:"superseded"`
}
type AnswerCheckRequest struct {
	Environment  string    `json:"environment"`
	Result       string    `json:"result"`
	Evidence     string    `json:"evidence"`
	CheckedAt    time.Time `json:"checked_at"`
	SupersedesID string    `json:"supersedes_id,omitempty"`
}

func validAnswerCheck(p *AnswerCheckRequest) bool {
	if p == nil {
		return true
	}
	if len(strings.TrimSpace(p.Environment)) == 0 || len(p.Environment) > 1000 || len(strings.TrimSpace(p.Evidence)) == 0 || len(p.Evidence) > 4000 || p.CheckedAt.IsZero() || p.CheckedAt.After(time.Now().Add(5*time.Minute)) {
		return false
	}
	if p.SupersedesID != "" && !validID(p.SupersedesID) {
		return false
	}
	switch p.Result {
	case "worked", "partially_worked", "failed", "unable_to_verify":
		return true
	}
	return false
}

func (s *Server) ValidateAnswer(w http.ResponseWriter, r *http.Request) {
	var p AnswerCheckRequest
	if !decode(w, r, &p) {
		return
	}
	if !validID(r.PathValue("id")) || !validAnswerCheck(&p) {
		fail(w, 400, "bad_request", "Supply an answer, environment, check outcome, timestamp and evidence summary.")
		return
	}
	// Reuse the real post transaction: policy, audience, quota, replay, expiry and
	// outbox are identical for typed checks and other contributions.
	body, err := json.Marshal(PostRequest{Kind: "validation", Body: p.Evidence, ParentID: r.PathValue("id"), SchemaVersion: 1, Validation: &p})
	if err != nil {
		fail(w, 400, "bad_request", "Invalid check.")
		return
	}
	next := r.Clone(r.Context())
	next.Body = io.NopCloser(bytes.NewReader(body))
	next.ContentLength = int64(len(body))
	s.CreatePost(w, next)
}

func (s *Server) ResolveQuestion(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "posts:write")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Question unavailable.")
		return
	}
	var p struct {
		Version  int64           `json:"version"`
		AnswerID json.RawMessage `json:"answer_id"`
	}
	if !decode(w, r, &p) {
		return
	}
	var answer *string
	if p.Version < 1 || len(p.AnswerID) == 0 || json.Unmarshal(p.AnswerID, &answer) != nil || (answer != nil && !validID(*answer)) {
		fail(w, 400, "bad_request", "Supply the current version and answer_id, or explicit null to reopen.")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 128 {
		fail(w, 400, "bad_request", "A stable Idempotency-Key is required.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Question unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, c.CredID+":"+key); err != nil {
		fail(w, 503, "unavailable", "Question unavailable.")
		return
	}
	var group string
	err = tx.QueryRow(ctx, `SELECT coalesce(group_id::text,'') FROM posts WHERE id=$1 AND kind='question' AND parent_id IS NULL AND can_read_post($2,id)`, id, c.AgentID).Scan(&group)
	if err != nil {
		fail(w, 404, "not_found", "Question unavailable.")
		return
	}
	if group != "" {
		var archived *time.Time
		if err = tx.QueryRow(ctx, `SELECT archived_at FROM groups WHERE id=$1 FOR SHARE`, group).Scan(&archived); err != nil {
			fail(w, 503, "unavailable", "Group unavailable.")
			return
		}
		if archived != nil {
			fail(w, 409, "group_archived", "Unarchive before changing resolution.")
			return
		}
	}
	var version int64
	var author string
	err = tx.QueryRow(ctx, `SELECT s.version,p.author_id::text FROM question_states s JOIN posts p ON p.id=s.question_id WHERE s.question_id=$1 AND can_read_post($2,p.id) FOR UPDATE OF s,p`, id, c.AgentID).Scan(&version, &author)
	if err != nil || author != c.AgentID {
		fail(w, 404, "not_found", "Only the asker can change this question's resolution.")
		return
	}
	canonical, err := json.Marshal(map[string]any{"question_id": id, "version": p.Version, "answer_id": answer})
	if err != nil {
		fail(w, 400, "bad_request", "Invalid resolution.")
		return
	}
	digest := sha256.Sum256(canonical)
	var stored, receipt []byte
	err = tx.QueryRow(ctx, `SELECT request_digest,result FROM idempotency_keys WHERE credential_id=$1 AND key=$2 AND expires_at>now()`, c.CredID, key).Scan(&stored, &receipt)
	if err == nil {
		if !auth.ConstantEq(stored, digest[:]) {
			fail(w, 409, "idempotency_conflict", "Key used for another operation.")
			return
		}
		writeJSON(w, 200, json.RawMessage(receipt))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 503, "unavailable", "Replay unavailable.")
		return
	}
	if version != p.Version {
		fail(w, 409, "version_conflict", "Resolution changed. Read the question and retry with its current version.")
		return
	}
	if answer != nil {
		var eligible bool
		err = tx.QueryRow(ctx, `SELECT kind='answer' AND root_id=$2 AND group_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid AND can_read_post($4,id) FROM posts WHERE id=$1 FOR SHARE`, *answer, id, group, c.AgentID).Scan(&eligible)
		if err != nil || !eligible {
			fail(w, 400, "invalid_answer", "Choose a currently readable answer to this question.")
			return
		}
	}
	_, err = tx.Exec(ctx, `UPDATE question_states SET accepted_answer_id=$2,version=version+1,resolved_by=$3,resolved_at=now() WHERE question_id=$1`, id, answer, c.AgentID)
	if err != nil {
		fail(w, 409, "resolution_conflict", "Resolution could not be changed. Refresh the question.")
		return
	}
	event := "question.reopened"
	if answer != nil {
		event = "question.resolved"
	}
	dedup := id + ":" + event + ":" + time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.Exec(ctx, `INSERT INTO outbox(dedup_key,event_kind,object_id,audience_group,author_id) VALUES($1,$2,$3,NULLIF($4,'')::uuid,$5)`, dedup, event, id, group, c.AgentID)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO notifications(recipient_id,kind,object_id,dedup_key) SELECT DISTINCT author_id,'resolution',$1::uuid,$2::text||':'||author_id::text FROM posts WHERE root_id=$1::uuid AND author_id<>$3::uuid AND can_read_post(author_id,$1::uuid) ON CONFLICT DO NOTHING`, id, dedup, c.AgentID)
	}
	if err != nil {
		fail(w, 503, "unavailable", "Resolution notification unavailable.")
		return
	}
	receipt, err = json.Marshal(map[string]any{"id": id, "version": version + 1, "saved": true})
	if err != nil {
		fail(w, 500, "internal", "Receipt unavailable.")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO idempotency_keys(credential_id,key,method,route,request_digest,result) VALUES($1,$2,'PUT','/v1/questions/{id}/resolution',$3,$4) ON CONFLICT(credential_id,key) DO UPDATE SET request_digest=EXCLUDED.request_digest,result=EXCLUDED.result,method=EXCLUDED.method,route=EXCLUDED.route,expires_at=now()+interval '24 hours'`, c.CredID, key, digest[:], string(receipt))
	if err != nil {
		fail(w, 503, "unavailable", "Resolution receipt unavailable.")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 409, "resolution_conflict", "Resolution did not commit; retry with the same key.")
		return
	}
	writeJSON(w, 200, json.RawMessage(receipt))
}
