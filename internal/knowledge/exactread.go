package knowledge

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Observation is the exact live canonical content of one published finding,
// resolved from a caller-supplied post ID only -- never a caller-selected
// brain, fact ID, path or credential. FactID is echoed for audit/debugging,
// not as a value a caller can pass back into any other API.
type Observation struct {
	PostID           string `json:"post_id"`
	FactID           string `json:"fact_id"`
	Fact             string `json:"fact"`
	Provenance       string `json:"provenance"`
	ValidUntil       string `json:"valid_until,omitempty"`
	ContentUntrusted bool   `json:"content_untrusted"`
}

// resolveExactFact returns the durable fact_id postID's global-brain
// publication currently maps to, or ErrNotFound for every reason it does
// not: missing, not readable by actor, revoked, expired, not source-
// eligible, or not yet past the write itself. One uniform outcome by
// design -- matching GetPost's own uniform 404 for both a missing post and
// one the actor cannot read -- so a caller can never use the failure shape
// as an oracle for which reason applies. Deliberately looser than Search's
// state='indexed'-only gate: state IN('applied','indexed') both guarantee
// worker.go already wrote a live fact_id (see its own state-transition
// comment), and an exact read has no dependency on search-index
// freshness, only on the canonical write itself having landed.
func (s *Service) resolveExactFact(ctx context.Context, actor, postID string) (string, error) {
	var factID string
	err := s.Pool.QueryRow(ctx, `SELECT k.fact_id FROM knowledge_publications k JOIN posts p ON p.id=k.post_id
 WHERE k.post_id=$1 AND k.brain_id=$2 AND k.state IN('applied','indexed')
 AND k.revoked_at IS NULL AND k.expires_at>now() AND knowledge_source_eligible(k.post_id)
 AND can_read_post($3,k.post_id)`, postID, s.BrainID, actor).Scan(&factID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return factID, nil
}

// ReadExact resolves postID to its durable global-brain fact mapping,
// authorizes the read, calls the canonical backend, then re-authorizes
// before returning any content -- a fence a delayed RPC's actor-access
// removal, source expiry/withdrawal, or fact remapping must still catch
// (READ COMMITTED sees any authorization change already committed while
// the RPC was in flight, the same property worker.go's own writes rely
// on). Re-checking the caller's own credential/scopes around this call is
// a separate, owned obligation of whoever wires in the HTTP handler --
// this only has an agent ID, never a credential (same explicit limitation
// GuardGroupBrain documents for the group-brain read path).
//
// postID is the only caller-supplied selector. BrainID is this Service's
// own configured global partition, never a caller choice; a group- or
// privately-bound observation is never eligible here, since
// knowledge_source_eligible and this Service's own single BrainID already
// exclude anything but a public, ungrouped, web-published root finding.
func (s *Service) ReadExact(ctx context.Context, actor, postID string) (Observation, error) {
	factID, err := s.resolveExactFact(ctx, actor, postID)
	if err != nil {
		return Observation{}, err
	}
	result, err := s.Backend.ReadExact(ctx, factID)
	if err != nil {
		return Observation{}, err
	}
	// Match the returned exact opaque fact ID to the durable publication
	// mapping, and require content_untrusted, before trusting anything
	// else in the response (protocol-level validation this Service owns
	// regardless of which Backend implementation answered).
	if result.ID != factID || !result.ContentUntrusted {
		return Observation{}, ErrUnavailable
	}
	recheckFactID, err := s.resolveExactFact(ctx, actor, postID)
	if err != nil {
		return Observation{}, err
	}
	if recheckFactID != factID {
		// The mapping moved (or access/eligibility was lost and
		// resolveExactFact itself already turned that into ErrNotFound
		// above) while the RPC was in flight -- suppress a body that no
		// longer matches current authority, never partially trust it.
		return Observation{}, ErrUnavailable
	}
	return Observation{
		PostID:           postID,
		FactID:           result.ID,
		Fact:             result.Fact,
		Provenance:       result.Provenance,
		ValidUntil:       result.ValidUntil,
		ContentUntrusted: result.ContentUntrusted,
	}, nil
}
