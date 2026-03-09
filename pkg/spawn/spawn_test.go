package spawn

import (
	"context"
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/authority"
	"github.com/lovyou-ai/hive/pkg/roles"
)

type testSigner struct{}

func (s *testSigner) Sign(data []byte) (types.Signature, error) {
	return types.NewSignature(make([]byte, 64))
}

func testSpawner(t *testing.T, approver authority.Approver) (*Spawner, types.ActorID) {
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
	gate := authority.NewGate(approver)

	spawner := NewSpawner(Config{
		Store:   s,
		Actors:  actors,
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

	// Check that lifecycle events were emitted with correct actions.
	actedType := types.MustEventType("agent.acted")
	page, err := spawner.store.ByType(actedType, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatal(err)
	}
	foundSpawnAgent := false
	foundSpawnRequested := false
	for _, ev := range page.Items() {
		if acted, ok := ev.Content().(event.AgentActedContent); ok {
			switch acted.Action {
			case "spawn_agent":
				foundSpawnAgent = true
			case "spawn_requested":
				foundSpawnRequested = true
			}
		}
	}
	if !foundSpawnRequested {
		t.Error("expected agent.acted event with Action=spawn_requested")
	}
	if !foundSpawnAgent {
		t.Error("expected agent.acted event with Action=spawn_agent")
	}

	roleType := types.MustEventType("agent.role.assigned")
	rolePage, err := spawner.store.ByType(roleType, 10, types.None[types.Cursor]())
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
		if gate <= 0 || gate > 1.0 {
			t.Errorf("TrustGate(%s) = %f, want (0, 1]", r, gate)
		}
	}

	// Integrator should have the highest gate.
	if roles.TrustGate(roles.RoleIntegrator) < roles.TrustGate(roles.RoleBuilder) {
		t.Error("Integrator should require higher trust than Builder")
	}
}

// Verify derivePublicKey produces valid Ed25519 keys.
func TestDerivePublicKeyValid(t *testing.T) {
	pub := DerivePublicKey("agent:valid-test")
	if len(pub) != ed25519.PublicKeySize {
		t.Errorf("key size = %d, want %d", len(pub), ed25519.PublicKeySize)
	}
}
