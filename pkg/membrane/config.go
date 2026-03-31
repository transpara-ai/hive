package membrane

import (
	"fmt"
	"strings"
	"time"
)

// AuthorityLevel represents the trust-derived approval requirement.
type AuthorityLevel int

const (
	AuthRequired     AuthorityLevel = iota // blocks until human approves
	AuthRecommended                        // auto-approves after timeout
	AuthNotification                       // immediate, logged only
)

func (a AuthorityLevel) String() string {
	switch a {
	case AuthRequired:
		return "required"
	case AuthRecommended:
		return "recommended"
	case AuthNotification:
		return "notification"
	default:
		return "unknown"
	}
}

// MembraneConfig defines a membrane agent instance.
type MembraneConfig struct {
	Name         string
	Role         string
	Model        string
	SystemPrompt string

	ServiceEndpoint string
	PollInterval    time.Duration
	AuthMethod      string            // "api_key", "oauth", "bearer"
	AuthConfig      map[string]string // method-specific config

	InboundMappings  []InboundMapping
	OutboundMappings []OutboundMapping

	EscalationTargets map[string]HumanTarget
	TrustThresholds   TrustBands

	WatchPatterns []string
	MaxIterations int
	MaxDuration   time.Duration
	GuardianHints []string
}

// InboundMapping translates a service event to an EventGraph event.
type InboundMapping struct {
	ServiceEvent string // e.g. "lead.state.qualified_handoff"
	GraphEvent   string // e.g. "agent.escalated"
	TransformID  string // registered transform name
}

// OutboundMapping translates a human decision to a service API call.
type OutboundMapping struct {
	GraphEvent    string // e.g. "bridge.action.approved"
	ServiceMethod string // HTTP method
	ServicePath   string // e.g. "/copilot/drafts/{id}/approve"
	TransformID   string
}

// HumanTarget identifies a human for escalation routing.
type HumanTarget struct {
	ActorID        string
	NotifyChannels []string // ["email", "teams"]
	Description    string
}

// TrustBands maps trust score ranges to authority levels.
type TrustBands struct {
	RequiredBelow    float64 // default 0.3
	RecommendedBelow float64 // default 0.6
}

// AuthorityFor returns the authority level for the given trust score.
func (tb TrustBands) AuthorityFor(trust float64) AuthorityLevel {
	if trust < tb.RequiredBelow {
		return AuthRequired
	}
	if trust < tb.RecommendedBelow {
		return AuthRecommended
	}
	return AuthNotification
}

var validAuthMethods = map[string]bool{
	"api_key": true,
	"oauth":   true,
	"bearer":  true,
}

// Validate checks all required fields.
func (c MembraneConfig) Validate() error {
	var errs []string
	if c.Name == "" {
		errs = append(errs, "Name is required")
	}
	if c.Role == "" {
		errs = append(errs, "Role is required")
	}
	if c.Model == "" {
		errs = append(errs, "Model is required")
	}
	if c.SystemPrompt == "" {
		errs = append(errs, "SystemPrompt is required")
	}
	if c.ServiceEndpoint == "" {
		errs = append(errs, "ServiceEndpoint is required")
	}
	if c.PollInterval <= 0 {
		errs = append(errs, "PollInterval must be positive")
	}
	if !validAuthMethods[c.AuthMethod] {
		errs = append(errs, fmt.Sprintf("AuthMethod %q is not valid (use api_key, oauth, or bearer)", c.AuthMethod))
	}
	if len(errs) > 0 {
		return fmt.Errorf("membrane config: %s", strings.Join(errs, "; "))
	}
	return nil
}

// EffectiveMaxIterations returns MaxIterations. 0 means unlimited.
func (c MembraneConfig) EffectiveMaxIterations() int {
	return c.MaxIterations
}

// EffectiveMaxDuration returns MaxDuration. 0 means unlimited.
func (c MembraneConfig) EffectiveMaxDuration() time.Duration {
	return c.MaxDuration
}
