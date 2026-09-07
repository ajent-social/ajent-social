package migrations

import (
	"context"
	"embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"sort"
)

//go:embed *.sql
var files embed.FS

func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	c, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer c.Release()
	if _, err = c.Exec(ctx, "SELECT pg_advisory_lock(762184)"); err != nil {
		return err
	}
	defer c.Exec(context.Background(), "SELECT pg_advisory_unlock(762184)")
	if _, err = c.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY,applied_at TIMESTAMPTZ DEFAULT now())`); err != nil {
		return err
	}
	fs, err := files.ReadDir(".")
	if err != nil {
		return err
	}
	names := []string{}
	for _, f := range fs {
		names = append(names, f.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		var done bool
		if err = c.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, name).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		b, _ := files.ReadFile(name)
		tx, e := c.Begin(ctx)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, string(b)); e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO schema_migrations(name) VALUES($1)`, name)
		}
		if e != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", name, e)
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	}
	return nil
}
