package improvements

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IssuePublisher interface {
	FindIssue(context.Context, string) (string, error)
	CreateIssue(context.Context, Projection) (string, error)
}

type Worker struct {
	Pool       *pgxpool.Pool
	Store      *Store
	Repository string
	Instance   string
	GitHub     IssuePublisher
}

func NewWorker(pool *pgxpool.Pool, repo string, publisher IssuePublisher) (*Worker, error) {
	if pool == nil || !ValidRepository(repo) {
		return nil, errors.New("invalid improvement worker configuration")
	}
	s, err := NewStore(pool)
	if err != nil {
		return nil, err
	}
	return &Worker{Pool: pool, Store: s, Repository: strings.ToLower(repo), Instance: uuid.NewString(), GitHub: publisher}, nil
}

// Start runs separately from the event publisher and retention chain.
func (w *Worker) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			passCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			err := w.Pass(passCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				slog.Warn("improvement worker incomplete", "event", "improvement_worker", "healthy", 0, "outcome", "pass_failed")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

func (w *Worker) Pass(ctx context.Context) error {
	c, err := ReadControl(ctx, w.Pool, w.Repository)
	if err != nil {
		return err
	}
	state, outcome, processed := "healthy", "ok", 0
	if c.Owner != "cloud" {
		return w.record(ctx, "disabled", "not_cloud_owner", 0)
	}
	if w.GitHub == nil {
		return w.record(ctx, "degraded", "github_not_configured", 0)
	}
	var operator bool
	if err = w.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM improvement_operators WHERE agent_id=nullif($1,'')::uuid)`, c.OperatorID).Scan(&operator); err != nil {
		return err
	}
	if !operator {
		return w.record(ctx, "degraded", "operator_not_provisioned", 0)
	}
	if c.Mode == "shadow" || c.Mode == "canary" {
		if _, err = RunTriage(ctx, w.Pool, c, TriageConfig{}); err != nil {
			return w.record(ctx, "degraded", "triage_failed", 0)
		}
	}
	// Select only not-yet-enqueued reports, so later pages cannot starve behind
	// already completed or ambiguous work. A unique key closes concurrent scans.
	rows, err := w.Pool.Query(ctx, `SELECT p.id::text FROM improvement_reports p WHERE p.merged_into IS NULL AND p.status IN('triaged','planned','shipped_awaiting_verification') AND p.issue_url='' AND ($3='' OR p.org_id=nullif($3,'')::uuid) AND (NOT p.synthetic OR ($2 AND p.org_id=nullif($3,'')::uuid)) AND EXISTS(SELECT 1 FROM improvement_receipts r JOIN improvement_reports source ON source.id=r.report_id WHERE coalesce(source.merged_into,source.id)=p.id AND r.withdrawn_at IS NULL) AND NOT EXISTS(SELECT 1 FROM improvement_jobs j WHERE j.repository=$1 AND j.report_id=p.id AND j.operation='projection' AND j.version_key='v1') ORDER BY p.created_at,p.id LIMIT 100`, c.Repository, c.AllowSynthetic, c.CanaryOrg)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = w.Store.Enqueue(ctx, JobKey{Repository: c.Repository, ReportID: id, Operation: "projection", VersionKey: "v1"}); err != nil {
			return err
		}
	}
	for i := 0; i < 10; i++ {
		current, e := ReadControl(ctx, w.Pool, c.Repository)
		if e != nil {
			return e
		}
		if current.Version != c.Version || current.Owner != "cloud" {
			outcome = "control_changed"
			break
		}
		job, e := w.Store.Claim(ctx, c.Repository, w.Instance, 120*time.Second)
		if errors.Is(e, ErrNoJob) {
			break
		}
		if e != nil {
			return e
		}
		e = w.processLeased(ctx, c, job)
		processed++
		if e != nil {
			state = "degraded"
			outcome = "job_needs_attention"
		}
	}
	if _, err = w.Store.Cleanup(ctx); err != nil {
		return err
	}
	return w.record(ctx, state, outcome, processed)
}

func (w *Worker) record(ctx context.Context, state, outcome string, n int) error {
	_, err := w.Pool.Exec(ctx, `INSERT INTO improvement_worker_health(repository,instance,state,outcome,processed) VALUES($1,$2,$3,$4,$5) ON CONFLICT(repository,instance) DO UPDATE SET completed_at=now(),state=excluded.state,outcome=excluded.outcome,processed=excluded.processed`, w.Repository, w.Instance, state, outcome, n)
	if err != nil {
		return err
	}
	h, err := WorkerHealth(ctx, w.Pool, w.Repository)
	if err != nil {
		return err
	}
	oldest := -1.0
	if h.OldestDueSeconds != nil {
		oldest = *h.OldestDueSeconds
	}
	if state == "healthy" && h.State != "healthy" {
		state, outcome = h.State, "backlog_needs_attention"
		if state == "stale" {
			state = "degraded"
		}
		if _, err = w.Pool.Exec(ctx, `UPDATE improvement_worker_health SET state=$3,outcome=$4 WHERE repository=$1 AND instance=$2`, w.Repository, w.Instance, state, outcome); err != nil {
			return err
		}
	}
	healthy := 0
	if state == "healthy" {
		healthy = 1
	}
	slog.Info("improvement worker pass", "event", "improvement_worker", "healthy", healthy, "state", state, "outcome", outcome, "processed", n, "oldest_due_seconds", oldest)
	_, err = w.Pool.Exec(ctx, `DELETE FROM improvement_worker_health WHERE completed_at<now()-interval '7 days'`)
	return err
}

func (w *Worker) processLeased(ctx context.Context, c Control, job Job) error {
	jobCtx, cancel := context.WithTimeout(ctx, time.Minute)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				if w.Store.Renew(jobCtx, job, 120*time.Second) != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	return w.project(jobCtx, c, job)
}

type report struct {
	ID, Category, Route, Status, IssueURL, Org string
	Version                                    int64
	Synthetic, Active                          bool
}

func (w *Worker) readReport(ctx context.Context, id string) (report, error) {
	var p report
	err := w.Pool.QueryRow(ctx, `SELECT p.id::text,p.category,p.route,p.status,p.issue_url,p.org_id::text,p.version,p.synthetic,EXISTS(SELECT 1 FROM improvement_receipts r JOIN improvement_reports source ON source.id=r.report_id WHERE coalesce(source.merged_into,source.id)=p.id AND r.withdrawn_at IS NULL) FROM improvement_reports p WHERE p.id=$1 AND p.merged_into IS NULL`, id).Scan(&p.ID, &p.Category, &p.Route, &p.Status, &p.IssueURL, &p.Org, &p.Version, &p.Synthetic, &p.Active)
	return p, err
}

func eligible(p report, c Control) bool {
	return p.Active && (c.CanaryOrg == "" || p.Org == c.CanaryOrg) && (p.Status == "triaged" || p.Status == "planned" || p.Status == "shipped_awaiting_verification") && (!p.Synthetic || c.AllowSynthetic && p.Org == c.CanaryOrg)
}

func (w *Worker) project(ctx context.Context, c Control, j Job) error {
	p, err := w.readReport(ctx, j.ReportID)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !eligible(p, c) {
		return w.Store.CancelReport(ctx, j.ReportID)
	}
	if err != nil {
		return err
	}
	projection, err := ReportProjection(c.Repository, p.ID, p.Category, p.Route, p.Synthetic)
	if err != nil {
		return err
	}
	if p.IssueURL != "" {
		return w.attach(ctx, c, j, p, p.IssueURL)
	}
	if j.IssueURL != "" {
		return w.attach(ctx, c, j, p, j.IssueURL)
	}
	url, err := w.GitHub.FindIssue(ctx, projection.Marker)
	if err != nil {
		return w.deferJob(ctx, j, "lookup_failed", false)
	}
	if url != "" {
		return w.attach(ctx, c, j, p, url)
	}
	if j.State == "sent" || j.State == "ambiguous" {
		return w.deferJob(ctx, j, "creation_unknown", true)
	}
	if err = w.permitSend(ctx, c, j, p, projection.Marker); err != nil {
		return err
	}
	url, err = w.GitHub.CreateIssue(ctx, projection)
	if err != nil {
		// The durable sent marker is deliberately never reset by this worker.
		// Even a definite rejection stays reviewable; safety wins over a blind
		// retry when an intermediary's response cannot prove non-creation.
		return w.deferJob(ctx, j, "creation_unknown", true)
	}
	return w.attach(ctx, c, j, p, url)
}

func (w *Worker) deferJob(ctx context.Context, j Job, reason string, ambiguous bool) error {
	if j.State == "sent" || j.State == "ambiguous" {
		ambiguous = true
	}
	if ambiguous {
		if err := w.Store.Finish(ctx, j, "ambiguous", "", reason); err != nil {
			return err
		}
		return errors.New("ambiguous improvement projection")
	}
	if j.Attempts >= 5 {
		if err := w.Store.Finish(ctx, j, "dead", "", reason); err != nil {
			return err
		}
		return errors.New("improvement projection exhausted")
	}
	delay := time.Minute * time.Duration(1<<min(j.Attempts, 4))
	if err := w.Store.Retry(ctx, j, delay, reason); err != nil {
		return err
	}
	return errors.New("improvement projection deferred")
}

// permit locks in a fixed order shared by control edits and report withdrawal.
// The report/policy decision and the sent intent commit at one boundary.
func (w *Worker) permit(ctx context.Context, tx pgx.Tx, c Control, p report) error {
	var version int64
	var owner string
	if err := tx.QueryRow(ctx, `SELECT version,owner FROM improvement_control WHERE repository=$1 FOR SHARE`, c.Repository).Scan(&version, &owner); err != nil {
		return err
	}
	if version != c.Version || owner != "cloud" {
		return ErrConflict
	}
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT version=$2 AND status IN('triaged','planned','shipped_awaiting_verification') AND merged_into IS NULL AND EXISTS(SELECT 1 FROM improvement_operators WHERE agent_id=nullif($3,'')::uuid) AND EXISTS(SELECT 1 FROM improvement_receipts r JOIN improvement_reports source ON source.id=r.report_id WHERE coalesce(source.merged_into,source.id)=p.id AND r.withdrawn_at IS NULL) FROM improvement_reports p WHERE p.id=$1 FOR UPDATE`, p.ID, p.Version, c.OperatorID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrConflict
	}
	return nil
}

