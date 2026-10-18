package api

import (
	"ajent/internal/auth"
	"ajent/internal/store"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

// Lock a valid supplied key before resource locks, but defer validation and
// receipt lookup until after resource authority has been checked.
func lockMutationKey(r *http.Request, tx pgx.Tx, c *store.Credential) error {
	key := r.Header.Get("Idempotency-Key")
	if key == "" || len(key) > 128 {
		return nil
	}
	_, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, c.CredID+":"+key)
	return err
}

// mutationReplay serializes keys across all group operations. Authority must be
// checked before returning a stored receipt; replay never grants access.
func mutationReplay(w http.ResponseWriter, r *http.Request, tx pgx.Tx, c *store.Credential, p any, required bool) ([]byte, bool) {
	key := r.Header.Get("Idempotency-Key")
	if len(key) > 128 || (required && key == "") {
		fail(w, 400, "bad_request", "Idempotency-Key required (max 128 bytes).")
		return nil, false
	}
	if key == "" {
		return nil, true
	}
	raw, _ := json.Marshal(p)
	digest := sha256.Sum256(append([]byte(r.Method+":"+r.URL.Path+":"), raw...))
	if err := lockMutationKey(r, tx, c); err != nil {
		fail(w, 503, "unavailable", "Replay unavailable.")
		return nil, false
	}
	var stored, result []byte
	err := tx.QueryRow(r.Context(), `SELECT request_digest,result FROM idempotency_keys WHERE credential_id=$1 AND key=$2 AND expires_at>now()`, c.CredID, key).Scan(&stored, &result)
	if err == nil {
		if !auth.ConstantEq(stored, digest[:]) {
			fail(w, 409, "idempotency_conflict", "Key used for another operation.")
			return nil, false
		}
		return result, true
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 503, "unavailable", "Replay unavailable.")
		return nil, false
	}
	return nil, true
}
func mutationSave(r *http.Request, tx pgx.Tx, c *store.Credential, p, result any) error {
	key := r.Header.Get("Idempotency-Key")
	if key == "" {
		return nil
	}
	raw, _ := json.Marshal(p)
	digest := sha256.Sum256(append([]byte(r.Method+":"+r.URL.Path+":"), raw...))
	res, _ := json.Marshal(result)
	_, err := tx.Exec(r.Context(), `INSERT INTO idempotency_keys(credential_id,key,method,route,request_digest,result) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(credential_id,key) DO UPDATE SET method=EXCLUDED.method,route=EXCLUDED.route,request_digest=EXCLUDED.request_digest,result=EXCLUDED.result,expires_at=now()+interval '24 hours'`, c.CredID, key, r.Method, r.URL.Path, digest[:], string(res))
	return err
}

// lockGroupEligibility protects the authorization decision until commit. Agent
// FOR UPDATE conflicts with status/rehome updates and the FOR KEY SHARE locks
// taken by both blocks foreign keys. Organization FOR SHARE also conflicts with
// non-key status updates. Always lock agents and organizations in UUID order.
// Existing groups must be locked before participants, matching membership/post
// writers. Read eligibility in a fresh statement after every lock has settled.
func lockGroupEligibility(ctx context.Context, tx pgx.Tx, actor, target, actorOrg, targetOrg string) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM agents WHERE id IN($1,$2) ORDER BY id FOR UPDATE`, actor, target)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
	}
	rows.Close()
	if err != nil {
		return false, err
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	rows, err = tx.Query(ctx, `SELECT id FROM organizations WHERE id IN($1,$2) ORDER BY id FOR SHARE`, actorOrg, targetOrg)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
	}
	rows.Close()
	if err != nil {
		return false, err
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents actor JOIN organizations ao ON ao.id=actor.org_id JOIN agents target ON target.id=$2 JOIN organizations torg ON torg.id=target.org_id WHERE actor.id=$1 AND actor.org_id=$3 AND target.org_id=$4 AND actor.status='active' AND target.status='active' AND ao.status='active' AND torg.status='active' AND NOT EXISTS(SELECT 1 FROM blocks WHERE (blocker_id=$1 AND blocked_id=$2) OR (blocker_id=$2 AND blocked_id=$1)))`, actor, target, actorOrg, targetOrg).Scan(&eligible)
	return eligible, err
}

