package improvements

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConflict = errors.New("improvement control changed")

type Control struct {
	Repository     string `json:"repository"`
	Version        int64  `json:"version"`
	Owner          string `json:"owner"`
	Mode           string `json:"mode"`
	OperatorID     string `json:"operator_id"`
	CanaryOrg      string `json:"canary_org"`
	AllowSynthetic bool   `json:"allow_synthetic"`
}

func EnsureControl(ctx context.Context, pool *pgxpool.Pool, repo string) error {
	if !ValidRepository(repo) {
		return errors.New("invalid improvement repository")
	}
	_, err := pool.Exec(ctx, `INSERT INTO improvement_control(repository) VALUES($1) ON CONFLICT DO NOTHING`, strings.ToLower(repo))
	return err
}

func ReadControl(ctx context.Context, pool *pgxpool.Pool, repo string) (Control, error) {
	var c Control
	err := pool.QueryRow(ctx, `SELECT repository,version,owner,mode,coalesce(operator_id::text,''),coalesce(canary_org::text,''),allow_synthetic FROM improvement_control WHERE repository=$1`, strings.ToLower(repo)).Scan(&c.Repository, &c.Version, &c.Owner, &c.Mode, &c.OperatorID, &c.CanaryOrg, &c.AllowSynthetic)
	return c, err
}

func UpdateControl(ctx context.Context, pool *pgxpool.Pool, c Control) (Control, error) {
	if !ValidRepository(c.Repository) || c.Version < 1 || (c.Owner != "local" && c.Owner != "cloud" && c.Owner != "off") || (c.Mode != "off" && c.Mode != "shadow" && c.Mode != "canary") {
		return Control{}, errors.New("invalid improvement control")
	}
	if c.OperatorID != "" {
		if _, err := uuid.Parse(c.OperatorID); err != nil {
			return Control{}, errors.New("invalid operator")
		}
	}
	if c.CanaryOrg != "" {
		if _, err := uuid.Parse(c.CanaryOrg); err != nil {
			return Control{}, errors.New("invalid canary workspace")
		}
	}
	if c.Owner == "cloud" && c.OperatorID == "" {
		return Control{}, errors.New("cloud worker requires an operator")
	}
	if c.Mode == "canary" && c.CanaryOrg == "" {
		return Control{}, errors.New("canary mode requires a workspace")
	}
	if c.AllowSynthetic && c.CanaryOrg == "" {
		return Control{}, errors.New("synthetic scope requires a workspace")
	}
	c.Repository = strings.ToLower(c.Repository)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Control{}, err
	}
	defer tx.Rollback(ctx)
	var oldOwner string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT owner,version FROM improvement_control WHERE repository=$1 FOR UPDATE`, c.Repository).Scan(&oldOwner, &version); err != nil {
		return Control{}, err
	}
	if version != c.Version {
		return Control{}, ErrConflict
	}
	// Every ownership transfer crosses off. This keeps cutover explicit and
	// prevents a single stale command switching directly between two writers.
	if oldOwner != "off" && c.Owner != "off" && c.Owner != oldOwner {
		return Control{}, errors.New("stop current owner before transferring ownership")
	}
	if c.OperatorID != "" {
		var allowed bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM improvement_operators WHERE agent_id=$1)`, c.OperatorID).Scan(&allowed); err != nil {
			return Control{}, err
		}
		if !allowed {
			return Control{}, errors.New("operator is not provisioned")
		}
	}
	_, err = tx.Exec(ctx, `UPDATE improvement_control SET version=version+1,owner=$2,mode=$3,operator_id=nullif($4,'')::uuid,canary_org=nullif($5,'')::uuid,allow_synthetic=$6,updated_at=now() WHERE repository=$1`, c.Repository, c.Owner, c.Mode, c.OperatorID, c.CanaryOrg, c.AllowSynthetic)
	if err != nil {
		return Control{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Control{}, err
	}
	c.Version++
	return c, nil
}

type Health struct {
	Control          Control          `json:"control"`
	State            string           `json:"state"`
	LastCompleted    *time.Time       `json:"last_completed_at"`
	Outcome          string           `json:"outcome"`
	Jobs             map[string]int64 `json:"jobs"`
	OldestDueSeconds *float64         `json:"oldest_due_seconds"`
}

func WorkerHealth(ctx context.Context, pool *pgxpool.Pool, repo string) (Health, error) {
	c, err := ReadControl(ctx, pool, repo)
	if err != nil {
		return Health{}, err
	}
	h := Health{Control: c, State: "disabled"}
	err = pool.QueryRow(ctx, `SELECT completed_at,state,outcome FROM improvement_worker_health WHERE repository=$1 ORDER BY completed_at DESC LIMIT 1`, c.Repository).Scan(&h.LastCompleted, &h.State, &h.Outcome)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Health{}, fmt.Errorf("read worker health: %w", err)
	}
	if c.Owner != "cloud" {
		h.State = "disabled"
	} else if h.LastCompleted == nil || time.Since(*h.LastCompleted) > 5*time.Minute {
		h.State = "stale"
	}
	h.Jobs = map[string]int64{}
	rows, e := pool.Query(ctx, `SELECT state,count(*) FROM improvement_jobs WHERE repository=$1 GROUP BY state`, c.Repository)
	if e != nil {
		return Health{}, e
	}
	for rows.Next() {
		var state string
		var n int64
		if e = rows.Scan(&state, &n); e != nil {
			rows.Close()
			return Health{}, e
		}
		h.Jobs[state] = n
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return Health{}, e
	}
	if e = pool.QueryRow(ctx, `SELECT extract(epoch FROM clock_timestamp()-min(next_attempt))::float8 FROM improvement_jobs WHERE repository=$1 AND state IN('pending','sent','ambiguous') AND next_attempt<=clock_timestamp()`, c.Repository).Scan(&h.OldestDueSeconds); e != nil {
		return Health{}, e
	}
	if h.State == "healthy" && (h.Jobs["ambiguous"] > 0 || h.Jobs["dead"] > 0 || h.OldestDueSeconds != nil && *h.OldestDueSeconds > 900) {
		h.State = "degraded"
	}
	return h, nil
}
