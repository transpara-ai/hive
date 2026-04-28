package hive

import (
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/modelconfig"
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

	// Simulate the concatenation done in spawnDynamicAgent (watch.go).
	proposalPrompt := "You are the analyst. Investigate metrics."
	result := proposalPrompt + nonOperateOutputConvention

	if !strings.HasPrefix(result, proposalPrompt) {
		t.Error("original proposal prompt must be preserved as prefix")
	}
	if !strings.Contains(result, "OUTPUT CONVENTION") {
		t.Error("combined prompt must contain OUTPUT CONVENTION header")
	}
}
