package knowledge

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// distinctActorID mints an actor identity guaranteed to differ from the
// shared fixture's source actor (newTestActorID reuses the fixture signer
// seed, so its ID EQUALS the event source — BySource(target) would then
// count the budget events themselves and corrupt the effectiveness windows).
func distinctActorID(t *testing.T, seed string) types.ActorID {
	t.Helper()
	h := sha256.Sum256([]byte(seed))
	priv := ed25519.NewKeyFromSeed(h[:])
	pub := priv.Public().(ed25519.PublicKey)
	pk, err := types.NewPublicKey([]byte(pub))
	require.NoError(t, err)
	as := actor.NewInMemoryActorStore()
	a, err := as.Register(pk, "distill-target", event.ActorTypeAI)
	require.NoError(t, err)
	return a.ID()
}

// newBudgetEventStore builds an in-memory store that can append
// agent.budget.adjusted events (the shared knowledge fixture registers only
// knowledge types).
func newBudgetEventStore(t *testing.T) (store.Store, func(content event.AgentBudgetAdjustedContent)) {
	t.Helper()
	s := store.NewInMemoryStore()
	registry := event.NewEventTypeRegistry()
	registry.Register(event.EventTypeAgentBudgetAdjusted, nil)
	registry.Register(types.MustEventType("system.bootstrapped"), nil)
	factory := event.NewEventFactory(registry)
	signer := newTestSigner()
	source := newTestActorID(t)
	convID := newTestConvID(t)

	bf := event.NewBootstrapFactory(registry)
	genesis, err := bf.Init(source, signer)
	require.NoError(t, err)
	_, err = s.Append(genesis)
	require.NoError(t, err)

	appendBudget := func(content event.AgentBudgetAdjustedContent) {
		head, err := s.Head()
		require.NoError(t, err)
		var causes []types.EventID
		if head.IsSome() {
			causes = []types.EventID{head.Unwrap().ID()}
		}
		ev, err := factory.Create(event.EventTypeAgentBudgetAdjusted, source, content, causes, convID, s, signer)
		require.NoError(t, err)
		_, err = s.Append(ev)
		require.NoError(t, err)
	}
	return s, appendBudget
}

// codex r1 #2: duration renewals are MINUTES — folding them into the
// iteration-effectiveness heuristic fabricates "budget increase had low
// effect" insights about adjustments that never touched iterations.
func TestDetectBudgetEffectivenessSkipsDurationAdjustments(t *testing.T) {
	s, appendBudget := newBudgetEventStore(t)
	aid := distinctActorID(t, "test:distill-target")

	for i := 0; i < 3; i++ {
		appendBudget(event.AgentBudgetAdjustedContent{
			AgentID:   aid,
			AgentName: "implementer",
			Action:    "increase",
			Delta:     60,
			NewBudget: 120 + 60*i,
			Resource:  "duration",
		})
	}
	d := NewDistiller(s, NewStore(), nil, time.Minute)
	if insights := d.detectBudgetEffectiveness(); len(insights) != 0 {
		t.Fatalf("3 duration renewals produced %d budget-effectiveness insights; want 0 — minutes are not iterations", len(insights))
	}

	// Control: the same shape in the ITERATION dimension must still be
	// evaluated (this quiet store yields the low-effectiveness insight),
	// proving the skip is resource-gated rather than the heuristic dead.
	for i := 0; i < 3; i++ {
		appendBudget(event.AgentBudgetAdjustedContent{
			AgentID:   aid,
			AgentName: "implementer",
			Action:    "increase",
			Delta:     25,
			NewBudget: 75 + 25*i,
		})
	}
	if insights := d.detectBudgetEffectiveness(); len(insights) == 0 {
		t.Fatal("control: 3 iteration increases produced no insight; the heuristic (or this fixture) is broken")
	}
}
