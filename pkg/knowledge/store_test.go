package knowledge

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeInsight(id string, opts ...func(*KnowledgeInsight)) KnowledgeInsight {
	ins := KnowledgeInsight{
		InsightID:     id,
		Domain:        DomainHealth,
		Summary:       "test insight " + id,
		RelevantRoles: []string{"implementer"},
		Confidence:    0.5,
		EvidenceCount: 3,
		Source:        SourceMemoryKeeper,
		RecordedAt:    time.Now(),
		Active:        true,
	}
	for _, fn := range opts {
		fn(&ins)
	}
	return ins
}

func withRoles(roles ...string) func(*KnowledgeInsight) {
	return func(ins *KnowledgeInsight) { ins.RelevantRoles = roles }
}

func withConfidence(c float64) func(*KnowledgeInsight) {
	return func(ins *KnowledgeInsight) { ins.Confidence = c }
}

func withDomain(d string) func(*KnowledgeInsight) {
	return func(ins *KnowledgeInsight) { ins.Domain = d }
}

func withRecordedAt(t time.Time) func(*KnowledgeInsight) {
	return func(ins *KnowledgeInsight) { ins.RecordedAt = t }
}

func withEvidence(n int) func(*KnowledgeInsight) {
	return func(ins *KnowledgeInsight) { ins.EvidenceCount = n }
}

func withExpiry(t time.Time) func(*KnowledgeInsight) {
	return func(ins *KnowledgeInsight) { ins.ExpiresAt = t }
}

// ---------------------------------------------------------------------------
// Record and Query
// ---------------------------------------------------------------------------

func TestRecord_Basic(t *testing.T) {
	s := NewStore()

	err := s.Record(makeInsight("ins-1", withRoles("implementer")))
	require.NoError(t, err)
	assert.Equal(t, 1, s.ActiveCount())

	// Matching role returns it.
	results := s.Query(KnowledgeFilter{Role: "implementer"}, 10)
	assert.Len(t, results, 1)
	assert.Equal(t, "ins-1", results[0].InsightID)

	// Non-matching role returns empty.
	results = s.Query(KnowledgeFilter{Role: "guardian"}, 10)
	assert.Empty(t, results)
}

func TestRecord_UniversalInsight(t *testing.T) {
	s := NewStore()

	err := s.Record(makeInsight("uni-1", withRoles()))
	require.NoError(t, err)

	// Any role should match a universal insight.
	for _, role := range []string{"implementer", "guardian", "strategist", ""} {
		results := s.Query(KnowledgeFilter{Role: role}, 10)
		assert.Len(t, results, 1, "role=%q should match universal insight", role)
	}
}

func TestQuery_MinConfidence(t *testing.T) {
	s := NewStore()

	require.NoError(t, s.Record(makeInsight("low", withConfidence(0.2), withRoles())))
	require.NoError(t, s.Record(makeInsight("mid", withConfidence(0.5), withRoles())))
	require.NoError(t, s.Record(makeInsight("high", withConfidence(0.8), withRoles())))

	results := s.Query(KnowledgeFilter{MinConfidence: 0.3}, 10)
	assert.Len(t, results, 2)

	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.InsightID] = true
	}
	assert.True(t, ids["mid"])
	assert.True(t, ids["high"])
	assert.False(t, ids["low"])
}

func TestQuery_MaxAge(t *testing.T) {
	s := NewStore()

	require.NoError(t, s.Record(makeInsight("recent", withRoles(), withRecordedAt(time.Now()))))
	require.NoError(t, s.Record(makeInsight("old", withRoles(), withRecordedAt(time.Now().Add(-48*time.Hour)))))

	results := s.Query(KnowledgeFilter{MaxAge: 24 * time.Hour}, 10)
	assert.Len(t, results, 1)
	assert.Equal(t, "recent", results[0].InsightID)
}

func TestQuery_DomainFilter(t *testing.T) {
	s := NewStore()

	require.NoError(t, s.Record(makeInsight("h", withRoles(), withDomain(DomainHealth))))
	require.NoError(t, s.Record(makeInsight("b", withRoles(), withDomain(DomainBudget))))
	require.NoError(t, s.Record(makeInsight("q", withRoles(), withDomain(DomainQuality))))

	results := s.Query(KnowledgeFilter{Domains: []string{DomainHealth, DomainBudget}}, 10)
	assert.Len(t, results, 2)

	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.InsightID] = true
	}
	assert.True(t, ids["h"])
	assert.True(t, ids["b"])
}

