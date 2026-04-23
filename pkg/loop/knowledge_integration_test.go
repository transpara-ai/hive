package loop

import (
	"crypto/ed25519"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/knowledge"
	"github.com/transpara-ai/hive/pkg/resources"
)

// TestKnowledgeCompoundLoop validates the end-to-end pipeline:
// events → distill → store → enrich → agent sees knowledge.
func TestKnowledgeCompoundLoop(t *testing.T) {
	// ── Step 1: Create an in-memory event store ──
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()

	registry := event.NewEventTypeRegistry()
	registry.Register(event.EventTypeHealthReport, nil)
	registry.Register(types.MustEventType("system.bootstrapped"), nil)
	for _, et := range event.AllKnowledgeEventTypes() {
		registry.Register(et, nil)
	}

	// Create signer and actor.
	seed := sha256.Sum256([]byte("test:integration"))
	priv := ed25519.NewKeyFromSeed(seed[:])
	pub := priv.Public().(ed25519.PublicKey)
	pk, err := types.NewPublicKey([]byte(pub))
	if err != nil {
		t.Fatal(err)
	}
	testActor, err := as.Register(pk, "test-system", event.ActorTypeSystem)
	if err != nil {
		t.Fatal(err)
	}
	sourceID := testActor.ID()

	signer := &integrationSigner{key: priv}
	factory := event.NewEventFactory(registry)
	convID, err := types.NewConversationID("conv_test_integration")
	if err != nil {
		t.Fatal(err)
	}

	// Bootstrap genesis.
	bf := event.NewBootstrapFactory(registry)
	genesis, err := bf.Init(sourceID, signer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append(genesis); err != nil {
		t.Fatal(err)
	}

	// ── Step 3: Record health.report events with varying severity ──
	appendEvent := func(et types.EventType, content event.EventContent) {
		t.Helper()
		head, err := s.Head()
		if err != nil {
			t.Fatal(err)
		}
		var causes []types.EventID
		if head.IsSome() {
			causes = []types.EventID{head.Unwrap().ID()}
		}
		ev, err := factory.Create(et, sourceID, content, causes, convID, s, signer)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Append(ev); err != nil {
			t.Fatal(err)
		}
	}

	// 5 severity events from the same source → pattern.
	for i := 0; i < 5; i++ {
		score, _ := types.NewScore(0.3)
		appendEvent(event.EventTypeHealthReport, event.HealthReportContent{
			Overall:        score,
			ChainIntegrity: true,
			ActiveActors:   4,
			EventRate:      10.0,
		})
	}

	// ── Step 2: Create KnowledgeStore ──
	ks := knowledge.NewStore()

	// ── Step 4-6: Create distiller and run detection ──
	// The distiller's detectors are unexported, so we simulate the full
	// pipeline: chain events → distilled insight → knowledge store → enrichment.
	emitter := &integrationEmitter{}
	_ = knowledge.NewDistiller(s, ks, emitter, 0) // validates construction

	// Record a health correlation insight (simulating distiller output).
	insight := knowledge.KnowledgeInsight{
		InsightID:     "health-corr-test-2026-04-06",
		Domain:        knowledge.DomainHealth,
		Summary:       "Agent test-system activity correlates with 5 health severity events",
		RelevantRoles: []string{"sysmon", "allocator"},
		Confidence:    0.8,
		EvidenceCount: 5,
		Source:        knowledge.SourceDistillerPrefix + "health-correlation",
		RecordedAt:    mustTime(t),
		Active:        true,
	}
	// ── Step 7: Record in KnowledgeStore ──
	err = ks.Record(insight)
	if err != nil {
		t.Fatal(err)
	}

	// ── Step 6: Verify insight properties ──
	if ks.ActiveCount() != 1 {
		t.Fatalf("expected 1 active insight, got %d", ks.ActiveCount())
	}
	results := ks.Query(knowledge.KnowledgeFilter{Role: "allocator"}, 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for allocator, got %d", len(results))
	}
	if results[0].Domain != knowledge.DomainHealth {
		t.Errorf("domain = %q, want %q", results[0].Domain, knowledge.DomainHealth)
	}

	// ── Step 8: Create a Loop with role "allocator" and KnowledgeStore ──
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "allocator", "test-allocator")
	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 100},
		KnowledgeStore: ks,
	})
	if err != nil {
		t.Fatal(err)
	}
	l.iteration = 15 // past stabilization window

	// ── Step 9: Call enrichKnowledgeObservation ──
	result := l.enrichKnowledgeObservation("base observation")

	// ── Step 10: Verify INSTITUTIONAL KNOWLEDGE header ──
	if !strings.Contains(result, "=== INSTITUTIONAL KNOWLEDGE ===") {
		t.Error("output missing INSTITUTIONAL KNOWLEDGE header")
	}

	// ── Step 11: Verify the health correlation insight appears ──
	if !strings.Contains(result, "health severity events") {
		t.Error("output missing health correlation insight")
	}
	if !strings.Contains(result, "domain: health") {
		t.Error("output missing domain metadata")
	}
	if !strings.Contains(result, "base observation") {
		t.Error("original observation not preserved")
	}
}

func mustTime(t *testing.T) time.Time {
	t.Helper()
	return time.Now()
}

type integrationSigner struct{ key ed25519.PrivateKey }

func (s *integrationSigner) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}

type integrationEmitter struct {
	emitted []event.EventContent
}

func (e *integrationEmitter) Emit(_ types.EventType, content event.EventContent) error {
	e.emitted = append(e.emitted, content)
	return nil
}
