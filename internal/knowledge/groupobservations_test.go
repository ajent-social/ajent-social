package knowledge

import (
	"ajent/migrations"
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const groupObsFactID = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

// fakeGroupBackend is a hand-rolled control-plane test double (no network,
// no authentication) -- it proves ProcessOneGroup's state-machine, locking
// and profile-selection logic, the same class of double
// internal/api/knowledge_test.go's own knowledgeBackend already uses for the
// global lifecycle. It is explicitly NOT an authenticated two-brain proof;
// see TestTwoDistinctAuthenticatedProfileBackendsNeverCross for that.
type fakeGroupBackend struct {
	mu        sync.Mutex
	remembers int
	cancels   int
	err       error
	onCall    func()
}

func (b *fakeGroupBackend) Remember(_ context.Context, in RememberInput) (RememberResult, error) {
	b.mu.Lock()
	b.remembers++
	cb, err := b.onCall, b.err
	b.mu.Unlock()
	if cb != nil {
		cb()
	}
	if err != nil {
		return RememberResult{}, err
	}
	return RememberResult{ID: groupObsFactID, ProtocolVersion: 1, SearchState: "semantic"}, nil
}
func (b *fakeGroupBackend) Cancel(context.Context, string) (CancelResult, error) {
	b.mu.Lock()
	b.cancels++
	b.mu.Unlock()
	return CancelResult{Canceled: true, ProtocolVersion: 1}, nil
}
func (b *fakeGroupBackend) Search(context.Context, string) ([]SearchHit, error) { return nil, nil }
func (b *fakeGroupBackend) ReadExact(context.Context, string) (ReadFactResult, error) {
	return ReadFactResult{}, ErrUnsupported
}

type goFixture struct {
	t                    *testing.T
	pool                 *pgxpool.Pool
	svc                  *Service
	org                  string
	owner, member, other string
	group                string
	brain                string
	profile              string
	backend              *fakeGroupBackend
	globalBackend        *fakeGroupBackend
}

func (f *goFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func newObsFixture(t *testing.T) *goFixture {
	t.Helper()
	dsn := os.Getenv("AJENT_TEST_DSN")
	if dsn == "" {
		t.Skip("AJENT_TEST_DSN required; use an isolated disposable PostgreSQL database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = migrations.Apply(ctx, pool); err != nil {
		t.Fatal(err)
	}
	f := &goFixture{t: t, pool: pool, backend: &fakeGroupBackend{}, globalBackend: &fakeGroupBackend{}}
	// ProcessOneGroup deliberately drains ANY due row in this table, across
	// every group-private brain -- correct for a real single deployment, but
	// this shared, accumulating disposable database is reused across many
	// fixtures in one test binary run. Without this, an earlier fixture's row
	// (retry_at defaults to now(), so it is "due" the instant it is inserted)
	// is a live candidate for a later fixture's Service, which was never
	// configured with that row's profile.
	f.exec(`DELETE FROM group_knowledge_publications`)
	f.org = uuid.NewString()
	f.exec(`INSERT INTO organizations(id,name,namespace) VALUES($1,'Org',$2)`, f.org, "org-"+f.org)
	f.owner, f.member, f.other = uuid.NewString(), uuid.NewString(), uuid.NewString()
	f.exec(`INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'owner'),($3,$2,'member'),($4,$2,'other')`, f.owner, f.org, f.member, f.other)
	f.group = uuid.NewString()
	f.exec(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'g','private')`, f.group, f.org)
	f.exec(`INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'owner','accepted')`, f.group, f.owner)
	// f.member is a distinct, non-owner accepted member and the post author in
	// every test below -- preserve_group_owner blocks removing the sole owner,
	// which is irrelevant to what membership-loss tests exercise.
	f.exec(`INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'member','accepted')`, f.group, f.member)
	f.brain = uuid.NewString()
	f.profile = uuid.NewString()
	f.svc = &Service{Pool: pool, BrainID: uuid.NewString(), Backend: f.globalBackend, ProfileBackends: map[string]Backend{f.profile: f.backend}}
	if err := f.svc.CreateBinding(ctx, f.group, f.brain, f.profile); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *goFixture) post(authorID, body string) string {
	f.t.Helper()
	id := uuid.NewString()
	f.exec(`INSERT INTO posts(id,author_id,group_id,kind,body) VALUES($1,$2,$3,'finding',$4)`, id, authorID, f.group, body)
	return id
}
func (f *goFixture) ungroupedPost(authorID string) string {
	f.t.Helper()
	id := uuid.NewString()
	f.exec(`INSERT INTO posts(id,author_id,kind,body) VALUES($1,$2,'finding','stray')`, id, authorID)
	return id
}
func (f *goFixture) dueRow() {
	f.t.Helper()
	f.exec(`UPDATE group_knowledge_publications SET retry_at=now() WHERE brain_id=$1`, f.brain)
}

func TestRequestGroupObservationRequiresAuthorAndEligibility(t *testing.T) {
	f := newObsFixture(t)
	ctx := context.Background()

	ungrouped := f.ungroupedPost(f.member)
	if _, err := f.svc.RequestGroupObservation(ctx, f.member, ungrouped, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ungrouped post admitted: %v", err)
	}

	id := f.post(f.member, "claim")
	if _, err := f.svc.RequestGroupObservation(ctx, f.other, id, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-author admitted: %v", err)
	}

	out, err := f.svc.RequestGroupObservation(ctx, f.member, id, true)
	if err != nil || out.BrainID != f.brain || out.State != "pending" || out.Revoked {
		t.Fatalf("legitimate grant rejected: %+v %v", out, err)
	}
	if f.backend.remembers != 0 {
		t.Fatal("request synchronously contacted writer")
	}
}

func TestRequestGroupObservationWithdrawalNeverContactsWriter(t *testing.T) {
	f := newObsFixture(t)
	ctx := context.Background()
	id := f.post(f.member, "claim")
	out, err := f.svc.RequestGroupObservation(ctx, f.member, id, false)
	if err != nil || !out.Revoked || out.State != "cancel_pending" {
		t.Fatalf("withdrawal-before-publish: %+v %v", out, err)
	}
	if f.backend.remembers != 0 || f.backend.cancels != 0 {
		t.Fatal("withdrawal contacted writer")
	}
}

func TestProcessOneGroupUsesResolvedProfileBackendNeverGlobal(t *testing.T) {
	f := newObsFixture(t)
	ctx := context.Background()
	id := f.post(f.member, "claim body")
	if _, err := f.svc.RequestGroupObservation(ctx, f.member, id, true); err != nil {
		t.Fatal(err)
	}
	processed, err := f.svc.ProcessOneGroup(ctx)
	if !processed || err != nil {
		t.Fatalf("process: %v %v", processed, err)
	}
	if f.backend.remembers != 1 {
		t.Fatalf("group profile backend not used: remembers=%d", f.backend.remembers)
	}
	if f.globalBackend.remembers != 0 {
		t.Fatal("private grant leaked to the global writer")
	}
	status, err := f.svc.StatusGroupObservation(ctx, f.member, id)
	if err != nil || status.State != "indexed" || status.FactID != groupObsFactID {
		t.Fatalf("status after process: %+v %v", status, err)
	}
}

func TestProcessOneGroupFailsClosedWhenProfileUnavailable(t *testing.T) {
	f := newObsFixture(t)
	ctx := context.Background()
	// Simulate a rotated/cleared credential profile: the row and binding are
	// intact, but this Service no longer holds a backend for it.
	f.svc.ProfileBackends = nil
	id := f.post(f.member, "claim")
	if _, err := f.svc.RequestGroupObservation(ctx, f.member, id, true); err != nil {
		t.Fatal(err)
	}
	processed, err := f.svc.ProcessOneGroup(ctx)
	if !processed || !errors.Is(err, ErrProfileUnavailable) {
		t.Fatalf("unresolved profile not reported: %v %v", processed, err)
	}
	if f.backend.remembers != 0 || f.globalBackend.remembers != 0 {
		t.Fatal("a writer was contacted despite no resolvable profile")
	}
	var code string
	if err := f.pool.QueryRow(ctx, `SELECT last_error_code FROM group_knowledge_publications WHERE post_id=$1`, id).Scan(&code); err != nil || code != "profile_unavailable" {
		t.Fatalf("last_error_code: %q %v", code, err)
	}
}

func TestProcessOneGroupRevocationDuringWriteCancelsWithoutReplay(t *testing.T) {
	f := newObsFixture(t)
	ctx := context.Background()
	id := f.post(f.member, "claim")
	if _, err := f.svc.RequestGroupObservation(ctx, f.member, id, true); err != nil {
		t.Fatal(err)
	}
	f.backend.err = ErrUnavailable
	f.backend.onCall = func() {
		// Membership loss during the in-flight (failing) write must fence the
		// grant durably; the retry below must never replay the body.
		if _, err := f.pool.Exec(context.Background(), `DELETE FROM group_members WHERE group_id=$1 AND agent_id=$2`, f.group, f.member); err != nil {
			t.Error(err)
		}
	}
	if _, err := f.svc.ProcessOneGroup(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	f.backend.err = nil
	f.dueRow()
	if _, err := f.svc.ProcessOneGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if f.backend.remembers != 1 {
		t.Fatalf("revoked body replayed: remembers=%d", f.backend.remembers)
	}
	if f.backend.cancels != 1 {
		t.Fatalf("fenced grant never canceled: cancels=%d", f.backend.cancels)
	}
}

func TestProcessOneGroupHasNoDueRowReturnsFalse(t *testing.T) {
	f := newObsFixture(t)
	processed, err := f.svc.ProcessOneGroup(context.Background())
	if processed || err != nil {
		t.Fatalf("empty outbox: %v %v", processed, err)
	}
}

func TestProcessOneGroupSuppressesResultWhenBindingRevokedDuringWrite(t *testing.T) {
	f := newObsFixture(t)
	ctx := context.Background()
	id := f.post(f.member, "claim")
	if _, err := f.svc.RequestGroupObservation(ctx, f.member, id, true); err != nil {
		t.Fatal(err)
	}
	f.backend.err = ErrUnavailable
	f.backend.onCall = func() {
		if err := f.svc.RevokeBinding(context.Background(), f.group); err != nil {
			t.Error(err)
		}
	}
	if _, err := f.svc.ProcessOneGroup(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	f.backend.err = nil
	f.dueRow()
	if _, err := f.svc.ProcessOneGroup(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := f.svc.StatusGroupObservation(ctx, f.member, id)
	if err != nil || status.State != "canceled" || status.FactID != "" {
		t.Fatalf("revoked-binding grant not fenced: %+v %v", status, err)
	}
	if f.backend.remembers != 1 {
		t.Fatal("revoked-binding body replayed")
	}
}

// TestStartDrainsGroupPrivateOutboxWithoutGlobalBrainID proves Start itself
// -- the real background worker cmd/ajentd wires up, not ProcessOneGroup
// called directly by a test -- drains group-private grants on its own timer,
// including for a Service with no global BrainID/Backend configured at all
// (the exact "group profile only, no global knowledge" deployment shape
// configureGroupKnowledgeProfile's startup wiring can produce).
func TestStartDrainsGroupPrivateOutboxWithoutGlobalBrainID(t *testing.T) {
	f := newObsFixture(t)
	f.svc.BrainID = ""
	f.svc.Backend = nil
	ctx := context.Background()
	id := f.post(f.member, "claim")
	if _, err := f.svc.RequestGroupObservation(ctx, f.member, id, true); err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := f.svc.Start(runCtx)
	t.Cleanup(func() {
		cancel()
		<-done
	})
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, err := f.svc.StatusGroupObservation(ctx, f.member, id)
		if err != nil {
			t.Fatal(err)
		}
		if status.State == "indexed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Start never drained the group-private outbox: %+v", status)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if f.backend.remembers != 1 {
		t.Fatalf("Start did not use the resolved profile backend: remembers=%d", f.backend.remembers)
	}
}
