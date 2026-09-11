package knowledge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// GroupObservation mirrors Publication for a group-private grant. It carries
// no credential, endpoint, or profile name -- those never leave the server.
type GroupObservation struct {
	PostID      string    `json:"post_id"`
	BrainID     string    `json:"brain_id"`
	State       string    `json:"state"`
	SearchState string    `json:"search_state"`
	FactID      string    `json:"fact_id,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	Revoked     bool      `json:"revoked"`
}

// RequestGroupObservation commits control/outbox state only; it never waits
// on a writer. It never accepts a caller-supplied group or brain ID: the
// binding is resolved server-side, in the same transaction and off the same
// row, as the eligibility/authority check below -- exactly the post's own
// CURRENT group_id and its live binding, never a caller assertion. A source
// grants one immutable publication; withdrawal cannot be undone by replay.
func (s *Service) RequestGroupObservation(ctx context.Context, actor, postID string, publish bool) (GroupObservation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return GroupObservation{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize grant creation with the authority-loss triggers in migration
	// 022 (same lock name they take): a revocation that commits around this
	// insert must either deny it or durably fence the row it just created.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('group-knowledge-authority',0))`); err != nil {
		return GroupObservation{}, err
	}

	var brainID string
	var eligible bool
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT gb.brain_id::text,group_knowledge_source_eligible(p.id),p.expires_at
 FROM posts p JOIN group_knowledge_bindings gb ON gb.group_id=p.group_id AND gb.revoked_at IS NULL
 WHERE p.id=$1 AND p.author_id=$2 FOR SHARE OF p`, postID, actor).Scan(&brainID, &eligible, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return GroupObservation{}, ErrNotFound
	}
	if err != nil {
		return GroupObservation{}, err
	}
	if publish && !eligible {
		return GroupObservation{}, ErrNotFound
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "group-knowledge-publication:"+brainID); err != nil {
		return GroupObservation{}, err
	}
	var hasCapacity bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM group_knowledge_publications WHERE post_id=$1) OR (SELECT count(*) FROM group_knowledge_publications WHERE brain_id=$2)<10000`, postID, brainID).Scan(&hasCapacity); err != nil {
		return GroupObservation{}, err
	}
	if !hasCapacity {
		return GroupObservation{}, ErrCapacity
	}

	_, err = tx.Exec(ctx, `INSERT INTO group_knowledge_publications(post_id,brain_id,operation_key,requested_by,expires_at,revoked_at,state)
 VALUES($1::uuid,$2,'ajent-group:'||$1::uuid::text,$3,$4,CASE WHEN $5 THEN NULL ELSE now() END,CASE WHEN $5 THEN 'pending' ELSE 'cancel_pending' END)
 ON CONFLICT(post_id) DO NOTHING`, postID, brainID, actor, expires, publish)
	if err != nil {
		return GroupObservation{}, err
	}
	if !publish {
		_, err = tx.Exec(ctx, `UPDATE group_knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state=CASE WHEN state='canceled' THEN state ELSE 'cancel_pending' END,retry_at=now(),updated_at=now() WHERE post_id=$1 AND brain_id=$2`, postID, brainID)
		if err != nil {
			return GroupObservation{}, err
		}
	}
	var out GroupObservation
	err = tx.QueryRow(ctx, `SELECT post_id::text,brain_id::text,state,search_state,fact_id,expires_at,revoked_at IS NOT NULL FROM group_knowledge_publications WHERE post_id=$1 AND brain_id=$2`, postID, brainID).Scan(&out.PostID, &out.BrainID, &out.State, &out.SearchState, &out.FactID, &out.ExpiresAt, &out.Revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return GroupObservation{}, ErrNotFound
	}
	if err != nil {
		return GroupObservation{}, err
	}
	if publish && out.Revoked {
		return GroupObservation{}, ErrRevoked
	}
	if err = tx.Commit(ctx); err != nil {
		return GroupObservation{}, err
	}
	return out, nil
}

// StatusGroupObservation is author-only control metadata, mirroring Status.
func (s *Service) StatusGroupObservation(ctx context.Context, actor, postID string) (GroupObservation, error) {
	var out GroupObservation
	err := s.Pool.QueryRow(ctx, `SELECT k.post_id::text,k.brain_id::text,k.state,k.search_state,k.fact_id,k.expires_at,k.revoked_at IS NOT NULL OR k.expires_at<=now() OR NOT group_knowledge_source_eligible(k.post_id)
 FROM group_knowledge_publications k JOIN posts p ON p.id=k.post_id WHERE k.post_id=$1 AND p.author_id=$2`, postID, actor).Scan(&out.PostID, &out.BrainID, &out.State, &out.SearchState, &out.FactID, &out.ExpiresAt, &out.Revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return GroupObservation{}, ErrNotFound
	}
	if out.Revoked {
		out.FactID = ""
		out.SearchState = "not_eligible"
	}
	return out, err
}

