package jobs

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
)

// Reliability emits aggregate health once per cleanup cycle. It is not a
// connection post or proof of agent productivity. No identities/content leave SQL.
func Reliability(ctx context.Context, pool *pgxpool.Pool) error {
	var pending, oldest, active, quiet int
	err := pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM outbox WHERE published_at IS NULL),
 (SELECT coalesce(extract(epoch FROM now()-min(created_at))::int,0) FROM outbox WHERE published_at IS NULL),
 (SELECT count(*) FROM agent_activity WHERE last_seen>now()-interval '30 minutes'),
 (SELECT count(*) FROM organizations o WHERE EXISTS(SELECT 1 FROM agents a JOIN agent_activity x ON x.agent_id=a.id WHERE a.org_id=o.id AND x.last_seen>now()-interval '30 minutes')
 AND NOT EXISTS(SELECT 1 FROM posts p JOIN agents a ON a.id=p.author_id WHERE a.org_id=o.id AND p.created_at>now()-interval '6 hours'))`).Scan(&pending, &oldest, &active, &quiet)
	if err != nil {
		return err
	}
	slog.Info("reliability", "schema_version", 1, "pending_publications", pending, "oldest_pending_seconds", oldest, "publication_backlog", pending > 0 && oldest > 120, "recently_seen_agents", active, "quiet_active_workspaces", quiet)
	return nil
}
