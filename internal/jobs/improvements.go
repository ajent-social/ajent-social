package jobs

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Retain inactive linked groups for 90 days, including replay hashes. Lock every
// report in a group before removal so a concurrent new occurrence cannot be
// accepted while its receipt is being removed. A merge group is removed together.
func cleanupImprovements(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `SELECT id::text FROM improvement_reports WHERE coalesce(merged_into,id) IN(
 SELECT coalesce(merged_into,id) FROM improvement_reports GROUP BY coalesce(merged_into,id)
 HAVING max(updated_at)<now()-interval '90 days' ORDER BY max(updated_at) LIMIT 10) ORDER BY id FOR UPDATE`)
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
	if rows.Err() != nil {
		return rows.Err()
	}
	if len(ids) == 0 {
		return nil
	}
	// Locks may have waited for a newly active report: recheck the whole group.
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM improvement_reports WHERE id=ANY($1::uuid[]) AND updated_at>=now()-interval '90 days')`, ids).Scan(&active); err != nil {
		return err
	}
	if active {
		return nil
	}
	for _, query := range []string{
		`DELETE FROM improvement_occurrences WHERE receipt_id IN(SELECT id FROM improvement_receipts WHERE report_id=ANY($1::uuid[]))`,
		`DELETE FROM improvement_verifications WHERE receipt_id IN(SELECT id FROM improvement_receipts WHERE report_id=ANY($1::uuid[]))`,
		`WITH removed AS(DELETE FROM improvement_prompts WHERE receipt_id IN(SELECT id FROM improvement_receipts WHERE report_id=ANY($1::uuid[])) RETURNING seq) UPDATE stream_watermark SET purged_through=greatest(purged_through,coalesce((SELECT max(seq) FROM removed),0))`,
		`DELETE FROM improvement_receipts WHERE report_id=ANY($1::uuid[])`,
		`DELETE FROM improvement_transitions WHERE report_id=ANY($1::uuid[])`,
		`DELETE FROM improvement_reports WHERE id=ANY($1::uuid[])`,
	} {
		if _, err = tx.Exec(ctx, query, ids); err != nil {
			return err
		}
	}
	return nil
}
