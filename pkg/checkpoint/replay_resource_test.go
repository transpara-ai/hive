package checkpoint

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

type budgetReplaySigner struct{ key ed25519.PrivateKey }

func (s *budgetReplaySigner) Sign(data []byte) (types.Signature, error) {
	return types.NewSignature(ed25519.Sign(s.key, data))
}

// newBudgetReplayStore builds an in-memory store seeded with a genesis event
// and returns an appender for agent.budget.adjusted events.
func newBudgetReplayStore(t *testing.T) (store.Store, func(content event.AgentBudgetAdjustedContent)) {
	t.Helper()
	h := sha256.Sum256([]byte("test:budget-replay"))
	signer := &budgetReplaySigner{key: ed25519.NewKeyFromSeed(h[:])}
	pub := signer.key.Public().(ed25519.PublicKey)
	pk, err := types.NewPublicKey([]byte(pub))
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	as := actor.NewInMemoryActorStore()
	a, err := as.Register(pk, "budget-replay-test", event.ActorTypeSystem)
	if err != nil {
		t.Fatalf("register actor: %v", err)
	}
	source := a.ID()
	convID, err := types.NewConversationID("conv_test_budget_replay")
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
			t.Fatalf("create budget event: %v", err)
		}
		if _, err := s.Append(ev); err != nil {
			t.Fatalf("append budget event: %v", err)
		}
	}
	return s, appendBudget
}

// codex r1 #2, the sharpest consequence: ReplayBudgetFromStore seeds
// recovered iteration budgets after a reboot. A duration renewal replayed as
// CurrentBudget would hand a rebooted agent a MINUTES number as its
// iteration budget — corrupting exactly the crash-recovery path round 4
// just exercised for real.
func TestReplayBudgetFromStoreSkipsDurationAdjustments(t *testing.T) {
	s, appendBudget := newBudgetReplayStore(t)

	appendBudget(event.AgentBudgetAdjustedContent{
		AgentName: "implementer", Action: "set", NewBudget: 100,
	})
	appendBudget(event.AgentBudgetAdjustedContent{
		AgentName: "implementer", Action: "set", NewBudget: 120, Resource: "duration",
	})

	result, err := ReplayBudgetFromStore(s)
	if err != nil {
		t.Fatalf("ReplayBudgetFromStore: %v", err)
	}
	state, ok := result["implementer"]
	if !ok {
		t.Fatal("implementer missing from replayed budgets")
	}
	if state.CurrentBudget != 100 {
		t.Fatalf("CurrentBudget = %d; want 100 — the later duration renewal (120 MINUTES) must not overwrite the iteration budget", state.CurrentBudget)
	}
	if state.AdjustmentCount != 1 {
		t.Fatalf("AdjustmentCount = %d; want 1 (iteration adjustments only)", state.AdjustmentCount)
	}
}
