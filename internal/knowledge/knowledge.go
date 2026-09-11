package knowledge

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrProfileUnavailable is returned when a private brain's credential_profile
// is empty or names no operator-configured writer. It is always fail-closed:
// never a fallback to Backend (the global writer) or to any other profile.
var ErrProfileUnavailable = errors.New("knowledge writer profile not configured")

// Backend is a supported remote writer protocol, not an in-process write facade.
// Implementations must durably cancel missing keys before acknowledging Cancel.
type Backend interface {
	Remember(context.Context, RememberInput) (RememberResult, error)
	Cancel(context.Context, string) (CancelResult, error)
	Search(context.Context, string) ([]SearchHit, error)
	ReadExact(context.Context, string) (ReadFactResult, error)
}

// Service selects one operator-owned global partition (BrainID/Backend) plus,
// optionally, a fixed set of operator-configured private writers keyed by
// credential profile name (ProfileBackends). No caller-supplied brain ID or
// profile name is ever accepted by an API handler or agent: a private
// operation always resolves its own brain server-side (GuardGroupBrain) and
// then its own profile (resolveProfileBackend), never a caller choice.
// ProfileBackends is built once at startup from explicit operator
// configuration and never mutated afterward; it holds only in-process backend
// handles, never a credential or endpoint string that could be echoed back.
type Service struct {
	Pool            *pgxpool.Pool
	BrainID         string
	Backend         Backend
	ProfileBackends map[string]Backend
}
