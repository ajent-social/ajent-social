package improvements

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TriageMetricsReport describes observed decisions, not independent affected
// users or product quality. Synthetic decisions are excluded from every field.
type TriageMetricsReport struct {
	WindowDays       int              `json:"window_days"`
	Decisions        int64            `json:"decisions"`
	States           map[string]int64 `json:"states"`
	Reasons          map[string]int64 `json:"reasons"`
	ReviewActions    map[string]int64 `json:"review_actions"`
	ModelCalls       int64            `json:"model_calls"`
	ReservedMicroUSD int64            `json:"reserved_micro_usd"`
	ObservedMicroUSD int64            `json:"observed_micro_usd"`
	UnknownCostCalls int64            `json:"unknown_cost_calls"`
	Coverage         string           `json:"coverage"`
}

func TriageMetrics(ctx context.Context, pool *pgxpool.Pool, repo string) (TriageMetricsReport, error) {
	m := TriageMetricsReport{WindowDays: 30, States: map[string]int64{}, Reasons: map[string]int64{}, ReviewActions: map[string]int64{}, Coverage: "eligible_workflow_denominator_unknown"}
	if !ValidRepository(repo) {
		return m, ErrInvalidDecision
	}
	repo = strings.ToLower(repo)
	rows, e := pool.Query(ctx, `SELECT state,reason,count(*),count(*) FILTER(WHERE processor_started),coalesce(sum(reserved_micro_usd),0),coalesce(sum(observed_micro_usd),0),count(*) FILTER(WHERE processor_started AND observed_micro_usd IS NULL) FROM improvement_decisions WHERE repository=$1 AND NOT synthetic AND created_at>now()-interval '30 days' GROUP BY state,reason`, repo)
	if e != nil {
		return m, e
	}
	for rows.Next() {
		var state, reason string
		var n, calls, reserved, observed, unknown int64
		if e = rows.Scan(&state, &reason, &n, &calls, &reserved, &observed, &unknown); e != nil {
			rows.Close()
			return m, e
		}
		m.Decisions += n
		m.States[state] += n
		m.Reasons[reason] += n
		m.ModelCalls += calls
		m.ReservedMicroUSD += reserved
		m.ObservedMicroUSD += observed
		m.UnknownCostCalls += unknown
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return m, e
	}
	rows, e = pool.Query(ctx, `SELECT r.action,count(*) FROM improvement_decision_reviews r JOIN improvement_decisions d ON d.id=r.decision_id WHERE d.repository=$1 AND NOT d.synthetic AND r.created_at>now()-interval '30 days' GROUP BY r.action`, repo)
	if e != nil {
		return m, e
	}
	defer rows.Close()
	for rows.Next() {
		var action string
		var n int64
		if e = rows.Scan(&action, &n); e != nil {
			return m, e
		}
		m.ReviewActions[action] = n
	}
	return m, rows.Err()
}