func TestQuery_ExcludeIDs(t *testing.T) {
	s := NewStore()

	require.NoError(t, s.Record(makeInsight("a", withRoles())))
	require.NoError(t, s.Record(makeInsight("b", withRoles())))
	require.NoError(t, s.Record(makeInsight("c", withRoles())))

	results := s.Query(KnowledgeFilter{ExcludeIDs: []string{"b"}}, 10)
	assert.Len(t, results, 2)

	for _, r := range results {
		assert.NotEqual(t, "b", r.InsightID)
	}
}

func TestQuery_MaxResults(t *testing.T) {
	s := NewStore()

	for i := 0; i < 10; i++ {
		require.NoError(t, s.Record(makeInsight(fmt.Sprintf("ins-%d", i), withRoles())))
	}

	results := s.Query(KnowledgeFilter{}, 3)
	assert.Len(t, results, 3)
}

func TestQuery_RelevanceOrder(t *testing.T) {
	s := NewStore()

	// Create insights with varying scores: high-confidence explicit role match
	// should rank above low-confidence universal.
	require.NoError(t, s.Record(makeInsight("low-score",
		withRoles(),          // universal = 0.2 role score
		withConfidence(0.1),  // 0.03 confidence
		withEvidence(1),      // 0.03 evidence
		withRecordedAt(time.Now().Add(-25*time.Hour)), // 0.05 recency
	)))
	require.NoError(t, s.Record(makeInsight("high-score",
		withRoles("planner"), // explicit = 0.4 role score
		withConfidence(0.9),  // 0.27 confidence
		withEvidence(15),     // 0.1 evidence
		withRecordedAt(time.Now()), // 0.2 recency
	)))

	results := s.Query(KnowledgeFilter{Role: "planner"}, 10)
	require.Len(t, results, 2)
	assert.Equal(t, "high-score", results[0].InsightID)
	assert.Equal(t, "low-score", results[1].InsightID)
}

// ---------------------------------------------------------------------------
// Relevance Scoring
// ---------------------------------------------------------------------------

func TestRelevanceScore_RoleMatch(t *testing.T) {
	filter := KnowledgeFilter{Role: "implementer"}
	now := time.Now()

	explicit := relevanceScore(KnowledgeInsight{
		RelevantRoles: []string{"implementer"},
		RecordedAt:    now,
	}, filter)

	universal := relevanceScore(KnowledgeInsight{
		RelevantRoles: nil,
		RecordedAt:    now,
	}, filter)

	// Isolate the role component by comparing the difference.
	// Both have the same recency/confidence/evidence, so the delta is pure role.
	assert.InDelta(t, 0.2, explicit-universal, 0.001, "explicit role match should add 0.2 more than universal")
}

func TestRelevanceScore_Confidence(t *testing.T) {
	filter := KnowledgeFilter{Role: "x"}
	now := time.Now()

	base := func(c float64) float64 {
		return relevanceScore(KnowledgeInsight{
			RelevantRoles: []string{"x"},
			Confidence:    c,
			RecordedAt:    now,
		}, filter)
	}

	full := base(1.0)
	half := base(0.5)

	// Confidence component = Confidence * 0.3. Delta between 1.0 and 0.5 = 0.15.
	assert.InDelta(t, 0.15, full-half, 0.001)
}

func TestRelevanceScore_Recency(t *testing.T) {
	filter := KnowledgeFilter{Role: "x"}
	base := KnowledgeInsight{
		RelevantRoles: []string{"x"},
		Confidence:    0.5,
		EvidenceCount: 3,
	}

	score := func(age time.Duration) float64 {
		ins := base
		ins.RecordedAt = time.Now().Add(-age)
		return relevanceScore(ins, filter)
	}

	s30m := score(30 * time.Minute)
	s3h := score(3 * time.Hour)
	s12h := score(12 * time.Hour)
	s2d := score(48 * time.Hour)

	// Recency tiers: 0.2, 0.15, 0.1, 0.05. Verify ordering and deltas.
	assert.Greater(t, s30m, s3h, "30min should score higher than 3h")
	assert.Greater(t, s3h, s12h, "3h should score higher than 12h")
	assert.Greater(t, s12h, s2d, "12h should score higher than 2d")
	assert.InDelta(t, 0.05, s30m-s3h, 0.001)
	assert.InDelta(t, 0.05, s3h-s12h, 0.001)
	assert.InDelta(t, 0.05, s12h-s2d, 0.001)
}

