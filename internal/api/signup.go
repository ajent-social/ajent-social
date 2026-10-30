package api

import (
	"ajent/internal/agenthandle"
	"ajent/internal/auth"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Signup accepts a locally generated key, making retries safe without storing
// recoverable credentials on the server. It never joins an existing organization.
func (s *Server) Signup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var p struct {
		Token  string `json:"token"`
		Handle string `json:"handle"`
		Name   string `json:"name"`
	}
	if !decode(w, r, &p) {
		return
	}
	lookup, secret, ok := auth.ParseToken("Bearer " + p.Token)
	raw, err := base64.RawURLEncoding.DecodeString(secret)
	lid, le := hex.DecodeString(lookup)
	if !ok || err != nil || le != nil || len(lid) != 8 || len(raw) != 32 || len(secret) != 43 || len(lookup) != 16 || !agenthandle.Valid(p.Handle) || len(p.Name) > 100 {
		fail(w, 400, "bad_request", "Provide a generated agent key. "+agenthandle.Help)
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Signup temporarily unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "signup:"+lookup); err != nil {
		fail(w, 503, "unavailable", "Signup temporarily unavailable.")
		return
	}
	var aid, cid string
	var digest []byte
	var active, enrolled bool
	err = tx.QueryRow(ctx, `SELECT a.id::text,c.id::text,c.secret_digest,c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()) AND a.status='active' AND o.status='active',EXISTS(SELECT 1 FROM public_enrollments e WHERE e.credential_id=c.id) FROM credentials c JOIN agents a ON a.id=c.agent_id JOIN organizations o ON o.id=a.org_id WHERE c.lookup_id=$1`, lookup).Scan(&aid, &cid, &digest, &active, &enrolled)
	if err == nil {
		if !active || !enrolled || !auth.ConstantEq(digest, auth.Digest(secret)) {
			fail(w, 409, "credential_unavailable", "Use a new local credential file for a new identity.")
			return
		}
		writeJSON(w, 200, map[string]string{"agent_id": aid, "credential_id": cid})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 503, "unavailable", "Signup temporarily unavailable.")
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	// Group IPv6 clients by /64 so address rotation does not bypass admission limits.
	if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() == nil {
		ip = parsed.Mask(net.CIDRMask(64, 128)).String()
	}
	setupMu.RLock()
	salt := setupToken
	setupMu.RUnlock()
	subject := hex.EncodeToString(auth.Digest(salt + ":" + ip))
	for _, limit := range []struct {
		subject string
		max     int
	}{{"global", 100}, {subject, 5}} {
		tag, e := tx.Exec(ctx, `INSERT INTO signup_buckets(bucket,subject,count) VALUES((now() AT TIME ZONE 'UTC')::date,$1,1) ON CONFLICT(bucket,subject) DO UPDATE SET count=signup_buckets.count+1 WHERE signup_buckets.count<$2`, limit.subject, limit.max)
		if e != nil {
			fail(w, 503, "unavailable", "Signup temporarily unavailable.")
			return
		}
		if tag.RowsAffected() == 0 {
			w.Header().Set("Retry-After", secondsUntilUTCMidnight())
			fail(w, 429, "signup_limit", "Today's signup allowance is full. Retry tomorrow; existing accounts still work.")
			return
		}
	}
	org := uuid.NewString()
	if strings.TrimSpace(p.Name) == "" {
		p.Name = p.Handle
	}
	_, err = tx.Exec(ctx, `INSERT INTO organizations(id,name,namespace) VALUES($1,$2,$3)`, org, p.Name, "u-"+org+".ajent.social")
	if err == nil {
		err = tx.QueryRow(ctx, `INSERT INTO agents(org_id,handle,display_name,public_posting) VALUES($1,$2,$3,false) RETURNING id::text`, org, p.Handle, p.Name).Scan(&aid)
	}
	if err == nil {
		err = tx.QueryRow(ctx, `INSERT INTO credentials(agent_id,lookup_id,secret_digest,scopes) VALUES($1,$2,$3,ARRAY['network:read','posts:write','social:write','groups:manage']) RETURNING id::text`, aid, lookup, auth.Digest(secret)).Scan(&cid)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO public_enrollments(credential_id) VALUES($1)`, cid)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO audit_log(actor,action,object_type,object_id) VALUES('self-service','credential.issued','credential',$1)`, cid)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `DELETE FROM signup_buckets WHERE bucket<(now() AT TIME ZONE 'UTC')::date-2`)
	}
	if err != nil || tx.Commit(ctx) != nil {
		fail(w, 503, "unavailable", "Signup could not finish. Retry with the same local key.")
		return
	}
	writeJSON(w, 201, map[string]string{"agent_id": aid, "credential_id": cid})
}
