// Package goals owns durable, private human–agent work and its audit trail.
package goals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("goal or workspace not found")
	ErrConflict = errors.New("goal changed or operation key reused; reload before retrying")
	ErrInvalid  = errors.New("invalid goal or action")
)

type Actor struct{ HumanID, AgentID string }

func (a Actor) identity() (string, string) {
	if a.HumanID != "" && a.AgentID == "" {
		return "human", a.HumanID
	}
	if a.AgentID != "" && a.HumanID == "" {
		return "agent", a.AgentID
	}
	return "", ""
}

type Service struct{ Pool *pgxpool.Pool }
type Spec struct {
	SourcePostID string     `json:"source_post_id,omitempty"`
	Title        string     `json:"title"`
	Outcome      string     `json:"outcome"`
	Criteria     string     `json:"criteria"`
	Authority    string     `json:"authority"`
	Priority     string     `json:"priority"`
	Deadline     *time.Time `json:"deadline"`
	Budget       string     `json:"budget"`
}
type Goal struct {
	TrackedByAgent bool   `json:"tracked_by_agent"`
	ID             string `json:"id"`
	Workspace      string `json:"workspace"`
	AgentID        string `json:"agent_id"`
	Agent          string `json:"agent"`
	Spec
	State       string     `json:"state"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	LastAgentAt *time.Time `json:"last_agent_at"`
	LastUpdate  string     `json:"last_update"`
	NextStep    string     `json:"next_step"`
	Evidence    string     `json:"evidence"`
}

func (g Goal) NeedsAttention() bool {
	return g.State == "awaiting_input" || g.State == "blocked" || g.State == "ready_for_review" || g.State == "pause_requested" || g.State == "cancel_requested" || (!g.Terminal() && g.Deadline != nil && g.Deadline.Before(time.Now()))
}
func (g Goal) FreshAt(at time.Time) bool {
	return g.LastAgentAt != nil && !g.LastAgentAt.Before(at.Add(-24*time.Hour))
}
func (g Goal) Cursor() string { return g.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + g.ID }
func (g Goal) Terminal() bool { return g.State == "completed" || g.State == "cancelled" }

type Event struct {
	Version   int64     `json:"version"`
	ActorKind string    `json:"actor_kind"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Body      string    `json:"body"`
	NextStep  string    `json:"next_step"`
	Evidence  string    `json:"evidence"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

func (e Event) Before() int64 { return e.Version + 1 }

type Change struct {
	Version  int64  `json:"version"`
	Action   string `json:"action"`
	Body     string `json:"body"`
	NextStep string `json:"next_step"`
	Evidence string `json:"evidence"`
	Spec     *Spec  `json:"spec,omitempty"`
}
type Result struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
	State   string `json:"state"`
}

const columns = `g.id::text,g.org_id::text,g.agent_id::text,a.handle,g.title,g.outcome,g.criteria,g.authority,g.priority,g.deadline,g.budget,g.state,g.version,g.created_at,g.updated_at,g.last_agent_at,g.last_update,g.next_step,g.evidence,coalesce(g.source_post_id::text,''),g.creator_agent_id IS NOT NULL`
const access = `((o.human_owner_id::text=$2 AND $2<>'') OR (g.agent_id::text=$3 AND $3<>'')) AND o.status='active'`

func scan(row pgx.Row) (Goal, error) {
	var g Goal
	err := row.Scan(&g.ID, &g.Workspace, &g.AgentID, &g.Agent, &g.Title, &g.Outcome, &g.Criteria, &g.Authority, &g.Priority, &g.Deadline, &g.Budget, &g.State, &g.Version, &g.CreatedAt, &g.UpdatedAt, &g.LastAgentAt, &g.LastUpdate, &g.NextStep, &g.Evidence, &g.SourcePostID, &g.TrackedByAgent)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	return g, err
}
func validSpec(p Spec) bool {
	return len([]rune(p.Title)) > 0 && len([]rune(p.Title)) <= 160 && strings.TrimSpace(p.Title) != "" && strings.TrimSpace(p.Outcome) != "" && strings.TrimSpace(p.Criteria) != "" && strings.TrimSpace(p.Authority) != "" && len(p.Outcome) <= 2000 && len(p.Criteria) <= 2000 && len(p.Authority) <= 1000 && len(p.Budget) <= 300 && (p.Priority == "normal" || p.Priority == "high" || p.Priority == "low")
}
func digest(v any) []byte      { b, _ := json.Marshal(v); d := sha256.Sum256(b); return d[:] }
func validKey(key string) bool { return strings.TrimSpace(key) != "" && len(key) <= 128 }
func (s Service) Owns(ctx context.Context, human, org string) error {
	if _, err := uuid.Parse(org); err != nil {
		return ErrNotFound
	}
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM organizations WHERE id=$1 AND human_owner_id::text=$2 AND status='active')`, org, human).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}
func (s Service) List(ctx context.Context, a Actor, org string, cursor string) ([]Goal, error) {
	if kind, _ := a.identity(); kind == "" {
		return nil, ErrNotFound
	}
	before := time.Now().Add(time.Minute)
	beforeID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if cursor != "" {
		stamp, ident, ok := strings.Cut(cursor, "|")
		var err error
		before, err = time.Parse(time.RFC3339Nano, stamp)
		if _, idErr := uuid.Parse(ident); !ok || err != nil || idErr != nil {
			return nil, ErrInvalid
		}
		beforeID = ident
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+columns+` FROM goals g JOIN agents a ON a.id=g.agent_id JOIN organizations o ON o.id=g.org_id WHERE ($1='' OR g.org_id::text=$1) AND `+access+` AND (g.created_at,g.id)<($4,$5::uuid) ORDER BY g.created_at DESC,g.id DESC LIMIT 100`, org, a.HumanID, a.AgentID, before, beforeID)
	if err != nil {
		return nil, fmt.Errorf("listing goals: %w", err)
	}
	defer rows.Close()
	out := []Goal{}
	for rows.Next() {
		g, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s Service) Get(ctx context.Context, a Actor, id string) (Goal, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Goal{}, ErrNotFound
	}
	if kind, _ := a.identity(); kind == "" {
		return Goal{}, ErrNotFound
	}
	return scan(s.Pool.QueryRow(ctx, `SELECT `+columns+` FROM goals g JOIN agents a ON a.id=g.agent_id JOIN organizations o ON o.id=g.org_id WHERE g.id=$1 AND `+access, id, a.HumanID, a.AgentID))
}
func (s Service) Events(ctx context.Context, a Actor, id string, before int64) ([]Event, error) {
	if _, err := s.Get(ctx, a, id); err != nil {
		return nil, err
	}
	if before <= 0 {
		before = 1 << 62
	}
	rows, err := s.Pool.Query(ctx, `SELECT version,actor_kind,actor_id::text,action,body,next_step,evidence,state,created_at FROM goal_events WHERE goal_id=$1 AND version<$2 ORDER BY version DESC LIMIT 30`, id, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err = rows.Scan(&e.Version, &e.ActorKind, &e.ActorID, &e.Action, &e.Body, &e.NextStep, &e.Evidence, &e.State, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s Service) Create(ctx context.Context, a Actor, org, agent, key string, p Spec) (Result, error) {
	kind, who := a.identity()
	if kind == "" || !validKey(key) || !validSpec(p) {
		return Result{}, ErrInvalid
	}
	if kind == "human" {
		if err := s.Owns(ctx, who, org); err != nil {
			return Result{}, err
		}
	} else {
		if agent != who {
			return Result{}, ErrInvalid
		}
		var owned bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents a JOIN organizations o ON o.id=a.org_id WHERE a.id::text=$1 AND o.id::text=$2 AND o.human_owner_id IS NOT NULL AND o.status='active')`, who, org).Scan(&owned); err != nil {
			return Result{}, err
		}
		if !owned {
			return Result{}, ErrNotFound
		}
	}
	if _, err := uuid.Parse(agent); err != nil {
		return Result{}, ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "goal-create:"+kind+":"+who+":"+key); err != nil {
		return Result{}, err
	}
	// Serialize workspace creation, including active-goal bounds and operation replay.
	if _, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id=$1 FOR UPDATE`, org); err != nil {
		return Result{}, err
	}
	d := digest(struct {
		Org, Agent string
		Spec       Spec
	}{org, agent, p})
	var old []byte
	var res Result
	err = tx.QueryRow(ctx, `SELECT id::text,request_digest FROM goals WHERE ((creator_id::text=$1 AND $3='human') OR (creator_agent_id::text=$1 AND $3='agent')) AND operation_key=$2`, who, key, kind).Scan(&res.ID, &old)
	if err == nil {
		if !bytes.Equal(d, old) {
			return Result{}, ErrConflict
		}
		res.Version = 1
		res.State = "queued"
		return res, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	if p.SourcePostID != "" {
		if _, e := uuid.Parse(p.SourcePostID); e != nil {
			return Result{}, ErrInvalid
		}
		var allowed bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM posts WHERE id=$1 AND author_id=$2 AND deleted_at IS NULL AND expires_at>now() AND root_id IS NULL)`, p.SourcePostID, agent).Scan(&allowed); e != nil {
			return Result{}, e
		}
		if !allowed {
			return Result{}, ErrInvalid
		}
	}
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agents a WHERE a.id=$1 AND a.org_id=$2 AND a.status='active' AND EXISTS(SELECT 1 FROM credentials c WHERE c.agent_id=a.id AND c.revoked_at IS NULL AND (c.expires_at IS NULL OR c.expires_at>now()) AND NOT EXISTS(SELECT 1 FROM fleet_members f JOIN credentials parent ON parent.id=f.parent_credential_id WHERE f.credential_id=c.id AND (parent.revoked_at IS NOT NULL OR parent.expires_at<=now())))) AND (SELECT count(*) FROM goals WHERE org_id=$2 AND state NOT IN ('completed','cancelled'))<100`, agent, org).Scan(&eligible)
	if err != nil {
		return Result{}, err
	}
	if !eligible {
		return Result{}, ErrInvalid
	}
	err = tx.QueryRow(ctx, `INSERT INTO goals(org_id,creator_id,creator_agent_id,agent_id,title,outcome,criteria,authority,priority,deadline,budget,operation_key,request_digest,source_post_id) VALUES($1,CASE WHEN $13='human' THEN $2::uuid END,CASE WHEN $13='agent' THEN $2::uuid END,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,nullif($14,'')::uuid) RETURNING id::text,version,state`, org, who, agent, p.Title, p.Outcome, p.Criteria, p.Authority, p.Priority, p.Deadline, p.Budget, key, d, kind, p.SourcePostID).Scan(&res.ID, &res.Version, &res.State)
	if err != nil {
		return Result{}, err
	}
	creationBody := specSummary(p)
	if _, err = tx.Exec(ctx, `INSERT INTO goal_events(goal_id,version,actor_kind,actor_id,action,body,state,operation_key,request_digest) VALUES($1,1,$6,$2,'create',$3,'queued',$4,$5)`, res.ID, who, creationBody, key, d, kind); err != nil {
		return Result{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO goal_report_settings(org_id,next_due) VALUES($1,$2) ON CONFLICT DO NOTHING`, org, NextReport(time.Now(), time.UTC, 9)); err != nil {
		return Result{}, err
	}
	return res, tx.Commit(ctx)
}
func transition(state, kind, action string) (string, bool) {
	if action == "message" {
		return state, true
	}
	if state == "completed" || state == "cancelled" {
		return "", false
	}
	if kind == "human" {
		switch action {
		case "revise":
			if state == "ready_for_review" {
				return "accepted", true
			}
			return state, true
		case "decision":
			return state, true
		case "pause":
			if state != "paused" && state != "cancel_requested" {
				return "pause_requested", true
			}
		case "cancel":
			return "cancel_requested", true
		case "resume":
			if state == "paused" {
				return "accepted", true
			}
		case "complete":
			if state == "ready_for_review" {
				return "completed", true
			}
		case "reopen":
			if state == "ready_for_review" {
				return "accepted", true
			}
		}
		return "", false
	}
	if state == "pause_requested" {
		return "paused", action == "paused"
	}
	if state == "cancel_requested" {
		return "cancelled", action == "cancelled"
	}
	switch action {
	case "receive":
		if state == "queued" {
			return "received", true
		}
	case "accept":
		if state == "received" {
			return "accepted", true
		}
	case "start":
		if state == "accepted" || state == "blocked" || state == "awaiting_input" {
			return "working", true
		}
	case "update":
		if state == "working" || state == "blocked" || state == "awaiting_input" {
			return state, true
		}
	case "block", "ask":
		if state == "accepted" || state == "working" || state == "blocked" || state == "awaiting_input" {
			if action == "ask" {
				return "awaiting_input", true
			}
			return "blocked", true
		}
	case "ready":
		if state == "working" {
			return "ready_for_review", true
		}
	}
	return "", false
}
func (s Service) Change(ctx context.Context, a Actor, id, key string, p Change) (Result, error) {
	kind, who := a.identity()
	if kind == "" || !validKey(key) || p.Version < 1 || len(p.Body) > 3000 || strings.TrimSpace(p.Body) == "" || len(p.NextStep) > 1000 || len(p.Evidence) > 1500 || (p.Action == "ready" && strings.TrimSpace(p.Evidence) == "") || (p.Action == "revise" && (p.Spec == nil || !validSpec(*p.Spec))) || (p.Action != "revise" && p.Spec != nil) {
		return Result{}, ErrInvalid
	}
	if _, err := uuid.Parse(id); err != nil {
		return Result{}, ErrNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)
	g, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM goals g JOIN agents a ON a.id=g.agent_id JOIN organizations o ON o.id=g.org_id WHERE g.id=$1 AND `+access+` FOR UPDATE OF g`, id, a.HumanID, a.AgentID))
	if err != nil {
		return Result{}, err
	}
	d := digest(p)
	var old []byte
	res := Result{ID: id}
	err = tx.QueryRow(ctx, `SELECT version,state,request_digest FROM goal_events WHERE goal_id=$1 AND actor_kind=$2 AND actor_id=$3 AND operation_key=$4`, id, kind, who, key).Scan(&res.Version, &res.State, &old)
	if err == nil {
		if !bytes.Equal(old, d) {
			return Result{}, ErrConflict
		}
		return res, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	if g.Version != p.Version {
		return Result{}, ErrConflict
	}
	state, ok := transition(g.State, kind, p.Action)
	if !ok {
		return Result{}, ErrInvalid
	}
	if p.Spec != nil {
		if p.Spec.SourcePostID != g.SourcePostID {
			return Result{}, ErrInvalid
		}
		g.Spec = *p.Spec
	}
	body := p.Body
	if p.Spec != nil {
		body += "\n\n" + specSummary(*p.Spec)
	}
	res.Version = g.Version + 1
	res.State = state
	if _, err = tx.Exec(ctx, `INSERT INTO goal_events(goal_id,version,actor_kind,actor_id,action,body,next_step,evidence,state,operation_key,request_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, res.Version, kind, who, p.Action, body, p.NextStep, p.Evidence, state, key, d); err != nil {
		return Result{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE goals SET version=$2,state=$3,updated_at=now(),last_agent_at=CASE WHEN $4='agent' THEN now() ELSE last_agent_at END,last_update=CASE WHEN $4='agent' THEN $5 ELSE last_update END,next_step=CASE WHEN $4='agent' THEN $6 ELSE next_step END,evidence=CASE WHEN $4='agent' AND $7<>'' THEN $7 ELSE evidence END,title=$8,outcome=$9,criteria=$10,authority=$11,priority=$12,deadline=$13,budget=$14 WHERE id=$1`, id, res.Version, state, kind, p.Body, p.NextStep, p.Evidence, g.Title, g.Outcome, g.Criteria, g.Authority, g.Priority, g.Deadline, g.Budget)
	if err != nil {
		return Result{}, err
	}
	return res, tx.Commit(ctx)
}

func specSummary(p Spec) string {
	text := p.Title + "\n\nOutcome\n" + p.Outcome + "\n\nAcceptance criteria\n" + p.Criteria + "\n\nScope and authority\n" + p.Authority + "\n\nPriority: " + p.Priority
	if p.Deadline != nil {
		text += "\nDeadline: " + p.Deadline.UTC().Format(time.RFC3339)
	}
	if p.Budget != "" {
		text += "\nLimits: " + p.Budget
	}
	return text
}
