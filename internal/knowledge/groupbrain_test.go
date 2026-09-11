package knowledge

import (
	"ajent/migrations"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type gbFixture struct {
	t                                                            *testing.T
	pool                                                         *pgxpool.Pool
	svc                                                          *Service
	orgA, orgB                                                   string
	member, pendingAgent, suspendedAgent, crossOrgAgent, removed string
	group, brain                                                 string
}

func (f *gbFixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}

func newGuardFixture(t *testing.T) *gbFixture {
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
	f := &gbFixture{t: t, pool: pool, svc: &Service{Pool: pool}}
	f.orgA, f.orgB = uuid.NewString(), uuid.NewString()
	f.exec(`INSERT INTO organizations(id,name,namespace) VALUES($1,'Org A',$2),($3,'Org B',$4)`, f.orgA, "org-a-"+f.orgA, f.orgB, "org-b-"+f.orgB)
	f.member, f.pendingAgent, f.suspendedAgent, f.crossOrgAgent, f.removed = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	f.exec(`INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'member'),($3,$2,'pending'),($4,$2,'suspended'),($5,$6,'crossorg'),($7,$2,'removed')`,
		f.member, f.orgA, f.pendingAgent, f.suspendedAgent, f.crossOrgAgent, f.orgB, f.removed)
	f.exec(`UPDATE agents SET status='suspended' WHERE id=$1`, f.suspendedAgent)
	f.group = uuid.NewString()
	f.exec(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'private-group','private')`, f.group, f.orgA)
	f.exec(`INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'owner','accepted')`, f.group, f.member)
	f.exec(`INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'member','pending')`, f.group, f.pendingAgent)
	f.exec(`INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'member','accepted')`, f.group, f.suspendedAgent)
	f.exec(`INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'member','accepted')`, f.group, f.removed)
	f.exec(`DELETE FROM group_members WHERE group_id=$1 AND agent_id=$2`, f.group, f.removed)
	f.brain = uuid.NewString()
	if err := f.svc.CreateBinding(context.Background(), f.group, f.brain); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestGuardGroupBrainAuthorizedMemberResolvesOwnBrain(t *testing.T) {
	f := newGuardFixture(t)
	got, err := f.svc.GuardGroupBrain(context.Background(), f.member, f.group)
	if err != nil || got != f.brain {
		t.Fatalf("got %q err %v want %q", got, err, f.brain)
	}
}

func TestGuardGroupBrainRejectsUnauthorizedActors(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	cases := map[string]string{
		"nonmember": uuid.NewString(),
		"pending":   f.pendingAgent,
		"removed":   f.removed,
		"cross_org": f.crossOrgAgent,
		"suspended": f.suspendedAgent,
	}
	for name, actor := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.svc.GuardGroupBrain(ctx, actor, f.group); !errors.Is(err, ErrGroupBrainUnavailable) {
				t.Fatalf("%s: got %v", name, err)
			}
		})
	}
}

func TestGuardGroupBrainRejectsArchivedGroup(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	f.exec(`UPDATE groups SET archived_at=now() WHERE id=$1`, f.group)
	if _, err := f.svc.GuardGroupBrain(ctx, f.member, f.group); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatalf("archived group admitted: %v", err)
	}
}

func TestGuardGroupBrainRejectsPublicGroup(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	f.exec(`UPDATE groups SET visibility='public' WHERE id=$1`, f.group)
	if _, err := f.svc.GuardGroupBrain(ctx, f.member, f.group); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatalf("public group admitted: %v", err)
	}
}

// A row that bypasses CreateBinding (e.g. legacy/corrupted data) and points at
// the global brain must still never resolve through the private guard path.
func TestGuardGroupBrainRejectsScopeMismatch(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	globalBrain := uuid.NewString()
	f.exec(`INSERT INTO knowledge_brains(id,scope) VALUES($1,'global_web_findings')`, globalBrain)
	otherGroup := uuid.NewString()
	f.exec(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'private-group-2','private')`, otherGroup, f.orgA)
	f.exec(`INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'owner','accepted')`, otherGroup, f.member)
	f.exec(`INSERT INTO group_knowledge_bindings(group_id,brain_id) VALUES($1,$2)`, otherGroup, globalBrain)
	if _, err := f.svc.GuardGroupBrain(ctx, f.member, otherGroup); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatalf("global-scoped brain admitted via private guard: %v", err)
	}
}

