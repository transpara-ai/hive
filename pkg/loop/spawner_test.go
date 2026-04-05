package loop

import (
	"strings"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/graph"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/resources"
)

// ════════════════════════════════════════════════════════════════════════
// Test helpers
// ════════════════════════════════════════════════════════════════════════

// createSpawnTestEvent builds a minimal event with the given type and content.
// Reuses the testSigner defined in loop_test.go.
func createSpawnTestEvent(t *testing.T, eventType types.EventType, content event.EventContent) event.Event {
	t.Helper()
	source := types.MustActorID("actor_00000000000000000000000000000042")

	as := actor.NewInMemoryActorStore()
	s := store.NewInMemoryStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })

	registry := event.DefaultRegistry()
	factory := event.NewEventFactory(registry)

	bsFactory := event.NewBootstrapFactory(registry)
	signer := &testSigner{}
	bootstrap, err := bsFactory.Init(source, signer)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := s.Append(bootstrap)
	if err != nil {
		t.Fatal(err)
	}

	convID, _ := types.NewConversationID("conv_spawn_test_000000000000000001")
	ev, err := factory.Create(
		eventType,
		source,
		content,
		[]types.EventID{stored.ID()},
		convID,
		s,
		signer,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// longPrompt returns a prompt string >= 100 chars.
func longPrompt() string {
	return strings.Repeat("x", 100)
}

// validSpawnCmd returns a spawn command that passes all validation rules.
func validSpawnCmd() *SpawnCommand {
	return &SpawnCommand{
		Name:          "code-reviewer",
		Model:         "sonnet",
		WatchPatterns: []string{"work.task.completed"},
		CanOperate:    false,
		MaxIterations: 50,
		Prompt:        longPrompt(),
		Reason:        "gap detected in code quality checks",
	}
}

// validSpawnCtx returns a SpawnContext that passes all validation rules.
func validSpawnCtx() *SpawnContext {
	return &SpawnContext{
		Iteration:          20,
		HasPendingProposal: false,
		AgentRoster:        []string{"guardian", "sysmon", "allocator", "cto"},
		RecentRejections:   map[string]int{},
	}
}

// ════════════════════════════════════════════════════════════════════════
// parseSpawnCommand
// ════════════════════════════════════════════════════════════════════════

func TestParseSpawnCommand_Valid(t *testing.T) {
	response := `Analyzing the gap event. A code-reviewer role would address quality.
/spawn {"name":"code-reviewer","model":"sonnet","watch_patterns":["work.task.completed"],"can_operate":false,"max_iterations":50,"prompt":"You review code...","reason":"quality gap detected"}
/signal {"signal": "IDLE"}`

	cmd := parseSpawnCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil SpawnCommand")
	}
	if cmd.Name != "code-reviewer" {
		t.Errorf("Name = %q, want %q", cmd.Name, "code-reviewer")
	}
	if cmd.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", cmd.Model, "sonnet")
	}
	if len(cmd.WatchPatterns) != 1 || cmd.WatchPatterns[0] != "work.task.completed" {
		t.Errorf("WatchPatterns = %v, want [work.task.completed]", cmd.WatchPatterns)
	}
	if cmd.CanOperate {
		t.Error("CanOperate should be false")
	}
	if cmd.MaxIterations != 50 {
		t.Errorf("MaxIterations = %d, want 50", cmd.MaxIterations)
	}
	if cmd.Prompt != "You review code..." {
		t.Errorf("Prompt = %q, want %q", cmd.Prompt, "You review code...")
	}
	if cmd.Reason != "quality gap detected" {
		t.Errorf("Reason = %q, want %q", cmd.Reason, "quality gap detected")
	}
}

