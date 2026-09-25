package knowledge

import (
	"context"
	"errors"
	"regexp"

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
	// revoked) group, the brain ID already names the global partition, the
	// credential profile is malformed, or the profile is already the writer
	// identity for a different brain (or this brain was already bound to a
	// different profile than the one now given).
	ErrBindingRejected = errors.New("group knowledge binding rejected")
)

// credentialProfilePattern matches Serenity's own --credential-profile
// grammar (RFC-BRAIN-AUTH-02/dec-1204) so a profile name an operator types
// here is valid on both sides of the boundary. Ajent never imports or calls
// into that mechanism directly; it only ever holds the label plus its own
// operator-configured (endpoint,token) writer for it (see knowledge_env.go).
var credentialProfilePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$`)

// ValidCredentialProfile reports whether name is a well-formed, non-empty
// profile label. An empty string is never valid here: CreateBinding always
// requires an explicit profile, and "" is reserved to mean "unconfigured"
// on a knowledge_brains row (see migration 022).
func ValidCredentialProfile(name string) bool {
	return name != "" && credentialProfilePattern.MatchString(name)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CreateBinding is operator-controlled internal setup, never an agent-writable
// API; callers authorize it out of band. It admits brainID as a private
// partition (creating its knowledge_brains row on first use) bound to
// credentialProfile -- the operator-typed label that must separately resolve
// to a configured writer at RPC time (resolveProfileBackend) -- and binds it
// to groupID in one transaction. group_knowledge_bindings' PRIMARY
// KEY(group_id) and UNIQUE(brain_id) are the actual source of truth for "one
// brain per group, one group per brain"; migration 022's partial unique index
// on knowledge_brains.credential_profile is the actual source of truth for
// "one writer identity per brain" -- this function never deletes or updates
// an existing row, so a prior binding or profile for either ID, revoked or
// not, always wins.
func (s *Service) CreateBinding(ctx context.Context, groupID, brainID, credentialProfile string) error {
	if !ValidCredentialProfile(credentialProfile) {
		return ErrBindingRejected
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// ON CONFLICT DO NOTHING keeps this a no-op error so the transaction is
	// never left aborted: Postgres refuses every later statement in a tx after
	// a raised error unless rolled back to a savepoint, and the scope check
	// below needs to run in the same transaction as the binding insert.
	if _, err = tx.Exec(ctx, `INSERT INTO knowledge_brains(id,scope,credential_profile) VALUES($1,'group_private',$2) ON CONFLICT(id) DO NOTHING`, brainID, credentialProfile); err != nil {
		if isUniqueViolation(err) {
			return ErrBindingRejected
		}
		return err
	}
	var scope, profile string
	if err = tx.QueryRow(ctx, `SELECT scope,credential_profile FROM knowledge_brains WHERE id=$1`, brainID).Scan(&scope, &profile); err != nil {
		return err
	}
	if scope != "group_private" || profile != credentialProfile {
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

// rowQuerier is satisfied by both *pgxpool.Pool and pgx.Tx, so
// resolveProfileBackend can be called either standalone or, more usually,
// inside the same transaction that just selected the row it is about to act
// on -- resolution and the row it authorizes never straddle two snapshots.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// resolveProfileBackend looks up brainID's current credential_profile and
// returns the operator-configured Backend for it. This is the one place a
// private RPC's writer identity is chosen, and it is always called again,
// freshly, immediately before that RPC -- never cached across calls or
// resolved from anything a caller/agent supplied. An empty or unconfigured
// profile fails closed (ErrProfileUnavailable): it never falls back to
// s.Backend (the global writer) or to any other brain's profile.
func (s *Service) resolveProfileBackend(ctx context.Context, q rowQuerier, brainID string) (Backend, error) {
	var profile string
	if err := q.QueryRow(ctx, `SELECT credential_profile FROM knowledge_brains WHERE id=$1 AND scope='group_private'`, brainID).Scan(&profile); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrGroupBrainUnavailable
		}
		return nil, err
	}
	if profile == "" {
		return nil, ErrProfileUnavailable
	}
	backend, ok := s.ProfileBackends[profile]
	if !ok {
		return nil, ErrProfileUnavailable
	}
	return backend, nil
}
