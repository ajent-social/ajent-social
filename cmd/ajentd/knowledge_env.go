package main

import (
	"ajent/internal/knowledge"
	"context"
	"errors"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func configureKnowledge(ctx context.Context, pool *pgxpool.Pool) (*knowledge.Service, error) {
	endpoint, token, brainID := os.Getenv("AJENT_SERENITY_ENDPOINT"), os.Getenv("AJENT_SERENITY_TOKEN"), os.Getenv("AJENT_KNOWLEDGE_BRAIN_ID")
	if endpoint == "" && token == "" && brainID == "" {
		return nil, nil
	}
	if endpoint == "" || token == "" || brainID == "" {
		return nil, errors.New("knowledge configuration requires endpoint, token and opaque brain ID")
	}
	parsed, err := uuid.Parse(brainID)
	if err != nil || parsed == uuid.Nil {
		return nil, errors.New("invalid knowledge brain ID")
	}
	backend, err := knowledge.NewSerenity(endpoint, token)
	if err != nil {
		return nil, errors.New("invalid knowledge writer configuration")
	}
	// A private group binding may already own this ID (see internal/knowledge
	// groupbrain.go). Never silently fold it into the global partition: fail
	// startup loudly instead of scoping a private brain's rows as public.
	var existingScope string
	err = pool.QueryRow(ctx, `SELECT scope FROM knowledge_brains WHERE id=$1`, parsed.String()).Scan(&existingScope)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err = pool.Exec(ctx, `INSERT INTO knowledge_brains(id,scope) VALUES($1,'global_web_findings')`, parsed.String()); err != nil {
			return nil, errors.New("knowledge binding unavailable")
		}
	case err != nil:
		return nil, errors.New("knowledge binding unavailable")
	case existingScope != "global_web_findings":
		return nil, errors.New("AJENT_KNOWLEDGE_BRAIN_ID is bound as a private group brain, not global")
	}
	return &knowledge.Service{Pool: pool, BrainID: parsed.String(), Backend: backend}, nil
}

// configureGroupKnowledgeProfile builds the operator-configured writer for
// exactly one group-private credential profile. It never touches
// knowledge_brains or group_knowledge_bindings: which brain(s) actually use
// this profile is decided entirely by CreateBinding's own out-of-band,
// operator-controlled setup (internal/knowledge groupbrain.go) -- this
// function only ever supplies the in-process (endpoint,token) writer handle
// a resolved profile name maps to at RPC time.
//
// Absent configuration is preserved exactly as before (nil, nil): a
// deployment that names no group profile keeps running with no group-private
// writer capability at all, the same as today. Partial or malformed
// configuration fails startup loudly instead of silently running with no
// private writer or falling back to any other credential -- this is the
// "no implicit profile or global fallback" contract at the one place a
// deployment's operator-set profile ever gets read from the environment.
func configureGroupKnowledgeProfile() (map[string]knowledge.Backend, error) {
	profile, endpoint, token := os.Getenv("AJENT_GROUP_BRAIN_PROFILE"), os.Getenv("AJENT_GROUP_BRAIN_ENDPOINT"), os.Getenv("AJENT_GROUP_BRAIN_TOKEN")
	if profile == "" && endpoint == "" && token == "" {
		return nil, nil
	}
	if profile == "" || endpoint == "" || token == "" {
		return nil, errors.New("group knowledge profile configuration requires a profile name, endpoint and token")
	}
	if !knowledge.ValidCredentialProfile(profile) {
		return nil, errors.New("invalid AJENT_GROUP_BRAIN_PROFILE")
	}
	backend, err := knowledge.NewSerenity(endpoint, token)
	if err != nil {
		return nil, errors.New("invalid group knowledge writer configuration")
	}
	return map[string]knowledge.Backend{profile: backend}, nil
}
