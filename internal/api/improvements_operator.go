package api

import (
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

func (s *Server) ListImprovements(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.improvementOperator(w, r); !ok {
		return
	}
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		cursor = "00000000-0000-0000-0000-000000000000"
	}
	if !validID(cursor) {
		fail(w, 400, "bad_cursor", "UUID cursor required.")
		return
	}
	status := r.URL.Query().Get("status")
	rows, err := s.DB.Pool.Query(r.Context(), `SELECT p.id::text,p.version,p.status,p.category,p.route,p.error_code,p.issue_url,p.deployed_revision,p.priority,p.owner,p.workaround,p.synthetic,p.created_at,
 (SELECT count(*) FROM improvement_occurrences o JOIN improvement_receipts x ON x.id=o.receipt_id WHERE (x.report_id=p.id OR x.report_id IN(SELECT id FROM improvement_reports WHERE merged_into=p.id))),
 coalesce((SELECT jsonb_agg(jsonb_build_object('evidence',o.evidence,'context',o.context)) FROM (SELECT o.evidence,o.context FROM improvement_occurrences o JOIN improvement_receipts x ON x.id=o.receipt_id WHERE (x.report_id=p.id OR x.report_id IN(SELECT id FROM improvement_reports WHERE merged_into=p.id)) AND x.withdrawn_at IS NULL ORDER BY o.created_at DESC LIMIT 5)o),'[]'::jsonb)
 FROM improvement_reports p WHERE p.merged_into IS NULL AND p.id>$1 AND ($2='' OR p.status=$2) ORDER BY p.id LIMIT 50`, cursor, status)
	if err != nil {
		fail(w, 503, "unavailable", "Triage unavailable.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	next := ""
	for rows.Next() {
		var id, state, category, route, errorCode, issue, revision, priority, owner, workaround string
		var version, count int64
		var synthetic bool
		var created time.Time
		var samples json.RawMessage
		if err = rows.Scan(&id, &version, &state, &category, &route, &errorCode, &issue, &revision, &priority, &owner, &workaround, &synthetic, &created, &count, &samples); err != nil {
			fail(w, 503, "unavailable", "Triage unavailable.")
			return
		}
		items = append(items, map[string]any{"id": id, "version": version, "status": state, "category": category, "route": route, "error_code": errorCode, "issue_url": issue, "deployed_revision": revision, "priority": priority, "owner": owner, "workaround": workaround, "synthetic": synthetic, "created_at": created, "occurrence_count": count, "samples": samples})
		next = id
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable", "Triage unavailable.")
		return
	}
	if len(items) < 50 {
		next = ""
	}
	writeJSON(w, 200, map[string]any{"items": items, "next_cursor": next})
}

type improvementPatch struct {
	MergeInto        *string `json:"merge_into"`
	MergeVersion     *int64  `json:"merge_version"`
	Version          int64   `json:"version"`
	Status           *string `json:"status"`
	Priority         *string `json:"priority"`
	Owner            *string `json:"owner"`
	Workaround       *string `json:"workaround"`
	Reason           *string `json:"reason"`
	IssueURL         *string `json:"issue_url"`
	DeployedRevision *string `json:"deployed_revision"`
}

func (s *Server) PatchImprovement(w http.ResponseWriter, r *http.Request) {
	c, ok := s.improvementOperator(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Report not found.")
		return
	}
	var p improvementPatch
	if !decode(w, r, &p) {
		return
	}
	for _, v := range []*string{p.Status, p.Priority, p.Owner, p.Workaround, p.Reason, p.IssueURL, p.DeployedRevision} {
		if v != nil && !feedbackTextOK(*v) {
			fail(w, 400, "invalid_feedback", "Triage text too long.")
			return
		}
	}
	if p.Version < 1 || p.IssueURL != nil && !validIssueURL(*p.IssueURL) || p.DeployedRevision != nil && !safeRevision.MatchString(*p.DeployedRevision) {
		fail(w, 400, "invalid_feedback", "Version, issue URL or revision invalid.")
		return
	}
	if p.Priority != nil && *p.Priority != "critical" && *p.Priority != "high" && *p.Priority != "normal" && *p.Priority != "low" {
		fail(w, 400, "invalid_feedback", "Unknown priority.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Triage unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	// Serialize sequence assignment with social event publishing so a later committed
	// event cannot advance a cursor past a still-uncommitted verification prompt.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(4242)`); err != nil {
		fail(w, 503, "unavailable", "Triage unavailable.")
		return
	}
	var old, revision string
	var version int64
	err = tx.QueryRow(ctx, `SELECT status,version,deployed_revision FROM improvement_reports WHERE id=$1 FOR UPDATE`, id).Scan(&old, &version, &revision)
	if err == pgx.ErrNoRows {
		fail(w, 404, "not_found", "Report not found.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Triage unavailable.")
		return
	}
	if version != p.Version {
		fail(w, 409, "version_conflict", "Refresh triage state before updating.")
		return
	}
	if p.MergeInto != nil {
		if !validID(*p.MergeInto) || *p.MergeInto == id || p.MergeVersion == nil {
			fail(w, 400, "invalid_merge", "Distinct target and its version required.")
			return
		}
		var targetVersion int64
		var allowed bool
		err = tx.QueryRow(ctx, `SELECT b.version,a.category='feature' AND b.category='feature' AND a.org_id=b.org_id AND a.merged_into IS NULL AND b.merged_into IS NULL AND a.status IN('new','triaged','planned') AND b.status IN('new','triaged','planned') AND NOT EXISTS(SELECT 1 FROM improvement_reports child WHERE child.merged_into=a.id) FROM improvement_reports a JOIN improvement_reports b ON b.id=$2 WHERE a.id=$1 FOR UPDATE OF b`, id, *p.MergeInto).Scan(&targetVersion, &allowed)
		if err == pgx.ErrNoRows {
			fail(w, 404, "not_found", "Merge target not found.")
			return
		}
		if err != nil {
			fail(w, 503, "unavailable", "Merge unavailable.")
			return
		}
		if !allowed || targetVersion != *p.MergeVersion {
			fail(w, 409, "merge_conflict", "Merge requires current same-workspace unshipped feature reports.")
			return
		}
		if _, err = tx.Exec(ctx, `UPDATE improvement_reports SET merged_into=$2,status='merged',version=version+1,updated_at=now() WHERE id=$1`, id, *p.MergeInto); err == nil {
			_, err = tx.Exec(ctx, `UPDATE improvement_reports SET version=version+1,updated_at=now() WHERE id=$1`, *p.MergeInto)
		}
		if err == nil {
			err = improvementTransition(ctx, tx, id, c.AgentID, old, "merged", revision)
		}
		if err != nil {
			fail(w, 503, "unavailable", "Merge unavailable.")
			return
		}
		if err = tx.Commit(ctx); err != nil {
			fail(w, 503, "unavailable", "Merge unavailable.")
			return
		}
		writeJSON(w, 200, map[string]any{"id": id, "merged_into": *p.MergeInto, "version": version + 1, "target_version": targetVersion + 1})
		return
	}
	if old == "merged" {
		fail(w, 409, "merged_report", "Update the canonical target report.")
		return
	}
	state := old
	if p.Status != nil {
		state = *p.Status
	}
	if p.DeployedRevision != nil {
		revision = *p.DeployedRevision
	}
	if p.DeployedRevision != nil && state != "shipped_awaiting_verification" {
		fail(w, 409, "invalid_transition", "Fix revisions can only be assigned to a shipped verification opportunity.")
		return
	}
	allowed := state == old || state == "declined" || state == "withdrawn" || old == "new" && state == "triaged" || old == "triaged" && state == "planned" || old == "planned" && state == "shipped_awaiting_verification" || old == "shipped_awaiting_verification" && state == "triaged" || old == "verified" && state == "triaged"
	if !allowed || state == "verified" && old != "verified" || state == "shipped_awaiting_verification" && revision == "" {
		fail(w, 409, "invalid_transition", "Follow triage, planned, deployed, then reporter verification.")
		return
	}
	if (state == "declined" || state == "withdrawn") && state != old && (p.Reason == nil || *p.Reason == "") {
		fail(w, 400, "reason_required", "Terminal transitions require a safe reason.")
		return
	}
	_, err = tx.Exec(ctx, `UPDATE improvement_reports SET status=$2,version=version+1,priority=coalesce($3,priority),owner=coalesce($4,owner),workaround=coalesce($5,workaround),reason=coalesce($6,reason),issue_url=coalesce($7,issue_url),deployed_revision=$8,updated_at=now() WHERE id=$1`, id, state, p.Priority, p.Owner, p.Workaround, p.Reason, p.IssueURL, revision)
	if err == nil {
		err = improvementTransition(ctx, tx, id, c.AgentID, old, state, revision)
	}
	if err == nil && state == "shipped_awaiting_verification" {
		_, err = tx.Exec(ctx, `INSERT INTO improvement_prompts(receipt_id,deployed_revision) SELECT id,$2 FROM improvement_receipts WHERE (report_id=$1 OR report_id IN(SELECT id FROM improvement_reports WHERE merged_into=$1)) AND withdrawn_at IS NULL ON CONFLICT DO NOTHING`, id, revision)
	}
	if err != nil {
		fail(w, 503, "unavailable", "Triage unavailable.")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 503, "unavailable", "Triage unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "status": state, "version": version + 1, "deployed_revision": revision})
}
func (s *Server) VerifyImprovement(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !validID(id) {
		fail(w, 404, "not_found", "Report not found.")
		return
	}
	var p struct {
		VerificationID   string `json:"verification_id"`
		DeployedRevision string `json:"deployed_revision"`
		Outcome          string `json:"outcome"`
		Evidence         string `json:"evidence"`
	}
	if !decode(w, r, &p) {
		return
	}
	if !validID(p.VerificationID) || !safeRevision.MatchString(p.DeployedRevision) || !feedbackTextOK(p.Evidence) {
		fail(w, 400, "invalid_feedback", "Bounded verification evidence, ID and revision required.")
		return
	}
	if p.Outcome != "reproduced" && p.Outcome != "not_reproduced" && p.Outcome != "unable_to_verify" && p.Outcome != "deferred" && p.Outcome != "dismissed" {
		fail(w, 400, "invalid_feedback", "Unknown verification outcome.")
		return
	}
	if (p.Outcome == "reproduced" || p.Outcome == "not_reproduced") && p.Evidence == "" {
		fail(w, 400, "evidence_required", "Describe the actual check before recording a result.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Verification unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	var canonical, state, revision string
	err = tx.QueryRow(ctx, `SELECT p.id::text,p.status,p.deployed_revision FROM improvement_receipts x JOIN improvement_reports source ON source.id=x.report_id JOIN improvement_reports p ON p.id=coalesce(source.merged_into,source.id) WHERE x.id=$1 AND x.reporter_id=$2 AND x.withdrawn_at IS NULL FOR UPDATE OF p,x`, id, c.AgentID).Scan(&canonical, &state, &revision)
	if err == pgx.ErrNoRows {
		fail(w, 404, "not_found", "Report not found.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Verification unavailable.")
		return
	}
	hash := feedbackHash(struct {
		ReceiptID string
		Payload   any
	}{id, p})
	var oldHash string
	err = tx.QueryRow(ctx, `SELECT payload_hash FROM improvement_verifications WHERE reporter_id=$1 AND verification_id=$2`, c.AgentID, p.VerificationID).Scan(&oldHash)
	if err == nil {
		if oldHash != hash {
			fail(w, 409, "occurrence_conflict", "Verification ID already used.")
			return
		}
		writeJSON(w, 200, map[string]any{"report_id": id, "status": state, "outcome": p.Outcome, "replayed": true})
		return
	}
	if err != pgx.ErrNoRows {
		fail(w, 503, "unavailable", "Verification unavailable.")
		return
	}
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM improvement_prompts WHERE receipt_id=$1 AND deployed_revision=$2 AND created_at>now()-interval '30 days')`, id, p.DeployedRevision).Scan(&eligible)
	if err != nil {
		fail(w, 503, "unavailable", "Verification unavailable.")
		return
	}
	if !eligible || revision != p.DeployedRevision || state != "shipped_awaiting_verification" && state != "verified" && state != "triaged" {
		fail(w, 409, "verification_not_eligible", "No current unexpired fix opportunity; inspect report status.")
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO improvement_verifications(reporter_id,verification_id,receipt_id,deployed_revision,outcome,evidence,payload_hash) VALUES($1,$2,$3,$4,$5,$6,$7)`, c.AgentID, p.VerificationID, id, p.DeployedRevision, p.Outcome, p.Evidence, hash)
	next := state
	if p.Outcome == "reproduced" {
		next = "triaged"
	}
	if p.Outcome == "not_reproduced" && state == "shipped_awaiting_verification" {
		next = "verified"
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE improvement_prompts SET responded_at=now() WHERE receipt_id=$1 AND deployed_revision=$2`, id, revision)
	}
	if err == nil && next != state {
		_, err = tx.Exec(ctx, `UPDATE improvement_reports SET status=$2,version=version+1,updated_at=now(),verified_at=CASE WHEN $2='verified' THEN now() ELSE verified_at END WHERE id=$1`, canonical, next)
		if err == nil {
			err = improvementTransition(ctx, tx, canonical, c.AgentID, state, next, revision)
		}
	}
	if err != nil {
		fail(w, 503, "unavailable", "Verification unavailable.")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 503, "unavailable", "Verification unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"report_id": id, "status": next, "outcome": p.Outcome, "replayed": false, "submission_state": "submitted"})
}
