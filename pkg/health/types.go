// Package health provides types and logic for SysMon's operational health monitoring.
//
// MonitorReport is SysMon's internal assessment of hive health — agent vitals,
// budget consumption, and system-level indicators. It is NOT an event content type;
// the event graph has its own HealthReportContent for health.report events.
package health

// Severity indicates the overall health status.
type Severity string

const (
	SeverityOK       Severity = "ok"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// HeartbeatStatus tracks agent liveness.
type HeartbeatStatus string

const (
	HeartbeatOK       HeartbeatStatus = "ok"
	HeartbeatWarning  HeartbeatStatus = "warning"
	HeartbeatCritical HeartbeatStatus = "critical"
	HeartbeatSilent   HeartbeatStatus = "silent"
)

// MonitorReport is SysMon's internal rich assessment of hive health.
// This is the structured data SysMon builds each reporting interval,
// which gets summarized into a health.report event on the graph.
type MonitorReport struct {
	Tick        int64        `json:"tick"`
	Interval    int64        `json:"interval"`
	Severity    Severity     `json:"severity"`
	AgentVitals []AgentVital `json:"agent_vitals"`
	Budget      BudgetHealth `json:"budget"`
	Hive        HiveHealth   `json:"hive"`
	Anomalies   []Anomaly    `json:"anomalies"`
}

// AgentVital captures a single agent's health snapshot.
type AgentVital struct {
	Name           string          `json:"name"`
	Heartbeat      HeartbeatStatus `json:"heartbeat"`
	LastEventAge   int64           `json:"last_event_age_ticks"`
	State          string          `json:"state"`
	IterationsUsed int             `json:"iterations_used"`
	IterationsMax  int             `json:"iterations_max"`
	IterationsPct  float64         `json:"iterations_pct"`
	ErrorCount     int             `json:"error_count"`
	TrustScore     float64         `json:"trust_score"`
}

// BudgetHealth captures aggregate resource consumption.
// Base fields (TokensUsed, InputTokens, OutputTokens, CostUSD, Iterations)
// mirror resources.BudgetSnapshot. Computed fields provide derived metrics.
type BudgetHealth struct {
	TokensUsed        int64   `json:"tokens_used"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	CostUSD           float64 `json:"cost_usd"`
	Iterations        int     `json:"iterations"`
	DailyCapUSD       float64 `json:"daily_cap_usd"`
	DailyPct          float64 `json:"daily_pct"`
	BurnRatePerHour   float64 `json:"burn_rate_per_hour"`
	ProjectedDailyPct float64 `json:"projected_daily_pct"`
	ExhaustionCount   int     `json:"exhaustion_count"`
	TopAgentName      string  `json:"top_agent_name"`
	TopAgentPct       float64 `json:"top_agent_pct"`
}

// HiveHealth captures system-level indicators.
type HiveHealth struct {
	AgentsActive       int     `json:"agents_active"`
	AgentsExpected     int     `json:"agents_expected"`
	EventThroughput    int64   `json:"event_throughput"`
	ThroughputBaseline int64   `json:"throughput_baseline"`
	ThroughputPct      float64 `json:"throughput_pct"`
	ChainIntegrity     string  `json:"chain_integrity"`
	TrustTrend         string  `json:"trust_trend"`
	TrustDelta         float64 `json:"trust_delta"`
}

// Anomaly represents a single detected health issue.
type Anomaly struct {
	Severity    Severity `json:"severity"`
	Category    string   `json:"category"`
	Agent       string   `json:"agent,omitempty"`
	Description string   `json:"description"`
}
