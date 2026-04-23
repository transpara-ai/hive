package knowledge

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type testSigner struct{ key ed25519.PrivateKey }

func (s *testSigner) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}

func newTestSigner() *testSigner {
	h := sha256.Sum256([]byte("test:replay"))
	priv := ed25519.NewKeyFromSeed(h[:])
	return &testSigner{key: priv}
}

func newTestConvID(t *testing.T) types.ConversationID {
	t.Helper()
	cid, err := types.NewConversationID("conv_test_replay")
	require.NoError(t, err)
	return cid
}

func newTestActorID(t *testing.T) types.ActorID {
	t.Helper()
	signer := newTestSigner()
	pub := signer.key.Public().(ed25519.PublicKey)
	pk, err := types.NewPublicKey([]byte(pub))
	require.NoError(t, err)

	as := actor.NewInMemoryActorStore()
	a, err := as.Register(pk, "test-system", event.ActorTypeSystem)
	require.NoError(t, err)
	return a.ID()
}

// testEventStore creates an in-memory store with a genesis event and returns
// a helper for appending typed knowledge events.
type testEventStore struct {
	t       *testing.T
	store   store.Store
	factory *event.EventFactory
	signer  *testSigner
	source  types.ActorID
	convID  types.ConversationID
}

func newTestEventStore(t *testing.T) *testEventStore {
	t.Helper()
	s := store.NewInMemoryStore()
	registry := event.NewEventTypeRegistry()

	// Register knowledge event types.
	for _, et := range event.AllKnowledgeEventTypes() {
		registry.Register(et, nil)
	}
	// Register bootstrap type for genesis.
	registry.Register(types.MustEventType("system.bootstrapped"), nil)

	factory := event.NewEventFactory(registry)
	signer := newTestSigner()
	source := newTestActorID(t)
	convID := newTestConvID(t)

	// Bootstrap genesis event using BootstrapFactory.
	bf := event.NewBootstrapFactory(registry)
	genesis, err := bf.Init(source, signer)
	require.NoError(t, err)
	_, err = s.Append(genesis)
	require.NoError(t, err)

	return &testEventStore{
		t:       t,
		store:   s,
		factory: factory,
		signer:  signer,
		source:  source,
		convID:  convID,
	}
}

func (te *testEventStore) appendEvent(eventType types.EventType, content event.EventContent) {
	te.t.Helper()
	head, err := te.store.Head()
	require.NoError(te.t, err)

	var causes []types.EventID
	if head.IsSome() {
		causes = []types.EventID{head.Unwrap().ID()}
	}

	ev, err := te.factory.Create(eventType, te.source, content, causes, te.convID, te.store, te.signer)
	require.NoError(te.t, err)
	_, err = te.store.Append(ev)
	require.NoError(te.t, err)
}

func (te *testEventStore) recordInsight(id, domain, summary string, confidence float64, ttl int) {
	te.appendEvent(event.EventTypeKnowledgeInsightRecorded, event.NewKnowledgeInsightContent(
		id, domain, summary,
		[]string{"implementer"},
		types.MustScore(confidence),
		5,
		SourceMemoryKeeper,
		ttl,
		types.None[string](),
	))
}

func (te *testEventStore) supersede(oldID, newID, reason string) {
	te.appendEvent(event.EventTypeKnowledgeInsightSuperseded, event.KnowledgeSupersessionContent{
		OldInsightID: oldID,
		NewInsightID: newID,
		Reason:       reason,
	})
}

func (te *testEventStore) expire(id, reason string) {
	te.appendEvent(event.EventTypeKnowledgeInsightExpired, event.KnowledgeExpirationContent{
		InsightID: id,
		Reason:    reason,
	})
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestReplayFromStore_Empty(t *testing.T) {
	te := newTestEventStore(t)
	ks := NewStore()

	err := ReplayFromStore(te.store, ks)
	require.NoError(t, err)
	assert.Equal(t, 0, ks.ActiveCount())
}

func TestReplayFromStore_RecordedInsight(t *testing.T) {
	te := newTestEventStore(t)
	te.recordInsight("ins-1", DomainHealth, "test insight", 0.8, 0)

	ks := NewStore()
	err := ReplayFromStore(te.store, ks)
	require.NoError(t, err)
	assert.Equal(t, 1, ks.ActiveCount())

	results := ks.Query(KnowledgeFilter{Role: "implementer"}, 10)
	require.Len(t, results, 1)
	assert.Equal(t, "ins-1", results[0].InsightID)
	assert.Equal(t, DomainHealth, results[0].Domain)
	assert.Equal(t, "test insight", results[0].Summary)
	assert.Equal(t, 0.8, results[0].Confidence)
	assert.True(t, results[0].Active)
}

func TestReplayFromStore_SupersededInsight(t *testing.T) {
	te := newTestEventStore(t)
	te.recordInsight("v1", DomainBudget, "old insight", 0.6, 0)
	te.recordInsight("v2", DomainBudget, "new insight", 0.9, 0)
	te.supersede("v1", "v2", "improved analysis")

	ks := NewStore()
	err := ReplayFromStore(te.store, ks)
	require.NoError(t, err)
	assert.Equal(t, 1, ks.ActiveCount())

	results := ks.Query(KnowledgeFilter{}, 10)
	require.Len(t, results, 1)
	assert.Equal(t, "v2", results[0].InsightID)
}

func TestReplayFromStore_ExpiredInsight(t *testing.T) {
	te := newTestEventStore(t)
	te.recordInsight("exp-1", DomainQuality, "transient insight", 0.5, 0)
	te.expire("exp-1", "ttl reached")

	ks := NewStore()
	err := ReplayFromStore(te.store, ks)
	require.NoError(t, err)
	assert.Equal(t, 0, ks.ActiveCount())
}

func TestConvertFromEventContent(t *testing.T) {
	now := time.Now().UTC()

	t.Run("without TTL", func(t *testing.T) {
		content := event.NewKnowledgeInsightContent(
			"ins-42", DomainArchitecture, "codebase uses hexagonal architecture",
			[]string{"planner", "implementer"},
			types.MustScore(0.95), 12,
			SourceDistillerPrefix+"arch-analyzer",
			0, types.None[string](),
		)

		insight := ConvertFromEventContent(content, now)

		assert.Equal(t, "ins-42", insight.InsightID)
		assert.Equal(t, DomainArchitecture, insight.Domain)
		assert.Equal(t, "codebase uses hexagonal architecture", insight.Summary)
		assert.Equal(t, []string{"implementer", "planner"}, insight.RelevantRoles) // sorted by constructor
		assert.Equal(t, 0.95, insight.Confidence)
		assert.Equal(t, 12, insight.EvidenceCount)
		assert.Equal(t, SourceDistillerPrefix+"arch-analyzer", insight.Source)
		assert.Equal(t, now, insight.RecordedAt)
		assert.True(t, insight.ExpiresAt.IsZero(), "no TTL → zero ExpiresAt")
		assert.True(t, insight.Active)
	})

	t.Run("with TTL", func(t *testing.T) {
		content := event.NewKnowledgeInsightContent(
			"ins-43", DomainPerformance, "latency spike in API",
			[]string{"guardian"},
			types.MustScore(0.7), 3,
			SourceOperator,
			24, types.None[string](),
		)

		insight := ConvertFromEventContent(content, now)

		expectedExpiry := now.Add(24 * time.Hour)
		assert.Equal(t, expectedExpiry, insight.ExpiresAt)
		assert.False(t, insight.ExpiresAt.IsZero())
	})
}
