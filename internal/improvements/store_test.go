package improvements

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required for PostgreSQL integration")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "store_" + uuid.NewString()
	schema = pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", "017_improvement_jobs.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	return s, pool
}
func enqueue(t *testing.T, s *Store) Job {
	t.Helper()
	j, err := s.Enqueue(context.Background(), JobKey{Repository: "ajent-social/ajent-social", ReportID: uuid.NewString(), Operation: "create_issue", VersionKey: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func claim(t *testing.T, s *Store) Job {
	t.Helper()
	j, err := s.Claim(context.Background(), "ajent-social/ajent-social", "worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return j
}
func execSQL(t *testing.T, p *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	if _, err := p.Exec(context.Background(), q, args...); err != nil {
		t.Fatal(err)
	}
}
func state(t *testing.T, p *pgxpool.Pool, id string) string {
	t.Helper()
	var s string
	if err := p.QueryRow(context.Background(), `SELECT state FROM improvement_jobs WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestStoreUnique(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	key := JobKey{Repository: "Ajent-Social/Ajent-Social", ReportID: uuid.NewString(), Operation: "create_issue", VersionKey: "v1"}
	const n = 16
	var wg sync.WaitGroup
	jobs := make([]Job, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-start; jobs[i], errs[i] = s.Enqueue(ctx, key) }(i)
	}
	close(start)
	wg.Wait()
	for i := range n {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if jobs[i].ID != jobs[0].ID {
			t.Fatal("duplicate durable operation")
		}
		if jobs[i].Repository != "ajent-social/ajent-social" {
			t.Fatal("repository not canonical")
		}
	}
	var count int
	if err := p.QueryRow(ctx, `SELECT count(*) FROM improvement_jobs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO improvement_jobs(repository,report_id,operation) VALUES('other/repo',$1,'create')`, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err = p.QueryRow(ctx, `SELECT count(*) FROM improvement_jobs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback count=%d error=%v", count, err)
	}
}
func TestStoreFencing(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	enqueue(t, s)
	type result struct {
		j Job
		e error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, owner := range []string{"one", "two"} {
		go func(owner string) {
			<-start
			j, e := s.Claim(ctx, "ajent-social/ajent-social", owner, time.Minute)
			results <- result{j, e}
		}(owner)
	}
	close(start)
	var old Job
	wins := 0
	for range 2 {
		r := <-results
		if r.e == nil {
			old = r.j
			wins++
		} else if !errors.Is(r.e, ErrNoJob) {
			t.Fatal(r.e)
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent winners=%d", wins)
	}
	if err := s.MarkSent(ctx, old, old.Marker); err != nil {
		t.Fatal(err)
	}
	// Expiry is deterministic SQL clock control, not sleep-based synchronization.
	execSQL(t, p, `UPDATE improvement_jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=$1`, old.ID)
	if err := s.Renew(ctx, old, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired renewal=%v", err)
	}
	if err := s.Finish(ctx, old, StateDone, "", "observed"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired finish=%v", err)
	}
	fresh, err := s.Claim(ctx, old.Repository, old.Owner, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Generation <= old.Generation || fresh.State != StateAmbiguous {
		t.Fatalf("reclaim=%+v", fresh)
	}
	if err := s.Finish(ctx, old, StateDone, "https://github.com/ajent-social/ajent-social/issues/1", "observed"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("late response=%v", err)
	}
	if err := s.MarkSent(ctx, fresh, fresh.Marker); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("ambiguous recreate=%v", err)
	}
	if err := s.Retry(ctx, fresh, time.Second, "timeout"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("ambiguous retry=%v", err)
	}
	if err := s.Renew(ctx, fresh, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, fresh, StateDone, "https://github.com/ajent-social/ajent-social/issues/1", "reconciled"); err != nil {
		t.Fatal(err)
	}
	if state(t, p, fresh.ID) != StateDone {
		t.Fatal("not reconciled")
	}
}
func TestStoreWithdrawal(t *testing.T) {
	for _, sent := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "sent"}[sent], func(t *testing.T) {
			s, p := testStore(t)
			ctx := context.Background()
			enqueue(t, s)
			j := claim(t, s)
			if sent {
				if err := s.MarkSent(ctx, j, j.Marker); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.CancelReport(ctx, j.ReportID); err != nil {
				t.Fatal(err)
			}
			expected := StateCancelled
			if sent {
				expected = StateAmbiguous
			}
			if got := state(t, p, j.ID); got != expected {
				t.Fatalf("state=%s", got)
			}
			if err := s.Finish(ctx, j, StateDone, "", "late"); !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("late owner=%v", err)
			}
			same, err := s.Enqueue(ctx, j.JobKey)
			if err != nil || same.ID != j.ID || same.State != expected {
				t.Fatalf("re-enqueue=%+v,%v", same, err)
			}
			if !sent {
				if _, err = s.Claim(ctx, j.Repository, "next", time.Minute); !errors.Is(err, ErrNoJob) {
					t.Fatalf("cancelled claim=%v", err)
				}
			}
		})
	}
}
func TestStoreRetention(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	ids := map[string]Job{}
	for _, st := range []string{StatePending, StateSent, StateAmbiguous, StateDone, StateCancelled, StateDead} {
		j := enqueue(t, s)
		execSQL(t, p, `UPDATE improvement_jobs SET state=$2,updated_at=clock_timestamp()-interval '91 days' WHERE id=$1`, j.ID, st)
		ids[st] = j
	}
	n, err := s.Cleanup(ctx)
	if err != nil || n != 3 {
		t.Fatalf("cleanup=%d,%v", n, err)
	}
	for _, st := range []string{StatePending, StateSent, StateAmbiguous} {
		j := ids[st]
		same, err := s.Enqueue(ctx, j.JobKey)
		if err != nil || same.ID != j.ID || same.State != st {
			t.Fatalf("lost %s tombstone: %+v,%v", st, same, err)
		}
	}
	// Tombstones contain UUID references without foreign-key retention coupling.
	var constraints int
	if err = p.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid='improvement_jobs'::regclass AND contype='f'`).Scan(&constraints); err != nil || constraints != 0 {
		t.Fatalf("report retention coupling=%d,%v", constraints, err)
	}
}
func TestStoreRetryAndIsolation(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	j := enqueue(t, s)
	if _, err := s.Claim(ctx, "other/repo", "worker", time.Minute); !errors.Is(err, ErrNoJob) {
		t.Fatalf("cross-repository claim=%v", err)
	}
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		j = claim(t, s)
		if j.Attempts != attempt {
			t.Fatalf("attempts=%d", j.Attempts)
		}
		if err := s.Retry(ctx, j, time.Second, "definite_rejection"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Claim(ctx, j.Repository, "next", time.Minute); !errors.Is(err, ErrNoJob) {
			t.Fatalf("delay bypass=%v", err)
		}
		execSQL(t, p, `UPDATE improvement_jobs SET next_attempt=clock_timestamp()-interval '1 second' WHERE id=$1`, j.ID)
	}
	if state(t, p, j.ID) != StateDead {
		t.Fatal("attempt cap missing")
	}
	if _, err := s.Claim(ctx, j.Repository, "next", time.Minute); !errors.Is(err, ErrNoJob) {
		t.Fatalf("dead claim=%v", err)
	}
}
func TestStoreIntentRollbackAndValidation(t *testing.T) {
	s, p := testStore(t)
	ctx := context.Background()
	enqueue(t, s)
	j := claim(t, s)
	tx, err := p.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MarkSentTx(ctx, tx, j, j.Marker); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if state(t, p, j.ID) != StatePending {
		t.Fatal("rolled-back intent persisted")
	}
	if err = s.MarkSent(ctx, j, "private context"); !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("arbitrary marker=%v", err)
	}
	if err = s.Finish(ctx, j, StateDone, "https://github.com/other/repo/issues/1", "success"); !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("foreign issue=%v", err)
	}
	if err = s.Finish(ctx, j, StateDone, "", "private reporter context"); !errors.Is(err, ErrInvalidJob) {
		t.Fatalf("arbitrary outcome=%v", err)
	}
}
