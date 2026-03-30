package membrane

import (
	"strings"
	"testing"
)

func TestMembraneConfigValidate(t *testing.T) {
	valid := MembraneConfig{
		Name:            "test-membrane",
		Role:            "membrane",
		Model:           "claude-sonnet-4-6",
		SystemPrompt:    "You are a test membrane agent.",
		ServiceEndpoint: "http://localhost:8000",
		PollInterval:    30,
		AuthMethod:      "api_key",
	}

	if err := valid.Validate(); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}

	tests := []struct {
		name    string
		modify  func(*MembraneConfig)
		wantErr string
	}{
		{"missing name", func(c *MembraneConfig) { c.Name = "" }, "Name"},
		{"missing role", func(c *MembraneConfig) { c.Role = "" }, "Role"},
		{"missing model", func(c *MembraneConfig) { c.Model = "" }, "Model"},
		{"missing system prompt", func(c *MembraneConfig) { c.SystemPrompt = "" }, "SystemPrompt"},
		{"missing endpoint", func(c *MembraneConfig) { c.ServiceEndpoint = "" }, "ServiceEndpoint"},
		{"zero poll interval", func(c *MembraneConfig) { c.PollInterval = 0 }, "PollInterval"},
		{"invalid auth method", func(c *MembraneConfig) { c.AuthMethod = "magic" }, "AuthMethod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid
			tt.modify(&c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestTrustBandsAuthority(t *testing.T) {
	bands := TrustBands{RequiredBelow: 0.3, RecommendedBelow: 0.6}

	tests := []struct {
		trust float64
		want  AuthorityLevel
	}{
		{0.0, AuthRequired},
		{0.15, AuthRequired},
		{0.29, AuthRequired},
		{0.3, AuthRecommended},
		{0.45, AuthRecommended},
		{0.59, AuthRecommended},
		{0.6, AuthNotification},
		{0.8, AuthNotification},
		{1.0, AuthNotification},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := bands.AuthorityFor(tt.trust)
			if got != tt.want {
				t.Errorf("trust %.2f: got %v, want %v", tt.trust, got, tt.want)
			}
		})
	}
}
