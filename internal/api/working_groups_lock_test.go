package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Observe the actual database lock dependency, not elapsed time or a guessed
// goroutine schedule. These tests are serial and require a disposable database.
func wgAwaitBlocked(t *testing.T, f *fixture, ctx context.Context, blocker uint32) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := f.s.DB.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, blocker).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("request did not reach the lock barrier: ", ctx.Err())
		case <-ticker.C:
		}
	}
}

func wgAsyncRequest(ctx context.Context, f *fixture, path, body, key string) <-chan *httptest.ResponseRecorder {
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		r := httptest.NewRequest("POST", path, strings.NewReader(body)).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+f.ta)
		r.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		f.m.ServeHTTP(w, r)
		done <- w
	}()
	return done
}

func wgWaitResponse(t *testing.T, ctx context.Context, done <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case w := <-done:
		return w
	case <-ctx.Done():
		t.Fatal("request failed to finish: ", ctx.Err())
		return nil
	}
}

func wgEligibilityChange(t *testing.T, f *fixture, ctx context.Context, change string) {
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.Pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Helper()
	switch change {
	case "block":
		exec(`INSERT INTO blocks(blocker_id,blocked_id) VALUES($1,$2)`, f.a, f.b)
	case "reverse_block":
		exec(`INSERT INTO blocks(blocker_id,blocked_id) VALUES($1,$2)`, f.b, f.a)
	case "target_disabled":
		exec(`UPDATE agents SET status='disabled' WHERE id=$1`, f.b)
	case "actor_disabled":
		exec(`UPDATE agents SET status='disabled' WHERE id=$1`, f.a)
	case "target_rehome", "actor_rehome":
		org := uuid.NewString()
		exec(`INSERT INTO organizations(id,name,namespace) VALUES($1,'Other',$2)`, org, "other-"+org)
		agent := f.b
		if change == "actor_rehome" {
			agent = f.a
		}
		exec(`UPDATE agents SET org_id=$1 WHERE id=$2`, org, agent)
	case "organization_disabled":
		exec(`UPDATE organizations SET status='disabled' WHERE id=$1`, f.org)
	default:
		t.Fatal("unknown eligibility change")
	}
}

