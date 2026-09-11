package knowledge

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrGroupBrainUnavailable is the single uniform result for every unauthorized
	// or missing private-binding read: a nonmember (including one from another
	// organization -- cross-org membership itself is legitimate, see
	// GuardGroupBrain), pending, removed, or suspended actor, a suspended
	// actor-org or group-owning-org, an archived/public group, and a missing/
	// revoked/mismatched binding are never distinguished in the response.
	ErrGroupBrainUnavailable = errors.New("group knowledge binding unavailable")
	// ErrBindingRejected covers every operator-setup failure: the group is
	// already bound, the brain is already bound to a different (possibly
	// revoked) group, or the brain ID already names the global partition.
	ErrBindingRejected = errors.New("group knowledge binding rejected")
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CreateBinding is operator-controlled internal setup, never an agent-writable
// API; callers authorize it out of band. It admits brainID as a private
// partition (creating its knowledge_brains row on first use) and binds it to
// groupID in one transaction. group_knowledge_bindings' PRIMARY KEY(group_id)
// and UNIQUE(brain_id) are the actual source of truth for "one brain per
// group, one group per brain" -- this function never deletes or updates an
// existing row, so a prior binding for either ID, revoked or not, always wins.
func (s *Service) CreateBinding(ctx context.Context, groupID, brainID string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// ON CONFLICT DO NOTHING keeps this a no-op error so the transaction is
	// never left aborted: Postgres refuses every later statement in a tx after
	// a raised error unless rolled back to a savepoint, and the scope check
	// below needs to run in the same transaction as the binding insert.
	if _, err = tx.Exec(ctx, `INSERT INTO knowledge_brains(id,scope) VALUES($1,'group_private') ON CONFLICT(id) DO NOTHING`, brainID); err != nil {
		return err
	}
	var scope string
	if err = tx.QueryRow(ctx, `SELECT scope FROM knowledge_brains WHERE id=$1`, brainID).Scan(&scope); err != nil {
		return err
	}
	if scope != "group_private" {
		return ErrBindingRejected
	}
	if _, err = tx.Exec(ctx, `INSERT INTO group_knowledge_bindings(group_id,brain_id) VALUES($1,$2)`, groupID, brainID); err != nil {
		if isUniqueViolation(err) {
			return ErrBindingRejected
		}
		return err
	}
	return tx.Commit(ctx)
}

// RevokeBinding is operator-controlled internal setup. It permanently retires
// a live binding; the row is kept (revoked_at set), never deleted, so
// CreateBinding can never reuse or resurrect groupID or its brain afterward.
func (s *Service) RevokeBinding(ctx context.Context, groupID string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE group_knowledge_bindings SET revoked_at=now() WHERE group_id=$1 AND revoked_at IS NULL`, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrGroupBrainUnavailable
	}
	return nil
}

// GuardGroupBrain authorizes an already-authenticated actor -- it receives only
// an agent ID, never a credential, and proves membership/actor/org/binding
// state given that identity, not credential validity. A caller wiring this
// into a real API must separately recheck the actor's actual credential
// (expiry/revocation/scopes) around any outbound call; that is an explicit
// owned dependency for whoever adds a real handler, not solved here, and this
// function must never be read as having done it.
//
// It requires actor's CURRENT accepted membership in groupID and resolves the
// live private brain groupID is bound to. A group ID is an untrusted selector,
// never authority: every predicate is evaluated fresh from current rows, never
// trusted from the caller or cached from a prior call. Group membership is
// legitimately cross-organization by existing product design (see
// internal/api/social.go's invite/accept path, which has no same-org check) --
// this does not special-case or ban it. It independently requires BOTH the
// actor's own organization AND the group's owning organization to be active
// (not that they are the same organization): an accepted member's own org
// being active is not sufficient if the group's home org has been suspended,
// and vice versa. This performs one unlocked read -- it must never hold a row
// or advisory lock across an outbound backend call, so a membership/binding
// revocation can always commit immediately and take effect on the very next
// call.
//
// Call it once to select a backend and once more, passing the previously
// resolved brain ID, after any outbound callback returns and before any
// private result reaches the caller -- see RecheckGroupBrain. A test driving
// Guard/Recheck directly around a simulated callback proves these primitives
// hold that contract; it does not prove any future real handler actually
// calls them that way -- that remains the real handler's obligation.
func (s *Service) GuardGroupBrain(ctx context.Context, actor, groupID string) (string, error) {
	var brainID string
	err := s.Pool.QueryRow(ctx, `SELECT gb.brain_id::text
 FROM group_members gm
 JOIN groups g ON g.id=gm.group_id
 JOIN organizations go ON go.id=g.org_id
 JOIN agents a ON a.id=gm.agent_id
 JOIN organizations ao ON ao.id=a.org_id
 JOIN group_knowledge_bindings gb ON gb.group_id=gm.group_id
 JOIN knowledge_brains b ON b.id=gb.brain_id
 WHERE gm.group_id=$1 AND gm.agent_id=$2 AND gm.state='accepted'
 AND g.archived_at IS NULL AND g.visibility='private'
 AND a.status='active' AND ao.status='active' AND go.status='active'
 AND gb.revoked_at IS NULL AND b.scope='group_private'`, groupID, actor).Scan(&brainID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrGroupBrainUnavailable
	}
	if err != nil {
		return "", err
	}
	return brainID, nil
}

// RecheckGroupBrain re-runs GuardGroupBrain and additionally requires the
// resolved brain to still be the exact one an outbound call already used -- a
// revoke landing while that call was in flight must suppress its result.
func (s *Service) RecheckGroupBrain(ctx context.Context, actor, groupID, expectBrainID string) error {
	brainID, err := s.GuardGroupBrain(ctx, actor, groupID)
	if err != nil {
		return err
	}
	if brainID != expectBrainID {
		return ErrGroupBrainUnavailable
	}
	return nil
}
