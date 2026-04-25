package loop

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/resources"
)

func TestParseHealthCommand_Valid(t *testing.T) {
	response := `I've assessed the hive health.
/health {"severity":"warning","chain_ok":true,"active_agents":4,"event_rate":23.5}
/signal {"signal": "IDLE"}`

	cmd := parseHealthCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil HealthCommand")
	}
	if cmd.Severity != "warning" {
		t.Errorf("Severity = %q, want %q", cmd.Severity, "warning")
	}
	if !cmd.ChainOK {
		t.Error("ChainOK = false, want true")
	}
	if cmd.ActiveAgents != 4 {
		t.Errorf("ActiveAgents = %d, want 4", cmd.ActiveAgents)
	}
	if cmd.EventRate != 23.5 {
		t.Errorf("EventRate = %f, want 23.5", cmd.EventRate)
	}
}

func TestParseHealthCommand_NoCommand(t *testing.T) {
	response := `Everything looks normal.
/signal {"signal": "IDLE"}`

	cmd := parseHealthCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

func TestParseHealthCommand_MalformedJSON(t *testing.T) {
	response := `/health {not valid json`

	cmd := parseHealthCommand(response)
	if cmd != nil {
		t.Errorf("expected nil for malformed JSON, got %+v", cmd)
	}
}

func TestParseHealthCommand_MultipleLines(t *testing.T) {
	response := `Analyzing the event stream...
Agent vitals look good.
Budget is under control.
/health {"severity":"ok","chain_ok":true,"active_agents":5,"event_rate":42.0}
No further action needed.
/signal {"signal": "IDLE"}`

	cmd := parseHealthCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil HealthCommand")
	}
	if cmd.Severity != "ok" {
		t.Errorf("Severity = %q, want %q", cmd.Severity, "ok")
	}
	if cmd.ActiveAgents != 5 {
		t.Errorf("ActiveAgents = %d, want 5", cmd.ActiveAgents)
	}
	if cmd.EventRate != 42.0 {
		t.Errorf("EventRate = %f, want 42.0", cmd.EventRate)
	}
}

func TestParseHealthCommand_IndentedLine(t *testing.T) {
	response := `  /health {"severity":"critical","chain_ok":false,"active_agents":2,"event_rate":1.0}`

	cmd := parseHealthCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil HealthCommand for indented line")
	}
	if cmd.Severity != "critical" {
		t.Errorf("Severity = %q, want %q", cmd.Severity, "critical")
	}
	if cmd.ChainOK {
		t.Error("ChainOK = true, want false")
	}
}

func TestSeverityToScore(t *testing.T) {
	tests := []struct {
		severity string
		want     float64
	}{
		{"ok", 1.0},
		{"warning", 0.5},
		{"critical", 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := severityToScore(tt.severity)
			want := types.MustScore(tt.want)
			if got != want {
				t.Errorf("severityToScore(%q) = %v, want %v", tt.severity, got, want)
			}
		})
	}
}

func TestSeverityToScore_Unknown(t *testing.T) {
	got := severityToScore("unknown")
	want := types.MustScore(1.0)
	if got != want {
		t.Errorf("severityToScore(%q) = %v, want %v (safe default)", "unknown", got, want)
	}
}

func TestSeverityToScore_Empty(t *testing.T) {
	got := severityToScore("")
	want := types.MustScore(1.0)
	if got != want {
		t.Errorf("severityToScore(%q) = %v, want %v (safe default)", "", got, want)
	}
}

func TestParseHealthCommand_WithAgentVitals(t *testing.T) {
	response := `Vitals look mixed.
/health {"severity":"warning","chain_ok":true,"active_agents":3,"event_rate":12.0,"agent_vitals":[{"agent_id":"actor_aaa","iterations_pct":0.4,"trust_score":0.9,"budget_burn_rate_per_hour":18.5,"last_heartbeat_ticks":3,"severity":"ok"},{"agent_id":"actor_bbb","iterations_pct":0.85,"trust_score":0.7,"budget_burn_rate_per_hour":42.0,"last_heartbeat_ticks":17,"severity":"warning"}]}
/signal {"signal": "IDLE"}`

	cmd := parseHealthCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil HealthCommand")
	}
	if cmd.Severity != "warning" {
		t.Errorf("Severity = %q, want %q", cmd.Severity, "warning")
	}
	if len(cmd.AgentVitals) != 2 {
		t.Fatalf("AgentVitals length = %d, want 2", len(cmd.AgentVitals))
	}
	a := cmd.AgentVitals[0]
	if a.AgentID != "actor_aaa" || a.IterationsPct != 0.4 || a.TrustScore != 0.9 ||
		a.BudgetBurnRatePerHour != 18.5 || a.LastHeartbeatTicks != 3 || a.Severity != "ok" {
		t.Errorf("AgentVitals[0] = %+v, mismatch on at least one field", a)
	}
	b := cmd.AgentVitals[1]
	if b.AgentID != "actor_bbb" || b.Severity != "warning" || b.IterationsPct != 0.85 {
		t.Errorf("AgentVitals[1] = %+v, mismatch on at least one field", b)
	}
}

func TestParseHealthCommand_WithoutAgentVitals_BackwardCompatible(t *testing.T) {
	// Pre-Stage-3 /health command shape — no agent_vitals field.
	// Parser must succeed and AgentVitals must be nil/empty, never error.
	response := `/health {"severity":"ok","chain_ok":true,"active_agents":4,"event_rate":20.0}`
	cmd := parseHealthCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil HealthCommand")
	}
	if len(cmd.AgentVitals) != 0 {
		t.Errorf("AgentVitals = %+v, want empty when field omitted", cmd.AgentVitals)
	}
}

func TestNormalizeAgentVitalSeverity_KnownValues(t *testing.T) {
	for _, s := range []string{"ok", "warning", "critical"} {
		if got := normalizeAgentVitalSeverity(s); got != s {
			t.Errorf("normalizeAgentVitalSeverity(%q) = %q, want %q", s, got, s)
		}
	}
}

func TestNormalizeAgentVitalSeverity_Unknown(t *testing.T) {
	for _, s := range []string{"OK", "Warning", "fatal", "", "info"} {
		if got := normalizeAgentVitalSeverity(s); got != "warning" {
			t.Errorf("normalizeAgentVitalSeverity(%q) = %q, want %q", s, got, "warning")
		}
	}
}

func TestEnrichHealthObservation_NonSysmon(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "guardian", "test-guardian")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	obs := "some observation"
	result := l.enrichHealthObservation(obs)
	if result != obs {
		t.Errorf("non-sysmon enrichment should be identity, got %q", result)
	}
}

func TestEnrichHealthObservation_Sysmon(t *testing.T) {
	provider := newMockProvider("test")
	agent := testHiveAgent(t, provider, "sysmon", "test-sysmon")

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
	})
	if err != nil {
		t.Fatal(err)
	}

	obs := "some observation"
	result := l.enrichHealthObservation(obs)
	if result == obs {
		t.Error("sysmon enrichment should add health metrics")
	}
	if !containsStr(result, "=== HEALTH METRICS ===") {
		t.Error("missing HEALTH METRICS header")
	}
	if !containsStr(result, "BUDGET:") {
		t.Error("missing BUDGET section")
	}
	if !containsStr(result, "HIVE:") {
		t.Error("missing HIVE section")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
