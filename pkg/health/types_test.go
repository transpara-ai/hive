package health

import (
	"encoding/json"
	"testing"
)

func TestMonitorReportJSONRoundTrip(t *testing.T) {
	original := MonitorReport{
		Tick:     42,
		Interval: 5,
		Severity: SeverityWarning,
		AgentVitals: []AgentVital{
			{
				Name:           "implementer",
				Heartbeat:      HeartbeatOK,
				LastEventAge:   1,
				State:          "Processing",
				IterationsUsed: 30,
				IterationsMax:  100,
				IterationsPct:  30.0,
				ErrorCount:     0,
				TrustScore:     0.85,
			},
			{
				Name:           "guardian",
				Heartbeat:      HeartbeatWarning,
				LastEventAge:   3,
				State:          "Idle",
				IterationsUsed: 50,
				IterationsMax:  200,
				IterationsPct:  25.0,
				ErrorCount:     1,
				TrustScore:     0.92,
			},
		},
		Budget: BudgetHealth{
			TokensUsed:        150000,
			InputTokens:       100000,
			OutputTokens:      50000,
			CostUSD:           1.25,
			Iterations:        80,
			DailyCapUSD:       10.0,
			DailyPct:          12.5,
			BurnRatePerHour:   5000.0,
			ProjectedDailyPct: 45.0,
			ExhaustionCount:   0,
			TopAgentName:      "implementer",
			TopAgentPct:       42.0,
		},
		Hive: HiveHealth{
			AgentsActive:       4,
			AgentsExpected:     4,
			EventThroughput:    120,
			ThroughputBaseline: 100,
			ThroughputPct:      120.0,
			ChainIntegrity:     "ok",
			TrustTrend:         "stable",
			TrustDelta:         0.01,
		},
		Anomalies: []Anomaly{
			{
				Severity:    SeverityWarning,
				Category:    "budget",
				Agent:       "implementer",
				Description: "budget concentration at 42%",
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded MonitorReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify top-level fields.
	if decoded.Tick != original.Tick {
		t.Errorf("Tick = %d, want %d", decoded.Tick, original.Tick)
	}
	if decoded.Interval != original.Interval {
		t.Errorf("Interval = %d, want %d", decoded.Interval, original.Interval)
	}
	if decoded.Severity != original.Severity {
		t.Errorf("Severity = %q, want %q", decoded.Severity, original.Severity)
	}

	// Verify agent vitals round-trip.
	if len(decoded.AgentVitals) != len(original.AgentVitals) {
		t.Fatalf("AgentVitals length = %d, want %d", len(decoded.AgentVitals), len(original.AgentVitals))
	}
	for i, want := range original.AgentVitals {
		got := decoded.AgentVitals[i]
		if got.Name != want.Name {
			t.Errorf("AgentVitals[%d].Name = %q, want %q", i, got.Name, want.Name)
		}
		if got.Heartbeat != want.Heartbeat {
			t.Errorf("AgentVitals[%d].Heartbeat = %q, want %q", i, got.Heartbeat, want.Heartbeat)
		}
		if got.LastEventAge != want.LastEventAge {
			t.Errorf("AgentVitals[%d].LastEventAge = %d, want %d", i, got.LastEventAge, want.LastEventAge)
		}
		if got.State != want.State {
			t.Errorf("AgentVitals[%d].State = %q, want %q", i, got.State, want.State)
		}
		if got.IterationsUsed != want.IterationsUsed {
			t.Errorf("AgentVitals[%d].IterationsUsed = %d, want %d", i, got.IterationsUsed, want.IterationsUsed)
		}
		if got.IterationsMax != want.IterationsMax {
			t.Errorf("AgentVitals[%d].IterationsMax = %d, want %d", i, got.IterationsMax, want.IterationsMax)
		}
		if got.IterationsPct != want.IterationsPct {
			t.Errorf("AgentVitals[%d].IterationsPct = %f, want %f", i, got.IterationsPct, want.IterationsPct)
		}
		if got.ErrorCount != want.ErrorCount {
			t.Errorf("AgentVitals[%d].ErrorCount = %d, want %d", i, got.ErrorCount, want.ErrorCount)
		}
		if got.TrustScore != want.TrustScore {
			t.Errorf("AgentVitals[%d].TrustScore = %f, want %f", i, got.TrustScore, want.TrustScore)
		}
	}

	// Verify budget health round-trip.
	b := decoded.Budget
	if b.TokensUsed != original.Budget.TokensUsed {
		t.Errorf("Budget.TokensUsed = %d, want %d", b.TokensUsed, original.Budget.TokensUsed)
	}
	if b.InputTokens != original.Budget.InputTokens {
		t.Errorf("Budget.InputTokens = %d, want %d", b.InputTokens, original.Budget.InputTokens)
	}
	if b.OutputTokens != original.Budget.OutputTokens {
		t.Errorf("Budget.OutputTokens = %d, want %d", b.OutputTokens, original.Budget.OutputTokens)
	}
	if b.CostUSD != original.Budget.CostUSD {
		t.Errorf("Budget.CostUSD = %f, want %f", b.CostUSD, original.Budget.CostUSD)
	}
	if b.Iterations != original.Budget.Iterations {
		t.Errorf("Budget.Iterations = %d, want %d", b.Iterations, original.Budget.Iterations)
	}
	if b.DailyCapUSD != original.Budget.DailyCapUSD {
		t.Errorf("Budget.DailyCapUSD = %f, want %f", b.DailyCapUSD, original.Budget.DailyCapUSD)
	}
	if b.DailyPct != original.Budget.DailyPct {
		t.Errorf("Budget.DailyPct = %f, want %f", b.DailyPct, original.Budget.DailyPct)
	}
	if b.BurnRatePerHour != original.Budget.BurnRatePerHour {
		t.Errorf("Budget.BurnRatePerHour = %f, want %f", b.BurnRatePerHour, original.Budget.BurnRatePerHour)
	}
	if b.ProjectedDailyPct != original.Budget.ProjectedDailyPct {
		t.Errorf("Budget.ProjectedDailyPct = %f, want %f", b.ProjectedDailyPct, original.Budget.ProjectedDailyPct)
	}
	if b.ExhaustionCount != original.Budget.ExhaustionCount {
		t.Errorf("Budget.ExhaustionCount = %d, want %d", b.ExhaustionCount, original.Budget.ExhaustionCount)
	}
	if b.TopAgentName != original.Budget.TopAgentName {
		t.Errorf("Budget.TopAgentName = %q, want %q", b.TopAgentName, original.Budget.TopAgentName)
	}
	if b.TopAgentPct != original.Budget.TopAgentPct {
		t.Errorf("Budget.TopAgentPct = %f, want %f", b.TopAgentPct, original.Budget.TopAgentPct)
	}

	// Verify hive health round-trip.
	h := decoded.Hive
	if h.AgentsActive != original.Hive.AgentsActive {
		t.Errorf("Hive.AgentsActive = %d, want %d", h.AgentsActive, original.Hive.AgentsActive)
	}
	if h.AgentsExpected != original.Hive.AgentsExpected {
		t.Errorf("Hive.AgentsExpected = %d, want %d", h.AgentsExpected, original.Hive.AgentsExpected)
	}
	if h.EventThroughput != original.Hive.EventThroughput {
		t.Errorf("Hive.EventThroughput = %d, want %d", h.EventThroughput, original.Hive.EventThroughput)
	}
	if h.ThroughputBaseline != original.Hive.ThroughputBaseline {
		t.Errorf("Hive.ThroughputBaseline = %d, want %d", h.ThroughputBaseline, original.Hive.ThroughputBaseline)
	}
	if h.ThroughputPct != original.Hive.ThroughputPct {
		t.Errorf("Hive.ThroughputPct = %f, want %f", h.ThroughputPct, original.Hive.ThroughputPct)
	}
	if h.ChainIntegrity != original.Hive.ChainIntegrity {
		t.Errorf("Hive.ChainIntegrity = %q, want %q", h.ChainIntegrity, original.Hive.ChainIntegrity)
	}
	if h.TrustTrend != original.Hive.TrustTrend {
		t.Errorf("Hive.TrustTrend = %q, want %q", h.TrustTrend, original.Hive.TrustTrend)
	}
	if h.TrustDelta != original.Hive.TrustDelta {
		t.Errorf("Hive.TrustDelta = %f, want %f", h.TrustDelta, original.Hive.TrustDelta)
	}

	// Verify anomalies round-trip.
	if len(decoded.Anomalies) != len(original.Anomalies) {
		t.Fatalf("Anomalies length = %d, want %d", len(decoded.Anomalies), len(original.Anomalies))
	}
	a := decoded.Anomalies[0]
	if a.Severity != original.Anomalies[0].Severity {
		t.Errorf("Anomalies[0].Severity = %q, want %q", a.Severity, original.Anomalies[0].Severity)
	}
	if a.Category != original.Anomalies[0].Category {
		t.Errorf("Anomalies[0].Category = %q, want %q", a.Category, original.Anomalies[0].Category)
	}
	if a.Agent != original.Anomalies[0].Agent {
		t.Errorf("Anomalies[0].Agent = %q, want %q", a.Agent, original.Anomalies[0].Agent)
	}
	if a.Description != original.Anomalies[0].Description {
		t.Errorf("Anomalies[0].Description = %q, want %q", a.Description, original.Anomalies[0].Description)
	}
}

func TestMonitorReportEmptyAnomaliesRoundTrip(t *testing.T) {
	original := MonitorReport{
		Tick:     1,
		Severity: SeverityOK,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded MonitorReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Tick != 1 {
		t.Errorf("Tick = %d, want 1", decoded.Tick)
	}
	if decoded.Severity != SeverityOK {
		t.Errorf("Severity = %q, want %q", decoded.Severity, SeverityOK)
	}
}

func TestAnomalyOmitsEmptyAgent(t *testing.T) {
	a := Anomaly{
		Severity:    SeverityCritical,
		Category:    "hive",
		Description: "chain broken",
	}

	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Agent field should be omitted when empty.
	s := string(data)
	if contains(s, `"agent"`) {
		t.Errorf("expected agent field to be omitted, got: %s", s)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
