package improvements

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Decision struct {
	ID               string         `json:"id"`
	Repository       string         `json:"repository"`
	ReportID         string         `json:"report_id"`
	ReportVersion    int64          `json:"report_version"`
	PolicyVersion    int64          `json:"policy_version"`
	Version          int64          `json:"version"`
	Mode             string         `json:"mode"`
	State            string         `json:"state"`
	Proposal         TriageProposal `json:"proposal"`
	Fingerprint      string         `json:"fingerprint"`
	Reason           string         `json:"reason"`
	ReviewPriority   string         `json:"review_priority"`
	Synthetic        bool           `json:"synthetic"`
	ReservedMicroUSD int64          `json:"reserved_micro_usd"`
	ObservedMicroUSD *int64         `json:"observed_micro_usd"`
	ProcessorStarted bool           `json:"processor_started"`
	CreatedAt        time.Time      `json:"created_at"`
}
type DecisionPage struct {
	Items      []Decision `json:"items"`
	NextCursor string     `json:"next_cursor"`
}
type DecisionReview struct {
	Version  int64           `json:"version"`
	Action   string          `json:"action"`
	Proposal *TriageProposal `json:"proposal,omitempty"`
}
type TriageSummary struct {
	Processed          int `json:"processed"`
	Applied            int `json:"applied"`
	Review             int `json:"review"`
	BudgetDeferred     int `json:"budget_deferred"`
	SyntheticProcessed int `json:"synthetic_processed"`
}

const decisionColumns = `id::text,repository,report_id::text,report_version,policy_version,version,mode,state,proposal,fingerprint,reason,synthetic,reserved_micro_usd,observed_micro_usd,processor_started,created_at`

func scanDecision(row pgx.Row) (Decision, error) {
	var d Decision
	var p []byte
	e := row.Scan(&d.ID, &d.Repository, &d.ReportID, &d.ReportVersion, &d.PolicyVersion, &d.Version, &d.Mode, &d.State, &p, &d.Fingerprint, &d.Reason, &d.Synthetic, &d.ReservedMicroUSD, &d.ObservedMicroUSD, &d.ProcessorStarted, &d.CreatedAt)
	if e == nil {
		e = json.Unmarshal(p, &d.Proposal)
		d.ReviewPriority = "normal"
		if d.Reason == "access_urgent_review" || d.Reason == "delivery_urgent_review" {
			d.ReviewPriority = "high"
		}
	}
	return d, e
}
func ListDecisions(ctx context.Context, pool *pgxpool.Pool, repo, cursor string) (DecisionPage, error) {
	if !ValidRepository(repo) {
		return DecisionPage{}, ErrInvalidDecision
	}
	if cursor == "" {
		cursor = uuid.Nil.String()
	}
	if _, e := uuid.Parse(cursor); e != nil {
		return DecisionPage{}, ErrInvalidDecision
	}
	rows, e := pool.Query(ctx, `SELECT `+decisionColumns+` FROM improvement_decisions WHERE repository=$1 AND id>$2 ORDER BY id LIMIT 50`, strings.ToLower(repo), cursor)
	if e != nil {
		return DecisionPage{}, e
	}
	defer rows.Close()
	page := DecisionPage{Items: []Decision{}}
	for rows.Next() {
		d, e := scanDecision(rows)
		if e != nil {
			return DecisionPage{}, e
		}
		page.Items = append(page.Items, d)
	}
	if e = rows.Err(); e != nil {
		return DecisionPage{}, e
	}
	if len(page.Items) == 50 {
		page.NextCursor = page.Items[49].ID
	}
	return page, nil
}
func lockedControl(ctx context.Context, tx pgx.Tx, repo string) (Control, error) {
	var c Control
	e := tx.QueryRow(ctx, `SELECT repository,version,owner,mode,coalesce(operator_id::text,''),coalesce(canary_org::text,''),allow_synthetic FROM improvement_control WHERE repository=$1 FOR SHARE`, repo).Scan(&c.Repository, &c.Version, &c.Owner, &c.Mode, &c.OperatorID, &c.CanaryOrg, &c.AllowSynthetic)
	return c, e
}
func operatorPresent(ctx context.Context, tx pgx.Tx, actor string) error {
	if _, e := uuid.Parse(actor); e != nil {
		return ErrDecisionConflict
	}
	var id string
	e := tx.QueryRow(ctx, `SELECT agent_id::text FROM improvement_operators WHERE agent_id=$1 FOR KEY SHARE`, actor).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrDecisionConflict
	}
	return e
}
func readTriageReport(ctx context.Context, tx pgx.Tx, id string) (triageReport, error) {
	var r triageReport
	e := tx.QueryRow(ctx, `SELECT id::text,org_id::text,category,route,issue_url,status,version,synthetic FROM improvement_reports WHERE id=$1 FOR UPDATE`, id).Scan(&r.ID, &r.Org, &r.Category, &r.Route, &r.IssueURL, &r.Status, &r.Version, &r.Synthetic)
	if e != nil {
		return r, e
	}
	rows, e := tx.Query(ctx, `SELECT x.id::text,o.evidence,o.context,o.created_at FROM improvement_occurrences o JOIN improvement_receipts x ON x.id=o.receipt_id WHERE x.report_id=$1 AND x.withdrawn_at IS NULL AND o.created_at>now()-interval '90 days' ORDER BY o.created_at DESC,o.occurrence_id LIMIT 5 FOR SHARE OF o,x`, id)
	if e != nil {
		return r, e
	}
	defer rows.Close()
	for rows.Next() {
		var s triageSample
		var ev, ct []byte
		if e = rows.Scan(&s.ReceiptID, &ev, &ct, &s.CreatedAt); e != nil {
			return r, e
		}
		if json.Unmarshal(ev, &s.Evidence) != nil || json.Unmarshal(ct, &s.Context) != nil {
			continue
		}
		r.Samples = append(r.Samples, s)
	}
	return r, rows.Err()
}

