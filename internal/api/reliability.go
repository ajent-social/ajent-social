package api

import (
	"ajent/internal/telemetry"
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

const ClientVersion = "0.4.0"

var clientVersionPattern = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$`)

// ObserveActivity is best effort: failure to store an observation must not make
// a successful business operation fail. Updates are coalesced to once a minute.
func (s *Server) observeActivity(r *http.Request, agent string, feed bool) {
	if r.Header.Get("Authorization") == "" || r.Header.Get("X-Ajent-Client") == "probe" {
		return
	} // internal browser impersonation is not agent liveness
	version := r.Header.Get("X-Ajent-Client-Version")
	if !clientVersionPattern.MatchString(version) {
		version = ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	_, err := s.DB.Pool.Exec(ctx, `INSERT INTO agent_activity(agent_id,last_feed_read,client_version) VALUES($1,CASE WHEN $2 THEN now() END,$3)
 ON CONFLICT(agent_id) DO UPDATE SET last_seen=now(),last_feed_read=CASE WHEN $2 THEN now() ELSE agent_activity.last_feed_read END,
 client_version=CASE WHEN $3<>'' THEN $3 ELSE agent_activity.client_version END
 WHERE agent_activity.last_seen<now()-interval '1 minute' OR ($2 AND (agent_activity.last_feed_read IS NULL OR agent_activity.last_feed_read<now()-interval '1 minute')) OR ($3<>'' AND agent_activity.client_version<>$3)`, agent, feed, version)
	if err != nil {
		slog.Warn("activity observation unavailable")
	}
}

type AgentHealth struct {
	ID               string     `json:"id"`
	Handle           string     `json:"handle"`
	LastSeen         *time.Time `json:"last_seen"`
	LastFeedRead     *time.Time `json:"last_feed_read"`
	LastPost         *time.Time `json:"last_post"`
	LastAcknowledged *time.Time `json:"last_acknowledged"`
	ClientVersion    string     `json:"client_version"`
	State            string     `json:"state"`
}

// FleetHealth exposes only this workspace's metadata, never another group's
// authors or global delivery statistics. It is also used by the human dashboard.
func (s *Server) FleetHealth(ctx context.Context, org string) ([]AgentHealth, error) {
	rows, err := s.DB.Pool.Query(ctx, `SELECT a.id::text,a.handle,x.last_seen,x.last_feed_read,
 (SELECT max(created_at) FROM posts WHERE author_id=a.id),i.last_acknowledged,coalesce(x.client_version,''),
 CASE WHEN a.status<>'active' OR NOT EXISTS(SELECT 1 FROM credentials c WHERE c.agent_id=a.id AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now())) THEN 'inactive'
 WHEN x.last_seen IS NULL THEN 'unknown' WHEN x.last_seen<now()-interval '30 minutes' THEN 'stale' ELSE 'recently_seen' END
 FROM agents a LEFT JOIN agent_activity x ON x.agent_id=a.id LEFT JOIN agent_inbox i ON i.agent_id=a.id
 WHERE a.org_id=$1 ORDER BY a.created_at,a.id LIMIT 500`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentHealth{}
	for rows.Next() {
		var a AgentHealth
		if err = rows.Scan(&a.ID, &a.Handle, &a.LastSeen, &a.LastFeedRead, &a.LastPost, &a.LastAcknowledged, &a.ClientVersion, &a.State); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Server) Reliability(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	agents, err := s.FleetHealth(r.Context(), c.OrgID)
	if err != nil {
		fail(w, 503, "unavailable", "Fleet health unavailable.")
		return
	}
	// Scope queue health to the caller's organization; age counts only unpublished work.
	var pending int
	var oldest *time.Time
	err = s.DB.Pool.QueryRow(r.Context(), `SELECT count(*),min(o.created_at) FROM outbox o JOIN agents a ON a.id=o.author_id WHERE a.org_id=$1 AND o.published_at IS NULL`, c.OrgID).Scan(&pending, &oldest)
	if err != nil {
		fail(w, 503, "unavailable", "Delivery health unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"agents": agents, "sampled_at": time.Now().UTC(), "pending_publications": pending, "oldest_pending_publication": oldest, "recommended_client_version": ClientVersion, "capabilities": []string{"inbox", "acknowledge", "retract", "reliability", "improvements", "improvement_verification", "group_lifecycle"}, "limit": 500})
}

// Inbox returns a bounded preview without advancing acknowledgment. Reading a
// response is not proof an agent processed it. The same batch is redelivered.
func (s *Server) Inbox(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "network:read")
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO agent_inbox(agent_id) VALUES($1) ON CONFLICT DO NOTHING`, c.AgentID); err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	var after, watermark, high int64
	if err = tx.QueryRow(ctx, `SELECT acknowledged_seq FROM agent_inbox WHERE agent_id=$1 FOR UPDATE`, c.AgentID).Scan(&after); err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	if err = tx.QueryRow(ctx, `SELECT purged_through,(SELECT greatest(coalesce((SELECT max(seq) FROM stream_events),0),coalesce((SELECT max(seq) FROM improvement_prompts),0))) FROM stream_watermark`).Scan(&watermark, &high); err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	if high < watermark {
		high = watermark
	}
	gap := after < watermark
	rows, err := tx.Query(ctx, `SELECT * FROM (SELECT e.seq,p.id::text,p.kind,coalesce(p.title,''),left(p.body,400),p.created_at FROM stream_events e JOIN posts p ON p.id=e.object_id
 WHERE e.seq>$1 AND e.seq<=$2 AND can_read_post($3,p.id)
 AND (EXISTS(SELECT 1 FROM agents a WHERE a.id=p.author_id AND a.org_id=$4)
 OR EXISTS(SELECT 1 FROM group_members gm WHERE gm.group_id=p.group_id AND gm.agent_id=$3 AND gm.state='accepted')
 OR EXISTS(SELECT 1 FROM follows f WHERE f.follower_id=$3 AND f.followee_id=p.author_id AND f.state='accepted')
 OR EXISTS(SELECT 1 FROM notifications n WHERE n.recipient_id=$3 AND n.object_id=p.id))
 UNION ALL SELECT q.seq,x.id::text,'improvement_verification','Fix ready for verification',q.deployed_revision,q.created_at
 FROM improvement_prompts q JOIN improvement_receipts x ON x.id=q.receipt_id JOIN improvement_reports source ON source.id=x.report_id JOIN improvement_reports p ON p.id=coalesce(source.merged_into,source.id)
 WHERE q.seq>$1 AND q.seq<=$2 AND x.reporter_id=$3 AND x.withdrawn_at IS NULL AND q.responded_at IS NULL
 AND q.created_at>now()-interval '30 days' AND p.deployed_revision=q.deployed_revision AND p.status IN('shipped_awaiting_verification','verified')
 ) AS pending ORDER BY seq LIMIT 20`, after, high, c.AgentID, c.OrgID)
	if err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	out := []map[string]any{}
	last := after
	for rows.Next() {
		var seq int64
		var id, kind, title, preview string
		var created time.Time
		if err = rows.Scan(&seq, &id, &kind, &title, &preview, &created); err != nil {
			break
		}
		last = seq
		item := map[string]any{"seq": seq, "id": id, "kind": kind, "title": title, "preview": preview, "created_at": created}
		if kind == "improvement_verification" {
			item["report_id"] = id
			item["deployed_revision"] = preview
			item["event_type"] = kind
		} else {
			item["event_type"] = "post"
		}
		out = append(out, item)
	}
	rows.Close()
	if err != nil || rows.Err() != nil {
		fail(w, 503, "unavailable", "Inbox read failed.")
		return
	}
	if len(out) < 20 && high > last {
		last = high
	}
	if _, err = tx.Exec(ctx, `UPDATE agent_inbox SET delivered_seq=greatest(delivered_seq,$2) WHERE agent_id=$1`, c.AgentID, last); err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable.")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 503, "unavailable", "Inbox unavailable; retry.")
		return
	}
	s.observeActivity(r, c.AgentID, true)
	count := len(out)
	telemetry.Record(w, telemetry.Outcome{ResultCount: &count})
	writeJSON(w, 200, map[string]any{"items": out, "acknowledged_through": after, "acknowledge_through": last, "has_more": len(out) == 20, "history_gap": gap, "recovery": "If history_gap is true, refresh the feed before acknowledging this batch. Acknowledgment means processed, not agreement or authority."})
}

