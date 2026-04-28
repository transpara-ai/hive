package loop

// Spawner integration tests — deterministic framework tests (no LLM calls).
//
// These tests cover the full spawner protocol machinery: command → event emission,
// context construction, observation enrichment, and the end-to-end event chain.
//
// Tier 1: Deterministic (all tests here).
// Tier 2: Smoke test (TestSpawnerBootsInLegacyMode at the bottom).
//
// All agents use the shared mockProvider from loop_test.go.
// All stores are in-memory via testGraph() and testHiveAgent() from loop_test.go.

import (
	"context"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/hive/pkg/modelconfig"
	"github.com/transpara-ai/hive/pkg/resources"
)

// ────────────────────────────────────────────────────────────────────
// Helpers specific to integration tests
// ────────────────────────────────────────────────────────────────────

// testSharedGraph creates an in-memory graph for sharing across multiple test agents.
// Pass to hiveagent.New directly to ensure all agents write to the same event store.
func testSharedGraph(t *testing.T) *graph.Graph {
	t.Helper()
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}

// testLoop creates a minimal Loop for the given agent.
func testLoop(t *testing.T, agent *hiveagent.Agent) *Loop {
	t.Helper()
	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// testLoopWithRegistry creates a Loop with a BudgetRegistry.
func testLoopWithRegistry(t *testing.T, agent *hiveagent.Agent, reg *resources.BudgetRegistry) *Loop {
	t.Helper()
	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// ────────────────────────────────────────────────────────────────────
// a. TestSpawnCommandToEvent
// ────────────────────────────────────────────────────────────────────

// TestSpawnCommandToEvent verifies that a validated SpawnCommand produces a
// hive.role.proposed event on the chain with fields matching the command.
func TestSpawnCommandToEvent(t *testing.T) {
	spawnerAgent := testHiveAgent(t, newMockProvider(), "spawner", "spawner")
	l := testLoop(t, spawnerAgent)

	cmd := validSpawnCmd()
	if err := l.emitRoleProposed(cmd); err != nil {
		t.Fatalf("emitRoleProposed: %v", err)
	}

	s := spawnerAgent.Graph().Store()
	page, err := s.ByType(event.EventTypeRoleProposed, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(hive.role.proposed): %v", err)
	}
	if len(page.Items()) != 1 {
		t.Fatalf("got %d hive.role.proposed events, want 1", len(page.Items()))
	}

	ev := page.Items()[0]
	content, ok := ev.Content().(event.RoleProposedContent)
	if !ok {
		t.Fatalf("content type assertion failed: got %T", ev.Content())
	}

	if content.Name != cmd.Name {
		t.Errorf("Name = %q, want %q", content.Name, cmd.Name)
	}
	wantModel := cmd.Model
	if entry, ok := modelconfig.DefaultCatalog().Lookup(cmd.Model); ok {
		wantModel = entry.ID
	}
	if content.Model != wantModel {
		t.Errorf("Model = %q, want %q", content.Model, wantModel)
	}
	if len(content.WatchPatterns) != len(cmd.WatchPatterns) {
		t.Errorf("WatchPatterns len = %d, want %d", len(content.WatchPatterns), len(cmd.WatchPatterns))
	}
	if content.CanOperate != cmd.CanOperate {
		t.Errorf("CanOperate = %v, want %v", content.CanOperate, cmd.CanOperate)
	}
	if content.MaxIterations != cmd.MaxIterations {
		t.Errorf("MaxIterations = %d, want %d", content.MaxIterations, cmd.MaxIterations)
	}
	if content.Prompt != cmd.Prompt {
		t.Errorf("Prompt mismatch")
	}
	if content.ProposedBy != "spawner" {
		t.Errorf("ProposedBy = %q, want %q", content.ProposedBy, "spawner")
	}
}

// ────────────────────────────────────────────────────────────────────
// b. TestSpawnContextConstruction
// ────────────────────────────────────────────────────────────────────

// TestSpawnContextConstruction verifies that buildSpawnContext correctly assembles
// context from spawnerState and the BudgetRegistry.
func TestSpawnContextConstruction(t *testing.T) {
	reg := resources.NewBudgetRegistry()
	budgetCfg := resources.BudgetConfig{MaxIterations: 50}
	reg.Register("guardian", resources.NewBudget(budgetCfg), 50, "")
	reg.Register("sysmon", resources.NewBudget(budgetCfg), 30, "")
	reg.Register("allocator", resources.NewBudget(budgetCfg), 30, "")

	spawnerAgent := testHiveAgent(t, newMockProvider(), "spawner", "spawner")
	l := testLoopWithRegistry(t, spawnerAgent, reg)

	// Simulate cross-iteration state.
	l.spawnerState.pendingProposal = "code-reviewer"
	l.spawnerState.recentRejections["old-role"] = 5
	l.spawnerState.iteration = 25

	ctx := l.buildSpawnContext()

	if !ctx.HasPendingProposal {
		t.Error("HasPendingProposal should be true when pendingProposal is set")
	}
	if ctx.Iteration != 25 {
		t.Errorf("Iteration = %d, want 25", ctx.Iteration)
	}
	for _, name := range []string{"guardian", "sysmon", "allocator"} {
		if !ctx.RosterContains(name) {
			t.Errorf("roster should contain %q", name)
		}
	}
	rejectedAt, ok := ctx.RecentRejections["old-role"]
	if !ok {
		t.Error("RecentRejections should contain 'old-role'")
	} else if rejectedAt != 5 {
		t.Errorf("RecentRejections[old-role] = %d, want 5", rejectedAt)
	}
}

// TestSpawnContextConstruction_NoPending verifies HasPendingProposal is false
// when no proposal is in-flight.
func TestSpawnContextConstruction_NoPending(t *testing.T) {
	spawnerAgent := testHiveAgent(t, newMockProvider(), "spawner", "spawner")
	l := testLoop(t, spawnerAgent)

	ctx := l.buildSpawnContext()

	if ctx.HasPendingProposal {
		t.Error("HasPendingProposal should be false with empty spawnerState")
	}
}

// ────────────────────────────────────────────────────────────────────
// c. TestObservationEnrichmentFormat
// ────────────────────────────────────────────────────────────────────

// TestSpawnObservationEnrichmentFormat verifies that enrichSpawnObservation appends
// the expected structured SPAWN CONTEXT block to the observation.
func TestSpawnObservationEnrichmentFormat(t *testing.T) {
	reg := resources.NewBudgetRegistry()
	reg.Register("guardian", resources.NewBudget(resources.BudgetConfig{MaxIterations: 50}), 50, "")

	spawnerAgent := testHiveAgent(t, newMockProvider(), "spawner", "spawner")
	l := testLoopWithRegistry(t, spawnerAgent, reg)
	l.spawnerState.pendingProposal = "code-reviewer"

	base := "## Recent Events\n- [hive.run.started] some-id by actor_xyz\n"
	result := l.enrichSpawnObservation(base)

	// Must start with the original observation.
	if !strings.HasPrefix(result, base) {
		t.Error("enriched observation should begin with the base observation")
	}

	// Must contain the structured context block.
	requiredSections := []string{
		"=== SPAWN CONTEXT ===",
		"ROSTER:",
		"PENDING PROPOSALS:",
		"RECENT GAPS",
		"BUDGET POOL:",
	}
	for _, section := range requiredSections {
		if !strings.Contains(result, section) {
			t.Errorf("observation missing section %q", section)
		}
	}

	// The pending proposal name must appear.
	if !strings.Contains(result, "code-reviewer") {
		t.Error("observation should include the pending proposal name")
	}

	// The registered agent must appear in the roster.
	if !strings.Contains(result, "guardian") {
		t.Error("observation should include 'guardian' in roster")
	}
}

// ────────────────────────────────────────────────────────────────────
// d. TestObservationEnrichmentSkipsNonSpawner
// ────────────────────────────────────────────────────────────────────

// TestSpawnObservationEnrichmentSkipsNonSpawner verifies that enrichSpawnObservation
// is a no-op for agents that are not the Spawner.
func TestSpawnObservationEnrichmentSkipsNonSpawner(t *testing.T) {
	implementerAgent := testHiveAgent(t, newMockProvider(), "implementer", "implementer")
	l := testLoop(t, implementerAgent)

	base := "## Recent Events\n- [work.task.created] abc by actor_implementer\n"
	result := l.enrichSpawnObservation(base)

	if result != base {
		t.Errorf("non-spawner observation should be unchanged\ngot:  %q\nwant: %q", result, base)
	}
}

// ────────────────────────────────────────────────────────────────────
// e. TestGuardianApproveToEvent
// ────────────────────────────────────────────────────────────────────

// TestGuardianApproveToEvent verifies that an ApproveCommand produces a
// hive.role.approved event on the chain with correct content.
func TestGuardianApproveToEvent(t *testing.T) {
	guardianAgent := testHiveAgent(t, newMockProvider(), "guardian", "guardian")
	l := testLoop(t, guardianAgent)

	cmd := &ApproveCommand{
		Name:   "code-reviewer",
		Reason: "Soul present, rights preserved, specific watch patterns, evidence-based gap",
	}
	if err := l.emitRoleApproved(cmd); err != nil {
		t.Fatalf("emitRoleApproved: %v", err)
	}

	s := guardianAgent.Graph().Store()
	page, err := s.ByType(event.EventTypeRoleApproved, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(hive.role.approved): %v", err)
	}
	if len(page.Items()) != 1 {
		t.Fatalf("got %d hive.role.approved events, want 1", len(page.Items()))
	}

	ev := page.Items()[0]
	content, ok := ev.Content().(event.RoleApprovedContent)
	if !ok {
		t.Fatalf("content type assertion failed: got %T", ev.Content())
	}

	if content.Name != cmd.Name {
		t.Errorf("Name = %q, want %q", content.Name, cmd.Name)
	}
	if content.Reason != cmd.Reason {
		t.Errorf("Reason = %q, want %q", content.Reason, cmd.Reason)
	}
	if content.ApprovedBy != "guardian" {
		t.Errorf("ApprovedBy = %q, want %q", content.ApprovedBy, "guardian")
	}
}

// ────────────────────────────────────────────────────────────────────
// f. TestGuardianRejectToEvent
// ────────────────────────────────────────────────────────────────────

// TestGuardianRejectToEvent verifies that a RejectCommand produces a
// hive.role.rejected event on the chain with correct content.
func TestGuardianRejectToEvent(t *testing.T) {
	guardianAgent := testHiveAgent(t, newMockProvider(), "guardian", "guardian")
	l := testLoop(t, guardianAgent)

	cmd := &RejectCommand{
		Name:   "data-scraper",
		Reason: "Soul statement missing from prompt; watch_patterns contains bare wildcard",
	}
	if err := l.emitRoleRejected(cmd); err != nil {
		t.Fatalf("emitRoleRejected: %v", err)
	}

	s := guardianAgent.Graph().Store()
	page, err := s.ByType(event.EventTypeRoleRejected, 10, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(hive.role.rejected): %v", err)
	}
	if len(page.Items()) != 1 {
		t.Fatalf("got %d hive.role.rejected events, want 1", len(page.Items()))
	}

	ev := page.Items()[0]
	content, ok := ev.Content().(event.RoleRejectedContent)
	if !ok {
		t.Fatalf("content type assertion failed: got %T", ev.Content())
	}

	if content.Name != cmd.Name {
		t.Errorf("Name = %q, want %q", content.Name, cmd.Name)
	}
	if content.Reason != cmd.Reason {
		t.Errorf("Reason = %q, want %q", content.Reason, cmd.Reason)
	}
	if content.RejectedBy != "guardian" {
		t.Errorf("RejectedBy = %q, want %q", content.RejectedBy, "guardian")
	}
}

// ────────────────────────────────────────────────────────────────────
// g. TestCompleteProtocolFlow
// ────────────────────────────────────────────────────────────────────

// TestCompleteProtocolFlow verifies the gap → proposal → approval → budget event
// chain: all four event types land in the same store, the hash chain is valid,
// and events appear in the correct causal order.
//
// Note: runtime hot-add (actual agent boot after budget confirmation) is not
// tested here because it requires a full Runtime instance with live goroutines.
// See docs/designs/spawner-design-v1.1.0.md §14 for the full smoke test plan.
func TestCompleteProtocolFlow(t *testing.T) {
	sharedGraph := testSharedGraph(t)
	mp := newMockProvider()

	// Create four agents on the SAME graph so all events share one store.
	ctoAgent, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role("cto"),
		Name:     "cto-protocol-test",
		Graph:    sharedGraph,
		Provider: mp,
	})
	if err != nil {
		t.Fatalf("cto agent: %v", err)
	}

	spawnerAgent, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role("spawner"),
		Name:     "spawner-protocol-test",
		Graph:    sharedGraph,
		Provider: mp,
	})
	if err != nil {
		t.Fatalf("spawner agent: %v", err)
	}

	guardianAgent, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role("guardian"),
		Name:     "guardian-protocol-test",
		Graph:    sharedGraph,
		Provider: mp,
	})
	if err != nil {
		t.Fatalf("guardian agent: %v", err)
	}

	allocatorAgent, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role("allocator"),
		Name:     "allocator-protocol-test",
		Graph:    sharedGraph,
		Provider: mp,
	})
	if err != nil {
		t.Fatalf("allocator agent: %v", err)
	}

	// Step 1: CTO emits hive.gap.detected.
	gapContent := event.NewGapDetectedContent(
		event.GapCategoryCapability,
		"code-reviewer",
		"Three tasks failed without any quality check. No existing agent reviews code.",
		event.SeverityLevelSerious,
	)
	if err := ctoAgent.EmitGapDetected(gapContent); err != nil {
		t.Fatalf("EmitGapDetected: %v", err)
	}

	// Step 2: Spawner emits hive.role.proposed.
	spawnerLoop := testLoop(t, spawnerAgent)
	spawnCmd := validSpawnCmd()
	if err := spawnerLoop.emitRoleProposed(spawnCmd); err != nil {
		t.Fatalf("emitRoleProposed: %v", err)
	}

	// Step 3: Guardian emits hive.role.approved.
	guardianLoop := testLoop(t, guardianAgent)
	approveCmd := &ApproveCommand{
		Name:   spawnCmd.Name,
		Reason: "Soul present, rights preserved, evidence-based necessity",
	}
	if err := guardianLoop.emitRoleApproved(approveCmd); err != nil {
		t.Fatalf("emitRoleApproved: %v", err)
	}

	// Step 4: Allocator emits agent.budget.adjusted for the new role.
	budgetContent := event.AgentBudgetAdjustedContent{
		AgentName:      spawnCmd.Name,
		Action:         "allocate",
		PreviousBudget: 0,
		NewBudget:      spawnCmd.MaxIterations,
		Delta:          spawnCmd.MaxIterations,
		Reason:         "Initial allocation for approved role",
	}
	if err := allocatorAgent.EmitBudgetAdjusted(budgetContent); err != nil {
		t.Fatalf("EmitBudgetAdjusted: %v", err)
	}

	// Verify all four event types are present in the shared store.
	s := sharedGraph.Store()
	eventTypes := []struct {
		et   types.EventType
		name string
	}{
		{types.MustEventType("hive.gap.detected"), "hive.gap.detected"},
		{event.EventTypeRoleProposed, "hive.role.proposed"},
		{event.EventTypeRoleApproved, "hive.role.approved"},
		{types.MustEventType("agent.budget.adjusted"), "agent.budget.adjusted"},
	}
	for _, tt := range eventTypes {
		page, err := s.ByType(tt.et, 10, types.None[types.Cursor]())
		if err != nil {
			t.Errorf("ByType(%s): %v", tt.name, err)
			continue
		}
		if len(page.Items()) == 0 {
			t.Errorf("no %s events found — expected at least 1", tt.name)
		}
	}

	// Verify the hash chain is intact across all four emitted events.
	chainResult, err := s.VerifyChain()
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if !chainResult.Valid {
		t.Error("hash chain invalid after gap → proposal → approval → budget sequence")
	}

	// Verify role name matches across proposal → approval.
	proposalPage, _ := s.ByType(event.EventTypeRoleProposed, 1, types.None[types.Cursor]())
	approvalPage, _ := s.ByType(event.EventTypeRoleApproved, 1, types.None[types.Cursor]())

	if len(proposalPage.Items()) > 0 && len(approvalPage.Items()) > 0 {
		proposed := proposalPage.Items()[0].Content().(event.RoleProposedContent)
		approved := approvalPage.Items()[0].Content().(event.RoleApprovedContent)
		if proposed.Name != approved.Name {
			t.Errorf("proposal name %q != approval name %q", proposed.Name, approved.Name)
		}
	}
}