func TestParseSpawnCommand_NoCommand(t *testing.T) {
	response := `No gap events to process. Observing.
/signal {"signal": "IDLE"}`

	cmd := parseSpawnCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

func TestParseSpawnCommand_MalformedJSON(t *testing.T) {
	response := `/spawn {not valid json at all`

	cmd := parseSpawnCommand(response)
	if cmd != nil {
		t.Errorf("expected nil for malformed JSON, got %+v", cmd)
	}
}

func TestParseSpawnCommand_MultipleLines(t *testing.T) {
	response := `Reviewing the gap event from CTO.
Evidence: 3 tasks failed without a security review.
The gap calls for a security-auditor role.
/spawn {"name":"security-auditor","model":"haiku","watch_patterns":["work.task.completed","hive.gap.detected"],"can_operate":false,"max_iterations":100,"prompt":"You audit security...","reason":"security gap detected in 3 recent tasks"}
The proposal is ready for Guardian review.
/signal {"signal": "IDLE"}`

	cmd := parseSpawnCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil SpawnCommand")
	}
	if cmd.Name != "security-auditor" {
		t.Errorf("Name = %q, want %q", cmd.Name, "security-auditor")
	}
	if cmd.Model != "haiku" {
		t.Errorf("Model = %q, want %q", cmd.Model, "haiku")
	}
	if len(cmd.WatchPatterns) != 2 {
		t.Errorf("WatchPatterns len = %d, want 2", len(cmd.WatchPatterns))
	}
	if cmd.MaxIterations != 100 {
		t.Errorf("MaxIterations = %d, want 100", cmd.MaxIterations)
	}
}

// ════════════════════════════════════════════════════════════════════════
// validateSpawnCommand
// ════════════════════════════════════════════════════════════════════════

func TestValidateSpawnCommand_Valid(t *testing.T) {
	cmd := validSpawnCmd()
	ctx := validSpawnCtx()

	err := validateSpawnCommand(cmd, ctx)
	if err != nil {
		t.Errorf("expected valid command, got error: %v", err)
	}
}

func TestValidateSpawnCommand_StabilizationWindow(t *testing.T) {
	cmd := validSpawnCmd()
	ctx := validSpawnCtx()
	ctx.Iteration = 19 // within stabilization window (< 20)

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected stabilization window error, got nil")
	}
}

func TestValidateSpawnCommand_PendingProposal(t *testing.T) {
	cmd := validSpawnCmd()
	ctx := validSpawnCtx()
	ctx.HasPendingProposal = true

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected pending proposal error, got nil")
	}
}

func TestValidateSpawnCommand_NameCollision(t *testing.T) {
	cmd := validSpawnCmd()
	ctx := validSpawnCtx()
	ctx.AgentRoster = append(ctx.AgentRoster, "code-reviewer") // name already exists

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected name collision error, got nil")
	}
}

func TestValidateSpawnCommand_InvalidModel(t *testing.T) {
	cmd := validSpawnCmd()
	cmd.Model = "gpt-4" // not haiku, sonnet, or opus
	ctx := validSpawnCtx()

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected invalid model error, got nil")
	}
}

func TestValidateSpawnCommand_IterationsTooLow(t *testing.T) {
	cmd := validSpawnCmd()
	cmd.MaxIterations = 9 // below minimum (10)
	ctx := validSpawnCtx()

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected iterations too low error, got nil")
	}
}

func TestValidateSpawnCommand_IterationsTooHigh(t *testing.T) {
	cmd := validSpawnCmd()
	cmd.MaxIterations = 201 // above maximum (200)
	ctx := validSpawnCtx()

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected iterations too high error, got nil")
	}
}

func TestValidateSpawnCommand_PromptTooShort(t *testing.T) {
	cmd := validSpawnCmd()
	cmd.Prompt = "too short" // < 100 chars
	ctx := validSpawnCtx()

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected prompt too short error, got nil")
	}
}

func TestValidateSpawnCommand_NoWatchPatterns(t *testing.T) {
	cmd := validSpawnCmd()
	cmd.WatchPatterns = nil
	ctx := validSpawnCtx()

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected no watch patterns error, got nil")
	}
}

func TestValidateSpawnCommand_WildcardWatch(t *testing.T) {
	cmd := validSpawnCmd()
	cmd.WatchPatterns = []string{"*"} // only Guardian watches everything
	ctx := validSpawnCtx()

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected wildcard watch error, got nil")
	}
}

func TestValidateSpawnCommand_CanOperateBlocked(t *testing.T) {
	cmd := validSpawnCmd()
	cmd.CanOperate = true // new roles cannot operate
	ctx := validSpawnCtx()

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected CanOperate=true error, got nil")
	}
}

func TestValidateSpawnCommand_RejectionCooldown(t *testing.T) {
	cmd := validSpawnCmd() // name = "code-reviewer"
	ctx := validSpawnCtx()
	ctx.Iteration = 60
	ctx.RecentRejections = map[string]int{
		"code-reviewer": 20, // rejected at iter 20, now at iter 60: 60-20=40 < 50
	}

	err := validateSpawnCommand(cmd, ctx)
	if err == nil {
		t.Error("expected rejection cooldown error, got nil")
	}

	// At iteration 71: 71-20=51 >= 50, cooldown expired.
	ctx.Iteration = 71
	err = validateSpawnCommand(cmd, ctx)
	if err != nil {
		t.Errorf("cooldown should have expired at iteration 71, got: %v", err)
	}
}