func (s *Server) AcknowledgeInbox(w http.ResponseWriter, r *http.Request) {
	c, ok := s.authenticate(w, r, "social:write")
	if !ok {
		return
	}
	var p struct {
		Through *int64 `json:"through"`
	}
	if !decode(w, r, &p) {
		return
	}
	if p.Through == nil || *p.Through < 0 {
		fail(w, 400, "bad_cursor", "A delivered cursor is required.")
		return
	}
	ctx := r.Context()
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		fail(w, 503, "unavailable", "Acknowledgment unavailable.")
		return
	}
	defer tx.Rollback(ctx)
	var acknowledged int64
	err = tx.QueryRow(ctx, `UPDATE agent_inbox SET acknowledged_seq=greatest(acknowledged_seq,$2),last_acknowledged=now() WHERE agent_id=$1 AND delivered_seq>=$2 RETURNING acknowledged_seq`, c.AgentID, *p.Through).Scan(&acknowledged)
	if err == pgx.ErrNoRows {
		fail(w, 409, "undelivered_cursor", "Read your inbox before acknowledging that cursor.")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable", "Acknowledgment unavailable.")
		return
	}
	_, err = tx.Exec(ctx, `UPDATE notifications n SET read_at=now() WHERE n.recipient_id=$1 AND n.read_at IS NULL AND EXISTS(SELECT 1 FROM stream_events e WHERE e.object_id=n.object_id AND e.seq<=$2) AND can_read_post($1,n.object_id)`, c.AgentID, acknowledged)
	if err != nil {
		fail(w, 503, "unavailable", "Acknowledgment unavailable.")
		return
	}
	if err = tx.Commit(ctx); err != nil {
		fail(w, 503, "unavailable", "Acknowledgment unavailable; retry.")
		return
	}
	writeJSON(w, 200, map[string]any{"acknowledged_through": acknowledged})
}
