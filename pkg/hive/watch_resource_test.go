package hive

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

type watchResourceSigner struct{ key ed25519.PrivateKey }

func (s *watchResourceSigner) Sign(data []byte) (types.Signature, error) {
	return types.NewSignature(ed25519.Sign(s.key, data))
}

// codex r1 #2: findBudgetForRole resolves the iteration grant the Allocator
// assigned to a newly approved role. A duration renewal with a matching
// AgentName must not satisfy that lookup — its NewBudget is MINUTES and
// would be read as the spawned agent's iteration budget.
func TestFindBudgetForRoleSkipsDurationAdjustments(t *testing.T) {
	h := sha256.Sum256([]byte("test:watch-resource"))
	signer := &watchResourceSigner{key: ed25519.NewKeyFromSeed(h[:])}
	pub := signer.key.Public().(ed25519.PublicKey)
	pk, err := types.NewPublicKey([]byte(pub))
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	as := actor.NewInMemoryActorStore()
	a, err := as.Register(pk, "watch-resource-test", event.ActorTypeSystem)
	if err != nil {
		t.Fatalf("register actor: %v", err)
	}
	source := a.ID()
	convID, err := types.NewConversationID("conv_test_watch_resource")
	if err != nil {
		t.Fatalf("conv id: %v", err)
	}

	s := store.NewInMemoryStore()
	registry := event.NewEventTypeRegistry()
	registry.Register(event.EventTypeAgentBudgetAdjusted, nil)
	registry.Register(types.MustEventType("system.bootstrapped"), nil)
	factory := event.NewEventFactory(registry)
	bf := event.NewBootstrapFactory(registry)
	genesis, err := bf.Init(source, signer)
	if err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if _, err := s.Append(genesis); err != nil {
		t.Fatalf("append genesis: %v", err)
	}
	appendBudget := func(content event.AgentBudgetAdjustedContent) {
		head, err := s.Head()
		if err != nil {
			t.Fatalf("head: %v", err)
		}
		var causes []types.EventID
		if head.IsSome() {
			causes = []types.EventID{head.Unwrap().ID()}
		}
		ev, err := factory.Create(event.EventTypeAgentBudgetAdjusted, source, content, causes, convID, s, signer)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := s.Append(ev); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	r := &Runtime{store: s}

	// A duration renewal alone must NOT resolve as the role's grant.
	appendBudget(event.AgentBudgetAdjustedContent{
		AgentName: "researcher", Action: "set", NewBudget: 240, Resource: "duration",
	})
	if c, found := r.findBudgetForRole("researcher"); found {
		t.Fatalf("findBudgetForRole resolved a duration renewal as the role grant (%+v); minutes are not an iteration budget", c)
	}

	// The iteration grant resolves.
	appendBudget(event.AgentBudgetAdjustedContent{
		AgentName: "researcher", Action: "set", NewBudget: 50, Resource: "iterations",
	})
	c, found := r.findBudgetForRole("researcher")
	if !found {
		t.Fatal("findBudgetForRole missed the iteration grant")
	}
	if c.NewBudget != 50 {
		t.Fatalf("resolved grant NewBudget = %d; want 50", c.NewBudget)
	}
}
