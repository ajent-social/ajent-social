package main

import (
	"ajent/migrations"
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConfigureKnowledgeStartupGuard(t *testing.T) {
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required; use an isolated disposable PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AJENT_SERENITY_ENDPOINT", "http://127.0.0.1/mcp")
	t.Setenv("AJENT_SERENITY_TOKEN", "test-token")

	fresh := uuid.NewString()
	t.Setenv("AJENT_KNOWLEDGE_BRAIN_ID", fresh)
	svc, err := configureKnowledge(ctx, pool)
	if err != nil || svc == nil || svc.BrainID != fresh {
		t.Fatalf("fresh global brain: svc=%+v err=%v", svc, err)
	}
	var scope string
	if err = pool.QueryRow(ctx, `SELECT scope FROM knowledge_brains WHERE id=$1`, fresh).Scan(&scope); err != nil || scope != "global_web_findings" {
		t.Fatalf("global scope not recorded: %q %v", scope, err)
	}

	// Restarting with the same ID must remain idempotent: existing global
	// behavior (ON CONFLICT DO NOTHING today) is preserved for this case.
	svc, err = configureKnowledge(ctx, pool)
	if err != nil || svc == nil || svc.BrainID != fresh {
		t.Fatalf("idempotent restart on existing global brain: svc=%+v err=%v", svc, err)
	}

	// A private group binding (created out-of-band, as CreateBinding would)
	// must never be silently folded into the global partition on startup.
	private := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO knowledge_brains(id,scope) VALUES($1,'group_private')`, private); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AJENT_KNOWLEDGE_BRAIN_ID", private)
	if svc, err = configureKnowledge(ctx, pool); err == nil {
		t.Fatalf("private binding silently accepted as global: %+v", svc)
	}
	var stillPrivate string
	if err = pool.QueryRow(ctx, `SELECT scope FROM knowledge_brains WHERE id=$1`, private).Scan(&stillPrivate); err != nil || stillPrivate != "group_private" {
		t.Fatalf("private scope mutated by a failed startup attempt: %q %v", stillPrivate, err)
	}
}
