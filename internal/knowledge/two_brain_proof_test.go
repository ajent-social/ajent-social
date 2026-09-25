package knowledge

import (
	"ajent/migrations"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// authenticatedWriter builds a real production *Serenity adapter (the exact
// type NewSerenity/Remember/Cancel that talks to a genuine Serenity
// deployment) against a dedicated httptest.Server that requires its OWN
// distinct bearer token and records every fact it actually receives. This is
// the same construction serenity_test.go already uses and treats as
// "authenticated" round-trip evidence for the production writer code path
// (real HTTP, real MCP wire protocol, real per-request Authorization check
// that 401s on any other token) -- it is NOT a hand-rolled fake satisfying
// the Backend interface directly (compare fakeGroupBackend above), and it is
// NOT a claim that two live production Serenity deployments were reached:
// this worker has no access to and is forbidden from provisioning real
// Serenity credentials/endpoints. Labeled explicitly in the caller's own
// test name and in outbox evidence, per instruction.
func authenticatedWriter(t *testing.T, token, factID string) (*Serenity, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var facts []string
	server := mcp.NewServer(&mcp.Implementation{Name: "two-brain-proof-fixture", Version: "1"}, nil)
	schema := map[string]any{"type": "object", "properties": map[string]any{"operation_key": map[string]any{"type": "string"}}}
	server.AddTool(&mcp.Tool{Name: "remember", InputSchema: schema}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			Fact string `json:"fact"`
		}
		_ = json.Unmarshal(r.Params.Arguments, &args)
		mu.Lock()
		facts = append(facts, args.Fact)
		mu.Unlock()
		data, err := json.Marshal(RememberResult{ID: factID, ProtocolVersion: 1, SearchState: "semantic"})
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
	})
	server.AddTool(&mcp.Tool{Name: "cancel_memory_operation", InputSchema: schema}, func(_ context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		data, err := json.Marshal(CancelResult{Canceled: true, ProtocolVersion: 1})
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
	})
	server.AddTool(&mcp.Tool{Name: "recall", InputSchema: schema}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		data, _ := json.Marshal(map[string]any{"protocol_version": 1, "results": []any{}})
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	adapter, err := NewSerenity(httpServer.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	return adapter, &facts
}

// TestTwoDistinctAuthenticatedProfileBackendsNeverCross is the authenticated
// two-brain proof RFC-PRIVATE-OBSERVATIONS-02 requires: two group-private
// brains, each bound to its OWN credential profile, each profile resolving
// (server-side, via resolveProfileBackend, immediately before the RPC) to a
// DISTINCT real *Serenity adapter authenticated with its OWN bearer token
// against its OWN httptest.Server -- plus a third, separately authenticated
// global writer that must never see private traffic at all. A wrong
// resolution, a fallback to the global writer, or a cross-brain leak would
// either 401 (wrong token reached the wrong server) or show up as content on
// the wrong recorder; neither happens.
//
// Explicit limitation: the three "brains" here are httptest.Server loopback
// processes, not three live production Serenity deployments -- this worker
// has no access to real Serenity credentials and is forbidden from seeking
// them out. This proves the resolution/authentication code path is genuine
// and per-brain-distinct; it does not, and cannot, stand in for a production
// live-network two-brain deployment proof.
func TestTwoDistinctAuthenticatedProfileBackendsNeverCross(t *testing.T) {
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
	if _, err = pool.Exec(ctx, `DELETE FROM group_knowledge_publications`); err != nil {
		t.Fatal(err)
	}

	const factA = "111111111111111111111111111111111111111111111111111111111111111a"
	const factB = "222222222222222222222222222222222222222222222222222222222222222b"
	writerA, factsA := authenticatedWriter(t, "token-brain-a", factA)
	writerB, factsB := authenticatedWriter(t, "token-brain-b", factB)
	writerGlobal, factsGlobal := authenticatedWriter(t, "token-global", "333333333333333333333333333333333333333333333333333333333333333c")

	org := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO organizations(id,name,namespace) VALUES($1,'Org',$2)`, org, "org-"+org); err != nil {
		t.Fatal(err)
	}
	owner := uuid.NewString()
	authorA, authorB := uuid.NewString(), uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'owner'),($3,$2,'author-a'),($4,$2,'author-b')`, owner, org, authorA, authorB); err != nil {
		t.Fatal(err)
	}
	groupA, groupB := uuid.NewString(), uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'group-a','private'),($3,$2,'group-b','private')`, groupA, org, groupB); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO group_members(group_id,agent_id,role,state) VALUES
 ($1,$2,'owner','accepted'),($1,$3,'member','accepted'),
 ($4,$2,'owner','accepted'),($4,$5,'member','accepted')`, groupA, owner, authorA, groupB, authorB); err != nil {
		t.Fatal(err)
	}

	profileA, profileB := "profile-brain-a", "profile-brain-b"
	svc := &Service{Pool: pool, BrainID: uuid.NewString(), Backend: writerGlobal, ProfileBackends: map[string]Backend{profileA: writerA, profileB: writerB}}
	brainA, brainB := uuid.NewString(), uuid.NewString()
	if err = svc.CreateBinding(ctx, groupA, brainA, profileA); err != nil {
		t.Fatal(err)
	}
	if err = svc.CreateBinding(ctx, groupB, brainB, profileB); err != nil {
		t.Fatal(err)
	}

	postA := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO posts(id,author_id,group_id,kind,title,body) VALUES($1,$2,$3,'finding','A','secret claim for group A only')`, postA, authorA, groupA); err != nil {
		t.Fatal(err)
	}
	postB := uuid.NewString()
	if _, err = pool.Exec(ctx, `INSERT INTO posts(id,author_id,group_id,kind,title,body) VALUES($1,$2,$3,'finding','B','secret claim for group B only')`, postB, authorB, groupB); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.RequestGroupObservation(ctx, authorA, postA, true); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.RequestGroupObservation(ctx, authorB, postB, true); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		processed, err := svc.ProcessOneGroup(ctx)
		if !processed || err != nil {
			t.Fatalf("process: %v %v", processed, err)
		}
	}

	statusA, err := svc.StatusGroupObservation(ctx, authorA, postA)
	if err != nil || statusA.State != "indexed" || statusA.FactID != factA {
		t.Fatalf("group A status: %+v %v", statusA, err)
	}
	statusB, err := svc.StatusGroupObservation(ctx, authorB, postB)
	if err != nil || statusB.State != "indexed" || statusB.FactID != factB {
		t.Fatalf("group B status: %+v %v", statusB, err)
	}

	if len(*factsA) != 1 || (*factsA)[0] != "A\n\nsecret claim for group A only" {
		t.Fatalf("brain A writer received: %+v", *factsA)
	}
	if len(*factsB) != 1 || (*factsB)[0] != "B\n\nsecret claim for group B only" {
		t.Fatalf("brain B writer received: %+v", *factsB)
	}
	if len(*factsGlobal) != 0 {
		t.Fatalf("private group traffic reached the global writer: %+v", *factsGlobal)
	}
}
