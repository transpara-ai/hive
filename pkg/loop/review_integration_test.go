package loop

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/knowledge"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/work"
)

// ════════════════════════════════════════════════════════════════════════
// Enrichment tests
// ════════════════════════════════════════════════════════════════════════

func TestEnrichReviewObservation_HasPendingTask(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "reviewer", "test-reviewer")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 100},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a work.task.completed event arriving via bus.
	taskID, _ := types.NewEventIDFromNew()
	completedContent := work.TaskCompletedContent{
		TaskID:      taskID,
		CompletedBy: types.MustActorID("actor_00000000000000000000000000000001"),
		Summary:     "Added health endpoint",
	}
	l.reviewerState.completedTasks[taskID.Value()] = completedContent

	result := l.enrichReviewObservation("base obs")

	if !strings.Contains(result, "=== CODE REVIEW CONTEXT ===") {
		t.Error("missing CODE REVIEW CONTEXT header")
	}
	if !strings.Contains(result, "TASK UNDER REVIEW:") {
		t.Error("missing TASK UNDER REVIEW section")
	}
	if !strings.Contains(result, taskID.Value()) {
		t.Error("missing task ID in output")
	}
	if !strings.Contains(result, "Added health endpoint") {
		t.Error("missing task summary in output")
	}
	if !strings.Contains(result, "PENDING REVIEWS: 1") {
		t.Error("missing pending review count")
	}
	if !strings.Contains(result, "PREVIOUS REVIEWS FOR THIS TASK: none") {
		t.Error("missing previous review info")
	}
	if !strings.HasPrefix(result, "base obs") {
		t.Error("original observation not preserved")
	}
}

func TestEnrichReviewObservation_NoPendingTasks(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "reviewer", "test-reviewer")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 100},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := l.enrichReviewObservation("base obs")

	if !strings.Contains(result, "No tasks pending review") {
		t.Error("should indicate no pending reviews")
	}
	if !strings.Contains(result, "=== CODE REVIEW CONTEXT ===") {
		t.Error("should still have the context header")
	}
}

func TestEnrichReviewObservation_SkipsNonReviewer(t *testing.T) {
	roles := []string{"guardian", "sysmon", "allocator", "cto", "spawner", "strategist", "planner", "implementer"}
	for _, role := range roles {
		t.Run(role, func(t *testing.T) {
			provider := newMockProvider("test")
			agent := testHiveAgent(t, provider, role, "test-"+role)
			l, err := New(Config{
				Agent:   agent,
				HumanID: humanID(),
				Budget:  resources.BudgetConfig{MaxIterations: 100},
			})
			if err != nil {
				t.Fatal(err)
			}

			obs := "some observation"
			result := l.enrichReviewObservation(obs)
			if result != obs {
				t.Errorf("role %q should not enrich observation", role)
			}
		})
	}
}

// ════════════════════════════════════════════════════════════════════════
// Diff truncation
// ════════════════════════════════════════════════════════════════════════

func TestDiffTruncation_Small(t *testing.T) {
	// 50 lines — should be returned in full.
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = fmt.Sprintf("+line %d", i)
	}
	diff := strings.Join(lines, "\n")

	result := truncateDiff(diff, 300)
	if result != diff {
		t.Error("small diff should be returned in full")
	}
}

func TestDiffTruncation_Medium(t *testing.T) {
	// 500 lines — should be truncated to first 200 + last 50 + omission.
	lines := make([]string, 500)
	for i := range lines {
		lines[i] = fmt.Sprintf("+line %d", i)
	}
	diff := strings.Join(lines, "\n")

	result := truncateDiff(diff, 300)

	if !strings.Contains(result, "+line 0") {
		t.Error("should contain first line")
	}
	if !strings.Contains(result, "+line 199") {
		t.Error("should contain line 199 (last of head)")
	}
	if !strings.Contains(result, "lines omitted") {
		t.Error("should contain omission marker")
	}
	if !strings.Contains(result, "+line 499") {
		t.Error("should contain last line")
	}
	if !strings.Contains(result, "+line 450") {
		t.Error("should contain line 450 (in tail)")
	}
	// Line 250 should NOT be present (it's in the omitted section).
	if strings.Contains(result, "+line 250") {
		t.Error("line 250 should be omitted")
	}
}

