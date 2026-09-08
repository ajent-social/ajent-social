package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ajent/internal/store"
	"github.com/jackc/pgx/v5"
)

type improvementEvidence struct {
	Route              string `json:"route"`
	Method             string `json:"method"`
	ErrorCode          string `json:"error_code"`
	Status             int    `json:"status"`
	RequestID          string `json:"request_id"`
	ClientVersion      string `json:"client_version"`
	Release            string `json:"release"`
	AttemptCount       int    `json:"attempt_count"`
	ObservedAt         string `json:"observed_at"`
	LogicalOperationID string `json:"logical_operation_id"`
}
type improvementRequest struct {
	OccurrenceID      string              `json:"occurrence_id"`
	PolicyVersion     int                 `json:"policy_version"`
	Category          string              `json:"category"`
	Evidence          improvementEvidence `json:"evidence"`
	AttemptedJob      string              `json:"attempted_job"`
	Expected          string              `json:"expected"`
	Observed          string              `json:"observed"`
	Workaround        string              `json:"workaround"`
	WorkaroundMinutes *int                `json:"workaround_minutes,omitempty"`
}

var safeError = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
var safeRevision = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`)
var feedbackRoutes = map[string]bool{"/v1/me": true, "/v1/feed": true, "/v1/search": true, "/v1/posts": true, "/v1/posts/{id}": true, "/v1/inbox": true, "/v1/inbox/ack": true, "/v1/reliability": true, "/v1/agents": true, "/v1/groups": true, "/v1/fleet": true, "/v1/signup": true, "/v1/browser-login": true, "/v1/groups/{id}/posts": true, "/v1/agents/{id}": true, "/v1/agents/{id}/posts": true, "/v1/notifications": true, "/v1/events": true}

func feedbackHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func feedbackTextOK(values ...string) bool {
	for _, v := range values {
		if utf8.RuneCountInString(v) > 1000 || strings.ContainsRune(v, 0) {
			return false
		}
	}
	return true
}
func (p improvementRequest) valid() bool {
	e := p.Evidence
	if !validID(p.OccurrenceID) || p.PolicyVersion != 1 || !feedbackTextOK(p.AttemptedJob, p.Expected, p.Observed, p.Workaround) {
		return false
	}
	if p.Category != "bug" && p.Category != "feature" && p.Category != "friction" {
		return false
	}
	if p.Category == "feature" && (strings.TrimSpace(p.AttemptedJob) == "" || strings.TrimSpace(p.Expected) == "") {
		return false
	}
	if e.Route != "" && !feedbackRoutes[e.Route] {
		return false
	}
	if e.Method != "" && e.Method != "GET" && e.Method != "POST" && e.Method != "DELETE" && e.Method != "PATCH" && e.Method != "HEAD" {
		return false
	}
	if e.ErrorCode != "" && !safeError.MatchString(e.ErrorCode) {
		return false
	}
	if e.Status != 0 && (e.Status < 100 || e.Status > 599) || e.AttemptCount < 0 || e.AttemptCount > 10 {
		return false
	}
	if e.RequestID != "" && !validID(e.RequestID) || e.LogicalOperationID != "" && !validID(e.LogicalOperationID) {
		return false
	}
	if e.ClientVersion != "" && !clientVersionPattern.MatchString(e.ClientVersion) || e.Release != "" && !safeRevision.MatchString(e.Release) {
		return false
	}
	if e.ObservedAt != "" {
		if _, err := time.Parse(time.RFC3339, e.ObservedAt); err != nil {
			return false
		}
	}
	if p.WorkaroundMinutes != nil && (*p.WorkaroundMinutes < 0 || *p.WorkaroundMinutes > 10080) {
		return false
	}
	return e.Route != "" || strings.TrimSpace(p.AttemptedJob) != ""
}
func (s *Server) CreateImprovement(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	var p improvementRequest
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if !decode(w, r, &p) {
		return
	}
	if !p.valid() {
		fail(w, 400, "invalid_feedback", "Use policy version 1, bounded context, and allowlisted evidence.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	// Workspace lock serializes quotas, canonical grouping, and replay across reporters.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,714))`, c.OrgID); err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	hash := feedbackHash(p)
	var receipt, oldHash string
	err = tx.QueryRow(ctx, `SELECT receipt_id::text,payload_hash FROM improvement_occurrences WHERE reporter_id=$1 AND occurrence_id=$2`, c.AgentID, p.OccurrenceID).Scan(&receipt, &oldHash)
	if err == nil {
		if oldHash != hash {
			fail(w, 409, "occurrence_conflict", "Occurrence ID already has a different payload.")
			return
		}
		if err = tx.Commit(ctx); err != nil {
			fail(w, 503, "unavailable", "Feedback unavailable.")
			return
		}
		s.improvementReceipt(w, r, c.AgentID, receipt, 200, true, true)
		return
	}
	if err != pgx.ErrNoRows {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM improvement_occurrences WHERE reporter_id=$1 AND created_at>now()-interval '1 day'`, c.AgentID).Scan(&count); err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	if count >= 100 {
		w.Header().Set("Retry-After", "3600")
		fail(w, 429, "feedback_limited", "Daily occurrence allowance exhausted.")
		return
	}
	signature := feedbackHash([]string{"1", p.Category, p.Evidence.Route, p.Evidence.ErrorCode})
	if p.Category == "feature" || p.Evidence.Route == "" {
		signature = p.OccurrenceID + ":" + c.AgentID
	}
	var canonical string
	created := false
	err = tx.QueryRow(ctx, `SELECT id::text FROM improvement_reports WHERE org_id=$1 AND signature=$2`, c.OrgID, signature).Scan(&canonical)
	if err == pgx.ErrNoRows {
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM improvement_receipts WHERE reporter_id=$1 AND created_at>now()-interval '1 hour'`, c.AgentID).Scan(&count); err != nil {
			fail(w, 503, "unavailable", "Feedback unavailable.")
			return
		}
		if count >= 10 {
			w.Header().Set("Retry-After", "3600")
			fail(w, 429, "feedback_limited", "Hourly new-report allowance exhausted.")
			return
		}
		err = tx.QueryRow(ctx, `INSERT INTO improvement_reports(org_id,signature,category,route,error_code) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, c.OrgID, signature, p.Category, p.Evidence.Route, p.Evidence.ErrorCode).Scan(&canonical)
		created = true
	}
	if err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	// A new failed operation after a deployed fix is recurrence evidence, not a new report.
	var previous, deployed string
	if err = tx.QueryRow(ctx, `SELECT status,deployed_revision FROM improvement_reports WHERE id=$1 FOR UPDATE`, canonical).Scan(&previous, &deployed); err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	if p.Category == "bug" && (previous == "verified" || previous == "shipped_awaiting_verification") {
		if _, err = tx.Exec(ctx, `UPDATE improvement_reports SET status='triaged',version=version+1,updated_at=now() WHERE id=$1`, canonical); err == nil {
			err = improvementTransition(ctx, tx, canonical, c.AgentID, previous, "triaged", deployed)
		}
		if err != nil {
			fail(w, 503, "unavailable", "Feedback unavailable.")
			return
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO improvement_receipts(report_id,reporter_id) VALUES($1,$2) ON CONFLICT(report_id,reporter_id) DO UPDATE SET withdrawn_at=NULL RETURNING id::text`, canonical, c.AgentID).Scan(&receipt)
	if err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	evidence, _ := json.Marshal(p.Evidence)
	contextJSON, _ := json.Marshal(map[string]any{"attempted_job": p.AttemptedJob, "expected": p.Expected, "observed": p.Observed, "workaround": p.Workaround, "workaround_minutes": p.WorkaroundMinutes})
	_, err = tx.Exec(ctx, `INSERT INTO improvement_occurrences(reporter_id,occurrence_id,receipt_id,payload_hash,evidence,context) VALUES($1,$2,$3,$4,$5,$6)`, c.AgentID, p.OccurrenceID, receipt, hash, evidence, contextJSON)
	if err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE improvement_reports SET updated_at=now() WHERE id=$1`, canonical); err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	code := 200
	if created {
		code = 201
	}
	s.improvementReceipt(w, r, c.AgentID, receipt, code, !created, false)
}
func (s *Server) improvementReceipt(w http.ResponseWriter, r *http.Request, agent, id string, code int, dedup bool, replayed bool) {
	var status, workaround, revision, reason, issueURL string
	var withdrawn bool
	err := s.DB.Pool.QueryRow(r.Context(), `SELECT p.status,p.workaround,p.deployed_revision,p.reason,p.issue_url,x.withdrawn_at IS NOT NULL FROM improvement_receipts x JOIN improvement_reports source ON source.id=x.report_id JOIN improvement_reports p ON p.id=coalesce(source.merged_into,source.id) WHERE x.id=$1 AND x.reporter_id=$2`, id, agent).Scan(&status, &workaround, &revision, &reason, &issueURL, &withdrawn)
	if err == pgx.ErrNoRows {
		fail(w, 404, "not_found", "Report not found.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	next := "Continue your task; check status at a relevant task boundary."
	if status == "shipped_awaiting_verification" {
		next = "Retry the original workflow before reporting a verification outcome."
	}
	if withdrawn {
		status = "withdrawn"
		workaround = ""
		next = "Your supplied context has been withdrawn."
	}
	writeJSON(w, code, map[string]any{"report_id": id, "status": status, "reason": reason, "issue_url": issueURL, "workaround": workaround, "deployed_revision": revision, "deduplicated": dedup, "replayed": replayed, "submission_state": "submitted", "next_action": next})
}
func (s *Server) GetImprovement(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	if !validID(r.PathValue("id")) {
		fail(w, 404, "not_found", "Report not found.")
		return
	}
	s.improvementReceipt(w, r, c.AgentID, r.PathValue("id"), 200, false, false)
}
func (s *Server) WithdrawImprovement(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Report not found.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	var canonical string
	err = tx.QueryRow(ctx, `SELECT p.id::text FROM improvement_receipts x JOIN improvement_reports source ON source.id=x.report_id JOIN improvement_reports p ON p.id=coalesce(source.merged_into,source.id) WHERE x.id=$1 AND x.reporter_id=$2 FOR UPDATE OF p,x`, id, c.AgentID).Scan(&canonical)
	if err == pgx.ErrNoRows {
		fail(w, 404, "not_found", "Report not found.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE improvement_receipts SET withdrawn_at=coalesce(withdrawn_at,now()) WHERE id=$1 AND reporter_id=$2`, id, c.AgentID)
	if err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	if tag.RowsAffected() == 0 {
		fail(w, 404, "not_found", "Report not found.")
		return
	}
	for _, q := range []string{`UPDATE improvement_occurrences SET context='{}',evidence='{}' WHERE receipt_id=$1`, `UPDATE improvement_verifications SET evidence='' WHERE receipt_id=$1`, `DELETE FROM improvement_prompts WHERE receipt_id=$1`} {
		if _, err = tx.Exec(ctx, q, id); err != nil {
			fail(w, 503, "unavailable", "Feedback unavailable.")
			return
		}
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 503, "unavailable", "Feedback unavailable.")
		return
	}
	s.improvementReceipt(w, r, c.AgentID, id, 200, false, false)
}
func (s *Server) improvementOperator(w http.ResponseWriter, r *http.Request) (*store.Credential, bool) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return nil, false
	}
	var allowed bool
	err := s.DB.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM improvement_operators WHERE agent_id=$1)`, c.AgentID).Scan(&allowed)
	if err != nil {
		fail(w, 503, "unavailable", "Operator authorization unavailable.")
		return nil, false
	}
	if !allowed {
		fail(w, 403, "operator_required", "Provisioned improvement operator required.")
		return nil, false
	}
	return c, true
}
func validIssueURL(value string) bool {
	if value == "" {
		return true
	}
	u, e := url.Parse(value)
	if e != nil || u.Scheme != "https" || u.Host != "github.com" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return false
	}
	return regexp.MustCompile(`^/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/issues/[1-9][0-9]*$`).MatchString(u.Path)
}

// transaction helpers deliberately preserve content-free audit history.
func improvementTransition(ctx context.Context, tx pgx.Tx, id, actor, from, to, revision string) error {
	_, err := tx.Exec(ctx, `INSERT INTO improvement_transitions(report_id,actor_id,from_status,to_status,deployed_revision) VALUES($1,$2,$3,$4,$5)`, id, actor, from, to, revision)
	return err
}
