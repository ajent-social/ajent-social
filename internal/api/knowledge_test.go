package api

import (
	"ajent/internal/knowledge"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

const knowledgeFact = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type knowledgeBackend struct {
	mu                 sync.Mutex
	remembers, cancels int
	rememberErr        error
	onRemember         func()
	onSearch           func()
	inputs             []knowledge.RememberInput
}

func (b *knowledgeBackend) Remember(_ context.Context, in knowledge.RememberInput) (knowledge.RememberResult, error) {
	b.mu.Lock()
	b.remembers++
	b.inputs = append(b.inputs, in)
	callback, err := b.onRemember, b.rememberErr
	b.mu.Unlock()
	if callback != nil {
		callback()
	}
	if err != nil {
		return knowledge.RememberResult{}, err
	}
	return knowledge.RememberResult{ID: knowledgeFact, ProtocolVersion: 1, SearchState: "semantic"}, nil
}
func (b *knowledgeBackend) Cancel(context.Context, string) (knowledge.CancelResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cancels++
	return knowledge.CancelResult{Canceled: true, ProtocolVersion: 1}, nil
}
func (b *knowledgeBackend) Search(context.Context, string) ([]knowledge.SearchHit, error) {
	if b.onSearch != nil {
		b.onSearch()
	}
	return []knowledge.SearchHit{{ID: knowledgeFact, Snippet: "fixture evidence"}, {ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Snippet: "unmapped private marker"}}, nil
}
func setupKnowledge(t *testing.T) (*fixture, *knowledgeBackend) {
	t.Helper()
	f := setup(t)
	backend := &knowledgeBackend{}
	brain := uuid.NewString()
	if _, err := f.s.DB.Pool.Exec(context.Background(), `INSERT INTO knowledge_brains(id,scope) VALUES($1,'global_web_findings')`, brain); err != nil {
		t.Fatal(err)
	}
	f.s.Knowledge = &knowledge.Service{Pool: f.s.DB.Pool, BrainID: brain, Backend: backend}
	return f, backend
}
func (f *fixture) webFinding() string {
	f.t.Helper()
	w := f.request("POST", "/v1/posts", f.ta, `{"kind":"finding","title":"Fixture: durable withdrawal","body":"A cancellation fence prevents late retries from creating withdrawn content.","audience":"public"}`, uuid.NewString())
	if w.Code != 201 {
		f.t.Fatalf("create fixture: %d", w.Code)
	}
	var out struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		f.t.Fatal("bad post")
	}
	if _, err := f.s.DB.Pool.Exec(context.Background(), `UPDATE posts SET web_published=true WHERE id=$1`, out.ID); err != nil {
		f.t.Fatal(err)
	}
	return out.ID
}
func (f *fixture) publication(id string, publish bool) int {
	f.t.Helper()
	body := `{"publish":false}`
	if publish {
		body = `{"publish":true}`
	}
	code := f.request("POST", "/v1/posts/"+id+"/knowledge", f.ta, body, "").Code
	return code
}
func (f *fixture) dueKnowledge() {
	f.t.Helper()
	if _, err := f.s.DB.Pool.Exec(context.Background(), `UPDATE knowledge_publications SET retry_at=now() WHERE brain_id=$1`, f.s.Knowledge.BrainID); err != nil {
		f.t.Fatal(err)
	}
}

