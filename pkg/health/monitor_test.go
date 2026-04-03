package health

import "testing"

func TestClassifyHeartbeat(t *testing.T) {
	cfg := DefaultConfig() // Warning=2, Critical=5

	tests := []struct {
		name            string
		ticksSinceEvent int64
		want            HeartbeatStatus
	}{
		{"recent event", 0, HeartbeatOK},
		{"one tick ago", 1, HeartbeatOK},
		{"at warning boundary", 2, HeartbeatWarning},
		{"between warning and critical", 3, HeartbeatWarning},
		{"just below critical", 4, HeartbeatWarning},
		{"at critical boundary", 5, HeartbeatCritical},
		{"well past critical", 10, HeartbeatCritical},
		{"no events ever", -1, HeartbeatSilent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyHeartbeat(tt.ticksSinceEvent, cfg)
			if got != tt.want {
				t.Errorf("ClassifyHeartbeat(%d) = %q, want %q", tt.ticksSinceEvent, got, tt.want)
			}
		})
	}
}

func TestClassifyIterationBurn(t *testing.T) {
	cfg := DefaultConfig() // Warning=70, Critical=90

	tests := []struct {
		name string
		used int
		max  int
		want Severity
	}{
		{"0 percent", 0, 100, SeverityOK},
		{"50 percent", 50, 100, SeverityOK},
		{"69 percent", 69, 100, SeverityOK},
		{"70 percent — at warning", 70, 100, SeverityWarning},
		{"75 percent", 75, 100, SeverityWarning},
		{"89 percent", 89, 100, SeverityWarning},
		{"90 percent — at critical", 90, 100, SeverityCritical},
		{"100 percent", 100, 100, SeverityCritical},
		{"unlimited max", 500, 0, SeverityOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyIterationBurn(tt.used, tt.max, cfg)
			if got != tt.want {
				t.Errorf("ClassifyIterationBurn(%d, %d) = %q, want %q", tt.used, tt.max, got, tt.want)
			}
		})
	}
}

func TestCheckBudgetProjection(t *testing.T) {
	cfg := DefaultConfig() // Warning=80, Critical=95

	tests := []struct {
		name         string
		costUSD      float64
		dailyCapUSD  float64
		hoursElapsed float64
		wantSeverity Severity
		wantPctMin   float64
		wantPctMax   float64
	}{
		{"low spend", 1.0, 10.0, 12.0, SeverityOK, 0, 80},
		{"warning projection", 4.0, 10.0, 12.0, SeverityWarning, 80, 95},
		{"critical projection", 5.0, 10.0, 12.0, SeverityCritical, 95, 200},
		{"no cap", 5.0, 0, 12.0, SeverityOK, 0, 0},
		{"zero hours elapsed", 5.0, 10.0, 0, SeverityOK, 0, 80},
		{"tiny elapsed big spend", 9.0, 10.0, 1.0, SeverityCritical, 95, 99999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSev, gotPct := CheckBudgetProjection(tt.costUSD, tt.dailyCapUSD, tt.hoursElapsed, cfg)
			if gotSev != tt.wantSeverity {
				t.Errorf("severity = %q, want %q (pct=%.1f)", gotSev, tt.wantSeverity, gotPct)
			}
			if gotPct < tt.wantPctMin || gotPct > tt.wantPctMax {
				t.Errorf("projected pct = %.1f, want [%.1f, %.1f]", gotPct, tt.wantPctMin, tt.wantPctMax)
			}
		})
	}
}

