package api

import (
	"encoding/json"
	"net/http"
)

// Metrics never infer eligible logical-operation denominators from report counts.
func (s *Server) ImprovementMetrics(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.improvementOperator(w, r); !ok {
		return
	}
	var reports, occurrences, verified, unresolved, prompts, responses, recurrences, minutes, samples int64
	var median, p90 *float64
	var outcomes json.RawMessage
	var checked, reproduced, represented int64
	err := s.DB.Pool.QueryRow(r.Context(), `WITH p AS(SELECT * FROM improvement_reports WHERE NOT synthetic AND updated_at>now()-interval '90 days'), x AS(SELECT x.*,coalesce(p.merged_into,p.id) canonical_id FROM improvement_receipts x JOIN p ON p.id=x.report_id), o AS(SELECT o.* FROM improvement_occurrences o JOIN x ON x.id=o.receipt_id WHERE o.created_at>now()-interval '90 days'), v AS(SELECT v.* FROM improvement_verifications v JOIN x ON x.id=v.receipt_id WHERE v.created_at>now()-interval '30 days'), q AS(SELECT q.* FROM improvement_prompts q JOIN x ON x.id=q.receipt_id WHERE q.created_at>now()-interval '30 days')
 SELECT (SELECT count(*) FROM p WHERE merged_into IS NULL),(SELECT count(*) FROM o),(SELECT count(*) FROM p WHERE status='verified'),(SELECT count(*) FROM p WHERE status NOT IN('verified','declined','withdrawn','merged')),
 (SELECT count(*) FROM q),(SELECT count(*) FROM q WHERE responded_at IS NOT NULL),(SELECT count(*) FROM v WHERE outcome='reproduced'),
 (SELECT coalesce(sum((context->>'workaround_minutes')::bigint),0) FROM o),(SELECT count(*) FROM o WHERE context->>'workaround_minutes' IS NOT NULL),
 (SELECT percentile_cont(0.5) WITHIN GROUP(ORDER BY extract(epoch FROM verified_at-created_at)) FROM p WHERE status='verified'),
 (SELECT percentile_cont(0.9) WITHIN GROUP(ORDER BY extract(epoch FROM verified_at-created_at)) FROM p WHERE status='verified'),
 (SELECT coalesce(jsonb_object_agg(outcome,n),'{}'::jsonb) FROM(SELECT outcome,count(*) n FROM v GROUP BY outcome) counts),
 (SELECT count(*) FROM v WHERE outcome IN('reproduced','not_reproduced')),
 (SELECT count(*) FROM v WHERE outcome='reproduced'),
 (SELECT count(DISTINCT x.canonical_id) FROM o JOIN x ON x.id=o.receipt_id)`).Scan(&reports, &occurrences, &verified, &unresolved, &prompts, &responses, &recurrences, &minutes, &samples, &median, &p90, &outcomes, &checked, &reproduced, &represented)
	if err != nil {
		fail(w, 503, "unavailable", "Improvement metrics unavailable.")
		return
	}
	var duplicateRatio *float64
	if occurrences > 0 {
		value := float64(occurrences-represented) / float64(occurrences)
		duplicateRatio = &value
	}
	var observedRate *float64
	if checked > 0 {
		value := float64(reproduced) / float64(checked)
		observedRate = &value
	}
	writeJSON(w, 200, map[string]any{"verification_outcomes": outcomes, "observed_workflow_retries": checked, "recurrence_among_observed_retries": observedRate, "reports": reports, "occurrences": occurrences, "verified_reports": verified, "unresolved_reports": unresolved, "verification_prompts": prompts, "verification_responses": responses, "no_response": prompts - responses, "recurrence_events": recurrences, "duplicate_occurrence_ratio": duplicateRatio, "workaround_minutes": minutes, "workaround_samples": samples, "resolution_median_seconds": median, "resolution_p90_seconds": p90, "eligible_operation_denominator": nil, "recurrence_rate": nil, "synthetic_excluded": true, "verification_window_days": 30, "report_window": "active within 90 days; occurrences from last 90 days", "coverage": "Report events only; matched logical-operation denominators require client usage evidence. No response is unknown."})
}
