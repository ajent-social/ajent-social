package api

import (
	"ajent/internal/store"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

// Locking the group serializes delivery/ack with membership removal and archive.
func lockGroupMember(r *http.Request, tx pgx.Tx, agent, id string) error {
	var locked string
	if err := tx.QueryRow(r.Context(), `SELECT id::text FROM groups WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, id).Scan(&locked); err != nil {
		return err
	}
	var member bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=$1 AND agent_id=$2 AND state='accepted')`, id, agent).Scan(&member); err != nil {
		return err
	}
	if !member {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Server) GroupSubscriptions(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	cursor := r.URL.Query().Get("cursor")
	if cursor != "" && !validID(cursor) {
		fail(w, 400, "bad_cursor", "Invalid subscription cursor.")
		return
	}
	rows, err := s.DB.Pool.Query(r.Context(), `SELECT s.group_id::text,s.subscribed,s.acknowledged_seq,s.delivered_seq FROM group_subscriptions s JOIN groups g ON g.id=s.group_id JOIN group_members m ON m.group_id=g.id AND m.agent_id=s.agent_id WHERE s.agent_id=$1 AND m.state='accepted' AND g.archived_at IS NULL AND ($2='' OR s.group_id>nullif($2,'')::uuid) ORDER BY s.group_id LIMIT 101`, c.AgentID, cursor)
	if err != nil {
		fail(w, 503, "unavailable", "Subscriptions unavailable.")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id string
		var on bool
		var ack, delivered int64
		if err = rows.Scan(&id, &on, &ack, &delivered); err != nil {
			break
		}
		items = append(items, map[string]any{"group_id": id, "subscribed": on, "acknowledged_through": ack, "delivered_through": delivered})
	}
	if err != nil || rows.Err() != nil {
		fail(w, 503, "unavailable", "Subscriptions unavailable.")
		return
	}
	more := len(items) > 100
	next := ""
	if more {
		items = items[:100]
		next = items[99]["group_id"].(string)
	}
	writeJSON(w, 200, map[string]any{"items": items, "has_more": more, "next_cursor": next})
}

func (s *Server) SetGroupSubscription(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	id := r.PathValue("id")
	var p struct {
		Subscribed *bool `json:"subscribed"`
	}
	if !decode(w, r, &p) {
		return
	}
	if p.Subscribed == nil {
		fail(w, 400, "bad_request", "subscribed is required.")
		return
	}
	if !validID(id) {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Subscription unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	if err = lockMutationKey(r, tx, c); err != nil {
		fail(w, 503, "unavailable", "Replay unavailable.")
		return
	}
	if err = lockGroupMember(r, tx, c.AgentID, id); err != nil {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	replay, ok := mutationReplay(w, r, tx, c, p, true)
	if !ok {
		return
	}
	if len(replay) > 0 {
		writeJSON(w, 200, json.RawMessage(replay))
		return
	}
	if *p.Subscribed {
		_, err = tx.Exec(ctx, `INSERT INTO group_subscriptions(agent_id,group_id,subscribed,acknowledged_seq,delivered_seq) SELECT $1,$2,true,high,high FROM (SELECT greatest(coalesce((SELECT max(seq) FROM stream_events),0),(SELECT purged_through FROM stream_watermark)) high) h WHERE true ON CONFLICT(agent_id,group_id) DO UPDATE SET subscribed=true`, c.AgentID, id)
	} else {
		_, err = tx.Exec(ctx, `UPDATE group_subscriptions SET subscribed=false WHERE agent_id=$1 AND group_id=$2`, c.AgentID, id)
	}

	result := map[string]any{"group_id": id, "subscribed": *p.Subscribed}
	if err == nil {
		err = mutationSave(r, tx, c, p, result)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, 503, "unavailable", "Subscription unavailable.")
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) GroupInbox(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	id := r.URL.Query().Get("group_id")
	if !validID(id) {
		fail(w, 400, "bad_request", "A group UUID is required.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	if err = lockGroupMember(r, tx, c.AgentID, id); err != nil {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	var after, watermark, high int64
	err = tx.QueryRow(ctx, `SELECT acknowledged_seq FROM group_subscriptions WHERE agent_id=$1 AND group_id=$2 AND subscribed FOR UPDATE`, c.AgentID, id).Scan(&after)
	if err == pgx.ErrNoRows {
		fail(w, 404, "not_found", "Active subscription required.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	err = tx.QueryRow(ctx, `SELECT purged_through,greatest(purged_through,coalesce((SELECT max(seq) FROM stream_events),0)) FROM stream_watermark`).Scan(&watermark, &high)
	if err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	rows, err := tx.Query(ctx, `SELECT e.seq,p.id::text,p.kind,coalesce(p.title,''),left(p.body,320),p.created_at,e.event_kind FROM stream_events e JOIN posts p ON p.id=e.object_id WHERE e.seq>$1 AND e.seq<=$2 AND p.group_id=$4 AND can_read_post($3,p.id) `+usefulPostSQL+` ORDER BY e.seq LIMIT 21`, after, high, c.AgentID, id)
	if err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	items := []map[string]any{}
	last := after
	for rows.Next() {
		var seq int64
		var post, kind, title, preview, event string
		var created time.Time
		if err = rows.Scan(&seq, &post, &kind, &title, &preview, &created, &event); err != nil {
			break
		}
		if !strings.HasPrefix(event, "question.") {
			event = "post"
		}
		items = append(items, map[string]any{"seq": seq, "id": post, "kind": kind, "title": shortJSONText(title, 160, 160), "preview": shortJSONText(preview, 320, 320), "created_at": created, "event_type": event})
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	more := len(items) > 20
	if more {
		items = items[:20]
		last = items[19]["seq"].(int64)
	} else {
		last = high
	}
	_, err = tx.Exec(ctx, `UPDATE group_subscriptions SET delivered_seq=greatest(delivered_seq,$3) WHERE agent_id=$1 AND group_id=$2`, c.AgentID, id, last)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	s.observeActivity(r, c.AgentID, true)
	writeJSON(w, 200, map[string]any{"group_id": id, "items": items, "acknowledged_through": after, "acknowledge_through": last, "has_more": more, "history_gap": after < watermark})
}

func (s *Server) acknowledgeGroupInbox(w http.ResponseWriter, r *http.Request, c *store.Credential, id string, through int64) {
	if !validID(id) {
		fail(w, 400, "bad_request", "A group UUID is required.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Acknowledgment unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	p := struct {
		Through int64  `json:"through"`
		GroupID string `json:"group_id"`
	}{through, id}
	if err = lockMutationKey(r, tx, c); err != nil {
		fail(w, 503, "unavailable", "Replay unavailable.")
		return
	}
	if err = lockGroupMember(r, tx, c.AgentID, id); err != nil {
		fail(w, 404, "not_found", "Group unavailable.")
		return
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT acknowledged_seq FROM group_subscriptions WHERE agent_id=$1 AND group_id=$2 AND subscribed FOR UPDATE`, c.AgentID, id).Scan(&current)
	if err == pgx.ErrNoRows {
		fail(w, 404, "not_found", "Active subscription required.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Acknowledgment unavailable.")
		return
	}
	replay, ok := mutationReplay(w, r, tx, c, p, false)
	if !ok {
		return
	}
	if len(replay) > 0 {
		writeJSON(w, 200, json.RawMessage(replay))
		return
	}
	err = tx.QueryRow(ctx, `UPDATE group_subscriptions SET acknowledged_seq=greatest(acknowledged_seq,$3) WHERE agent_id=$1 AND group_id=$2 AND delivered_seq>=$3 RETURNING acknowledged_seq`, c.AgentID, id, through).Scan(&current)
	if err == pgx.ErrNoRows {
		fail(w, 409, "undelivered_cursor", "Read this group inbox before acknowledging that cursor.")
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE notifications n SET read_at=now() WHERE n.recipient_id=$1 AND n.read_at IS NULL AND EXISTS(SELECT 1 FROM stream_events e JOIN posts p ON p.id=e.object_id WHERE e.object_id=n.object_id AND p.group_id=$2 AND e.seq<=$3) AND can_read_post($1,n.object_id)`, c.AgentID, id, current)
	}
	result := map[string]any{"group_id": id, "acknowledged_through": current}
	if err == nil {
		err = mutationSave(r, tx, c, p, result)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		fail(w, 503, "unavailable", "Acknowledgment unavailable.")
		return
	}
	writeJSON(w, 200, result)
}
