package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AlertThreshold defines percentage thresholds (0-100) for anomaly detection.
type AlertThreshold struct {
	NodeCPUPercent  float64 `yaml:"nodeCPUPercent"`
	NodeMemPercent  float64 `yaml:"nodeMemPercent"`
	NodeDiskPercent float64 `yaml:"nodeDiskPercent"`
	PodCPUPercent   float64 `yaml:"podCPUPercent"`
	PodMemPercent   float64 `yaml:"podMemPercent"`
}

// DefaultAlertThreshold returns sensible defaults for alert thresholds.
func DefaultAlertThreshold() AlertThreshold {
	return AlertThreshold{
		NodeCPUPercent:  80.0,
		NodeMemPercent:  80.0,
		NodeDiskPercent: 85.0,
		PodCPUPercent:   90.0,
		PodMemPercent:   90.0,
	}
}

// Validate checks that all threshold values are in the range 0-100.
func (a *AlertThreshold) Validate() error {
	var errs []error
	check := func(name string, v float64) {
		if v < 0 || v > 100 {
			errs = append(errs, fmt.Errorf("%s must be 0-100, got %.1f", name, v))
		}
	}

	check("nodeCPUPercent", a.NodeCPUPercent)
	check("nodeMemPercent", a.NodeMemPercent)
	check("nodeDiskPercent", a.NodeDiskPercent)
	check("podCPUPercent", a.PodCPUPercent)
	check("podMemPercent", a.PodMemPercent)

	return errors.Join(errs...)
}

// LoadAlertConfig reads alert thresholds from a YAML file.
func LoadAlertConfig(path string) (AlertThreshold, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AlertThreshold{}, fmt.Errorf("reading alert config: %w", err)
	}

	alerts := DefaultAlertThreshold()
	if err := yaml.Unmarshal(data, &alerts); err != nil {
		return AlertThreshold{}, fmt.Errorf("parsing alert config: %w", err)
	}

	return alerts, nil
}