type triageWork struct {
	decision Decision
	report   triageReport
	payload  []byte
	runModel bool
	reason   string
}

// claimTriage serializes selection with a report row lock and commits a unique
// decision plus cost reservation before any external request. A crashed claim is
// review-only; no lease expiry authorizes a second model call for the same key.
func claimTriage(ctx context.Context, pool *pgxpool.Pool, c Control, cfg TriageConfig) (triageWork, error) {
	var work triageWork
	tx, e := pool.Begin(ctx)
	if e != nil {
		return work, e
	}
	defer tx.Rollback(ctx)
	current, e := lockedControl(ctx, tx, c.Repository)
	if e != nil {
		return work, e
	}
	if current != c || c.Owner != "cloud" || c.Mode == "off" {
		return work, ErrDecisionConflict
	}
	if e = operatorPresent(ctx, tx, c.OperatorID); e != nil {
		return work, e
	}
	var id string
	e = tx.QueryRow(ctx, `SELECT p.id::text FROM improvement_reports p WHERE p.status='new' AND p.merged_into IS NULL AND (NOT p.synthetic OR ($3 AND p.org_id=nullif($4,'')::uuid)) AND ($5<>'canary' OR p.org_id=nullif($4,'')::uuid) AND NOT EXISTS(SELECT 1 FROM improvement_decisions d WHERE d.repository=$1 AND d.report_id=p.id AND d.report_version=p.version AND d.policy_version=$2) ORDER BY p.created_at,p.id LIMIT 1 FOR UPDATE OF p SKIP LOCKED`, c.Repository, c.Version, c.AllowSynthetic, c.CanaryOrg, c.Mode).Scan(&id)
	if e != nil {
		return work, e
	}
	r, e := readTriageReport(ctx, tx, id)
	if e != nil {
		return work, e
	}
	work.report = r
	p, reason := localProposal(c.Repository, r, time.Now())
	work.reason = reason
	payload, reserve, e := modelPayload(cfg, r)
	if e != nil {
		return work, e
	}
	work.payload = payload
	run := cfg.configured(c.Version) && len(r.Samples) > 0
	if cfg.ProcessorEnabled && !cfg.configured(c.Version) {
		work.reason = "processor_unconfigured"
		p = reviewProposal()
	}
	if run {
		_, e = tx.Exec(ctx, `INSERT INTO improvement_triage_budget(repository,day) VALUES($1,(now() AT TIME ZONE 'UTC')::date) ON CONFLICT DO NOTHING`, c.Repository)
		if e != nil {
			return work, e
		}
		tag, e := tx.Exec(ctx, `UPDATE improvement_triage_budget SET reserved_micro_usd=reserved_micro_usd+$2 WHERE repository=$1 AND day=(now() AT TIME ZONE 'UTC')::date AND reserved_micro_usd<=$3::bigint-$2::bigint`, c.Repository, reserve, cfg.cap())
		if e != nil {
			return work, e
		}
		if tag.RowsAffected() == 0 {
			run = false
			reserve = 0
			p = reviewProposal()
			work.reason = "budget_deferred"
		}
	} else {
		reserve = 0
	}
	b, e := json.Marshal(p)
	if e != nil {
		return work, e
	}
	d, e := scanDecision(tx.QueryRow(ctx, `INSERT INTO improvement_decisions(repository,report_id,report_version,policy_version,mode,proposal,fingerprint,reason,synthetic,reserved_micro_usd,processor_started) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING `+decisionColumns, c.Repository, r.ID, r.Version, c.Version, c.Mode, b, fingerprint(r), work.reason, r.Synthetic, reserve, run))
	if e != nil {
		return work, e
	}
	if e = tx.Commit(ctx); e != nil {
		return work, e
	}
	work.decision = d
	work.runModel = run
	return work, nil
}
func RunTriage(ctx context.Context, pool *pgxpool.Pool, c Control, cfg TriageConfig) (TriageSummary, error) {
	var summary TriageSummary
	if c.Mode == "off" || c.Owner != "cloud" {
		return summary, nil
	}
	if !ValidRepository(c.Repository) || (c.Mode != "shadow" && c.Mode != "canary") {
		return summary, ErrInvalidDecision
	}
	// Reserved costs survive process loss; expired processing is never retried.
	_, e := pool.Exec(ctx, `UPDATE improvement_decisions SET state='proposed',proposal='{"action":"needs_review","priority":"normal","evidence_indexes":[]}',reason='processor_unknown',version=version+1,updated_at=now() WHERE repository=$1 AND state='processing' AND created_at<now()-interval '2 minutes'`, c.Repository)
	if e != nil {
		return summary, e
	}
	for i := 0; i < 10; i++ {
		if e = ctx.Err(); e != nil {
			return summary, e
		}
		jobCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		work, e := claimTriage(jobCtx, pool, c, cfg)
		if errors.Is(e, pgx.ErrNoRows) {
			cancel()
			break
		}
		if e != nil {
			cancel()
			return summary, e
		}
		p, reason := work.decision.Proposal, work.reason
		var observed *int64
		if work.runModel {
			p, observed, reason = callTriageModel(jobCtx, cfg, work.report, c.Repository, work.payload)
		}
		// Finalization gets a separate bounded cleanup context if a request consumed
		// its deadline. It only records failure/review; authority is rechecked in SQL.
		cancel()
		finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		d, e := finishTriage(finishCtx, pool, c, work, p, observed, reason)
		finishCancel()
		if e != nil {
			return summary, e
		}
		if d.Synthetic {
			summary.SyntheticProcessed++
			continue
		}
		summary.Processed++
		if d.State == "applied" {
			summary.Applied++
		} else {
			summary.Review++
		}
		if work.reason == "budget_deferred" {
			summary.BudgetDeferred++
		}
	}
	return summary, nil
}
func finishTriage(ctx context.Context, pool *pgxpool.Pool, c Control, w triageWork, p TriageProposal, observed *int64, reason string) (Decision, error) {
	tx, e := pool.Begin(ctx)
	if e != nil {
		return Decision{}, e
	}
	defer tx.Rollback(ctx)
	current, e := lockedControl(ctx, tx, c.Repository)
	if e != nil {
		return Decision{}, e
	}
	d, e := scanDecision(tx.QueryRow(ctx, `SELECT `+decisionColumns+` FROM improvement_decisions WHERE id=$1 FOR UPDATE`, w.decision.ID))
	if e != nil {
		return d, e
	}
	if d.State != "processing" || d.Version != w.decision.Version {
		return d, nil // operator/recovery already resolved this intent; never overwrite it
	}
	r, e := readTriageReport(ctx, tx, d.ReportID)
	if e != nil {
		return d, e
	}
	state := "proposed"
	if current != c || current.Mode == "off" || r.Version != d.ReportVersion || r.Status != "new" || fingerprint(r) != d.Fingerprint || len(r.Samples) == 0 {
		state = "stale"
		reason = "evidence_or_policy_changed"
		p = reviewProposal()
	} else {
		if e = operatorPresent(ctx, tx, c.OperatorID); e != nil {
			if !errors.Is(e, ErrDecisionConflict) {
				return d, e
			}
			state = "stale"
			reason = "operator_revoked"
			p = reviewProposal()
		}
		if validateProposal(p, r, c.Repository) != nil {
			p = reviewProposal()
			reason = "invalid_proposal"
		}
		local, localReason := localProposal(c.Repository, r, time.Now())
		// Provider proposals can make a safe rule more cautious, never expand its
		// authority. Urgent rule reason survives a provider's suggestion.
		if local.Action == "needs_review" && (reason == "processor_proposal" || localReason == "access_urgent_review" || localReason == "delivery_urgent_review") {
			reason = localReason
		}
		if state != "stale" && c.Mode == "canary" && r.Org == c.CanaryOrg && (!r.Synthetic || c.AllowSynthetic) && p.Action == local.Action && p.Action != "needs_review" {
			if e = applyTriage(ctx, tx, r, c.OperatorID, p); e != nil {
				return d, e
			}
			state = "applied"
		}
	}
	return saveDecision(ctx, tx, d, p, state, reason, observed)
}
func applyTriage(ctx context.Context, tx pgx.Tx, r triageReport, actor string, p TriageProposal) error {
	if p.Action == "link" {
		return nil
	} // existing link only; no foreign create/link action
	if p.Action != "triage" {
		return ErrInvalidDecision
	}
	_, e := tx.Exec(ctx, `UPDATE improvement_reports SET status='triaged',priority=$2,version=version+1,updated_at=now() WHERE id=$1`, r.ID, p.Priority)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO improvement_transitions(report_id,actor_id,from_status,to_status) VALUES($1,$2,'new','triaged')`, r.ID, actor)
	return e
}
func saveDecision(ctx context.Context, tx pgx.Tx, d Decision, p TriageProposal, state, reason string, observed *int64) (Decision, error) {
	b, e := json.Marshal(p)
	if e != nil {
		return d, e
	}
	d, e = scanDecision(tx.QueryRow(ctx, `UPDATE improvement_decisions SET proposal=$2,state=$3,reason=$4,observed_micro_usd=$5,version=version+1,updated_at=now() WHERE id=$1 RETURNING `+decisionColumns, d.ID, b, state, reason, observed))
	if e != nil {
		return d, e
	}
	return d, tx.Commit(ctx)
}
func ReviewDecision(ctx context.Context, pool *pgxpool.Pool, repo, id, actor string, review DecisionReview) (Decision, error) {
	if !ValidRepository(repo) || review.Version < 1 || (review.Action != "accept" && review.Action != "change" && review.Action != "reject") {
		return Decision{}, ErrInvalidDecision
	}
	for _, v := range []string{id, actor} {
		if _, e := uuid.Parse(v); e != nil {
			return Decision{}, ErrInvalidDecision
		}
	}
	if (review.Action == "change") != (review.Proposal != nil) {
		return Decision{}, ErrInvalidDecision
	}
	tx, e := pool.Begin(ctx)
	if e != nil {
		return Decision{}, e
	}
	defer tx.Rollback(ctx)
	c, e := lockedControl(ctx, tx, strings.ToLower(repo))
	if e != nil {
		return Decision{}, e
	}
	if e = operatorPresent(ctx, tx, actor); e != nil {
		return Decision{}, e
	}
	d, e := scanDecision(tx.QueryRow(ctx, `SELECT `+decisionColumns+` FROM improvement_decisions WHERE repository=$1 AND id=$2 FOR UPDATE`, strings.ToLower(repo), id))
	if errors.Is(e, pgx.ErrNoRows) {
		return d, ErrDecisionNotFound
	}
	if e != nil {
		return d, e
	}
	if d.Version != review.Version || (d.State != "proposed" && d.State != "processing") {
		return d, ErrDecisionConflict
	}
	p := d.Proposal
	state, reason := "rejected", "operator_rejected"
	if review.Action != "reject" {
		r, e := readTriageReport(ctx, tx, d.ReportID)
		if e != nil {
			return d, e
		}
		if c.Version != d.PolicyVersion || r.Version != d.ReportVersion || r.Status != "new" || fingerprint(r) != d.Fingerprint || len(r.Samples) == 0 {
			return d, ErrDecisionConflict
		}
		if review.Proposal != nil {
			p = *review.Proposal
		}
		if validateProposal(p, r, repo) != nil || p.Action == "needs_review" {
			return d, ErrInvalidDecision
		}
		if e = applyTriage(ctx, tx, r, actor, p); e != nil {
			return d, e
		}
		state = "applied"
		reason = "operator_accepted"
	}
	_, e = tx.Exec(ctx, `INSERT INTO improvement_decision_reviews(decision_id,actor_id,action,decision_version) VALUES($1,$2,$3,$4)`, d.ID, actor, review.Action, d.Version)
	if e != nil {
		return d, e
	}
	return saveDecision(ctx, tx, d, p, state, reason, d.ObservedMicroUSD)
}
