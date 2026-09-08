// Package improvements provides durable ownership for bounded external effects.
package improvements

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoJob      = errors.New("no runnable improvement job")
	ErrLeaseLost  = errors.New("improvement job lease lost")
	ErrInvalidJob = errors.New("invalid improvement job")
)

const (
	StatePending   = "pending"
	StateSent      = "sent"
	StateAmbiguous = "ambiguous"
	StateDone      = "done"
	StateCancelled = "cancelled"
	StateDead      = "dead"
	MaxAttempts    = 5
)

var safeKey = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:/-]{0,127}$`)
var safeRepo = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,99}/[a-zA-Z0-9][a-zA-Z0-9_.-]{0,99}$`)
var safeReason = regexp.MustCompile(`^[a-z0-9_]{0,64}$`)

type JobKey struct{ Repository, ReportID, Operation, VersionKey string }
type Job struct {
	JobKey
	ID, State, Owner, Marker, IssueURL, Reason string
	Generation                                 int64
	Attempts                                   int
	NextAttempt                                time.Time
	LeaseUntil                                 *time.Time
	CreatedAt, UpdatedAt                       time.Time
}
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, fmt.Errorf("create improvement store: %w", ErrInvalidJob)
	}
	return &Store{pool: pool}, nil
}

const jobColumns = `id::text,repository,report_id::text,operation,version_key,state,owner,generation,attempts,next_attempt,lease_until,marker,issue_url,reason,created_at,updated_at`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.Repository, &j.ReportID, &j.Operation, &j.VersionKey, &j.State, &j.Owner, &j.Generation, &j.Attempts, &j.NextAttempt, &j.LeaseUntil, &j.Marker, &j.IssueURL, &j.Reason, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}
func markerFor(k JobKey) string {
	return fmt.Sprintf("<!-- ajent-improvement:%x -->", sha256.Sum256([]byte(k.Repository+":"+k.ReportID)))
}

// Enqueue preserves an existing operation, including unresolved external intents.
// The conflict update is a no-op RETURNING rather than a snapshot-racy CTE read.
func (s *Store) Enqueue(ctx context.Context, k JobKey) (Job, error) {
	k.Repository = strings.ToLower(k.Repository)
	id, err := uuid.Parse(k.ReportID)
	if err != nil || !safeRepo.MatchString(k.Repository) || !safeKey.MatchString(k.Operation) || (k.VersionKey != "" && !safeKey.MatchString(k.VersionKey)) {
		return Job{}, ErrInvalidJob
	}
	k.ReportID = id.String()
	j, err := scanJob(s.pool.QueryRow(ctx, `INSERT INTO improvement_jobs(repository,report_id,operation,version_key,marker) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(repository,report_id,operation,version_key) DO UPDATE SET repository=improvement_jobs.repository RETURNING `+jobColumns, k.Repository, k.ReportID, k.Operation, k.VersionKey, markerFor(k)))
	if err != nil {
		return Job{}, fmt.Errorf("enqueue improvement job: %w", err)
	}
	return j, nil
}

// Claim fences expired ownership using database time. Reclaimed sent intents are
// ambiguous and can never become another create request through Retry.
func (s *Store) Claim(ctx context.Context, repo, owner string, lease time.Duration) (Job, error) {
	if !safeRepo.MatchString(repo) || !safeKey.MatchString(owner) || lease <= 0 || lease > 10*time.Minute {
		return Job{}, ErrInvalidJob
	}
	j, err := scanJob(s.pool.QueryRow(ctx, `WITH due AS (
 SELECT id FROM improvement_jobs WHERE repository=$1 AND state IN('pending','sent','ambiguous') AND attempts<$4
 AND next_attempt<=clock_timestamp() AND (lease_until IS NULL OR lease_until<=clock_timestamp())
 ORDER BY next_attempt,created_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
 UPDATE improvement_jobs j SET state=CASE WHEN j.state='sent' THEN 'ambiguous' ELSE j.state END,
 owner=$2,generation=generation+1,attempts=attempts+1,lease_until=clock_timestamp()+make_interval(secs=>$3),updated_at=clock_timestamp()
 WHERE j.id IN(SELECT id FROM due) RETURNING `+jobColumns, strings.ToLower(repo), owner, lease.Seconds(), MaxAttempts))
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNoJob
	}
	if err != nil {
		return Job{}, fmt.Errorf("claim improvement job: %w", err)
	}
	return j, nil
}

const fence = ` id=$1 AND owner=$2 AND generation=$3 AND lease_until>clock_timestamp() `

