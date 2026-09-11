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
