// Package anomaly detects threshold violations and anomalies in collected metrics.
package anomaly

import "time"

// Severity classifies how urgent an alert is.
type Severity string

const (
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Alert represents an active threshold violation.
type Alert struct {
	ID        string   `json:"id"`
	Severity  Severity `json:"severity"`
	Resource  string   `json:"resource"`  // e.g. "node/worker-1" or "pod/default/nginx"
	Metric    string   `json:"metric"`    // e.g. "cpu", "memory", "disk"
	Value     float64  `json:"value"`     // current value (percent)
	Threshold float64  `json:"threshold"` // threshold that was exceeded
	Message   string   `json:"message"`
	Since     time.Time `json:"since"`
}

// Detector provides access to active alerts.
type Detector interface {
	// Active returns all currently active alerts.
	Active() []Alert
}
