// Package domains manages DNS ownership independently from agent scope grants.
package domains

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrProof = errors.New("DNS proof not found; publish the TXT record and retry")
var ErrConflict = errors.New("domain is verified by another organization")
var namePattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func Normalize(name string) (string, error) {
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if len(name) > 253 || !namePattern.MatchString(name) || net.ParseIP(name) != nil {
		return "", errors.New("enter a DNS domain without a URL or port")
	}
	return name, nil
}

type Lookup func(context.Context, string) ([]string, error)

func Matches(ctx context.Context, lookup Lookup, domain, token string) bool {
	records, err := lookup(ctx, "_ajent-verification."+domain)
	if err != nil || token == "" {
		return false
	}
	for _, record := range records {
		if record == "ajent-verification="+token {
			return true
		}
	}
	return false
}
func Challenge(ctx context.Context, pool *pgxpool.Pool, org, actor, domain string) (string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, org).Scan(&id); err != nil {
		return "", err
	}
	// Reuse a pending challenge or persistent proof; rerunning installation is idempotent.
	var token string
	err = tx.QueryRow(ctx, `SELECT token FROM domain_challenges WHERE org_id=$1 AND domain=$2 AND expires_at>now() UNION ALL SELECT proof_token FROM verified_domains WHERE org_id=$1 AND domain=$2 AND proof_token<>'' LIMIT 1`, org, domain).Scan(&token)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM domain_challenges WHERE org_id=$1 AND domain<>$2`, org, domain).Scan(&count); err != nil {
			return "", err
		}
		if count >= 20 {
			return "", errors.New("maximum 20 domain claims")
		}
		b := make([]byte, 32)
		if _, err = rand.Read(b); err != nil {
			return "", err
		}
		token = base64.RawURLEncoding.EncodeToString(b)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO domain_challenges(org_id,domain,token) VALUES($1,$2,$3) ON CONFLICT(org_id,domain) DO UPDATE SET token=$3,expires_at=now()+interval '24 hours'`, org, domain, token); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,'domain.challenge','domain',$2)`, actor, domain); err != nil {
		return "", err
	}
	return token, tx.Commit(ctx)
}
func Check(ctx context.Context, pool *pgxpool.Pool, org, actor, domain string, lookup Lookup) error {
	var token string
	if err := pool.QueryRow(ctx, `SELECT token FROM domain_challenges WHERE org_id=$1 AND domain=$2 AND expires_at>now()`, org, domain).Scan(&token); err != nil {
		return fmt.Errorf("create a DNS challenge first: %w", err)
	}
	if !Matches(ctx, lookup, domain, token) {
		return ErrProof
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM organizations WHERE id=$1 AND status='active' FOR UPDATE`, org).Scan(&current); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT token FROM domain_challenges WHERE org_id=$1 AND domain=$2 AND token=$3 AND expires_at>now() FOR UPDATE`, org, domain, token).Scan(&current); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO verified_domains(domain,org_id,verified_until,proof_token,next_check_at) VALUES($1,$2,now()+interval '30 days',$3,now()+interval '1 day') ON CONFLICT(domain) DO UPDATE SET org_id=$2,verified_until=now()+interval '30 days',proof_token=$3,next_check_at=now()+interval '1 day',last_check_error='' WHERE verified_domains.org_id=$2 OR verified_domains.verified_until<=now()`, domain, org, token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	if _, err = tx.Exec(ctx, `DELETE FROM domain_grants g USING credentials c,agents a WHERE g.parent_credential_id=c.id AND c.agent_id=a.id AND g.domain=$1 AND a.org_id<>$2`, domain, org); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM agent_domains ad USING agents a WHERE ad.agent_id=a.id AND ad.domain=$1 AND a.org_id<>$2`, domain, org); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES($1,'domain.check','domain',$2)`, actor, domain); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Renew checks one due domain per call. Row locking prevents duplicate work across replicas.
func Renew(ctx context.Context, pool *pgxpool.Pool, lookup Lookup) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT domain,proof_token FROM verified_domains WHERE proof_token<>'' AND next_check_at<=now() ORDER BY next_check_at LIMIT 1 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	var domain, token string
	if rows.Next() {
		err = rows.Scan(&domain, &token)
	}
	rows.Close()
	if err != nil {
		return err
	}
	if rows.Err() != nil {
		return rows.Err()
	}
	if domain == "" {
		return nil
	}
	if Matches(ctx, lookup, domain, token) {
		_, err = tx.Exec(ctx, `UPDATE verified_domains SET verified_until=now()+interval '30 days',next_check_at=now()+interval '1 day',last_check_error='' WHERE domain=$1`, domain)
	} else {
		// A transient DNS failure must not extend or prematurely revoke the existing lease.
		_, err = tx.Exec(ctx, `UPDATE verified_domains SET next_check_at=now()+interval '1 hour',last_check_error='DNS proof not found; check your TXT record before verification expires.' WHERE domain=$1`, domain)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