func changed(tag pgconn.CommandTag, err error, verb string) error {
	if err != nil {
		return fmt.Errorf("%s improvement job: %w", verb, err)
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}
func (s *Store) Renew(ctx context.Context, j Job, lease time.Duration) error {
	if lease <= 0 || lease > 10*time.Minute {
		return ErrInvalidJob
	}
	tag, err := s.pool.Exec(ctx, `UPDATE improvement_jobs SET lease_until=clock_timestamp()+make_interval(secs=>$4),updated_at=clock_timestamp() WHERE `+fence+` AND state IN('pending','sent','ambiguous')`, j.ID, j.Owner, j.Generation, lease.Seconds())
	return changed(tag, err, "renew")
}

// MarkSent is committed before issuing HTTP. A crash at any later point leaves
// an intent which is reconciled by marker, never blindly recreated.
type jobExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func (s *Store) MarkSent(ctx context.Context, j Job, marker string) error {
	return markSent(ctx, s.pool, j, marker)
}

// MarkSentTx lets the caller lock and validate the report and worker control in
// the same transaction as the pre-send intent. Commit before making HTTP calls.
func (s *Store) MarkSentTx(ctx context.Context, tx pgx.Tx, j Job, marker string) error {
	if tx == nil {
		return ErrInvalidJob
	}
	return markSent(ctx, tx, j, marker)
}
func markSent(ctx context.Context, q jobExecer, j Job, marker string) error {
	if marker != markerFor(j.JobKey) {
		return ErrInvalidJob
	}
	tag, err := q.Exec(ctx, `UPDATE improvement_jobs SET state='sent',marker=$4,updated_at=clock_timestamp() WHERE `+fence+` AND state='pending' AND marker=$4`, j.ID, j.Owner, j.Generation, marker)
	return changed(tag, err, "record sent intent")
}

func validIssue(raw, repo string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	prefix := "/" + repo + "/issues/"
	if u.Scheme != "https" || u.Host != "github.com" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || !strings.HasPrefix(u.Path, prefix) {
		return false
	}
	n := strings.TrimPrefix(u.Path, prefix)
	if n == "" {
		return false
	}
	for _, c := range n {
		if c < '0' || c > '9' {
			return false
		}
	}
	return n != "0"
}

// Finish accepts only safe result codes, never report context or provider text.
// Cancellation/dead-letter cannot erase an unresolved sent intent.
func (s *Store) Finish(ctx context.Context, j Job, state, issueURL, reason string) error {
	if state != StateDone && state != StateAmbiguous && state != StateCancelled && state != StateDead {
		return ErrInvalidJob
	}
	if !safeReason.MatchString(reason) || !validIssue(issueURL, j.Repository) {
		return ErrInvalidJob
	}
	tag, err := s.pool.Exec(ctx, `UPDATE improvement_jobs SET state=$4,issue_url=CASE WHEN $5='' THEN issue_url ELSE $5 END,reason=$6,owner='',lease_until=NULL,next_attempt=CASE WHEN $4='ambiguous' THEN clock_timestamp()+interval '5 minutes' ELSE next_attempt END,updated_at=clock_timestamp()
 WHERE `+fence+` AND state IN('pending','sent','ambiguous') AND ($4 NOT IN('cancelled','dead') OR state='pending')`, j.ID, j.Owner, j.Generation, state, issueURL, reason)
	return changed(tag, err, "finish")
}

// Retry is only for failed preflight work before an external intent. Any sent
// response failure uses Finish(ambiguous), even a definite provider rejection.
func (s *Store) Retry(ctx context.Context, j Job, delay time.Duration, reason string) error {
	if delay < time.Second || delay > 24*time.Hour || !safeReason.MatchString(reason) {
		return ErrInvalidJob
	}
	tag, err := s.pool.Exec(ctx, `UPDATE improvement_jobs SET state=CASE WHEN attempts>=$6 THEN 'dead' ELSE 'pending' END,
 next_attempt=clock_timestamp()+make_interval(secs=>$4),reason=$5,owner='',lease_until=NULL,updated_at=clock_timestamp()
 WHERE `+fence+` AND state='pending'`, j.ID, j.Owner, j.Generation, delay.Seconds(), reason, MaxAttempts)
	return changed(tag, err, "retry")
}

// CancelReport cancels unstarted effects. Already-sent effects retain their
// marker and become ambiguous; late owners are fenced from reviving the job.
func (s *Store) CancelReport(ctx context.Context, reportID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE improvement_jobs SET state=CASE WHEN state='pending' THEN 'cancelled' ELSE 'ambiguous' END,
 owner='',lease_until=NULL,generation=generation+1,updated_at=clock_timestamp(),reason='report_withdrawn'
 WHERE report_id=$1 AND state IN('pending','sent','ambiguous')`, reportID)
	if err != nil {
		return fmt.Errorf("cancel improvement jobs: %w", err)
	}
	return nil
}

// Cleanup expires bounded completed history; unresolved sent/ambiguous records
// deliberately have no report foreign key and survive report/context retention.
func (s *Store) Cleanup(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM improvement_jobs WHERE id IN(SELECT id FROM improvement_jobs WHERE state IN('done','cancelled','dead') AND updated_at<clock_timestamp()-interval '90 days' ORDER BY updated_at LIMIT 1000)`)
	if err != nil {
		return 0, fmt.Errorf("retain improvement jobs: %w", err)
	}
	return tag.RowsAffected(), nil
}
