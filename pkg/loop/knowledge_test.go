package loop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/knowledge"
	"github.com/lovyou-ai/hive/pkg/resources"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func knowledgeLoop(t *testing.T, role string, ks knowledge.KnowledgeStore) *Loop {
	t.Helper()
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, role, "test-"+role)
	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 100},
		KnowledgeStore: ks,
	})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func recordTestInsight(t *testing.T, ks knowledge.KnowledgeStore, id, domain, summary string, roles []string, confidence float64) {
	t.Helper()
	err := ks.Record(knowledge.KnowledgeInsight{
		InsightID:     id,
		Domain:        domain,
		Summary:       summary,
		RelevantRoles: roles,
		Confidence:    confidence,
		EvidenceCount: 10,
		Source:        knowledge.SourceMemoryKeeper,
		RecordedAt:    time.Now(),
		Active:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestEnrichKnowledge_NilStore(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "guardian", "test-guardian")
	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 100},
		KnowledgeStore: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	l.iteration = 15

	obs := "some observation"
	result := l.enrichKnowledgeObservation(obs)
	if result != obs {
		t.Errorf("nil store should return obs unchanged, got %q", result)
	}
}

func TestEnrichKnowledge_StabilizationWindow(t *testing.T) {
	ks := knowledge.NewStore()
	recordTestInsight(t, ks, "ins-1", knowledge.DomainHealth, "test", nil, 0.9)

	l := knowledgeLoop(t, "guardian", ks)
	l.iteration = 3

	obs := "some observation"
	result := l.enrichKnowledgeObservation(obs)
	if result != obs {
		t.Errorf("iteration < 10 should return obs unchanged, got %q", result)
	}
}

func TestEnrichKnowledge_NoInsights(t *testing.T) {
	ks := knowledge.NewStore()
	l := knowledgeLoop(t, "guardian", ks)
	l.iteration = 15

	obs := "some observation"
	result := l.enrichKnowledgeObservation(obs)
	if result != obs {
		t.Errorf("empty store should return obs unchanged, got %q", result)
	}
}

func TestEnrichKnowledge_HasInsights(t *testing.T) {
	ks := knowledge.NewStore()
	recordTestInsight(t, ks, "ins-1", knowledge.DomainHealth, "Memory pressure rising", nil, 0.9)
	recordTestInsight(t, ks, "ins-2", knowledge.DomainBudget, "Token costs stable", nil, 0.8)
	recordTestInsight(t, ks, "ins-3", knowledge.DomainQuality, "Test coverage at 85%", nil, 0.7)

	l := knowledgeLoop(t, "implementer", ks)
	l.iteration = 15

	result := l.enrichKnowledgeObservation("base obs")

	if !strings.Contains(result, "=== INSTITUTIONAL KNOWLEDGE ===") {
		t.Error("missing INSTITUTIONAL KNOWLEDGE header")
	}
	if !strings.Contains(result, "Memory pressure rising") {
		t.Error("missing insight 1 summary")
	}
	if !strings.Contains(result, "Token costs stable") {
		t.Error("missing insight 2 summary")
	}
	if !strings.Contains(result, "Test coverage at 85%") {
		t.Error("missing insight 3 summary")
	}
	if !strings.Contains(result, "===\n") {
		t.Error("missing closing === footer")
	}
	if !strings.HasPrefix(result, "base obs") {
		t.Error("original observation not preserved at start")
	}
}

func TestEnrichKnowledge_RoleFiltering(t *testing.T) {
	ks := knowledge.NewStore()
	recordTestInsight(t, ks, "alloc-1", knowledge.DomainBudget, "Budget insight for allocator", []string{"allocator"}, 0.8)
	recordTestInsight(t, ks, "cto-1", knowledge.DomainArchitecture, "Architecture insight for cto", []string{"cto"}, 0.8)

	l := knowledgeLoop(t, "allocator", ks)
	l.iteration = 15

	result := l.enrichKnowledgeObservation("obs")

	if !strings.Contains(result, "Budget insight for allocator") {
		t.Error("allocator insight should be present")
	}
	if strings.Contains(result, "Architecture insight for cto") {
		t.Error("cto insight should NOT be present for allocator role")
	}
}

func TestEnrichKnowledge_UniversalInsights(t *testing.T) {
	ks := knowledge.NewStore()
	// Empty RelevantRoles = universal.
	recordTestInsight(t, ks, "uni-1", knowledge.DomainProcess, "Process improvement applies to all", nil, 0.8)

	l := knowledgeLoop(t, "implementer", ks)
	l.iteration = 15

	result := l.enrichKnowledgeObservation("obs")
	if !strings.Contains(result, "Process improvement applies to all") {
		t.Error("universal insight should appear for any role")
	}
}

func TestEnrichKnowledge_MaxItems(t *testing.T) {
	ks := knowledge.NewStore()
	for i := 0; i < 10; i++ {
		recordTestInsight(t, ks, fmt.Sprintf("ins-%d", i), knowledge.DomainHealth,
			fmt.Sprintf("Insight number %d", i), nil, 0.5+float64(i)*0.01)
	}

	l := knowledgeLoop(t, "guardian", ks)
	l.iteration = 15

	result := l.enrichKnowledgeObservation("obs")

	// Count numbered items [N].
	count := 0
	for i := 1; i <= 10; i++ {
		if strings.Contains(result, fmt.Sprintf("[%d]", i)) {
			count++
		}
	}
	if count > knowledge.MaxEnrichmentItems {
		t.Errorf("got %d items, want at most %d", count, knowledge.MaxEnrichmentItems)
	}
}

func TestEnrichKnowledge_TruncateLongSummary(t *testing.T) {
	ks := knowledge.NewStore()
	longSummary := strings.Repeat("x", 500)
	recordTestInsight(t, ks, "long-1", knowledge.DomainHealth, longSummary, nil, 0.9)

	l := knowledgeLoop(t, "guardian", ks)
	l.iteration = 15

	result := l.enrichKnowledgeObservation("obs")

	// The full 500-char string should not appear.
	if strings.Contains(result, longSummary) {
		t.Error("full 500-char summary should be truncated")
	}
	if !strings.Contains(result, "...") {
		t.Error("truncated summary should end with ...")
	}
	// The truncated prefix (MaxItemChars) should appear.
	truncated := longSummary[:knowledge.MaxItemChars]
	if !strings.Contains(result, truncated) {
		t.Error("truncated prefix should be present")
	}
}

func TestEnrichKnowledge_TotalBlockSize(t *testing.T) {
	ks := knowledge.NewStore()
	longSummary := strings.Repeat("A", 400)
	for i := 0; i < 5; i++ {
		recordTestInsight(t, ks, fmt.Sprintf("big-%d", i), knowledge.DomainHealth,
			longSummary, nil, 0.9)
	}

	l := knowledgeLoop(t, "guardian", ks)
	l.iteration = 15

	result := l.enrichKnowledgeObservation("obs")

	// The enrichment block is everything after "obs".
	block := strings.TrimPrefix(result, "obs")
	if len(block) > knowledge.MaxBlockChars {
		t.Errorf("block length %d exceeds MaxBlockChars %d", len(block), knowledge.MaxBlockChars)
	}
}

func TestEnrichKnowledge_AllRolesReceive(t *testing.T) {
	roles := []string{"guardian", "sysmon", "allocator", "cto", "implementer"}

	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			ks := knowledge.NewStore()
			recordTestInsight(t, ks, "uni-1", knowledge.DomainProcess, "universal insight", nil, 0.9)

			l := knowledgeLoop(t, role, ks)
			l.iteration = 15

			result := l.enrichKnowledgeObservation("obs")
			if !strings.Contains(result, "=== INSTITUTIONAL KNOWLEDGE ===") {
				t.Errorf("role %q did not receive knowledge block", role)
			}
			if !strings.Contains(result, "universal insight") {
				t.Errorf("role %q did not receive universal insight", role)
			}
		})
	}
}

