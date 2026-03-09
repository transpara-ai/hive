package spawn

import (
	"context"
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/authority"
	"github.com/lovyou-ai/hive/pkg/roles"
)

type testSigner struct{}

func (s *testSigner) Sign(data []byte) (types.Signature, error) {
	return types.NewSignature(make([]byte, 64))
}

// testSpawnerOpts configures the test spawner.
type testSpawnerOpts struct {
	approver   authority.Approver
	trustModel *trust.DefaultTrustModel
}

func testSpawner(t *testing.T, approver authority.Approver) (*Spawner, types.ActorID) {
	t.Helper()
	return testSpawnerWithOpts(t, testSpawnerOpts{approver: approver})
}

func testSpawnerWithOpts(t *testing.T, opts testSpawnerOpts) (*Spawner, types.ActorID) {
	t.Helper()

	s := store.NewInMemoryStore()
	actors := actor.NewInMemoryActorStore()

	// Register human.
	humanPub, _ := types.NewPublicKey([]byte("human-key-00000000000000000000000"))
	humanActor, err := actors.Register(humanPub, "TestHuman", "Human")
	if err != nil {
		t.Fatal(err)
	}
	humanID := humanActor.ID()

	// Bootstrap graph.
	registry := event.DefaultRegistry()
	bsFactory := event.NewBootstrapFactory(registry)
	signer := &testSigner{}
	bootstrap, err := bsFactory.Init(humanID, signer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		t.Fatal(err)
	}

	factory := event.NewEventFactory(registry)
	convID, _ := types.NewConversationID("conv_test_0000000000000000000000001")
	gate := authority.NewGate(opts.approver)

	spawner := NewSpawner(Config{
		Store:   s,
		Actors:  actors,
		Trust:   opts.trustModel,
		Gate:    gate,
		HumanID: humanID,
		Signer:  signer,
		Factory: factory,
		ConvID:  convID,
	})

	return spawner, humanID
}

