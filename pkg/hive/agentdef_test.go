package hive

import (
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
)

func TestAgentDefValidate(t *testing.T) {
	valid := AgentDef{
		Name:         "test",
		Role:         "tester",
		Model:        "claude-sonnet-4-6",
		SystemPrompt: "You are a test agent.",
	}

	if err := valid.Validate(); err != nil {
		t.Errorf("valid def should pass: %v", err)
	}

	// Model is now optional (resolved via role defaults / ModelPolicy).
	valid.Model = ""
	if err := valid.Validate(); err != nil {
		t.Errorf("empty Model should pass validation: %v", err)
	}
	valid.Model = "claude-sonnet-4-6" // restore for remaining tests

	tests := []struct {
		name    string
		modify  func(*AgentDef)
		wantErr string
	}{
		{"missing name", func(d *AgentDef) { d.Name = "" }, "Name"},
		{"missing role", func(d *AgentDef) { d.Role = "" }, "Role"},
		{"missing prompt", func(d *AgentDef) { d.SystemPrompt = "" }, "SystemPrompt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := valid // copy
			tt.modify(&d)
			err := d.Validate()
			if err == nil {
				t.Errorf("should fail for %s", tt.name)
			}
		})
	}
}

func TestAgentDefDefaults(t *testing.T) {
	d := AgentDef{}

	if d.EffectiveMaxIterations() != 50 {
		t.Errorf("default max iterations = %d, want 50", d.EffectiveMaxIterations())
	}
	if d.EffectiveMaxDuration() != 30*time.Minute {
		t.Errorf("default max duration = %v, want 30m", d.EffectiveMaxDuration())
	}

	d.MaxIterations = 10
	d.MaxDuration = 5 * time.Minute

	if d.EffectiveMaxIterations() != 10 {
		t.Errorf("custom max iterations = %d, want 10", d.EffectiveMaxIterations())
	}
	if d.EffectiveMaxDuration() != 5*time.Minute {
		t.Errorf("custom max duration = %v, want 5m", d.EffectiveMaxDuration())
	}
}

func TestStarterAgents(t *testing.T) {
	agents := StarterAgents("TestHuman")

	if len(agents) != 9 {
		t.Fatalf("got %d agents, want 9", len(agents))
	}

	names := map[string]bool{}
	roles := map[string]bool{}
	for _, a := range agents {
		if err := a.Validate(); err != nil {
			t.Errorf("agent %q invalid: %v", a.Name, err)
		}
		if names[a.Name] {
			t.Errorf("duplicate agent name: %q", a.Name)
		}
		names[a.Name] = true
		roles[a.Role] = true

		// Verify human name is injected into the prompt.
		if a.SystemPrompt == "" {
			t.Errorf("agent %q has empty system prompt", a.Name)
		}
	}

	// Verify expected roles exist.
	expectedRoles := []string{"guardian", "sysmon", "allocator", "cto", "spawner", "reviewer", "strategist", "planner", "implementer"}
	for _, role := range expectedRoles {
		if !roles[role] {
			t.Errorf("missing expected role: %s", role)
		}
	}

	// Verify boot order: guardian → sysmon → allocator → cto → spawner → reviewer → strategist → planner → implementer.
	bootOrder := []string{"guardian", "sysmon", "allocator", "cto", "spawner", "reviewer", "strategist", "planner", "implementer"}
	for i, want := range bootOrder {
		if agents[i].Role != want {
			t.Errorf("boot order[%d]: got role %q, want %q", i, agents[i].Role, want)
		}
	}
}

