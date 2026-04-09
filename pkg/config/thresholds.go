// Package config provides configuration types for the hive monitoring stack.
package config

// AlertThreshold defines percentage thresholds that trigger anomaly detection.
// All values are fractions in the range 0.0-1.0.
type AlertThreshold struct {
	CPUWarning  float64 // CPU usage fraction that triggers a warning (default 0.80)
	CPUCritical float64 // CPU usage fraction that triggers a critical alert (default 0.95)
	MemWarning  float64 // Memory usage fraction that triggers a warning (default 0.80)
	MemCritical float64 // Memory usage fraction that triggers a critical alert (default 0.95)
	DiskWarning float64 // Disk usage fraction that triggers a warning (default 0.85)
	DiskCritical float64 // Disk usage fraction that triggers a critical alert (default 0.95)
}

// DefaultAlertThreshold returns sensible defaults for alert thresholds.
func DefaultAlertThreshold() AlertThreshold {
	return AlertThreshold{
		CPUWarning:   0.80,
		CPUCritical:  0.95,
		MemWarning:   0.80,
		MemCritical:  0.95,
		DiskWarning:  0.85,
		DiskCritical: 0.95,
	}
}
