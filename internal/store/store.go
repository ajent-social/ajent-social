package store

import (
	"ajent/internal/auth"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ Pool *pgxpool.Pool }

func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	if cfg.ConnConfig.ConnectTimeout == 0 {
		cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	}
	if _, ok := cfg.ConnConfig.RuntimeParams["statement_timeout"]; !ok {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	}
	if _, ok := cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"]; !ok {
		cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "15000"
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{Pool: pool}, nil
}

type Credential struct {
	CredID  string
	AgentID string
	OrgID   string
	Scopes  []string
}

var ErrUnauthenticated = errors.New("invalid or revoked credential")

func (s *Store) Authenticate(ctx context.Context, lookupID, secret string) (Credential, error) {
	var c Credential
	var digest []byte
	var expires, revoked *time.Time
	err := s.Pool.QueryRow(ctx, `
		SELECT cr.id, cr.agent_id, a.org_id, cr.scopes, cr.secret_digest, cr.expires_at, cr.revoked_at
		FROM credentials cr
		JOIN agents a ON a.id = cr.agent_id
		JOIN organizations o ON o.id = a.org_id
		WHERE cr.lookup_id = $1 AND a.status = 'active' AND o.status = 'active'`,
		lookupID).Scan(&c.CredID, &c.AgentID, &c.OrgID, &c.Scopes, &digest, &expires, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrUnauthenticated
	}
	if err != nil {
		return c, err
	}
	if revoked != nil || (expires != nil && expires.Before(time.Now())) {
		return c, ErrUnauthenticated
	}
	if !authConstantEq(digest, secret) {
		return c, ErrUnauthenticated
	}
	return c, nil
}

func (s *Store) HasScope(c Credential, scope string) bool {
	for _, s2 := range c.Scopes {
		if s2 == scope {
			return true
		}
	}
	return false
}

func authConstantEq(a []byte, secret string) bool { return auth.ConstantEq(a, auth.Digest(secret)) }
