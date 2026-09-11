package knowledge

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Backend is a supported remote writer protocol, not an in-process write facade.
// Implementations must durably cancel missing keys before acknowledging Cancel.
type Backend interface {
	Remember(context.Context, RememberInput) (RememberResult, error)
	Cancel(context.Context, string) (CancelResult, error)
	Search(context.Context, string) ([]SearchHit, error)
}

// Service selects one operator-owned partition. No caller-supplied brain ID is
// accepted by API handlers. The first partition admits web-public findings only.
type Service struct {
	Pool    *pgxpool.Pool
	BrainID string
	Backend Backend
}
