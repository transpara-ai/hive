package sdr

import (
	"testing"
	"time"
)

func TestSDRConfigValid(t *testing.T) {
	cfg := NewSDRConfig("http://localhost:8000")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("SDR config should be valid: %v", err)
	}
}

func TestSDRConfigDefaults(t *testing.T) {
	cfg := NewSDRConfig("http://localhost:8000")

	if cfg.Name != "sdr" {
		t.Errorf("name = %q, want sdr", cfg.Name)
	}
	if cfg.Role != "membrane" {
		t.Errorf("role = %q, want membrane", cfg.Role)
	}
	if cfg.PollInterval != 30*time.Second {
		t.Errorf("poll interval = %v, want 30s", cfg.PollInterval)
	}
	if cfg.TrustThresholds.RequiredBelow != 0.3 {
		t.Errorf("required below = %v, want 0.3", cfg.TrustThresholds.RequiredBelow)
	}
	if len(cfg.InboundMappings) == 0 {
		t.Error("expected inbound mappings")
	}
}
