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

func TestConfigureGroupKnowledgeProfileStartupGuard(t *testing.T) {
	t.Setenv("AJENT_GROUP_BRAIN_PROFILE", "")
	t.Setenv("AJENT_GROUP_BRAIN_ENDPOINT", "")
	t.Setenv("AJENT_GROUP_BRAIN_TOKEN", "")

	// Legacy startup: no group profile configuration at all must stay a
	// silent, error-free no-op, exactly as before this configuration
	// surface existed.
	profiles, err := configureGroupKnowledgeProfile()
	if err != nil || profiles != nil {
		t.Fatalf("unconfigured group profile: profiles=%+v err=%v", profiles, err)
	}

	// Partial configuration (any one or two of the three set) fails closed --
	// never silently runs with no private writer or an incomplete one.
	partial := [][3]string{
		{"a-profile", "", ""},
		{"", "http://127.0.0.1/mcp", ""},
		{"", "", "a-token"},
		{"a-profile", "http://127.0.0.1/mcp", ""},
	}
	for _, p := range partial {
		t.Setenv("AJENT_GROUP_BRAIN_PROFILE", p[0])
		t.Setenv("AJENT_GROUP_BRAIN_ENDPOINT", p[1])
		t.Setenv("AJENT_GROUP_BRAIN_TOKEN", p[2])
		if profiles, err = configureGroupKnowledgeProfile(); err == nil {
			t.Fatalf("partial config %+v silently accepted: %+v", p, profiles)
		}
	}

	// A malformed profile label fails closed even with a well-formed
	// endpoint/token pair.
	t.Setenv("AJENT_GROUP_BRAIN_PROFILE", "Not_Valid")
	t.Setenv("AJENT_GROUP_BRAIN_ENDPOINT", "http://127.0.0.1/mcp")
	t.Setenv("AJENT_GROUP_BRAIN_TOKEN", "a-token")
	if profiles, err = configureGroupKnowledgeProfile(); err == nil {
		t.Fatalf("malformed profile silently accepted: %+v", profiles)
	}

	// Complete, well-formed configuration is accepted and resolvable by name.
	t.Setenv("AJENT_GROUP_BRAIN_PROFILE", "valid-profile")
	t.Setenv("AJENT_GROUP_BRAIN_ENDPOINT", "http://127.0.0.1/mcp")
	t.Setenv("AJENT_GROUP_BRAIN_TOKEN", "a-token")
	profiles, err = configureGroupKnowledgeProfile()
	if err != nil || len(profiles) != 1 {
		t.Fatalf("valid config rejected: profiles=%+v err=%v", profiles, err)
	}
	if _, ok := profiles["valid-profile"]; !ok {
		t.Fatalf("configured profile not keyed by its own name: %+v", profiles)
	}
}