func TestStarterAgents_HaveRoleDefinitions(t *testing.T) {
	agents := StarterAgents("TestHuman")
	roles := StarterRoleDefinitions()

	for _, a := range agents {
		if a.RoleDefinition == nil {
			t.Errorf("agent %q has nil RoleDefinition", a.Name)
			continue
		}
		rd := a.RoleDefinition

		// RoleDefinition name must match agent role.
		if rd.Name != a.Role {
			t.Errorf("agent %q: RoleDefinition.Name=%q, want %q", a.Name, rd.Name, a.Role)
		}

		// Must have description, category, tier.
		if rd.Description == "" {
			t.Errorf("agent %q: RoleDefinition.Description is empty", a.Name)
		}
		if rd.Category == "" {
			t.Errorf("agent %q: RoleDefinition.Category is empty", a.Name)
		}
		if rd.Tier == "" {
			t.Errorf("agent %q: RoleDefinition.Tier is empty", a.Name)
		}

		// CanOperate must match between AgentDef and RoleDefinition.
		if rd.CanOperate != a.CanOperate {
			t.Errorf("agent %q: RoleDefinition.CanOperate=%v, AgentDef.CanOperate=%v", a.Name, rd.CanOperate, a.CanOperate)
		}

		// Must have a model policy.
		if rd.ModelPolicy == nil {
			t.Errorf("agent %q: RoleDefinition.ModelPolicy is nil", a.Name)
		}

		// Must be in the role definitions map.
		if _, ok := roles[a.Role]; !ok {
			t.Errorf("agent %q: role %q not in StarterRoleDefinitions()", a.Name, a.Role)
		}
	}

	// All role definitions must be referenced by at least one agent.
	agentRoles := map[string]bool{}
	for _, a := range agents {
		agentRoles[a.Role] = true
	}
	for name := range roles {
		if !agentRoles[name] {
			t.Errorf("StarterRoleDefinitions has %q but no agent uses it", name)
		}
	}
}

// TestImplementerWatchesTaskArtifact guards Finding 3 (the wakeup race): the
// implementer must wake when the Planner attaches readiness gates
// (work.task.artifact). Without this subscription, an idle keepalive implementer
// that ran before the gates landed is never re-woken when the task becomes ready.
func TestImplementerWatchesTaskArtifact(t *testing.T) {
	agents := StarterAgents("TestHuman")
	var impl *AgentDef
	for i := range agents {
		if agents[i].Name == "implementer" {
			impl = &agents[i]
			break
		}
	}
	if impl == nil {
		t.Fatal("implementer AgentDef not found in StarterAgents")
	}
	found := false
	for _, p := range impl.WatchPatterns {
		if p == "work.task.artifact" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("implementer WatchPatterns must include work.task.artifact (Finding 3 wakeup race); got %v", impl.WatchPatterns)
	}
}

// TestContractsEnforceScopeExclusivity guards the content-fidelity fix: the
// Planner's authoritative-document acceptance_criteria and the Reviewer's
// verify-against-source standard must enforce EXCLUSIVITY (a scope ceiling), not
// only completeness. Without a ceiling the society over-enumerates: round 1 of the
// roles-catalog run produced 46 roles (the 24 in-scope plus 22 roadmap/aspirational
// roles) because both contracts demanded "enumerate every item the source has" but
// neither forbade ADDING items absent from the cited sources. The fix makes
// over-enumeration as blocking as omission, symmetrically, in both contracts.
func TestContractsEnforceScopeExclusivity(t *testing.T) {
	agents := StarterAgents("TestHuman")
	promptFor := func(name string) string {
		for i := range agents {
			if agents[i].Name == name {
				return agents[i].SystemPrompt
			}
		}
		t.Fatalf("agent %q not found in StarterAgents", name)
		return ""
	}

	planner := promptFor("planner")
	for _, phrase := range []string{"Exclusivity (scope ceiling)", "over-enumeration"} {
		if !strings.Contains(planner, phrase) {
			t.Errorf("planner contract must demand scope exclusivity in acceptance_criteria; missing %q", phrase)
		}
	}

	reviewer := promptFor("reviewer")
	for _, phrase := range []string{"Exclusivity vs cited source", "symmetric with omission"} {
		if !strings.Contains(reviewer, phrase) {
			t.Errorf("reviewer contract must block over-enumeration; missing %q", phrase)
		}
	}
}

// TestContractsEnforceDraftLifecycleHonesty guards the round-3 finding: a document
// delivered as a draft / unmerged PR pending human approval (Gate-E) must declare a
// PRE-acceptance lifecycle (status draft/review/candidate, canonical:false) until it
// is accepted — not active/accepted/canonical:true. The round-3 catalog was content-
// and scope-perfect but FAILED AC-1 because it self-declared status:active +
// canonical:true while still an unmerged draft, contradicting the repo's own
// governance definition of "active" (= accepted and currently governing). The Planner
// AC-demands and the Reviewer verify-against-source must both enforce this.
func TestContractsEnforceDraftLifecycleHonesty(t *testing.T) {
	agents := StarterAgents("TestHuman")
	promptFor := func(name string) string {
		for i := range agents {
			if agents[i].Name == name {
				return agents[i].SystemPrompt
			}
		}
		t.Fatalf("agent %q not found in StarterAgents", name)
		return ""
	}

	planner := promptFor("planner")
	if !strings.Contains(planner, "Draft-PR lifecycle honesty") {
		t.Errorf("planner contract must demand draft-PR lifecycle honesty in acceptance_criteria (status draft/canonical:false until accepted)")
	}

	reviewer := promptFor("reviewer")
	if !strings.Contains(reviewer, "Lifecycle honesty (draft vs accepted)") {
		t.Errorf("reviewer contract must block accepted/active/canonical lifecycle claims on an unmerged draft")
	}
}