func TestRelevanceScore_Evidence(t *testing.T) {
	filter := KnowledgeFilter{Role: "x"}
	now := time.Now()

	score := func(ev int) float64 {
		return relevanceScore(KnowledgeInsight{
			RelevantRoles: []string{"x"},
			Confidence:    0.5,
			EvidenceCount: ev,
			RecordedAt:    now,
		}, filter)
	}

	s15 := score(15)
	s7 := score(7)
	s2 := score(2)

	assert.InDelta(t, 0.03, s15-s7, 0.001, "15 vs 7 evidence delta")
	assert.InDelta(t, 0.04, s7-s2, 0.001, "7 vs 2 evidence delta")
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func TestSupersede(t *testing.T) {
	s := NewStore()

	require.NoError(t, s.Record(makeInsight("old", withRoles())))
	require.NoError(t, s.Record(makeInsight("new", withRoles())))
	require.NoError(t, s.Supersede("old", "new"))

	assert.Equal(t, 1, s.ActiveCount())

	results := s.Query(KnowledgeFilter{}, 10)
	require.Len(t, results, 1)
	assert.Equal(t, "new", results[0].InsightID)
}

func TestExpire(t *testing.T) {
	s := NewStore()

	require.NoError(t, s.Record(makeInsight("exp-1",
		withRoles(),
		withExpiry(time.Now().Add(-1*time.Hour)),
	)))
	assert.Equal(t, 1, s.ActiveCount())

	pruned := s.PruneExpired()
	assert.Equal(t, 1, pruned)
	assert.Equal(t, 0, s.ActiveCount())
}

func TestPruneExpired_MixedTTL(t *testing.T) {
	s := NewStore()

	// Permanent (no expiry).
	require.NoError(t, s.Record(makeInsight("permanent", withRoles())))

	// Already expired.
	require.NoError(t, s.Record(makeInsight("expired",
		withRoles(),
		withExpiry(time.Now().Add(-1*time.Hour)),
	)))

	// Not yet expired.
	require.NoError(t, s.Record(makeInsight("future",
		withRoles(),
		withExpiry(time.Now().Add(24*time.Hour)),
	)))

	assert.Equal(t, 3, s.ActiveCount())

	pruned := s.PruneExpired()
	assert.Equal(t, 1, pruned)
	assert.Equal(t, 2, s.ActiveCount())
}

func TestRecord_WithSupersession(t *testing.T) {
	s := NewStore()

	require.NoError(t, s.Record(makeInsight("v1", withRoles())))
	require.NoError(t, s.Record(makeInsight("v2", withRoles())))

	// Supersede v1 with v2 immediately after recording v2.
	require.NoError(t, s.Supersede("v1", "v2"))

	assert.Equal(t, 1, s.ActiveCount())

	results := s.Query(KnowledgeFilter{}, 10)
	require.Len(t, results, 1)
	assert.Equal(t, "v2", results[0].InsightID)
}

// ---------------------------------------------------------------------------
// Cardinality
// ---------------------------------------------------------------------------

func TestCardinality_MaxActive(t *testing.T) {
	s := NewStore()

	// Record MaxActiveInsights + 5 insights with ascending confidence.
	total := MaxActiveInsights + 5
	for i := 0; i < total; i++ {
		conf := float64(i) / float64(total)
		require.NoError(t, s.Record(makeInsight(
			fmt.Sprintf("ins-%d", i),
			withRoles(),
			withConfidence(conf),
		)))
	}

	assert.Equal(t, MaxActiveInsights, s.ActiveCount())

	// The 5 lowest-confidence insights (ins-0 through ins-4) should be inactive.
	results := s.Query(KnowledgeFilter{}, 0)
	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.InsightID] = true
	}
	for i := 0; i < 5; i++ {
		assert.False(t, ids[fmt.Sprintf("ins-%d", i)],
			"ins-%d should have been evicted (low confidence)", i)
	}
}

// ---------------------------------------------------------------------------
// Thread Safety
// ---------------------------------------------------------------------------

func TestConcurrentAccess(t *testing.T) {
	s := NewStore()

	var wg sync.WaitGroup
	const goroutines = 10

	// 5 writers.
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = s.Record(makeInsight(
					fmt.Sprintf("w%d-%d", id, j),
					withRoles(),
					withConfidence(float64(j)/50.0),
				))
			}
		}(i)
	}

	// 5 readers.
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = s.Query(KnowledgeFilter{Role: "implementer"}, 10)
				_ = s.ActiveCount()
				_ = s.PruneExpired()
			}
		}()
	}

	wg.Wait()

	// If we got here without a panic or race detector complaint, the test passes.
	assert.True(t, s.ActiveCount() <= MaxActiveInsights,
		"active count %d should not exceed max %d", s.ActiveCount(), MaxActiveInsights)
}
