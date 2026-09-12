package goals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
	_ "time/tzdata"
)

type ReportSettings struct {
	Timezone string    `json:"timezone"`
	Hour     int       `json:"hour"`
	Enabled  bool      `json:"enabled"`
	NextDue  time.Time `json:"next_due"`
}
type Report struct {
	ID          string
	ScheduledAt time.Time
	CreatedAt   time.Time
	Goals       []Goal
}

// NextReport chooses a strictly future daily time. Missing local times run at
// the first real instant after the gap; repeated times use their first occurrence.
func NextReport(after time.Time, loc *time.Location, hour int) time.Time {
	local := after.In(loc)
	wallClock := func(t time.Time) time.Time {
		v := t.In(loc)
		return time.Date(v.Year(), v.Month(), v.Day(), v.Hour(), v.Minute(), v.Second(), v.Nanosecond(), time.UTC)
	}
	for day := 0; day < 3; day++ {
		date := time.Date(local.Year(), local.Month(), local.Day()+day, 12, 0, 0, 0, loc)
		candidate := time.Date(date.Year(), date.Month(), date.Day(), hour, 0, 0, 0, loc)
		target := time.Date(date.Year(), date.Month(), date.Day(), hour, 0, 0, 0, time.UTC)
		// Date may normalize a gap to either side, even into the previous day.
		// Compare complete wall times and retain sub-hour timezone transitions.
		for wallClock(candidate).Before(target) {
			candidate = candidate.Add(time.Second)
		}
		for previous := candidate.Add(-time.Second); !wallClock(previous).Before(target); previous = candidate.Add(-time.Second) {
			candidate = previous
		}
		if candidate.After(after) {
			return candidate.UTC()
		}
	}
	return after.Add(24 * time.Hour).UTC()
}
func (s Service) Settings(ctx context.Context, human, org string) (ReportSettings, error) {
	p := ReportSettings{Timezone: "UTC", Hour: 9, Enabled: true}
	if err := s.Owns(ctx, human, org); err != nil {
		return p, err
	}
	err := s.Pool.QueryRow(ctx, `SELECT timezone,hour,enabled,next_due FROM goal_report_settings WHERE org_id=$1`, org).Scan(&p.Timezone, &p.Hour, &p.Enabled, &p.NextDue)
	if errors.Is(err, pgx.ErrNoRows) {
		p.NextDue = NextReport(time.Now(), time.UTC, 9)
		return p, nil
	}
	return p, err
}
func (s Service) SaveSettings(ctx context.Context, human, org string, p ReportSettings) error {
	if err := s.Owns(ctx, human, org); err != nil {
		return err
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil || p.Hour < 0 || p.Hour > 23 {
		return ErrInvalid
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO goal_report_settings(org_id,timezone,hour,enabled,next_due) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id) DO UPDATE SET timezone=excluded.timezone,hour=excluded.hour,enabled=excluded.enabled,next_due=excluded.next_due`, org, p.Timezone, p.Hour, p.Enabled, NextReport(time.Now(), loc, p.Hour))
	return err
}
func (s Service) Reports(ctx context.Context, human, org string) ([]Report, error) {
	if err := s.Owns(ctx, human, org); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text,scheduled_at,created_at,snapshot FROM goal_reports WHERE org_id=$1 ORDER BY created_at DESC LIMIT 1`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		var r Report
		var b []byte
		if err = rows.Scan(&r.ID, &r.ScheduledAt, &r.CreatedAt, &b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &r.Goals); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GenerateReports runs independently of cleanup and domain renewal. Row locks
// and an atomic schedule advance make concurrent workers and crash retries safe.
func (s Service) GenerateReports(ctx context.Context, now time.Time) error {
	for n := 0; n < 20; n++ {
		done, err := s.generateReport(ctx, now)
		if err != nil {
			return fmt.Errorf("generating goal report: %w", err)
		}
		if done {
			return nil
		}
	}
	return nil
}
func (s Service) generateReport(ctx context.Context, now time.Time) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var org, zone string
	var hour int
	var due time.Time
	err = tx.QueryRow(ctx, `SELECT s.org_id::text,s.timezone,s.hour,s.next_due FROM goal_report_settings s JOIN organizations o ON o.id=s.org_id WHERE s.enabled AND s.next_due<=$1 AND o.status='active' ORDER BY s.next_due LIMIT 1 FOR UPDATE OF s SKIP LOCKED`, now).Scan(&org, &zone, &hour, &due)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM goals g JOIN agents a ON a.id=g.agent_id WHERE g.org_id=$1 AND (g.state NOT IN ('completed','cancelled') OR g.updated_at>$2::timestamptz-interval '24 hours') ORDER BY g.created_at DESC,g.id LIMIT 200`, org, now)
	if err != nil {
		return false, err
	}
	list := []Goal{}
	for rows.Next() {
		g, e := scan(rows)
		if e != nil {
			rows.Close()
			return false, e
		}
		list = append(list, g)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return false, err
	}
	// Store evidence, not model-generated conclusions. Coverage is bounded and
	// reports explicitly include their captured goal count in the interface.
	data, err := json.Marshal(list)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO goal_reports(org_id,scheduled_at,created_at,snapshot) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, org, due, now, data); err != nil {
		return false, err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE goal_report_settings SET next_due=$2 WHERE org_id=$1`, org, NextReport(now, loc, hour)); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

// ReportHistory exposes dates without loading every historical snapshot.
func (s Service) ReportHistory(ctx context.Context, human, org string) ([]Report, error) {
	if err := s.Owns(ctx, human, org); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id::text,scheduled_at,created_at FROM goal_reports WHERE org_id=$1 ORDER BY created_at DESC LIMIT 30`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		var r Report
		if err = rows.Scan(&r.ID, &r.ScheduledAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s Service) Report(ctx context.Context, human, org, id string) (Report, error) {
	if err := s.Owns(ctx, human, org); err != nil {
		return Report{}, err
	}
	var r Report
	var data []byte
	err := s.Pool.QueryRow(ctx, `SELECT id::text,scheduled_at,created_at,snapshot FROM goal_reports WHERE org_id=$1 AND id::text=$2`, org, id).Scan(&r.ID, &r.ScheduledAt, &r.CreatedAt, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(data, &r.Goals)
	return r, err
}
