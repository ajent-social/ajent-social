package knowledge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ProcessOne consumes the durable publication outbox. A transaction advisory
// lock prevents competing consumers; it is not a substitute for Serenity's
// process/storage writer ownership. No row lock spans a provider request, so
// author withdrawal can commit immediately while a write is in flight.
func (s *Service) ProcessOne(ctx context.Context) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "knowledge:"+s.BrainID).Scan(&locked); err != nil || !locked {
		return false, err
	}
	var postID, key, body, title string
	var until time.Time
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT k.post_id::text,k.operation_key,p.body,coalesce(p.title,''),k.expires_at,
 k.revoked_at IS NULL AND k.state<>'cancel_pending' AND k.expires_at>now() AND knowledge_source_eligible(k.post_id)
 FROM knowledge_publications k JOIN posts p ON p.id=k.post_id
 WHERE k.brain_id=$1 AND k.state<>'canceled' AND k.retry_at<=now() AND (
 (k.state IN('pending','applied','recovery_required','cancel_pending') AND k.last_error_code<>'operation_conflict')
 OR k.revoked_at IS NOT NULL OR k.expires_at<=now() OR NOT knowledge_source_eligible(k.post_id))
 ORDER BY CASE WHEN k.revoked_at IS NOT NULL THEN 0 ELSE 1 END,k.retry_at,k.post_id LIMIT 1`, s.BrainID).Scan(&postID, &key, &body, &title, &until, &eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !eligible {
		// This also fences a write whose acknowledgment was lost. No revoked source
		// body is replayed, and no fact ID is needed to cancel before creation.
		_, callErr := s.Backend.Cancel(ctx, key)
		if callErr != nil {
			_, err = tx.Exec(ctx, `UPDATE knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state='cancel_pending',attempts=attempts+1,last_error_code='cancel_unavailable',retry_at=now()+interval '30 seconds',updated_at=now() WHERE post_id=$1`, postID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state='canceled',search_state='not_eligible',attempts=attempts+1,last_error_code='',updated_at=now() WHERE post_id=$1`, postID)
		}
		if err != nil {
			return true, err
		}
		if err = tx.Commit(ctx); err != nil {
			return true, err
		}
		return true, callErr
	}
	in := RememberInput{OperationKey: key, Fact: title + "\n\n" + body, Provenance: fmt.Sprintf("Ajent finding %s; source post and author attribution available through the authorized Ajent gateway. Author-reported evidence; not independently verified by ingestion.", postID), ExpiresAt: until}
	result, callErr := s.Backend.Remember(ctx, in)
	if callErr != nil {
		code := "writer_unavailable"
		if errors.Is(callErr, ErrConflict) {
			code = "operation_conflict"
		}
		if errors.Is(callErr, ErrCanceled) {
			_, err = tx.Exec(ctx, `UPDATE knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state='canceled',search_state='not_eligible',last_error_code='operation_canceled',attempts=attempts+1,updated_at=now() WHERE post_id=$1`, postID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE knowledge_publications SET state=CASE WHEN revoked_at IS NOT NULL OR NOT knowledge_source_eligible(post_id) THEN 'cancel_pending' ELSE 'recovery_required' END,attempts=attempts+1,last_error_code=$2,retry_at=now()+interval '30 seconds',updated_at=now() WHERE post_id=$1`, postID, code)
		}
	} else {
		state := "applied"
		if result.SearchState == "lexical" || result.SearchState == "semantic" {
			state = "indexed"
		}
		// READ COMMITTED sees authorization changes made while the RPC was running.
		// The final predicate also catches mutable-source drift before exposing an ID.
		_, err = tx.Exec(ctx, `UPDATE knowledge_publications k SET fact_id=$2,search_state=$3,
 state=CASE WHEN revoked_at IS NOT NULL OR $6 OR NOT knowledge_source_eligible(post_id)
 OR NOT EXISTS(SELECT 1 FROM posts p WHERE p.id=k.post_id AND p.body=$7 AND coalesce(p.title,'')=$8 AND p.expires_at=k.expires_at)
 THEN 'cancel_pending' ELSE $4 END,
 revoked_at=CASE WHEN $6 THEN coalesce(revoked_at,now()) ELSE revoked_at END,
 attempts=attempts+1,last_error_code='',retry_at=now()+interval '30 seconds',updated_at=now() WHERE post_id=$1 AND operation_key=$5`, postID, result.ID, result.SearchState, state, key, result.Expired, body, title)
	}
	if err != nil {
		return true, err
	}
	if err = tx.Commit(ctx); err != nil {
		return true, err
	}
	return true, callErr
}

// Start uses the product's cancellable background-job lifecycle. Content and
// provider errors never enter logs; durable metadata exposes safe status codes.
func (s *Service) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				callCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
				_, _ = s.ProcessOne(callCtx)
				cancel()
			}
		}
	}()
	return done
}
