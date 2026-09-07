package events

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Transaction-scoped advisory lock is automatically released on rollback or disconnect.
func PublishOnce(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(4242)`).Scan(&locked); err != nil {
		return 0, err
	}
	if !locked {
		return 0, nil
	}
	tag, err := tx.Exec(ctx, `WITH batch AS (SELECT * FROM outbox WHERE published_at IS NULL ORDER BY id LIMIT 500), inserted AS (INSERT INTO stream_events(dedup_key,event_kind,object_id,audience_group,author_id) SELECT dedup_key,event_kind,object_id,audience_group,author_id FROM batch ORDER BY id ON CONFLICT DO NOTHING RETURNING dedup_key) UPDATE outbox SET published_at=now() WHERE id IN(SELECT id FROM batch)`)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