func TestCheckBudgetConcentration(t *testing.T) {
	t.Run("single agent above threshold", func(t *testing.T) {
		vitals := []AgentVital{
			{Name: "implementer", IterationsUsed: 50},
			{Name: "planner", IterationsUsed: 30},
			{Name: "guardian", IterationsUsed: 20},
		}
		anomalies := CheckBudgetConcentration(vitals, 40.0)
		if len(anomalies) != 1 {
			t.Fatalf("got %d anomalies, want 1", len(anomalies))
		}
		if anomalies[0].Agent != "implementer" {
			t.Errorf("agent = %q, want %q", anomalies[0].Agent, "implementer")
		}
		if anomalies[0].Severity != SeverityWarning {
			t.Errorf("severity = %q, want %q", anomalies[0].Severity, SeverityWarning)
		}
	})

	t.Run("no agent above threshold", func(t *testing.T) {
		vitals := []AgentVital{
			{Name: "a", IterationsUsed: 30},
			{Name: "b", IterationsUsed: 35},
			{Name: "c", IterationsUsed: 35},
		}
		anomalies := CheckBudgetConcentration(vitals, 40.0)
		if len(anomalies) != 0 {
			t.Errorf("got %d anomalies, want 0", len(anomalies))
		}
	})

	t.Run("multiple agents above threshold", func(t *testing.T) {
		vitals := []AgentVital{
			{Name: "a", IterationsUsed: 45},
			{Name: "b", IterationsUsed: 45},
			{Name: "c", IterationsUsed: 10},
		}
		anomalies := CheckBudgetConcentration(vitals, 40.0)
		if len(anomalies) != 2 {
			t.Fatalf("got %d anomalies, want 2", len(anomalies))
		}
	})

	t.Run("zero total iterations", func(t *testing.T) {
		vitals := []AgentVital{
			{Name: "a", IterationsUsed: 0},
		}
		anomalies := CheckBudgetConcentration(vitals, 40.0)
		if len(anomalies) != 0 {
			t.Errorf("got %d anomalies, want 0", len(anomalies))
		}
	})
}

func TestCheckThroughput(t *testing.T) {
	cfg := DefaultConfig() // Low=30, High=300

	tests := []struct {
		name         string
		current      int64
		baseline     int64
		wantSeverity Severity
	}{
		{"normal range", 100, 100, SeverityOK},
		{"at low boundary", 30, 100, SeverityOK},
		{"below low — too quiet", 29, 100, SeverityWarning},
		{"at high boundary", 300, 100, SeverityOK},
		{"above high — event storm", 301, 100, SeverityWarning},
		{"no baseline", 100, 0, SeverityOK},
		{"zero current with baseline", 0, 100, SeverityWarning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSev, _ := CheckThroughput(tt.current, tt.baseline, cfg)
			if gotSev != tt.wantSeverity {
				t.Errorf("CheckThroughput(%d, %d) severity = %q, want %q",
					tt.current, tt.baseline, gotSev, tt.wantSeverity)
			}
		})
	}
}

