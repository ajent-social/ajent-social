package jobs

import (
	"ajent/internal/domains"
	"ajent/internal/events"
	"context"
	"log/slog"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Start closes its result when all background work has stopped.
func Start(ctx context.Context, pool *pgxpool.Pool) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		n := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				jobCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				_, err := events.PublishOnce(jobCtx, pool)
				cancel()
				if err != nil && ctx.Err() == nil {
					slog.Error("publisher failed", "error", err)
				}
				n++
				if n%60 == 0 {
					jobCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
					err := Cleanup(jobCtx, pool)
					if err == nil {
						err = Reliability(jobCtx, pool)
					}
					if err == nil {
						err = domains.Renew(jobCtx, pool, net.DefaultResolver.LookupTXT)
					}
					cancel()
					if err != nil && ctx.Err() == nil {
						slog.Error("cleanup failed", "error", err)
					}
				}
			}
		}
	}()
	return done
}

// Cleanup limits each mutation to 1000 rows. Only one instance cleans at a time.
// Tombstones remain to preserve conversation references and moderation evidence.
func Cleanup(ctx context.Context, pool *pgxpool.Pool) error {
	tx, e := pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var locked bool
	if e = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(4243)`).Scan(&locked); e != nil {
		return e
	}
	if !locked {
		return nil
	}
	statements := []string{
		`UPDATE improvement_reports SET owner='',workaround='',reason='' WHERE id IN(SELECT id FROM improvement_reports WHERE (updated_at<now()-interval '90 days' OR status IN('verified','declined','withdrawn') AND updated_at<now()-interval '30 days') AND (owner<>'' OR workaround<>'' OR reason<>'') LIMIT 1000)`,
		`UPDATE improvement_occurrences SET evidence='{}',context='{}' WHERE (reporter_id,occurrence_id) IN(SELECT reporter_id,occurrence_id FROM improvement_occurrences WHERE (created_at<now()-interval '90 days' OR EXISTS(SELECT 1 FROM improvement_receipts x JOIN improvement_reports source ON source.id=x.report_id JOIN improvement_reports p ON p.id=coalesce(source.merged_into,source.id) WHERE x.id=improvement_occurrences.receipt_id AND p.status IN('verified','declined','withdrawn') AND p.updated_at<now()-interval '30 days')) AND (evidence<>'{}'::jsonb OR context<>'{}'::jsonb) LIMIT 1000)`,
		`UPDATE improvement_verifications SET evidence='' WHERE (reporter_id,verification_id) IN(SELECT reporter_id,verification_id FROM improvement_verifications WHERE (created_at<now()-interval '90 days' OR EXISTS(SELECT 1 FROM improvement_receipts x JOIN improvement_reports source ON source.id=x.report_id JOIN improvement_reports p ON p.id=coalesce(source.merged_into,source.id) WHERE x.id=improvement_verifications.receipt_id AND p.status IN('verified','declined','withdrawn') AND p.updated_at<now()-interval '30 days')) AND evidence<>'' LIMIT 1000)`,
		`WITH expired AS(SELECT seq FROM improvement_prompts WHERE created_at<now()-interval '30 days' ORDER BY seq LIMIT 1000), removed AS(DELETE FROM improvement_prompts WHERE seq IN(SELECT seq FROM expired) RETURNING seq) UPDATE stream_watermark SET purged_through=greatest(purged_through,coalesce((SELECT max(seq) FROM removed),0))`,
		`DELETE FROM human_sessions WHERE digest IN(SELECT digest FROM human_sessions WHERE expires_at<=now() LIMIT 1000)`,
		`DELETE FROM human_ceremonies WHERE digest IN(SELECT digest FROM human_ceremonies WHERE expires_at<=now() LIMIT 1000)`,
		`DELETE FROM installation_links WHERE lookup_id IN(SELECT lookup_id FROM installation_links WHERE expires_at<=now() LIMIT 1000)`,
		`UPDATE question_states SET accepted_answer_id=NULL,version=version+1 WHERE question_id IN (SELECT s.question_id FROM question_states s JOIN posts a ON a.id=s.accepted_answer_id WHERE a.deleted_at IS NOT NULL OR a.hidden_reason IS NOT NULL OR a.expires_at<=now() ORDER BY s.question_id LIMIT 1000)`,
		`UPDATE posts SET body='',title='',structured='{}',deleted_at=coalesce(deleted_at,now()) WHERE id IN (SELECT p.id FROM posts p WHERE (p.body<>'' OR p.title<>'' OR p.structured<>'{}'::jsonb) AND (p.deleted_at IS NOT NULL OR p.expires_at<=now() OR EXISTS(SELECT 1 FROM posts root WHERE root.id=p.root_id AND (root.deleted_at IS NOT NULL OR root.expires_at<=now()))) ORDER BY p.expires_at,p.id LIMIT 1000)`,
		`DELETE FROM notifications WHERE id IN (SELECT n.id FROM notifications n JOIN posts p ON p.id=n.object_id WHERE p.deleted_at IS NOT NULL OR p.expires_at<=now() LIMIT 1000)`,
		`DELETE FROM bookmarks WHERE ctid IN (SELECT b.ctid FROM bookmarks b JOIN posts p ON p.id=b.post_id WHERE p.deleted_at IS NOT NULL OR p.expires_at<=now() LIMIT 1000)`,
		`DELETE FROM idempotency_keys WHERE ctid IN (SELECT ctid FROM idempotency_keys WHERE expires_at<=now() ORDER BY expires_at LIMIT 1000)`,
		`DELETE FROM web_sessions WHERE digest IN (SELECT digest FROM web_sessions WHERE expires_at<=now() ORDER BY expires_at LIMIT 1000)`,
		`WITH expired AS (SELECT seq FROM stream_events WHERE created_at<now()-interval '7 days' ORDER BY seq LIMIT 1000), removed AS (DELETE FROM stream_events WHERE seq IN(SELECT seq FROM expired) RETURNING seq) UPDATE stream_watermark SET purged_through=greatest(purged_through,coalesce((SELECT max(seq) FROM removed),0))`,
		`DELETE FROM outbox WHERE id IN (SELECT id FROM outbox WHERE published_at IS NOT NULL AND created_at<now()-interval '7 days' ORDER BY created_at LIMIT 1000)`,
		`DELETE FROM usage_counters WHERE ctid IN (SELECT ctid FROM usage_counters WHERE bucket<(now() AT TIME ZONE 'UTC')::date-32 LIMIT 1000)`,
	}
	for _, sql := range statements {
		if _, e = tx.Exec(ctx, sql); e != nil {
			return e
		}
	}
	if e = cleanupImprovements(ctx, tx); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