func TestSpawnApproved(t *testing.T) {
	spawner, humanID := testSpawner(t, func(req authority.Request) (bool, string) {
		return true, "approved"
	})

	result, err := spawner.Spawn(context.Background(), SpawnRequest{
		Role:          roles.RoleBuilder,
		Name:          "test-builder",
		Justification: "need a builder for testing",
		RequestedBy:   humanID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Approved {
		t.Fatal("expected approved")
	}
	if result.ActorID == (types.ActorID{}) {
		t.Fatal("expected non-zero actor ID")
	}
	if result.Role != roles.RoleBuilder {
		t.Errorf("role = %s, want %s", result.Role, roles.RoleBuilder)
	}
}

func TestSpawnDenied(t *testing.T) {
	spawner, humanID := testSpawner(t, func(req authority.Request) (bool, string) {
		return false, "not needed right now"
	})

	result, err := spawner.Spawn(context.Background(), SpawnRequest{
		Role:          roles.RoleBuilder,
		Name:          "test-builder",
		Justification: "testing denial",
		RequestedBy:   humanID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Approved {
		t.Fatal("expected denied")
	}
	if !strings.Contains(result.Reason, "not needed") {
		t.Errorf("reason = %q, should mention denial reason", result.Reason)
	}
}

func TestSpawnEmitsEvents(t *testing.T) {
	spawner, humanID := testSpawner(t, func(req authority.Request) (bool, string) {
		return true, "approved"
	})

	_, err := spawner.Spawn(context.Background(), SpawnRequest{
		Role:          roles.RoleTester,
		Name:          "test-tester",
		Justification: "need a tester",
		RequestedBy:   humanID,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check agent.acted events (spawn_requested).
	page, err := spawner.store.ByType(event.EventTypeAgentActed, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	foundSpawnRequested := false
	for _, ev := range page.Items() {
		if acted, ok := ev.Content().(event.AgentActedContent); ok {
			if acted.Action == "spawn_requested" {
				foundSpawnRequested = true
			}
		}
	}
	if !foundSpawnRequested {
		t.Error("expected agent.acted event with Action=spawn_requested")
	}

	// Check authority.requested event.
	authReqPage, err := spawner.store.ByType(event.EventTypeAuthorityRequested, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	if len(authReqPage.Items()) == 0 {
		t.Error("expected authority.requested event")
	}

	// Check authority.resolved event.
	authResPage, err := spawner.store.ByType(event.EventTypeAuthorityResolved, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	if len(authResPage.Items()) == 0 {
		t.Error("expected authority.resolved event")
	}

	// Check agent.identity.created event.
	identityPage, err := spawner.store.ByType(event.EventTypeAgentIdentityCreated, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	if len(identityPage.Items()) == 0 {
		t.Error("expected agent.identity.created event")
	}

	// Check agent.lifespan.started event.
	lifespanPage, err := spawner.store.ByType(event.EventTypeAgentLifespanStarted, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	if len(lifespanPage.Items()) == 0 {
		t.Error("expected agent.lifespan.started event")
	}

	// Check agent.role.assigned event.
	rolePage, err := spawner.store.ByType(event.EventTypeAgentRoleAssigned, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	if len(rolePage.Items()) == 0 {
		t.Error("expected agent.role.assigned event")
	}
}

func TestSpawnDerivePublicKey(t *testing.T) {
	// Deterministic — same seed gives same key.
	k1 := DerivePublicKey("agent:test")
	k2 := DerivePublicKey("agent:test")
	if !k1.Equal(k2) {
		t.Error("same seed should give same key")
	}

	// Different seeds give different keys.
	k3 := DerivePublicKey("agent:other")
	if k1.Equal(k3) {
		t.Error("different seeds should give different keys")
	}
}

func TestTrustGates(t *testing.T) {
	// Verify trust gates are defined for all known roles.
	knownRoles := []roles.Role{
		roles.RoleCTO, roles.RoleGuardian, roles.RoleResearcher,
		roles.RoleArchitect, roles.RoleBuilder, roles.RoleReviewer,
		roles.RoleTester, roles.RoleIntegrator, roles.RoleSysMon,
		roles.RoleSpawner, roles.RoleAllocator,
	}
	for _, r := range knownRoles {
		gate := roles.TrustGate(r)
		if gate < 0 || gate > 1.0 {
			t.Errorf("TrustGate(%s) = %f, want [0, 1]", r, gate)
		}
	}

	// Integrator should have the highest gate.
	if roles.TrustGate(roles.RoleIntegrator) < roles.TrustGate(roles.RoleBuilder) {
		t.Error("Integrator should require higher trust than Builder")
	}
}

func TestAgentInitiatedSpawnDeniedByTrustGate(t *testing.T) {
	// Agent-initiated spawn (RequestedBy != humanID) with no trust model
	// should fail with ErrTrustNotConfigured, not a silent policy denial.
	spawner, humanID := testSpawnerWithOpts(t, testSpawnerOpts{
		approver: func(req authority.Request) (bool, string) {
			t.Fatal("approver should not be called when trust gate rejects")
			return false, ""
		},
		trustModel: nil, // no trust model
	})

	// Register an agent actor to use as requester.
	agentPub := DerivePublicKey("agent:some-agent")
	agentPK, _ := types.NewPublicKey([]byte(agentPub))
	agentActor, err := spawner.actors.Register(agentPK, "some-agent", event.ActorTypeAI)
	if err != nil {
		t.Fatal(err)
	}
	_ = humanID // unused — agent requests, not human

	result, err := spawner.Spawn(context.Background(), SpawnRequest{
		Role:          roles.RoleBuilder, // trust gate 0.3
		Name:          "new-builder",
		Justification: "agent wants a builder",
		RequestedBy:   agentActor.ID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Approved {
		t.Fatal("agent-initiated spawn without trust model should be denied")
	}
	if !strings.Contains(result.Reason, "trust model not configured") {
		t.Errorf("expected config error in reason, got %q", result.Reason)
	}
}

func TestAgentInitiatedSpawnDeniedByLowTrust(t *testing.T) {
	// Agent with default trust (0.0) should fail the trust gate for Builder (0.3).
	trustModel := trust.NewDefaultTrustModel()
	spawner, humanID := testSpawnerWithOpts(t, testSpawnerOpts{
		approver: func(req authority.Request) (bool, string) {
			t.Fatal("approver should not be called when trust gate rejects")
			return false, ""
		},
		trustModel: trustModel,
	})

	agentPub := DerivePublicKey("agent:low-trust")
	agentPK, _ := types.NewPublicKey([]byte(agentPub))
	agentActor, err := spawner.actors.Register(agentPK, "low-trust", event.ActorTypeAI)
	if err != nil {
		t.Fatal(err)
	}
	_ = humanID

	result, err := spawner.Spawn(context.Background(), SpawnRequest{
		Role:          roles.RoleBuilder, // trust gate 0.3
		Name:          "new-builder",
		Justification: "low-trust agent wants a builder",
		RequestedBy:   agentActor.ID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Approved {
		t.Fatal("low-trust agent should be denied by trust gate")
	}
	if !strings.Contains(result.Reason, "trust gate denied") {
		t.Errorf("expected trust gate denial, got %q", result.Reason)
	}
}

// Verify DerivePublicKey produces valid Ed25519 keys.
func TestDerivePublicKeyValid(t *testing.T) {
	pub := DerivePublicKey("agent:valid-test")
	if len(pub) != ed25519.PublicKeySize {
		t.Errorf("key size = %d, want %d", len(pub), ed25519.PublicKeySize)
	}
}