// ════════════════════════════════════════════════════════════════════════
// isValidRoleName
// ════════════════════════════════════════════════════════════════════════

func TestIsValidRoleName_Valid(t *testing.T) {
	valid := []string{
		"code-reviewer",
		"security-auditor",
		"task-prioritizer",
		"ab",                          // minimum length (2)
		"a1",                          // digit allowed
		"incident-commander",
		"memory-keeper",
		strings.Repeat("a", 50),       // maximum length (50)
	}
	for _, name := range valid {
		if !isValidRoleName(name) {
			t.Errorf("isValidRoleName(%q) = false, want true", name)
		}
	}
}

func TestIsValidRoleName_Invalid(t *testing.T) {
	invalid := []string{
		"",                            // empty
		"a",                           // too short (1 char)
		strings.Repeat("a", 51),       // too long (51 chars)
		"CodeReviewer",                // uppercase not allowed
		"code reviewer",               // space not allowed
		"code_reviewer",               // underscore not allowed
		"-code-reviewer",              // starts with hyphen
		"code-reviewer-",              // ends with hyphen
		"code--reviewer",              // consecutive hyphens
		"GUARDIAN",                    // reserved (uppercase check)
		"guardian",                    // reserved
		"sysmon",                      // reserved
		"allocator",                   // reserved
		"cto",                         // reserved
		"spawner",                     // reserved
		"strategist",                  // reserved
		"planner",                     // reserved
		"implementer",                 // reserved
	}
	for _, name := range invalid {
		if isValidRoleName(name) {
			t.Errorf("isValidRoleName(%q) = true, want false", name)
		}
	}
}

// ════════════════════════════════════════════════════════════════════════
// spawnerState / updateSpawnerState
// ════════════════════════════════════════════════════════════════════════

func TestUpdateSpawnerState_ProposalTracking(t *testing.T) {
	state := newSpawnerState()

	proposedEv := createSpawnTestEvent(t, event.EventTypeRoleProposed, event.RoleProposedContent{
		Name:          "code-reviewer",
		Model:         "sonnet",
		WatchPatterns: []string{"work.task.completed"},
		Prompt:        longPrompt(),
		Reason:        "gap detected",
		ProposedBy:    "spawner",
	})

	state.update([]event.Event{proposedEv})

	if state.pendingProposal != "code-reviewer" {
		t.Errorf("pendingProposal = %q, want %q", state.pendingProposal, "code-reviewer")
	}
}

func TestUpdateSpawnerState_RejectionTracking(t *testing.T) {
	state := newSpawnerState()
	// First call increments iteration to 1.
	state.update(nil)
	// Second call increments iteration to 2, processes rejection.
	rejectedEv := createSpawnTestEvent(t, event.EventTypeRoleRejected, event.RoleRejectedContent{
		Name:       "code-reviewer",
		RejectedBy: "guardian",
		Reason:     "prompt lacks soul statement",
	})

	state.update([]event.Event{rejectedEv})

	if state.pendingProposal != "" {
		t.Errorf("pendingProposal = %q, want empty after rejection", state.pendingProposal)
	}
	rejectedAt, ok := state.recentRejections["code-reviewer"]
	if !ok {
		t.Fatal("expected code-reviewer in recentRejections")
	}
	if rejectedAt != state.iteration {
		t.Errorf("recentRejections[code-reviewer] = %d, want %d (current iteration)", rejectedAt, state.iteration)
	}
}

func TestUpdateSpawnerState_ApprovalClearsProposal(t *testing.T) {
	state := newSpawnerState()
	state.pendingProposal = "code-reviewer" // simulate pending state

	approvedEv := createSpawnTestEvent(t, event.EventTypeRoleApproved, event.RoleApprovedContent{
		Name:       "code-reviewer",
		ApprovedBy: "guardian",
		Reason:     "soul present, rights preserved",
	})

	state.update([]event.Event{approvedEv})

	if state.pendingProposal != "" {
		t.Errorf("pendingProposal = %q, want empty after approval", state.pendingProposal)
	}
}

// ════════════════════════════════════════════════════════════════════════
// enrichSpawnObservation
// ════════════════════════════════════════════════════════════════════════