func (w *Worker) permitSend(ctx context.Context, c Control, j Job, p report, marker string) error {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = w.permit(ctx, tx, c, p); err != nil {
		return err
	}
	if err = w.Store.MarkSentTx(ctx, tx, j, marker); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Worker) attach(ctx context.Context, c Control, j Job, p report, url string) error {
	if url == "" || !validIssue(url, c.Repository) {
		return errors.New("invalid projected issue URL")
	}
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = w.permit(ctx, tx, c, p); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE improvement_jobs SET state='done',issue_url=$4,reason='',owner='',lease_until=NULL,updated_at=clock_timestamp() WHERE id=$1 AND owner=$2 AND generation=$3 AND lease_until>clock_timestamp() AND state IN('pending','sent','ambiguous')`, j.ID, j.Owner, j.Generation, url)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	if p.IssueURL != "" && p.IssueURL != url {
		return ErrConflict
	}
	if p.IssueURL == "" {
		if _, err = tx.Exec(ctx, `UPDATE improvement_reports SET issue_url=$2,version=version+1,updated_at=now() WHERE id=$1`, p.ID, url); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO improvement_transitions(report_id,actor_id,from_status,to_status) VALUES($1,$2,$3,$3)`, p.ID, c.OperatorID, p.Status); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit issue projection: %w", err)
	}
	return nil
}
