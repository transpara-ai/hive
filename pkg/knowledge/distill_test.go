package knowledge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// ---------------------------------------------------------------------------
// Mock emitter
// ---------------------------------------------------------------------------

type mockEmitter struct {
	emitted []event.EventContent
}

func (m *mockEmitter) Emit(_ types.EventType, content event.EventContent) error {
	m.emitted = append(m.emitted, content)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newDistillerTestStore extends testEventStore with helpers for health
// and budget events.
func newDistillerTestStore(t *testing.T) *testEventStore {
	t.Helper()
	te := newTestEventStore(t)
	// Register health and agent event types.
	registry := event.NewEventTypeRegistry()
	for _, et := range event.AllKnowledgeEventTypes() {
		registry.Register(et, nil)
	}
	registry.Register(event.EventTypeHealthReport, nil)
	registry.Register(event.EventTypeAgentBudgetAdjusted, nil)
	registry.Register(types.MustEventType("system.bootstrapped"), nil)
	te.factory = event.NewEventFactory(registry)
	return te
}

func (te *testEventStore) appendHealthReport(overall float64) {
	te.t.Helper()
	score, err := types.NewScore(overall)
	require.NoError(te.t, err)
	te.appendEvent(event.EventTypeHealthReport, event.HealthReportContent{
		Overall:        score,
		ChainIntegrity: true,
		ActiveActors:   4,
		EventRate:      10.0,
	})
}

func (te *testEventStore) appendBudgetAdjustment(agentID types.ActorID, agentName, action string, delta int) {
	te.t.Helper()
	te.appendEvent(event.EventTypeAgentBudgetAdjusted, event.AgentBudgetAdjustedContent{
		AgentID:        agentID,
		AgentName:      agentName,
		Action:         action,
		PreviousBudget: 100,
		NewBudget:      100 + delta,
		Delta:          delta,
		Reason:         "test adjustment",
		PoolRemaining:  500,
	})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestDistiller_Dedup(t *testing.T) {
	te := newDistillerTestStore(t)
	// Create enough severity events to trigger the health correlation detector.
	for i := 0; i < 5; i++ {
		te.appendHealthReport(0.5)
	}

	ks := NewStore()
	emitter := &mockEmitter{}
	d := NewDistiller(te.store, ks, emitter, time.Minute)

	// First run produces insights.
	d.runAllDetectors()
	firstCount := len(emitter.emitted)
	assert.Greater(t, firstCount, 0, "first run should produce insights")

	// Second run with same chain → no new insights (dedup).
	d.runAllDetectors()
	assert.Equal(t, firstCount, len(emitter.emitted), "second run should not emit duplicates")
}

func TestDistiller_HealthCorrelation_Found(t *testing.T) {
	te := newDistillerTestStore(t)
	// Create several severity events — the source actor (te.source) will
	// correlate because it's the only actor appending events.
	for i := 0; i < 5; i++ {
		te.appendHealthReport(0.3) // warning/critical
	}

	ks := NewStore()
	emitter := &mockEmitter{}
	d := NewDistiller(te.store, ks, emitter, time.Minute)

	insights := d.detectHealthCorrelations()
	assert.NotEmpty(t, insights)

	found := false
	for _, ins := range insights {
		assert.Equal(t, DomainHealth, ins.Domain)
		assert.Contains(t, ins.Source, "health-correlation")
		if ins.EvidenceCount > 0 {
			found = true
		}
	}
	assert.True(t, found, "should find at least one health correlation insight")
}

func TestDistiller_HealthCorrelation_NoPattern(t *testing.T) {
	te := newDistillerTestStore(t)
	// All healthy reports → no severity events → no pattern.
	for i := 0; i < 5; i++ {
		te.appendHealthReport(1.0)
	}

	ks := NewStore()
	emitter := &mockEmitter{}
	d := NewDistiller(te.store, ks, emitter, time.Minute)

	insights := d.detectHealthCorrelations()
	assert.Empty(t, insights)
}

func TestDistiller_BudgetEffectiveness_DiminishingReturns(t *testing.T) {
	te := newDistillerTestStore(t)
	agentID := te.source // use same actor for simplicity

	// 4 budget increases with no surrounding output events → diminishing returns.
	for i := 0; i < 4; i++ {
		te.appendBudgetAdjustment(agentID, "implementer", "increase", 25)
	}

	ks := NewStore()
	emitter := &mockEmitter{}
	d := NewDistiller(te.store, ks, emitter, time.Minute)

	insights := d.detectBudgetEffectiveness()
	assert.NotEmpty(t, insights)

	for _, ins := range insights {
		assert.Equal(t, DomainBudget, ins.Domain)
		assert.Contains(t, ins.Source, "budget-effectiveness")
		assert.Contains(t, ins.Summary, "diminishing returns")
	}
}

func TestDistiller_BudgetEffectiveness_Effective(t *testing.T) {
	te := newDistillerTestStore(t)
	// Only 2 increases → below threshold of 3, so no insight.
	agentID := te.source
	te.appendBudgetAdjustment(agentID, "implementer", "increase", 25)
	te.appendBudgetAdjustment(agentID, "implementer", "increase", 25)

	ks := NewStore()
	emitter := &mockEmitter{}
	d := NewDistiller(te.store, ks, emitter, time.Minute)

	insights := d.detectBudgetEffectiveness()
	assert.Empty(t, insights)
}

func TestDistiller_EmptyChain(t *testing.T) {
	te := newDistillerTestStore(t)
	// Only genesis event, no health or budget events.

	ks := NewStore()
	emitter := &mockEmitter{}
	d := NewDistiller(te.store, ks, emitter, time.Minute)

	health := d.detectHealthCorrelations()
	budget := d.detectBudgetEffectiveness()

	assert.Empty(t, health)
	assert.Empty(t, budget)
	// No panic is the primary assertion.
}

func TestDistiller_InsightEmission(t *testing.T) {
	te := newDistillerTestStore(t)
	for i := 0; i < 5; i++ {
		te.appendHealthReport(0.4)
	}

	ks := NewStore()
	emitter := &mockEmitter{}
	d := NewDistiller(te.store, ks, emitter, time.Minute)

	d.runAllDetectors()

	// Emitter should have received at least one insight.
	assert.NotEmpty(t, emitter.emitted, "emitter should receive insights")

	// Knowledge store should also contain the insight.
	assert.Greater(t, ks.ActiveCount(), 0, "knowledge store should have active insights")

	// The emitted content should be KnowledgeInsightContent.
	for _, c := range emitter.emitted {
		kic, ok := c.(event.KnowledgeInsightContent)
		assert.True(t, ok, "emitted content should be KnowledgeInsightContent")
		assert.NotEmpty(t, kic.InsightID)
		assert.NotEmpty(t, kic.Domain)
	}
}