// newSpawnerLoop creates a minimal Loop with role="spawner" so spawnerState
// is initialised. Uses a mock provider that always signals IDLE.
func newSpawnerLoop(t *testing.T) *Loop {
	t.Helper()
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent := testHiveAgent(t, provider, "spawner", "spawner")
	l, err := New(Config{
		Agent:  agent,
		Budget: resources.BudgetConfig{MaxIterations: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestEnrichSpawnObservation_Format(t *testing.T) {
	l := newSpawnerLoop(t)

	// Populate spawnerState with known data.
	l.spawnerState.pendingProposal = "code-reviewer"
	l.spawnerState.recentRejections["security-auditor"] = 10
	l.spawnerState.processedGaps["gap-id-001"] = true

	// Set up a BudgetRegistry with one entry.
	reg := resources.NewBudgetRegistry()
	budget := resources.NewBudget(resources.BudgetConfig{MaxIterations: 50})
	reg.Register("guardian", budget, 50)
	l.config.BudgetRegistry = reg

	enriched := l.enrichSpawnObservation("base observation\n")

	sections := []string{
		"=== SPAWN CONTEXT ===",
		"ROSTER:",
		"PENDING PROPOSALS:",
		"code-reviewer",
		"RECENT GAPS",
		"RECENT OUTCOMES:",
		"security-auditor",
		"BUDGET POOL:",
		"===",
	}
	for _, want := range sections {
		if !strings.Contains(enriched, want) {
			t.Errorf("enriched output missing %q\ngot:\n%s", want, enriched)
		}
	}

	// Base observation must be preserved.
	if !strings.HasPrefix(enriched, "base observation\n") {
		t.Errorf("base observation not preserved; got prefix: %q", enriched[:30])
	}
}

func TestEnrichSpawnObservation_SkipsNonSpawner(t *testing.T) {
	provider := newMockProvider(`/signal {"signal": "IDLE"}`)
	agent := testHiveAgent(t, provider, "cto", "cto")
	l, err := New(Config{
		Agent:  agent,
		Budget: resources.BudgetConfig{MaxIterations: 50},
	})
	if err != nil {
		t.Fatal(err)
	}

	obs := "unchanged observation"
	got := l.enrichSpawnObservation(obs)
	if got != obs {
		t.Errorf("non-spawner observation should be unchanged; got %q", got)
	}
}

// ════════════════════════════════════════════════════════════════════════
// buildSpawnContext
// ════════════════════════════════════════════════════════════════════════

func TestBuildSpawnContext(t *testing.T) {
	l := newSpawnerLoop(t)

	// Advance spawnerState to iteration 25 with a pending proposal.
	for i := 0; i < 25; i++ {
		l.spawnerState.update(nil)
	}
	l.spawnerState.pendingProposal = "code-reviewer"
	l.spawnerState.recentRejections["security-auditor"] = 20

	// Register two agents in BudgetRegistry.
	reg := resources.NewBudgetRegistry()
	reg.Register("guardian", resources.NewBudget(resources.BudgetConfig{MaxIterations: 200}), 200)
	reg.Register("sysmon", resources.NewBudget(resources.BudgetConfig{MaxIterations: 150}), 150)
	l.config.BudgetRegistry = reg

	ctx := l.buildSpawnContext()

	if ctx.Iteration != 25 {
		t.Errorf("Iteration = %d, want 25", ctx.Iteration)
	}
	if !ctx.HasPendingProposal {
		t.Error("HasPendingProposal = false, want true")
	}
	if len(ctx.AgentRoster) != 2 {
		t.Errorf("AgentRoster len = %d, want 2", len(ctx.AgentRoster))
	}
	rejectedAt, ok := ctx.RecentRejections["security-auditor"]
	if !ok {
		t.Fatal("expected security-auditor in RecentRejections")
	}
	if rejectedAt != 20 {
		t.Errorf("RecentRejections[security-auditor] = %d, want 20", rejectedAt)
	}

	// Verify RosterContains works.
	if !ctx.RosterContains("guardian") {
		t.Error("RosterContains(guardian) = false, want true")
	}
	if ctx.RosterContains("nonexistent") {
		t.Error("RosterContains(nonexistent) = true, want false")
	}

	// ctx.Iteration=25, rejectedAt=20, window=50 → 25-20=5 < 50 → recently rejected.
	if !ctx.RecentlyRejected("security-auditor", 50) {
		t.Error("RecentlyRejected(security-auditor, 50) = false, want true")
	}
}