func TestDiffTruncation_Large(t *testing.T) {
	// 2000 lines — too large for inline review.
	lines := make([]string, 2000)
	for i := range lines {
		lines[i] = fmt.Sprintf("+line %d", i)
	}
	diff := strings.Join(lines, "\n")

	result := truncateDiff(diff, 300)

	if !strings.Contains(result, "Diff too large") {
		t.Error("should indicate diff is too large")
	}
	if !strings.Contains(result, "2000 lines") {
		t.Error("should include line count")
	}
	// Should NOT contain actual diff content.
	if strings.Contains(result, "+line 100") {
		t.Error("should not contain diff content for large diffs")
	}
}

func TestDiffTruncation_Empty(t *testing.T) {
	result := truncateDiff("", 300)
	if result != "" {
		t.Errorf("empty diff should return empty string, got %q", result)
	}
}

// ════════════════════════════════════════════════════════════════════════
// Event emission
// ════════════════════════════════════════════════════════════════════════

func TestReviewCommandToEvent(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent := testHiveAgent(t, provider, "reviewer", "test-reviewer")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &ReviewCommand{
		TaskID:     "task-abc-123",
		Verdict:    "approve",
		Summary:    "Clean implementation with good error handling.",
		Issues:     []string{},
		Confidence: 0.9,
	}

	if err := l.emitCodeReview(cmd); err != nil {
		t.Fatalf("emitCodeReview: %v", err)
	}

	// Query the store for code.review.submitted events.
	g := agent.Graph()
	page, err := g.Store().ByType(
		event.EventTypeCodeReviewSubmitted,
		10,
		types.None[types.Cursor](),
	)
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}

	events := page.Items()
	if len(events) == 0 {
		t.Fatal("no code.review.submitted events found in store")
	}

	ev := events[len(events)-1]
	content, ok := ev.Content().(event.CodeReviewContent)
	if !ok {
		t.Fatalf("event content is %T, want CodeReviewContent", ev.Content())
	}

	if content.TaskID != "task-abc-123" {
		t.Errorf("TaskID = %q, want %q", content.TaskID, "task-abc-123")
	}
	if content.Verdict != "approve" {
		t.Errorf("Verdict = %q, want %q", content.Verdict, "approve")
	}
	if content.Summary != "Clean implementation with good error handling." {
		t.Errorf("Summary = %q, want %q", content.Summary, "Clean implementation with good error handling.")
	}
	if content.Confidence != 0.9 {
		t.Errorf("Confidence = %f, want 0.9", content.Confidence)
	}

	if ev.Source() != agent.ID() {
		t.Errorf("Source = %v, want %v", ev.Source(), agent.ID())
	}
}

func TestReviewEventContent(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent := testHiveAgent(t, provider, "reviewer", "test-reviewer")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	cmd := &ReviewCommand{
		TaskID:     "task-roundtrip",
		Verdict:    "request_changes",
		Summary:    "Two issues need fixing.",
		Issues:     []string{"Unchecked error on line 47", "Missing nil guard"},
		Confidence: 0.75,
	}

	if err := l.emitCodeReview(cmd); err != nil {
		t.Fatalf("emitCodeReview: %v", err)
	}

	g := agent.Graph()
	page, _ := g.Store().ByType(event.EventTypeCodeReviewSubmitted, 10, types.None[types.Cursor]())
	events := page.Items()
	if len(events) == 0 {
		t.Fatal("no events")
	}

	content := events[len(events)-1].Content().(event.CodeReviewContent)

	// Verify all 5 fields round-trip.
	if content.TaskID != "task-roundtrip" {
		t.Errorf("TaskID = %q", content.TaskID)
	}
	if content.Verdict != "request_changes" {
		t.Errorf("Verdict = %q", content.Verdict)
	}
	if content.Summary != "Two issues need fixing." {
		t.Errorf("Summary = %q", content.Summary)
	}
	if len(content.Issues) != 2 {
		t.Errorf("Issues count = %d, want 2", len(content.Issues))
	} else {
		if content.Issues[0] != "Unchecked error on line 47" {
			t.Errorf("Issues[0] = %q", content.Issues[0])
		}
	}
	if content.Confidence != 0.75 {
		t.Errorf("Confidence = %f, want 0.75", content.Confidence)
	}
}

