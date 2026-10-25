// Package mcpoauth adapts the shared OAuth protocol to Ajent PostgreSQL custody.
package mcpoauth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	protocol "github.com/ajent-social/go/mcpoauth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store's optional Tx lets the browser approval and binding creation commit once.
type Store struct {
	Pool     *pgxpool.Pool
	Tx       pgx.Tx
	Metadata ClientMetadataResolver
}

func (s *Store) transaction(ctx context.Context, fn func(pgx.Tx) error) error {
	if s.Tx != nil {
		return fn(s.Tx)
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// Serialize bounded protocol storage admission and refresh/revoke transitions.
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(762185)"); e != nil {
		return e
	}
	if e = fn(tx); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func insert(ctx context.Context, tx pgx.Tx, kind, id string, v any, expires time.Time) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `INSERT INTO mcp_oauth_records(kind,id,record,expires_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, kind, id, b, expires)
	if e == nil && tag.RowsAffected() != 1 {
		return protocol.ErrExists
	}
	return e
}
func read(ctx context.Context, tx pgx.Tx, kind, id string, v any, consume bool) error {
	q := `SELECT record FROM mcp_oauth_records WHERE kind=$1 AND id=$2 FOR UPDATE`
	if consume {
		q = `DELETE FROM mcp_oauth_records WHERE kind=$1 AND id=$2 RETURNING record`
	}
	var b []byte
	e := tx.QueryRow(ctx, q, kind, id).Scan(&b)
	if errors.Is(e, pgx.ErrNoRows) {
		return protocol.ErrNotFound
	}
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func update(ctx context.Context, tx pgx.Tx, kind, id string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE mcp_oauth_records SET record=$3 WHERE kind=$1 AND id=$2`, kind, id, b)
	return e
}
func (s *Store) create(ctx context.Context, kind, id string, v any, expires time.Time) error {
	return s.transaction(ctx, func(tx pgx.Tx) error {
		// Keep used refresh hashes through the absolute grant deadline for replay detection.
		if _, e := tx.Exec(ctx, `DELETE FROM mcp_oauth_records WHERE expires_at<now()`); e != nil {
			return e
		}
		var n int
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM mcp_oauth_records WHERE kind=$1`, kind).Scan(&n); e != nil {
			return e
		}
		if n >= 10000 {
			return protocol.ErrDenied
		}
		if kind == "consents" {
			var client string
			if c, ok := v.(protocol.ConsentRecord); ok {
				client = c.ClientID
			}
			if e := tx.QueryRow(ctx, `SELECT count(*) FROM mcp_oauth_records WHERE kind='consents' AND record->>'client_id'=$1`, client).Scan(&n); e != nil {
				return e
			}
			if n >= 100 {
				return protocol.ErrDenied
			}
		}
		return insert(ctx, tx, kind, id, v, expires)
	})
}
func (s *Store) load(ctx context.Context, kind, id string, v any, consume bool) error {
	return s.transaction(ctx, func(tx pgx.Tx) error { return read(ctx, tx, kind, id, v, consume) })
}
func (s *Store) CreateClient(ctx context.Context, v protocol.Client) error {
	return s.create(ctx, "clients", v.ID, v, time.Now().Add(90*24*time.Hour))
}
func (s *Store) Client(ctx context.Context, id string) (v protocol.Client, e error) {
	if validClientMetadataURL(id) {
		if s.Metadata == nil {
			return v, protocol.ErrNotFound
		}
		return s.Metadata.Resolve(ctx, id)
	}
	e = s.load(ctx, "clients", id, &v, false)
	return
}
func (s *Store) CreateConsent(ctx context.Context, v protocol.ConsentRecord) error {
	return s.create(ctx, "consents", v.ID, v, v.ExpiresAt)
}
func (s *Store) Consent(ctx context.Context, id string) (v protocol.ConsentRecord, e error) {
	e = s.load(ctx, "consents", id, &v, false)
	return
}
func (s *Store) ConsumeConsent(ctx context.Context, id string) (v protocol.ConsentRecord, e error) {
	e = s.load(ctx, "consents", id, &v, true)
	return
}
func (s *Store) CreateCode(ctx context.Context, v protocol.CodeRecord) error {
	return s.create(ctx, "codes", v.ID, v, v.ExpiresAt)
}
func (s *Store) ConsumeCode(ctx context.Context, id string) (v protocol.CodeRecord, e error) {
	e = s.load(ctx, "codes", id, &v, true)
	return
}
func (s *Store) Grant(ctx context.Context, id string) (v protocol.GrantRecord, e error) {
	e = s.load(ctx, "grants", id, &v, false)
	return
}
func (s *Store) Token(ctx context.Context, id string) (v protocol.TokenRecord, e error) {
	e = s.load(ctx, "tokens", id, &v, false)
	return
}
func (s *Store) Refresh(ctx context.Context, id string) (v protocol.RefreshRecord, e error) {
	e = s.load(ctx, "refresh", id, &v, false)
	return
}

// Binding rechecks human ownership and live agent/workspace/credential state.
func Binding(ctx context.Context, tx pgx.Tx, human, credential, client string) error {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT true FROM mcp_oauth_bindings b JOIN credentials c ON c.id=b.credential_id JOIN agents a ON a.id=c.agent_id JOIN organizations o ON o.id=a.org_id WHERE b.credential_id::text=$1 AND b.human_id::text=$2 AND b.client_id=$3 AND o.human_owner_id=b.human_id AND o.status='active' AND a.status='active' AND c.revoked_at IS NULL AND c.expires_at>now() FOR SHARE OF b,c,a,o`, credential, human, client).Scan(&ok)
	if errors.Is(e, pgx.ErrNoRows) {
		return protocol.ErrDenied
	}
	return e
}
func (s *Store) CreateGrant(ctx context.Context, v protocol.GrantIssue) error {
	return s.transaction(ctx, func(tx pgx.Tx) error {
		if e := Binding(ctx, tx, v.Grant.Subject, v.Grant.Binding, v.Grant.ClientID); e != nil {
			return e
		}
		if v.Token.GrantID != v.Grant.ID || (v.Refresh != nil && v.Refresh.GrantID != v.Grant.ID) {
			return protocol.ErrInvalid
		}
		var n int
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM mcp_oauth_records WHERE kind='grants' AND record->>'subject'=$1 AND expires_at>now() AND NOT record ? 'revoked_at'`, v.Grant.Subject).Scan(&n); e != nil {
			return e
		}
		if n >= 100 {
			return protocol.ErrDenied
		}
		if e := insert(ctx, tx, "grants", v.Grant.ID, v.Grant, v.Grant.ExpiresAt); e != nil {
			return e
		}
		if e := insert(ctx, tx, "tokens", v.Token.ID, v.Token, v.Token.ExpiresAt); e != nil {
			return e
		}
		if v.Refresh != nil {
			return insert(ctx, tx, "refresh", v.Refresh.ID, v.Refresh, v.Refresh.ExpiresAt)
		}
		return nil
	})
}
func (s *Store) RotateRefresh(ctx context.Context, v protocol.RefreshRotation) error {
	reused := false
	e := s.transaction(ctx, func(tx pgx.Tx) error {
		var old protocol.RefreshRecord
		if e := read(ctx, tx, "refresh", v.RefreshID, &old, false); e != nil {
			return e
		}
		var g protocol.GrantRecord
		if e := read(ctx, tx, "grants", old.GrantID, &g, false); e != nil {
			return e
		}
		if !old.UsedAt.IsZero() {
			reused = true
			if g.RevokedAt.IsZero() {
				g.RevokedAt = v.At
			}
			return update(ctx, tx, "grants", g.ID, g)
		}
		if !g.RevokedAt.IsZero() || !v.At.Before(g.ExpiresAt) || !v.At.Before(old.ExpiresAt) {
			return protocol.ErrDenied
		}
		if v.Token.GrantID != g.ID || v.Refresh.GrantID != g.ID || v.Refresh.Generation != old.Generation+1 || v.Refresh.Generation > 1024 {
			return protocol.ErrInvalid
		}
		if e := Binding(ctx, tx, g.Subject, g.Binding, g.ClientID); e != nil {
			return e
		}
		if e := insert(ctx, tx, "tokens", v.Token.ID, v.Token, v.Token.ExpiresAt); e != nil {
			return e
		}
		if e := insert(ctx, tx, "refresh", v.Refresh.ID, v.Refresh, v.Refresh.ExpiresAt); e != nil {
			return e
		}
		old.UsedAt = v.At
		old.ReplacedBy = v.Refresh.ID
		return update(ctx, tx, "refresh", old.ID, old)
	})
	if e == nil && reused {
		return protocol.ErrReused
	}
	return e
}
func (s *Store) RevokeGrant(ctx context.Context, id string, at time.Time) error {
	return s.transaction(ctx, func(tx pgx.Tx) error {
		var g protocol.GrantRecord
		if e := read(ctx, tx, "grants", id, &g, false); e != nil {
			return e
		}
		if !g.RevokedAt.IsZero() {
			return nil
		}
		g.RevokedAt = at
		return update(ctx, tx, "grants", id, g)
	})
}

var _ protocol.Store = (*Store)(nil)
