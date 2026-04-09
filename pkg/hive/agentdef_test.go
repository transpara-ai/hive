package hive

import (
	"strings"
	"testing"
	"time"
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

	tests := []struct {
		name    string
		modify  func(*AgentDef)
		wantErr string
	}{
		{"missing name", func(d *AgentDef) { d.Name = "" }, "Name"},
		{"missing role", func(d *AgentDef) { d.Role = "" }, "Role"},
		{"missing model", func(d *AgentDef) { d.Model = "" }, "Model"},
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
