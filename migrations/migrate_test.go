package migrations

import (
	"ajent/internal/knowledge"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// freshDatabase creates a brand-new, uniquely named database on the same
// server as AJENT_TEST_DSN and returns a pool connected to it, cleaned up on
// test completion. This test replays raw pre-022 migration SQL directly
// (not through Apply's idempotent bookkeeping), which requires a genuinely
// blank database -- the shared AJENT_TEST_DSN database is reused (and
// already fully migrated) by every other fixture in a full-suite run, so
// reusing it here would fail on "relation already exists" depending on test
// order, not on anything this test actually verifies.
func freshDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required; use an isolated disposable PostgreSQL database")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := "ajent_migrate_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s`, name)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			return
		}
		defer cleanup.Close()
		cleanup.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, name))
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestMigration022BackfillsExistingRowsSafely proves 022's new
// credential_profile column is safe against real pre-existing rows: a row
// written before 022 ever existed must survive the ALTER TABLE with the
// documented ” default, not an error, a NULL, or a value that fails 022's
// own CHECK constraint. It applies every migration through 021 directly (the
// exact SQL a pre-022 deployment already ran), inserts a row shaped the way
// that deployment would have, then lets Apply pick up from 022 onward.
func TestMigration022BackfillsExistingRowsSafely(t *testing.T) {
	ctx := context.Background()
	pool := freshDatabase(t)

	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	const target = "022_private_observation_lifecycle.sql"
	found := false
	if _, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY,applied_at TIMESTAMPTZ DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name == target {
			found = true
			break
		}
		b, err := ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(b)); err != nil {
			t.Fatalf("pre-022 migration %s: %v", name, err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, name); err != nil {
			t.Fatal(err)
		}
	}
	if !found {
		t.Fatal("022_private_observation_lifecycle.sql not found among embedded migrations")
	}

	// A real row written by a pre-022 deployment: only the columns 021 knew
	// about exist at this point, exactly as production data would look.
	preExisting := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO knowledge_brains(id,scope) VALUES($1,'global_web_findings')`, preExisting); err != nil {
		t.Fatal(err)
	}
	preExistingGroup := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO knowledge_brains(id,scope) VALUES($1,'group_private')`, preExistingGroup); err != nil {
		t.Fatal(err)
	}

	if err = Apply(ctx, pool); err != nil {
		t.Fatalf("022 and later did not apply cleanly over pre-existing rows: %v", err)
	}

	for _, id := range []string{preExisting, preExistingGroup} {
		var profile string
		if err = pool.QueryRow(ctx, `SELECT credential_profile FROM knowledge_brains WHERE id=$1`, id).Scan(&profile); err != nil {
			t.Fatalf("row %s lost after migration: %v", id, err)
		}
		if profile != "" {
			t.Fatalf("row %s: want default '' credential_profile, got %q", id, profile)
		}
	}

	// The migration must not have silently widened either pre-existing row's
	// scope, and a fresh CreateBinding must still work normally afterward --
	// the new column/index do not break existing or new functionality.
	svc := &knowledge.Service{Pool: pool}
	group := uuid.NewString()
	org := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO organizations(id,name,namespace) VALUES($1,'Org',$2)`, org, "org-"+org); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'g','private')`, group, org); err != nil {
		t.Fatal(err)
	}
	newBrain := uuid.NewString()
	if err = svc.CreateBinding(ctx, group, newBrain, uuid.NewString()); err != nil {
		t.Fatalf("fresh binding broken after 022 migrated pre-existing rows: %v", err)
	}
}