// ProcessOneGroup drains the group-private outbox one row at a time, across
// every group-private brain this Service holds a resolvable writer for. It
// never touches the global BrainID/Backend, and it never falls back to them:
// a brain whose credential_profile does not resolve (resolveProfileBackend)
// is left for a later retry, exactly like a transient writer_unavailable
// outcome, not silently skipped or written through the wrong writer.
//
// Mirrors ProcessOne's own transaction/locking shape: a transaction advisory
// lock keyed by brain ID prevents competing consumers on the SAME brain (two
// different brains drain concurrently, by design); no row lock spans a
// provider request, so author withdrawal or a revocation can commit
// immediately while a write is in flight; resolveProfileBackend runs inside
// the same transaction, right before the RPC, off the row it authorizes.
func (s *Service) ProcessOneGroup(ctx context.Context) (bool, error) {
	var candidateBrain string
	err := s.Pool.QueryRow(ctx, `SELECT k.brain_id::text FROM group_knowledge_publications k
 WHERE k.state<>'canceled' AND k.retry_at<=now() LIMIT 1`).Scan(&candidateBrain)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

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
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))`, "group-knowledge:"+candidateBrain).Scan(&locked); err != nil || !locked {
		return false, err
	}

	var postID, key, body, title string
	var until time.Time
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT k.post_id::text,k.operation_key,p.body,coalesce(p.title,''),k.expires_at,
 k.revoked_at IS NULL AND k.state<>'cancel_pending' AND k.expires_at>now() AND group_knowledge_source_eligible(k.post_id)
 FROM group_knowledge_publications k JOIN posts p ON p.id=k.post_id
 WHERE k.brain_id=$1 AND k.state<>'canceled' AND k.retry_at<=now() AND (
 (k.state IN('pending','applied','recovery_required','cancel_pending') AND k.last_error_code<>'operation_conflict')
 OR k.revoked_at IS NOT NULL OR k.expires_at<=now() OR NOT group_knowledge_source_eligible(k.post_id))
 ORDER BY CASE WHEN k.revoked_at IS NOT NULL THEN 0 ELSE 1 END,k.retry_at,k.post_id LIMIT 1`, candidateBrain).Scan(&postID, &key, &body, &title, &until, &eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	backend, err := s.resolveProfileBackend(ctx, tx, candidateBrain)
	if err != nil {
		// The row's own writer identity is not configured/resolvable right
		// now (unset or rotated credential_profile) -- retry later. Never
		// fall back to s.Backend or any other brain's backend.
		if _, uerr := tx.Exec(ctx, `UPDATE group_knowledge_publications SET attempts=attempts+1,last_error_code='profile_unavailable',retry_at=now()+interval '30 seconds',updated_at=now() WHERE post_id=$1`, postID); uerr != nil {
			return true, uerr
		}
		if cerr := tx.Commit(ctx); cerr != nil {
			return true, cerr
		}
		return true, err
	}

	if !eligible {
		_, callErr := backend.Cancel(ctx, key)
		if callErr != nil {
			_, err = tx.Exec(ctx, `UPDATE group_knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state='cancel_pending',attempts=attempts+1,last_error_code='cancel_unavailable',retry_at=now()+interval '30 seconds',updated_at=now() WHERE post_id=$1`, postID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE group_knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state='canceled',search_state='not_eligible',attempts=attempts+1,last_error_code='',updated_at=now() WHERE post_id=$1`, postID)
		}
		if err != nil {
			return true, err
		}
		if err = tx.Commit(ctx); err != nil {
			return true, err
		}
		return true, callErr
	}

	in := RememberInput{OperationKey: key, Fact: title + "\n\n" + body, Provenance: fmt.Sprintf("Ajent group finding %s; source post and author attribution available through the authorized Ajent gateway. Author-reported evidence; not independently verified by ingestion.", postID), ExpiresAt: until}
	result, callErr := backend.Remember(ctx, in)
	if callErr != nil {
		code := "writer_unavailable"
		if errors.Is(callErr, ErrConflict) {
			code = "operation_conflict"
		}
		if errors.Is(callErr, ErrCanceled) {
			_, err = tx.Exec(ctx, `UPDATE group_knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state='canceled',search_state='not_eligible',last_error_code='operation_canceled',attempts=attempts+1,updated_at=now() WHERE post_id=$1`, postID)
		} else {
			_, err = tx.Exec(ctx, `UPDATE group_knowledge_publications SET state=CASE WHEN revoked_at IS NOT NULL OR NOT group_knowledge_source_eligible(post_id) THEN 'cancel_pending' ELSE 'recovery_required' END,attempts=attempts+1,last_error_code=$2,retry_at=now()+interval '30 seconds',updated_at=now() WHERE post_id=$1`, postID, code)
		}
	} else {
		state := "applied"
		if result.SearchState == "lexical" || result.SearchState == "semantic" {
			state = "indexed"
		}
		_, err = tx.Exec(ctx, `UPDATE group_knowledge_publications k SET fact_id=$2,search_state=$3,
 state=CASE WHEN revoked_at IS NOT NULL OR $6 OR NOT group_knowledge_source_eligible(post_id)
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