func TestFormatKnowledgeBlock_Format(t *testing.T) {
	insights := []knowledge.KnowledgeInsight{
		{
			InsightID:     "ins-1",
			Domain:        knowledge.DomainHealth,
			Summary:       "Memory pressure correlates with Opus iterations",
			Confidence:    0.85,
			EvidenceCount: 23,
		},
		{
			InsightID:     "ins-2",
			Domain:        knowledge.DomainBudget,
			Summary:       "Token costs spike at 3am UTC",
			Confidence:    0.72,
			EvidenceCount: 8,
		},
	}

	block := formatKnowledgeBlock(insights)

	if !strings.HasPrefix(block, "\n=== INSTITUTIONAL KNOWLEDGE ===\n") {
		t.Error("missing header")
	}
	if !strings.Contains(block, "accumulated experience") {
		t.Error("missing preamble text")
	}
	if !strings.Contains(block, "[1] (domain: health, confidence: 0.85, evidence: 23 events)") {
		t.Error("missing item 1 metadata")
	}
	if !strings.Contains(block, "    Memory pressure correlates with Opus iterations") {
		t.Error("missing item 1 summary")
	}
	if !strings.Contains(block, "[2] (domain: budget, confidence: 0.72, evidence: 8 events)") {
		t.Error("missing item 2 metadata")
	}
	if !strings.Contains(block, "    Token costs spike at 3am UTC") {
		t.Error("missing item 2 summary")
	}
	if !strings.HasSuffix(block, "===\n") {
		t.Error("missing closing footer")
	}
}