func TestComputeOverallSeverity(t *testing.T) {
	tests := []struct {
		name      string
		anomalies []Anomaly
		want      Severity
	}{
		{"empty — all ok", nil, SeverityOK},
		{"empty slice", []Anomaly{}, SeverityOK},
		{"warning only", []Anomaly{
			{Severity: SeverityWarning},
		}, SeverityWarning},
		{"multiple warnings", []Anomaly{
			{Severity: SeverityWarning},
			{Severity: SeverityWarning},
		}, SeverityWarning},
		{"any critical wins", []Anomaly{
			{Severity: SeverityWarning},
			{Severity: SeverityCritical},
			{Severity: SeverityWarning},
		}, SeverityCritical},
		{"critical only", []Anomaly{
			{Severity: SeverityCritical},
		}, SeverityCritical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeOverallSeverity(tt.anomalies)
			if got != tt.want {
				t.Errorf("ComputeOverallSeverity() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildReport(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("healthy hive", func(t *testing.T) {
		report := BuildReport(BuildReportInput{
			Tick:     10,
			Interval: 5,
			AgentVitals: []AgentVital{
				{Name: "strategist", Heartbeat: HeartbeatOK, IterationsUsed: 5, IterationsMax: 50, IterationsPct: 10},
				{Name: "planner", Heartbeat: HeartbeatOK, IterationsUsed: 4, IterationsMax: 50, IterationsPct: 8},
				{Name: "implementer", Heartbeat: HeartbeatOK, IterationsUsed: 6, IterationsMax: 100, IterationsPct: 6},
				{Name: "guardian", Heartbeat: HeartbeatOK, IterationsUsed: 5, IterationsMax: 200, IterationsPct: 2.5},
			},
			Budget: BudgetHealth{CostUSD: 0.50, DailyCapUSD: 10.0, ProjectedDailyPct: 12.0},
			Hive:   HiveHealth{AgentsActive: 4, AgentsExpected: 4, EventThroughput: 100, ThroughputBaseline: 100, ThroughputPct: 100},
			Cfg:    cfg,
		})

		if report.Tick != 10 {
			t.Errorf("Tick = %d, want 10", report.Tick)
		}
		if report.Interval != 5 {
			t.Errorf("Interval = %d, want 5", report.Interval)
		}
		if report.Severity != SeverityOK {
			t.Errorf("Severity = %q, want %q (anomalies: %v)", report.Severity, SeverityOK, report.Anomalies)
		}
		if len(report.Anomalies) != 0 {
			t.Errorf("got %d anomalies, want 0: %v", len(report.Anomalies), report.Anomalies)
		}
	})

	t.Run("unhealthy agent triggers anomalies", func(t *testing.T) {
		report := BuildReport(BuildReportInput{
			Tick:     20,
			Interval: 5,
			AgentVitals: []AgentVital{
				{Name: "implementer", Heartbeat: HeartbeatCritical, IterationsUsed: 95, IterationsMax: 100, IterationsPct: 95},
				{Name: "guardian", Heartbeat: HeartbeatOK, IterationsUsed: 10, IterationsMax: 200, IterationsPct: 5},
			},
			Budget: BudgetHealth{CostUSD: 1.0, DailyCapUSD: 10.0, ProjectedDailyPct: 50.0},
			Hive:   HiveHealth{AgentsActive: 4, AgentsExpected: 4, EventThroughput: 100, ThroughputBaseline: 100, ThroughputPct: 100},
			Cfg:    cfg,
		})

		if report.Severity != SeverityCritical {
			t.Errorf("Severity = %q, want %q", report.Severity, SeverityCritical)
		}

		// Expect: heartbeat critical + iteration burn critical + budget concentration
		foundHeartbeat := false
		foundIteration := false
		for _, a := range report.Anomalies {
			if a.Agent == "implementer" && a.Category == "agent" {
				if a.Severity == SeverityCritical {
					foundHeartbeat = true
				}
			}
			if a.Agent == "implementer" && a.Description == "iteration burn at 95%" {
				foundIteration = true
			}
		}
		if !foundHeartbeat {
			t.Error("missing heartbeat critical anomaly for implementer")
		}
		if !foundIteration {
			t.Error("missing iteration burn anomaly for implementer")
		}
	})

	t.Run("budget exhaustion is always critical", func(t *testing.T) {
		report := BuildReport(BuildReportInput{
			Tick:     30,
			Interval: 5,
			Budget:   BudgetHealth{ExhaustionCount: 1, DailyCapUSD: 10.0},
			Hive:     HiveHealth{AgentsActive: 4, AgentsExpected: 4},
			Cfg:      cfg,
		})

		if report.Severity != SeverityCritical {
			t.Errorf("Severity = %q, want %q", report.Severity, SeverityCritical)
		}
	})

	t.Run("missing agents triggers anomaly", func(t *testing.T) {
		report := BuildReport(BuildReportInput{
			Tick:     40,
			Interval: 5,
			Hive:     HiveHealth{AgentsActive: 3, AgentsExpected: 5, EventThroughput: 100, ThroughputBaseline: 100, ThroughputPct: 100},
			Cfg:      cfg,
		})

		found := false
		for _, a := range report.Anomalies {
			if a.Category == "hive" && a.Severity == SeverityWarning {
				found = true
			}
		}
		if !found {
			t.Error("expected hive anomaly for missing agents")
		}
	})

	t.Run("degraded hive is critical", func(t *testing.T) {
		report := BuildReport(BuildReportInput{
			Tick:     50,
			Interval: 5,
			Hive:     HiveHealth{AgentsActive: 2, AgentsExpected: 5, EventThroughput: 100, ThroughputBaseline: 100, ThroughputPct: 100},
			Cfg:      cfg,
		})

		found := false
		for _, a := range report.Anomalies {
			if a.Category == "hive" && a.Severity == SeverityCritical {
				found = true
			}
		}
		if !found {
			t.Error("expected critical hive anomaly for degraded hive (<=2 agents)")
		}
	})

	t.Run("additional anomalies are included", func(t *testing.T) {
		extra := Anomaly{Severity: SeverityWarning, Category: "chain", Description: "test anomaly"}
		report := BuildReport(BuildReportInput{
			Tick:                50,
			Interval:            5,
			Hive:                HiveHealth{AgentsActive: 4, AgentsExpected: 4, EventThroughput: 100, ThroughputBaseline: 100, ThroughputPct: 100},
			AdditionalAnomalies: []Anomaly{extra},
			Cfg:                 cfg,
		})

		found := false
		for _, a := range report.Anomalies {
			if a.Description == "test anomaly" {
				found = true
			}
		}
		if !found {
			t.Error("additional anomaly not included in report")
		}
	})
}