// TestPlannerGateBodiesForbidGovernedTerminalStep guards the v10 run's
// contract-layer finding (v10-F3, Finding 19 — the contract half of the
// Finding 17 breach's root cause): the planner attached gate bodies that
// demanded the governed terminal step (feat branch push / draft PR) from the
// implementer. The implementer is barred from remote mutations, so such a
// gate either deadlocks the run or pressures an agent toward ungoverned
// escape — which is exactly what v10 observed. The planner contract must
// scope gate bodies to repository-local outcomes and route delivery to the
// authority-gated terminal path (Gate-E → Epic 11), fail-closed.
func TestPlannerGateBodiesForbidGovernedTerminalStep(t *testing.T) {
	agents := StarterAgents("TestHuman")
	var planner string
	for i := range agents {
		if agents[i].Name == "planner" {
			planner = agents[i].SystemPrompt
		}
	}
	if planner == "" {
		t.Fatal("planner agent not found in StarterAgents")
	}

	// The constraint block exists and is fail-closed.
	for _, phrase := range []string{
		"GATE-BODY SCOPE (FAIL-CLOSED)",
		"ONLY repository-local outcomes",
		"must NEVER demand a branch push, draft PR, PR URL, remote mutation, or any publication step",
		"authority-gated terminal path (Gate-E",
		"never by an implementer task",
		"scope it OUT of your gate bodies",
	} {
		if !strings.Contains(planner, phrase) {
			t.Errorf("planner contract missing gate-body scope clause %q", phrase)
		}
	}

	// The draft-PR lifecycle bullet must attribute PR delivery to the
	// order's terminal path, not presuppose the implementer delivers it.
	if !strings.Contains(planner, "delivered as a draft / unmerged pull request by the order's authority-gated terminal path") {
		t.Error("planner draft-PR lifecycle clause must attribute PR delivery to the authority-gated terminal path, never the implementer")
	}
}

func TestEffectiveModelPolicy(t *testing.T) {
	rdPolicy := &modelconfig.RoleModelPolicy{PreferredTier: modelconfig.TierVolume}
	defPolicy := &modelconfig.RoleModelPolicy{PreferredTier: modelconfig.TierJudgment}

	// No policy anywhere → nil.
	d := AgentDef{Name: "test", Role: "test"}
	if d.EffectiveModelPolicy() != nil {
		t.Error("expected nil when no policy set")
	}

	// RoleDefinition policy only → returns it.
	d.RoleDefinition = &modelconfig.RoleDefinition{ModelPolicy: rdPolicy}
	if got := d.EffectiveModelPolicy(); got != rdPolicy {
		t.Error("expected RoleDefinition.ModelPolicy")
	}

	// AgentDef policy overrides RoleDefinition.
	d.ModelPolicy = defPolicy
	if got := d.EffectiveModelPolicy(); got != defPolicy {
		t.Error("expected AgentDef.ModelPolicy to take precedence")
	}
}

func TestRoleDefinitionContent(t *testing.T) {
	c := RoleDefinitionContent{
		Name:        "guardian",
		Description: "Independent integrity monitor",
		Category:    "process",
		Tier:        TierA,
		CanOperate:  false,
		Origin:      "bootstrap",
	}

	if c.EventTypeName() != "hive.role.definition" {
		t.Errorf("EventTypeName() = %q, want %q", c.EventTypeName(), "hive.role.definition")
	}
	if c.Name != "guardian" {
		t.Error("Name mismatch")
	}
	if c.Origin != "bootstrap" {
		t.Error("Origin mismatch")
	}
}

func TestEventTypeRoleDefinitionRegistered(t *testing.T) {
	// Verify the event type constant is valid.
	if EventTypeRoleDefinition.Value() != "hive.role.definition" {
		t.Errorf("EventTypeRoleDefinition = %q, want %q", EventTypeRoleDefinition.Value(), "hive.role.definition")
	}

	// Verify it's in the allHiveEventTypes list.
	found := false
	for _, et := range allHiveEventTypes() {
		if et == EventTypeRoleDefinition {
			found = true
			break
		}
	}
	if !found {
		t.Error("EventTypeRoleDefinition not in allHiveEventTypes()")
	}
}

