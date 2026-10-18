package api

import (
	"context"
	"github.com/google/uuid"
	"testing"
	"time"
)

// Assert the chosen SQL lock modes actually exclude each relevant writer,
// independently of the advisory-barrier endpoint regressions above.
func TestWorkingGroupsEligibilityLocksWriters(t *testing.T) {
	for _, change := range []string{"block", "reverse_block", "target_disabled", "actor_disabled", "target_rehome", "actor_rehome", "organization_disabled"} {
		t.Run(change, func(t *testing.T) {
			f := setup(t)
			wgSQL(t, f, `UPDATE agents SET org_id=$1 WHERE id=$2`, f.org, f.b)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			holder, err := f.s.DB.Pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Release()
			tx, err := holder.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			eligible, err := lockGroupEligibility(ctx, tx, f.a, f.b, f.org, f.org)
			if err != nil || !eligible {
				t.Fatalf("eligibility %t %v", eligible, err)
			}
			writer, err := f.s.DB.Pool.Acquire(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Release()
			var sql string
			var args []any
			switch change {
			case "block":
				sql = `INSERT INTO blocks(blocker_id,blocked_id) VALUES($1,$2)`
				args = []any{f.a, f.b}
			case "reverse_block":
				sql = `INSERT INTO blocks(blocker_id,blocked_id) VALUES($1,$2)`
				args = []any{f.b, f.a}
			case "target_disabled":
				sql = `UPDATE agents SET status='disabled' WHERE id=$1`
				args = []any{f.b}
			case "actor_disabled":
				sql = `UPDATE agents SET status='disabled' WHERE id=$1`
				args = []any{f.a}
			case "organization_disabled":
				sql = `UPDATE organizations SET status='disabled' WHERE id=$1`
				args = []any{f.org}
			case "target_rehome", "actor_rehome":
				org := uuid.NewString()
				wgSQL(t, f, `INSERT INTO organizations(id,name,namespace) VALUES($1,'Other',$2)`, org, "other-"+org)
				agent := f.b
				if change == "actor_rehome" {
					agent = f.a
				}
				sql = `UPDATE agents SET org_id=$1 WHERE id=$2`
				args = []any{org, agent}
			}
			done := make(chan error, 1)
			go func() { _, err := writer.Exec(ctx, sql, args...); done <- err }()
			wgAwaitBlocked(t, f, ctx, holder.Conn().PgConn().PID())
			select {
			case err := <-done:
				t.Fatalf("writer bypassed eligibility locks: %v", err)
			default:
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}
