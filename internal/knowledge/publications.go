package knowledge

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("knowledge source unavailable")
var ErrCapacity = errors.New("knowledge partition capacity reached")

var ErrRevoked = errors.New("knowledge publication was withdrawn; create a new deliberate source")

type Publication struct {
	PostID      string    `json:"post_id"`
	State       string    `json:"state"`
	SearchState string    `json:"search_state"`
	FactID      string    `json:"fact_id,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	Revoked     bool      `json:"revoked"`
}

// Request commits control/outbox state only. It never waits on the brain.
// A source grants one immutable publication; withdrawal cannot be undone by replay.
func (s *Service) Request(ctx context.Context, actor, postID string, publish bool) (Publication, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Publication{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize grant creation with authority-loss triggers. A revocation that
	// commits around this insert must either deny it or durably fence its row.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('knowledge-authority',0))`); err != nil {
		return Publication{}, err
	}

	var eligible bool
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT knowledge_source_eligible(id),expires_at FROM posts WHERE id=$1 AND author_id=$2 FOR SHARE`, postID, actor).Scan(&eligible, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Publication{}, ErrNotFound
	}
	if err != nil {
		return Publication{}, err
	}
	if publish && !eligible {
		return Publication{}, ErrNotFound
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "knowledge-publication:"+s.BrainID); err != nil {
		return Publication{}, err
	}
	var hasCapacity bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM knowledge_publications WHERE post_id=$1) OR (SELECT count(*) FROM knowledge_publications WHERE brain_id=$2)<10000`, postID, s.BrainID).Scan(&hasCapacity); err != nil {
		return Publication{}, err
	}
	if !hasCapacity {
		return Publication{}, ErrCapacity
	}

	_, err = tx.Exec(ctx, `INSERT INTO knowledge_publications(post_id,brain_id,operation_key,requested_by,expires_at,revoked_at,state)
 VALUES($1::uuid,$2,'ajent-global:'||$1::uuid::text,$3,$4,CASE WHEN $5 THEN NULL ELSE now() END,CASE WHEN $5 THEN 'pending' ELSE 'cancel_pending' END)
 ON CONFLICT(post_id) DO NOTHING`, postID, s.BrainID, actor, expires, publish)
	if err != nil {
		return Publication{}, err
	}
	if !publish {
		_, err = tx.Exec(ctx, `UPDATE knowledge_publications SET revoked_at=coalesce(revoked_at,now()),state=CASE WHEN state='canceled' THEN state ELSE 'cancel_pending' END,retry_at=now(),updated_at=now() WHERE post_id=$1 AND brain_id=$2`, postID, s.BrainID)
		if err != nil {
			return Publication{}, err
		}
	}
	var out Publication
	err = tx.QueryRow(ctx, `SELECT post_id::text,state,search_state,fact_id,expires_at,revoked_at IS NOT NULL FROM knowledge_publications WHERE post_id=$1 AND brain_id=$2`, postID, s.BrainID).Scan(&out.PostID, &out.State, &out.SearchState, &out.FactID, &out.ExpiresAt, &out.Revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return Publication{}, ErrNotFound
	}
	if err != nil {
		return Publication{}, err
	}
	if publish && out.Revoked {
		return Publication{}, ErrRevoked
	}
	if err = tx.Commit(ctx); err != nil {
		return Publication{}, err
	}
	return out, nil
}

// Status is author-only control metadata. Other readers use filtered search and
// the currently accessible source post; revoked status is not a backdoor read.
func (s *Service) Status(ctx context.Context, actor, postID string) (Publication, error) {
	var out Publication
	err := s.Pool.QueryRow(ctx, `SELECT k.post_id::text,k.state,k.search_state,k.fact_id,k.expires_at,k.revoked_at IS NOT NULL OR k.expires_at<=now() OR NOT knowledge_source_eligible(k.post_id)
 FROM knowledge_publications k JOIN posts p ON p.id=k.post_id WHERE k.post_id=$1 AND k.brain_id=$2 AND p.author_id=$3`, postID, s.BrainID, actor).Scan(&out.PostID, &out.State, &out.SearchState, &out.FactID, &out.ExpiresAt, &out.Revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return Publication{}, ErrNotFound
	}
	if out.Revoked {
		out.FactID = ""
		out.SearchState = "not_eligible"
	}
	return out, err
}