func TestWorkingGroupsDirectReauthorizesAfterSerialization(t *testing.T) {
	for _, barrierKind := range []string{"key", "pair"} {
		for _, phase := range []string{"create", "reuse", "replay"} {
			for _, change := range []string{"block", "reverse_block", "target_disabled", "actor_disabled", "target_rehome", "actor_rehome", "organization_disabled"} {
				t.Run(barrierKind+"/"+phase+"/"+change, func(t *testing.T) {
					f := setup(t)
					wgSQL(t, f, `UPDATE agents SET org_id=$1 WHERE id=$2`, f.org, f.b)
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					body := fmt.Sprintf(`{"agent_id":%q,"purpose_key":"lock-review"}`, f.b)
					key := uuid.NewString()
					existing := ""
					if phase != "create" {
						w := f.request("POST", "/v1/groups/direct", f.ta, body, key)
						if w.Code != 201 {
							t.Fatalf("baseline %d %s", w.Code, w.Body)
						}
						existing = wgID(t, w.Body.Bytes())
						if phase == "reuse" {
							key = uuid.NewString()
						}
					}
					conn, err := f.s.DB.Pool.Acquire(ctx)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Release()
					barrier, err := conn.Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					defer barrier.Rollback(context.Background())
					var credential string
					if err = barrier.QueryRow(ctx, `SELECT id::text FROM credentials WHERE agent_id=$1`, f.a).Scan(&credential); err != nil {
						t.Fatal(err)
					}
					lockName := credential + ":" + key
					if barrierKind == "pair" {
						low, high := f.a, f.b
						if low > high {
							low, high = high, low
						}
						lockName = f.org + ":" + low + ":" + high + ":lock-review"
					}
					if _, err = barrier.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, lockName); err != nil {
						t.Fatal(err)
					}
					done := wgAsyncRequest(ctx, f, "/v1/groups/direct", body, key)
					wgAwaitBlocked(t, f, ctx, conn.Conn().PgConn().PID())
					// Authentication and the old early eligibility read have finished. The
					// changes commit while the handler is demonstrably waiting on serialization.
					wgEligibilityChange(t, f, ctx, change)
					if err = barrier.Commit(ctx); err != nil {
						t.Fatal(err)
					}
					w := wgWaitResponse(t, ctx, done)
					if w.Code != 404 {
						t.Fatalf("stale authorization accepted %s/%s: %d %s", phase, change, w.Code, w.Body)
					}
					if existing != "" && strings.Contains(w.Body.String(), existing) {
						t.Fatal("unauthorized group metadata leaked")
					}
					var count int
					if err = f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM group_direct_keys WHERE org_id=$1 AND purpose_key='lock-review'`, f.org).Scan(&count); err != nil {
						t.Fatal(err)
					}
					want := 0
					if phase != "create" {
						want = 1
					}
					if count != want {
						t.Fatalf("unauthorized group created: %d want %d", count, want)
					}
				})
			}
		}
	}
}

func TestWorkingGroupsInvitationReauthorizesAfterSerialization(t *testing.T) {
	for _, phase := range []string{"add", "replay"} {
		for _, change := range []string{"block", "reverse_block", "target_disabled", "actor_disabled", "target_rehome", "actor_rehome", "organization_disabled"} {
			t.Run(phase+"/"+change, func(t *testing.T) {
				f := setup(t)
				wgSQL(t, f, `UPDATE agents SET org_id=$1 WHERE id=$2`, f.org, f.b)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				key := uuid.NewString()
				path := "/v1/groups/" + f.g + "/members/" + f.b
				if phase == "replay" {
					w := f.request("POST", path, f.ta, `{"action":"add"}`, key)
					if w.Code != 200 {
						t.Fatal(w.Body)
					}
				}
				conn, err := f.s.DB.Pool.Acquire(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Release()
				barrier, err := conn.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer barrier.Rollback(context.Background())
				var credential string
				if err = barrier.QueryRow(ctx, `SELECT id::text FROM credentials WHERE agent_id=$1`, f.a).Scan(&credential); err != nil {
					t.Fatal(err)
				}
				if _, err = barrier.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, credential+":"+key); err != nil {
					t.Fatal(err)
				}
				done := wgAsyncRequest(ctx, f, path, `{"action":"add"}`, key)
				wgAwaitBlocked(t, f, ctx, conn.Conn().PgConn().PID())
				wgEligibilityChange(t, f, ctx, change)
				if err = barrier.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if w := wgWaitResponse(t, ctx, done); w.Code != 404 {
					t.Fatalf("stale invitation authorization %d %s", w.Code, w.Body)
				}
				var count int
				if err = f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM group_members WHERE group_id=$1 AND agent_id=$2`, f.g, f.b).Scan(&count); err != nil {
					t.Fatal(err)
				}
				want := 0
				if phase == "replay" {
					want = 1
				}
				if count != want {
					t.Fatal("unauthorized invitation mutated membership")
				}
			})
		}
	}
}

func TestWorkingGroupsInvitationPreservesExistingCrossOrganizationManager(t *testing.T) {
	f := setup(t)
	// Existing accepted managers may belong to another organization. Invitation
	// targets remain scoped to the group's organization, not the manager's.
	wgSQL(t, f, `INSERT INTO group_members(group_id,agent_id,role) VALUES($1,$2,'manager')`, f.g, f.b)
	target := uuid.NewString()
	wgSQL(t, f, `INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'local-target')`, target, f.org)
	key := uuid.NewString()
	path := "/v1/groups/" + f.g + "/members/" + target
	for i := 0; i < 2; i++ {
		w := f.request("POST", path, f.tb, `{"action":"add"}`, key)
		if w.Code != 200 {
			t.Fatalf("existing manager invite/replay %d %s", w.Code, w.Body)
		}
	}
	w := f.request("POST", "/v1/groups/direct", f.tb, fmt.Sprintf(`{"agent_id":%q,"purpose_key":"foreign"}`, target), uuid.NewString())
	if w.Code != 404 {
		t.Fatalf("direct broadened cross-organization access: %d %s", w.Code, w.Body)
	}
}