func TestKnowledgePublicationIsExplicitAndDoesNotDependOnWriter(t *testing.T) {
	f, b := setupKnowledge(t)
	ctx := context.Background()
	private := f.post(f.ta, "", "")
	if got := f.publication(private, true); got != 404 {
		t.Fatalf("private source admitted: %d", got)
	}
	grouped := f.post(f.ta, f.g, "")
	if got := f.publication(grouped, true); got != 404 {
		t.Fatalf("group source admitted: %d", got)
	}
	id := f.webFinding()
	if w := f.request("POST", "/v1/posts/"+id+"/knowledge", f.tb, `{"publish":true}`, ""); w.Code != 404 {
		t.Fatal("other author could publish")
	}
	for range 2 {
		if got := f.publication(id, true); got != 202 {
			t.Fatalf("publication %d", got)
		}
	}
	if b.remembers != 0 {
		t.Fatal("request synchronously contacted writer")
	}
	var count int
	if err := f.s.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM knowledge_publications WHERE post_id=$1`, id).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate control operations", err)
	}
	b.rememberErr = knowledge.ErrUnavailable
	processed, err := f.s.Knowledge.ProcessOne(ctx)
	if !processed || !errors.Is(err, knowledge.ErrUnavailable) {
		t.Fatal("outage not reported", err)
	}
	status, err := f.s.Knowledge.Status(ctx, f.a, id)
	if err != nil || status.State != "recovery_required" {
		t.Fatalf("false saved status: %+v %v", status, err)
	}
	b.rememberErr = nil
	f.dueKnowledge()
	if _, err = f.s.Knowledge.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	if len(b.inputs) != 2 || b.inputs[0] != b.inputs[1] {
		t.Fatal("retry changed immutable input")
	}
	status, err = f.s.Knowledge.Status(ctx, f.a, id)
	if err != nil || status.State != "indexed" || status.FactID != knowledgeFact {
		t.Fatalf("index status: %+v %v", status, err)
	}
}

func TestKnowledgeRevocationDuringAmbiguousWriteCancelsWithoutReplay(t *testing.T) {
	f, b := setupKnowledge(t)
	id := f.webFinding()
	if f.publication(id, true) != 202 {
		t.Fatal("publish")
	}
	b.rememberErr = knowledge.ErrUnavailable
	b.onRemember = func() {
		if f.publication(id, false) != 202 {
			t.Error("withdrawal blocked during write")
		}
	}
	if _, err := f.s.Knowledge.ProcessOne(context.Background()); !errors.Is(err, knowledge.ErrUnavailable) {
		t.Fatal(err)
	}
	// Author deletion clears the source body before unknown-outcome cleanup.
	if w := f.request("DELETE", "/v1/posts/"+id, f.ta, "", ""); w.Code != 200 {
		t.Fatal("delete failed")
	}
	f.dueKnowledge()
	if _, err := f.s.Knowledge.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.remembers != 1 || b.cancels != 1 {
		t.Fatalf("revoked body replayed: writes=%d cancels=%d", b.remembers, b.cancels)
	}
	status, err := f.s.Knowledge.Status(context.Background(), f.a, id)
	if err != nil || !status.Revoked || status.State != "canceled" || status.FactID != "" {
		t.Fatalf("status %+v %v", status, err)
	}
}

func TestKnowledgeSearchRechecksAuthorityAfterRecall(t *testing.T) {
	f, b := setupKnowledge(t)
	id := f.webFinding()
	if f.publication(id, true) != 202 {
		t.Fatal("publish")
	}
	if _, err := f.s.Knowledge.ProcessOne(context.Background()); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.Knowledge.Search(context.Background(), f.b, "cancellation")
	if err != nil || len(out.Results) != 1 || out.Results[0].PostID != id {
		t.Fatalf("positive control %+v %v", out, err)
	}
	b.onSearch = func() {
		if f.publication(id, false) != 202 {
			t.Error("withdraw failed")
		}
	}
	out, err = f.s.Knowledge.Search(context.Background(), f.b, "cancellation")
	if err != nil || len(out.Results) != 0 {
		t.Fatalf("revoked result leaked %+v %v", out, err)
	}
	if got := f.publication(id, true); got != 409 {
		t.Fatalf("replay revived grant: %d", got)
	}
	if w := f.request("GET", "/v1/posts/"+id+"/knowledge", f.tb, "", ""); w.Code != 404 {
		t.Fatal("status leaked to other author")
	}
}

func TestKnowledgeSourceWithdrawalAndExpirySuppressImmediately(t *testing.T) {
	for _, mode := range []string{"web", "expiry", "domain", "blocked"} {
		t.Run(mode, func(t *testing.T) {
			f, _ := setupKnowledge(t)
			id := f.webFinding()
			if f.publication(id, true) != 202 {
				t.Fatal("publish")
			}
			if _, err := f.s.Knowledge.ProcessOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			var err error
			switch mode {
			case "web":
				_, err = f.s.DB.Pool.Exec(context.Background(), `UPDATE posts SET web_published=false WHERE id=$1`, id)
			case "expiry":
				_, err = f.s.DB.Pool.Exec(context.Background(), `UPDATE knowledge_publications SET expires_at=$2 WHERE post_id=$1`, id, time.Now().Add(-time.Second))
			case "domain":
				_, err = f.s.DB.Pool.Exec(context.Background(), `DELETE FROM agent_domains WHERE agent_id=$1`, f.a)
			case "blocked":
				_, err = f.s.DB.Pool.Exec(context.Background(), `INSERT INTO blocks(blocker_id,blocked_id) VALUES($1,$2)`, f.b, f.a)
			}
			if err != nil {
				t.Fatal(err)
			}
			out, err := f.s.Knowledge.Search(context.Background(), f.b, "cancellation")
			if err != nil || len(out.Results) != 0 {
				t.Fatalf("%s leaked %+v %v", mode, out, err)
			}
		})
	}
}

func TestKnowledgeRevokeRestoreCannotReviveWhileConsumerPaused(t *testing.T) {
	for _, mode := range []string{"agent", "organization", "domain_membership", "verified_domain"} {
		t.Run(mode, func(t *testing.T) {
			f, _ := setupKnowledge(t)
			id := f.webFinding()
			if f.publication(id, true) != 202 {
				t.Fatal("publish")
			}
			if _, err := f.s.Knowledge.ProcessOne(context.Background()); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			pool := f.s.DB.Pool
			exec := func(sql string, args ...any) {
				t.Helper()
				if _, err := pool.Exec(ctx, sql, args...); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "agent":
				exec(`UPDATE agents SET public_posting=false WHERE id=$1`, f.a)
				exec(`UPDATE agents SET public_posting=true WHERE id=$1`, f.a)
			case "organization":
				exec(`UPDATE organizations SET status='suspended' WHERE id=$1`, f.org)
				exec(`UPDATE organizations SET status='active' WHERE id=$1`, f.org)
			case "domain_membership":
				exec(`DELETE FROM agent_domains WHERE agent_id=$1`, f.a)
				exec(`INSERT INTO agent_domains(agent_id,domain) VALUES($1,$2)`, f.a, f.a+".example.test")
			case "verified_domain":
				exec(`UPDATE verified_domains SET verified_until=now()-interval '1 second' WHERE domain=$1`, f.a+".example.test")
				exec(`UPDATE verified_domains SET verified_until=now()+interval '1 day' WHERE domain=$1`, f.a+".example.test")
			}
			// The consumer never ran between revoke and restore.
			out, err := f.s.Knowledge.Search(ctx, f.b, "cancellation")
			if err != nil || len(out.Results) != 0 {
				t.Fatalf("%s revived %+v %v", mode, out, err)
			}
			if got := f.publication(id, true); got != 409 {
				t.Fatalf("old grant revived: %d", got)
			}
		})
	}
}

func TestKnowledgePublicationWaitsForConcurrentAuthorityChange(t *testing.T) {
	f, _ := setupKnowledge(t)
	id := f.webFinding()
	ctx := context.Background()
	tx, err := f.s.DB.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE agents SET public_posting=false WHERE id=$1`, f.a); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := f.s.Knowledge.Request(ctx, f.a, id, true); result <- err }()
	select {
	case err := <-result:
		t.Fatalf("publication passed uncommitted authority boundary: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, knowledge.ErrNotFound) {
			t.Fatalf("stale grant admitted: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("publication remained blocked")
	}
}

func TestKnowledgeUnusedExpiredParentDoesNotRevokeEffectiveGrant(t *testing.T) {
	f, _ := setupKnowledge(t)
	ctx := context.Background()
	pool := f.s.DB.Pool
	parents := []string{uuid.NewString(), uuid.NewString()}
	grants := []string{uuid.NewString(), uuid.NewString()}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 2 {
		exec(`INSERT INTO credentials(id,agent_id,lookup_id,secret_digest,scopes) SELECT $1,agent_id,$2,secret_digest,scopes FROM credentials WHERE agent_id=$3 LIMIT 1`, parents[i], uuid.NewString(), f.a)
		exec(`INSERT INTO domain_grants(id,parent_credential_id,domain,scope_key) VALUES($1,$2,$3,'user')`, grants[i], parents[i], f.a+".example.test")
		exec(`INSERT INTO agent_domain_grants(agent_id,grant_id) VALUES($1,$2)`, f.a, grants[i])
	}
	exec(`UPDATE agent_domains SET grant_id=$2 WHERE agent_id=$1`, f.a, grants[0])
	id := f.webFinding()
	if f.publication(id, true) != 202 {
		t.Fatal("publish")
	}
	if _, err := f.s.Knowledge.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	// The second parent keeps effective domain authority continuously valid.
	exec(`UPDATE credentials SET expires_at=now()-interval '1 second' WHERE id=$1`, parents[0])
	exec(`UPDATE credentials SET expires_at=now()+interval '1 day' WHERE id=$1`, parents[0])
	out, err := f.s.Knowledge.Search(ctx, f.b, "cancellation")
	if err != nil || len(out.Results) != 1 {
		t.Fatalf("unrelated renewal canceled source: %+v %v", out, err)
	}
}

func TestKnowledgeCapabilityRequiresConfiguredBinding(t *testing.T) {
	f, _ := setupKnowledge(t)
	for _, enabled := range []bool{true, false} {
		if !enabled {
			f.s.Knowledge = nil
		}
		w := f.request("GET", "/v1/reliability", f.ta, "", "")
		if w.Code != 200 {
			t.Fatalf("reliability: %d", w.Code)
		}
		var out struct {
			Capabilities []string `json:"capabilities"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, capability := range out.Capabilities {
			if capability == "global_knowledge_findings" {
				found = true
			}
		}
		if found != enabled {
			t.Fatalf("capability configured=%v advertised=%v", enabled, found)
		}
	}
}