func TestReviewCausalChain(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent := testHiveAgent(t, provider, "reviewer", "test-reviewer")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	// First review.
	cmd1 := &ReviewCommand{
		TaskID: "task-1", Verdict: "request_changes",
		Summary: "Needs fixes.", Issues: []string{"issue A"}, Confidence: 0.8,
	}
	if err := l.emitCodeReview(cmd1); err != nil {
		t.Fatalf("first review: %v", err)
	}

	// Second review.
	cmd2 := &ReviewCommand{
		TaskID: "task-1", Verdict: "approve",
		Summary: "Fixed.", Issues: []string{}, Confidence: 0.9,
	}
	if err := l.emitCodeReview(cmd2); err != nil {
		t.Fatalf("second review: %v", err)
	}

	// Both should be on the chain.
	g := agent.Graph()
	page, _ := g.Store().ByType(event.EventTypeCodeReviewSubmitted, 10, types.None[types.Cursor]())
	events := page.Items()
	if len(events) < 2 {
		t.Fatalf("expected at least 2 review events, got %d", len(events))
	}

	// ByType returns reverse-chrono. events[0] is most recent (approve), events[1] is older (request_changes).
	approve := events[0].Content().(event.CodeReviewContent)
	reqChanges := events[1].Content().(event.CodeReviewContent)
	if approve.Verdict != "approve" {
		t.Errorf("most recent event verdict = %q, want approve", approve.Verdict)
	}
	if reqChanges.Verdict != "request_changes" {
		t.Errorf("earlier event verdict = %q, want request_changes", reqChanges.Verdict)
	}
}

// ════════════════════════════════════════════════════════════════════════
// Integration
// ════════════════════════════════════════════════════════════════════════

func TestReviewerBootsInLegacyMode(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent := testHiveAgent(t, provider, "reviewer", "test-reviewer")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 100},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify reviewerState was initialized.
	if l.reviewerState == nil {
		t.Fatal("reviewerState should be initialized for reviewer role")
	}
	if l.reviewerState.reviewHistory == nil {
		t.Fatal("reviewHistory map should be initialized")
	}
	if l.reviewerState.completedTasks == nil {
		t.Fatal("completedTasks map should be initialized")
	}
}

func TestEnrichmentOrdering(t *testing.T) {
	// CRITICAL: Verify enrichReviewObservation runs BEFORE enrichKnowledgeObservation.
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "reviewer", "test-reviewer")

	ks := knowledge.NewStore()
	_ = ks.Record(knowledge.KnowledgeInsight{
		InsightID:     "test-insight",
		Domain:        knowledge.DomainQuality,
		Summary:       "Test quality insight",
		RelevantRoles: nil, // universal
		Confidence:    0.9,
		EvidenceCount: 10,
		Source:        knowledge.SourceMemoryKeeper,
		RecordedAt:    time.Now(),
		Active:        true,
	})

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Budget:         resources.BudgetConfig{MaxIterations: 100},
		KnowledgeStore: ks,
	})
	if err != nil {
		t.Fatal(err)
	}
	l.iteration = 15 // past stabilization for knowledge enrichment

	// Add a pending completed task so the review context block appears.
	taskID, _ := types.NewEventIDFromNew()
	l.reviewerState.completedTasks[taskID.Value()] = work.TaskCompletedContent{
		TaskID:      taskID,
		CompletedBy: types.MustActorID("actor_00000000000000000000000000000001"),
		Summary:     "Test task",
	}

	// Run the enrichment chain (same calls as observe()).
	obs := "base"
	enriched := l.enrichHealthObservation(obs)
	enriched = l.enrichBudgetObservation(enriched, l.iteration)
	enriched = l.enrichCTOObservation(enriched)
	enriched = l.enrichSpawnObservation(enriched)
	enriched = l.enrichReviewObservation(enriched)
	enriched = l.enrichKnowledgeObservation(enriched)

	reviewIdx := strings.Index(enriched, "=== CODE REVIEW CONTEXT ===")
	knowledgeIdx := strings.Index(enriched, "=== INSTITUTIONAL KNOWLEDGE ===")

	if reviewIdx == -1 {
		t.Fatal("missing CODE REVIEW CONTEXT block")
	}
	if knowledgeIdx == -1 {
		t.Fatal("missing INSTITUTIONAL KNOWLEDGE block")
	}
	if reviewIdx >= knowledgeIdx {
		t.Errorf("CODE REVIEW CONTEXT (at %d) must appear BEFORE INSTITUTIONAL KNOWLEDGE (at %d)",
			reviewIdx, knowledgeIdx)
	}
}