func TestNonOperateOutputConvention(t *testing.T) {
	// The constant is appended to every dynamically spawned agent's system
	// prompt (CanOperate=false). It must tell the agent:
	//   1. It cannot write files
	//   2. It should use /task comment for output delivery
	//   3. It should reference output in /task complete
	required := []string{
		"OUTPUT CONVENTION",
		"file write access",
		"/task comment",
		"/task complete",
	}
	for _, phrase := range required {
		if !strings.Contains(nonOperateOutputConvention, phrase) {
			t.Errorf("nonOperateOutputConvention missing required phrase %q", phrase)
		}
	}

	// The exact composition spawnDynamicAgent (watch.go) uses.
	proposalPrompt := "You are the analyst. Investigate metrics."
	result := composeSpawnedPrompt(proposalPrompt)

	if !strings.HasPrefix(result, proposalPrompt) {
		t.Error("original proposal prompt must be preserved as prefix")
	}
	if !strings.Contains(result, "OUTPUT CONVENTION") {
		t.Error("combined prompt must contain OUTPUT CONVENTION header")
	}
}

// TestContractsEnforceCompletionDiscipline guards the v9 run's binding finding
// (v9-F2, = v8-F4 widened): the strategist completed the order task as
// decomposition bookkeeping, the spawner completed the SAME task three seconds
// later claiming a deliverable that existed only as prose in a task comment,
// the store accepted both, and the reviewer needed four review cycles and a
// constitutional HALT to stop it. The shared mission contract must bind EVERY
// civic agent to completion discipline — complete only what is assigned to
// you, and never claim a deliverable that does not exist in the form the task
// demands — and the two observed offenders carry explicit reinforcement.
func TestContractsEnforceCompletionDiscipline(t *testing.T) {
	agents := StarterAgents("TestHuman")
	promptFor := func(name string) string {
		for i := range agents {
			if agents[i].Name == name {
				return agents[i].SystemPrompt
			}
		}
		t.Fatalf("agent %q not found in StarterAgents", name)
		return ""
	}

	// The class: every civic agent carries the shared discipline block.
	for i := range agents {
		p := agents[i].SystemPrompt
		for _, phrase := range []string{
			"COMPLETION DISCIPLINE",
			"only complete a task that is assigned to YOU",
			"committed in the repository",
			"leave the task open",
			"Never re-complete",
			"cannot SEE what a task demands",
		} {
			if !strings.Contains(p, phrase) {
				t.Errorf("agent %q mission preamble missing completion-discipline clause %q", agents[i].Name, phrase)
			}
		}
	}

	// The observed offenders carry explicit reinforcement.
	if !strings.Contains(promptFor("spawner"), "comment is not a deliverable") {
		t.Error("spawner contract must state that a comment is not a deliverable")
	}
	if !strings.Contains(promptFor("strategist"), "Decomposing a task is not completing it") {
		t.Error("strategist contract must state that decomposing a task is not completing it")
	}
}

// TestComposeSpawnedPromptCarriesCompletionDiscipline guards the codex finding
// on #150: spawnDynamicAgent composed spawned prompts as
// proposal.Prompt + nonOperateOutputConvention, so dynamic CanOperate=false
// agents kept the unqualified comment-as-deliverable convention with none of
// the completion discipline the starter agents gained — the same class of
// under-blocking one spawn away. The composition is now a shared function so
// the contract and this test cannot drift apart.
func TestComposeSpawnedPromptCarriesCompletionDiscipline(t *testing.T) {
	proposalPrompt := "You are the analyst. Investigate metrics."
	result := composeSpawnedPrompt(proposalPrompt)

	if !strings.HasPrefix(result, proposalPrompt) {
		t.Error("original proposal prompt must be preserved as prefix")
	}
	for _, phrase := range []string{
		"OUTPUT CONVENTION",
		"COMPLETION DISCIPLINE",
		"can NEVER be completed by you",
		"cannot SEE what a task demands",
		"Never re-complete",
	} {
		if !strings.Contains(result, phrase) {
			t.Errorf("spawned prompt missing required phrase %q", phrase)
		}
	}
}