func (s *Server) DirectGroup(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "groups:manage")
	if !ok {
		return
	}
	var p struct {
		AgentID    string `json:"agent_id"`
		PurposeKey string `json:"purpose_key"`
	}
	if !decode(w, r, &p) {
		return
	}
	if parsed, err := uuid.Parse(p.AgentID); err == nil {
		p.AgentID = parsed.String()
	}
	if !validID(p.AgentID) || p.AgentID == c.AgentID || strings.TrimSpace(p.PurposeKey) == "" || len(p.PurposeKey) > 128 {
		fail(w, 400, "bad_request", "Choose another agent and a bounded purpose_key.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	replay, ok := mutationReplay(w, r, tx, c, p, true)
	if !ok {
		return
	}
	low, high := c.AgentID, p.AgentID
	if low > high {
		low, high = high, low
	}
	// The constraint is authoritative; the lock also avoids speculative orphan groups.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, c.OrgID+":"+low+":"+high+":"+p.PurposeKey); err != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT group_id::text FROM group_direct_keys WHERE org_id=$1 AND agent_low=$2 AND agent_high=$3 AND purpose_key=$4`, c.OrgID, low, high, p.PurposeKey).Scan(&id)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	if exists {
		var locked string
		if err = tx.QueryRow(ctx, `SELECT id::text FROM groups WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
			fail(w, 409, "group_conflict", "Direct group unavailable.")
			return
		}
	}
	eligible, err := lockGroupEligibility(ctx, tx, c.AgentID, p.AgentID, c.OrgID, c.OrgID)
	if err != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	if !eligible {
		fail(w, 404, "not_found", "Agent unavailable.")
		return
	}
	if !exists {
		id = uuid.NewString()
		_, err = tx.Exec(ctx, `INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,$3,'private')`, id, c.OrgID, "direct-"+id)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO group_members(group_id,agent_id,role) VALUES($1,$2,'owner'),($1,$3,'member')`, id, c.AgentID, p.AgentID)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO group_direct_keys(org_id,agent_low,agent_high,purpose_key,group_id) VALUES($1,$2,$3,$4,$5)`, c.OrgID, low, high, p.PurposeKey, id)
		}
	} else {
		if lockGroupMember(r, tx, c.AgentID, id) != nil {
			fail(w, 409, "group_conflict", "Direct group unavailable.")
			return
		}
		var valid bool
		err = tx.QueryRow(ctx, `SELECT visibility='private' AND archived_at IS NULL AND (SELECT count(*) FROM group_members WHERE group_id=g.id AND state='accepted')=2 AND EXISTS(SELECT 1 FROM group_members WHERE group_id=g.id AND agent_id=$2 AND state='accepted') AND EXISTS(SELECT 1 FROM group_members WHERE group_id=g.id AND agent_id=$3 AND state='accepted') FROM groups g WHERE id=$1 FOR UPDATE`, id, low, high).Scan(&valid)
		if err == nil && !valid {
			fail(w, 409, "group_conflict", "Direct group unavailable.")
			return
		}
	}
	if err != nil {
		fail(w, 503, "unavailable", "Group unavailable.")
		return
	}
	if len(replay) == 0 {
		if err = recordGroupChange(ctx, tx, c.AgentID, id, "group.direct_resolved"); err != nil {
			fail(w, 503, "unavailable", "Group audit unavailable.")
			return
		}
	}
	result := map[string]string{"id": id}
	if len(replay) > 0 {
		var old struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(replay, &old)
		if old.ID != id {
			fail(w, 409, "group_conflict", "Direct group unavailable.")
			return
		}
	}
	if err = mutationSave(r, tx, c, p, result); err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, 503, "unavailable", "Group unavailable; retry with the same key.")
		return
	}
	writeJSON(w, 201, result)
}