func TestGuardGroupBrainNeverResolvesAnotherGroupsBrain(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	groupB := uuid.NewString()
	f.exec(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'group-b','private')`, groupB, f.orgB)
	memberB := uuid.NewString()
	f.exec(`INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'memberb')`, memberB, f.orgB)
	f.exec(`INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'owner','accepted')`, groupB, memberB)
	brainB := uuid.NewString()
	if err := f.svc.CreateBinding(ctx, groupB, brainB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GuardGroupBrain(ctx, f.member, groupB); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatal("group A member resolved group B's brain")
	}
	if _, err := f.svc.GuardGroupBrain(ctx, memberB, f.group); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatal("group B member resolved group A's brain")
	}
	if got, err := f.svc.GuardGroupBrain(ctx, memberB, groupB); err != nil || got != brainB {
		t.Fatalf("own brain resolution broke: %v %v", got, err)
	}
}

// The resolver never accepts a caller-asserted brain ID; RecheckGroupBrain must
// reject when a supposed prior selection does not match the server-resolved one.
// This is the injection risk the design is meant to close: only a server-derived
// ID may ever reach a backend.
func TestRecheckGroupBrainRejectsMismatchedBrain(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	if err := f.svc.RecheckGroupBrain(ctx, f.member, f.group, uuid.NewString()); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatal("mismatched brain accepted by recheck")
	}
	if err := f.svc.RecheckGroupBrain(ctx, f.member, f.group, f.brain); err != nil {
		t.Fatalf("matching brain rejected: %v", err)
	}
}

func TestCreateBindingEnforcesUniqueness(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	if err := f.svc.CreateBinding(ctx, f.group, uuid.NewString()); !errors.Is(err, ErrBindingRejected) {
		t.Fatalf("second brain admitted for already-bound group: %v", err)
	}
	otherGroup := uuid.NewString()
	f.exec(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'other','private')`, otherGroup, f.orgA)
	if err := f.svc.CreateBinding(ctx, otherGroup, f.brain); !errors.Is(err, ErrBindingRejected) {
		t.Fatalf("second group admitted for already-bound brain: %v", err)
	}
}

func TestCreateBindingForbidsExistingGlobalBrain(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	global := uuid.NewString()
	f.exec(`INSERT INTO knowledge_brains(id,scope) VALUES($1,'global_web_findings')`, global)
	otherGroup := uuid.NewString()
	f.exec(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'other-2','private')`, otherGroup, f.orgA)
	if err := f.svc.CreateBinding(ctx, otherGroup, global); !errors.Is(err, ErrBindingRejected) {
		t.Fatalf("existing global brain admitted as private: %v", err)
	}
}

func TestCreateBindingCannotResurrectRevokedGrant(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	if err := f.svc.RevokeBinding(ctx, f.group); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.CreateBinding(ctx, f.group, uuid.NewString()); !errors.Is(err, ErrBindingRejected) {
		t.Fatalf("revoked group rebound to a fresh brain: %v", err)
	}
	otherGroup := uuid.NewString()
	f.exec(`INSERT INTO groups(id,org_id,name,visibility) VALUES($1,$2,'other-3','private')`, otherGroup, f.orgA)
	if err := f.svc.CreateBinding(ctx, otherGroup, f.brain); !errors.Is(err, ErrBindingRejected) {
		t.Fatalf("revoked brain rebound to a new group: %v", err)
	}
	if _, err := f.svc.GuardGroupBrain(ctx, f.member, f.group); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatalf("revoked binding still resolves: %v", err)
	}
}

// GuardGroupBrain/RecheckGroupBrain must hold no lock across an outbound call:
// a revoke started while a "callback" is in flight must commit immediately, and
// the recheck afterward must suppress the already-resolved result.
func TestGuardGroupBrainSuppressesResultWhenRevokedDuringCallback(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	brainID, err := f.svc.GuardGroupBrain(ctx, f.member, f.group)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- f.svc.RevokeBinding(context.Background(), f.group) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("revoke blocked behind an in-flight resolution")
	}
	if err := f.svc.RecheckGroupBrain(ctx, f.member, f.group, brainID); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatalf("stale result not suppressed after revoke: %v", err)
	}
}

func TestGuardGroupBrainSuppressesResultWhenMembershipRemovedDuringCallback(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	// A non-owner accepted member: preserve_group_owner blocks removing the
	// sole owner, which is irrelevant to what this test exercises.
	leaver := uuid.NewString()
	f.exec(`INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'leaver')`, leaver, f.orgA)
	f.exec(`INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'member','accepted')`, f.group, leaver)
	brainID, err := f.svc.GuardGroupBrain(ctx, leaver, f.group)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := f.pool.Exec(context.Background(), `DELETE FROM group_members WHERE group_id=$1 AND agent_id=$2`, f.group, leaver)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("membership removal blocked behind an in-flight resolution")
	}
	if err := f.svc.RecheckGroupBrain(ctx, leaver, f.group, brainID); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatalf("stale result not suppressed after membership removal: %v", err)
	}
}

// An uncommitted membership grant must never be usable: concurrent binding and
// membership changes cannot manufacture an authorized read before they commit.
func TestGuardGroupBrainDoesNotSeeUncommittedMembership(t *testing.T) {
	f := newGuardFixture(t)
	ctx := context.Background()
	outsider := uuid.NewString()
	f.exec(`INSERT INTO agents(id,org_id,handle) VALUES($1,$2,'outsider')`, outsider, f.orgA)
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO group_members(group_id,agent_id,role,state) VALUES($1,$2,'member','accepted')`, f.group, outsider); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GuardGroupBrain(ctx, outsider, f.group); !errors.Is(err, ErrGroupBrainUnavailable) {
		t.Fatal("uncommitted membership grant admitted")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GuardGroupBrain(ctx, outsider, f.group); err != nil {
		t.Fatalf("committed membership not admitted: %v", err)
	}
}