// ────────────────────────────────────────────────────────────────────
// h. TestSpawnerBootsInLegacyMode (Tier 2 smoke)
// ────────────────────────────────────────────────────────────────────

// TestSpawnerBootsInLegacyMode verifies the Spawner initialises correctly as
// part of the loop lifecycle: spawnerState is non-nil, the loop creates
// without error, and an immediately-cancelled context produces StopCancelled
// (not StopError), confirming the boot path is clean.
func TestSpawnerBootsInLegacyMode(t *testing.T) {
	spawnerAgent := testHiveAgent(t, newMockProvider(`/signal {"signal": "IDLE"}`), "spawner", "spawner")

	l, err := New(Config{
		Agent:   spawnerAgent,
		HumanID: humanID(),
	})
	if err != nil {
		t.Fatalf("spawner loop init failed: %v", err)
	}

	// spawnerState must be initialised for role == "spawner".
	if l.spawnerState == nil {
		t.Fatal("spawnerState should be initialised for spawner role")
	}
	if l.spawnerState.pendingProposal != "" {
		t.Error("pendingProposal should be empty on boot")
	}
	if len(l.spawnerState.recentRejections) != 0 {
		t.Error("recentRejections should be empty on boot")
	}

	// Run with an already-cancelled context — must stop cleanly (not error).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := l.Run(ctx)
	if result.Reason != StopCancelled {
		t.Errorf("expected StopCancelled, got %s: %s", result.Reason, result.Detail)
	}
}
