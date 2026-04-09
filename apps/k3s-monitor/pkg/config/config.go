// Package config provides configuration types and loading for k3s-monitor.
package config

import (
	"errors"
	"fmt"
	"time"
)

// Config holds all runtime settings for the k3s monitoring app.
type Config struct {
	// Kubeconfig is the path to the kubeconfig file. Empty means use InCluster.
	Kubeconfig string `yaml:"kubeconfig"`

	// InCluster uses the pod's service account for kubernetes auth.
	InCluster bool `yaml:"inCluster"`

	// ScrapeInterval is how often metrics are collected.
	ScrapeInterval time.Duration `yaml:"scrapeInterval"`

	// Retention is how long metrics are kept in the store.
	Retention time.Duration `yaml:"retention"`

	// Addr is the HTTP listen address for the API server.
	Addr string `yaml:"addr"`

	// Alerts defines thresholds for anomaly detection.
	Alerts AlertThreshold `yaml:"alerts"`
}

// Validate checks that all config values are within acceptable ranges.
func (c *Config) Validate() error {
	var errs []error

	if c.ScrapeInterval <= 0 {
		errs = append(errs, fmt.Errorf("scrape-interval must be positive, got %s", c.ScrapeInterval))
	}
	if c.Retention <= 0 {
		errs = append(errs, fmt.Errorf("retention must be positive, got %s", c.Retention))
	}
	if c.Retention < c.ScrapeInterval {
		errs = append(errs, fmt.Errorf("retention (%s) must be >= scrape-interval (%s)", c.Retention, c.ScrapeInterval))
	}
	if c.Addr == "" {
		errs = append(errs, fmt.Errorf("addr must not be empty"))
	}
	if err := c.Alerts.Validate(); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
