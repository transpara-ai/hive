package hive

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/transpara-ai/hive/pkg/safety"
)

// BootstrapProfile is the closed bootstrap-roster vocabulary exposed by the
// Civilization runtime.
type BootstrapProfile string

const (
	BootstrapProfileFull      BootstrapProfile = "full"
	BootstrapProfileOrganicV1 BootstrapProfile = "organic-v1"

	OrganicV1GrowthPolicyVersion  = "organic-v1"
	OrganicV1MaximumDynamicActors = 3
)

// BootstrapProfileError is returned for an explicitly unknown or empty
// bootstrap profile.
type BootstrapProfileError struct {
	Profile BootstrapProfile
}

func (e BootstrapProfileError) Error() string {
	return fmt.Sprintf("unknown bootstrap profile %q (want full or organic-v1)", e.Profile)
}

// ProtectedActionError is returned when configuration names an action outside
// the closed protected-action vocabulary.
type ProtectedActionError struct {
	Action safety.ProtectedAction
}

func (e ProtectedActionError) Error() string {
	return fmt.Sprintf("unknown protected action %q", e.Action)
}

// OrganicConfigError is returned when organic-v1 is paired with a setting
// that would weaken or contradict its bounded growth policy.
type OrganicConfigError struct {
	Reason string
}

func (e OrganicConfigError) Error() string {
	return "invalid organic-v1 configuration: " + e.Reason
}

// Validate rejects any value outside the typed profile vocabulary. An omitted
// Config profile is normalized to full before this method is called; callers
// of the public typed API must supply an explicit value.
func (p BootstrapProfile) Validate() error {
	switch p {
	case BootstrapProfileFull, BootstrapProfileOrganicV1:
		return nil
	default:
		return BootstrapProfileError{Profile: p}
	}
}

func normalizeConfigBootstrapProfile(p BootstrapProfile) (BootstrapProfile, error) {
	if p == "" {
		p = BootstrapProfileFull
	}
	if err := p.Validate(); err != nil {
		return "", err
	}
	return p, nil
}

// NormalizeProtectedActions validates, de-duplicates, and sorts a protected
// action allowlist so event evidence and admission checks are deterministic.
func NormalizeProtectedActions(actions []safety.ProtectedAction) ([]safety.ProtectedAction, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	seen := make(map[safety.ProtectedAction]struct{}, len(actions))
	normalized := make([]safety.ProtectedAction, 0, len(actions))
	for _, action := range actions {
		if !safety.IsProtectedAction(action) {
			return nil, ProtectedActionError{Action: action}
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		normalized = append(normalized, action)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})
	return normalized, nil
}

func validateBootstrapConfig(cfg Config) (BootstrapProfile, []safety.ProtectedAction, error) {
	profile, err := normalizeConfigBootstrapProfile(cfg.BootstrapProfile)
	if err != nil {
		return "", nil, err
	}
	actions, err := NormalizeProtectedActions(cfg.AutomaticallyApprovedActions)
	if err != nil {
		return "", nil, err
	}
	if profile != BootstrapProfileOrganicV1 {
		return profile, actions, nil
	}
	if cfg.ApproveRequests {
		return "", nil, OrganicConfigError{Reason: "broad ApproveRequests is forbidden"}
	}
	if cfg.MaximumDynamicActors != OrganicV1MaximumDynamicActors {
		return "", nil, OrganicConfigError{
			Reason: fmt.Sprintf("MaximumDynamicActors must be %d, got %d", OrganicV1MaximumDynamicActors, cfg.MaximumDynamicActors),
		}
	}
	if cfg.GrowthPolicyVersion != OrganicV1GrowthPolicyVersion {
		return "", nil, OrganicConfigError{
			Reason: fmt.Sprintf("GrowthPolicyVersion must be %q, got %q", OrganicV1GrowthPolicyVersion, cfg.GrowthPolicyVersion),
		}
	}
	for _, action := range actions {
		if action != safety.ActionAgentSpawnPersistent {
			return "", nil, OrganicConfigError{
				Reason: fmt.Sprintf("automatically approved action %q is forbidden", action),
			}
		}
	}
	return profile, actions, nil
}

// ValidateBootstrapConfig exposes the side-effect-free preflight used by CLI
// entry points before they open or bootstrap persistent stores.
func ValidateBootstrapConfig(cfg Config) error {
	_, _, err := validateBootstrapConfig(cfg)
	return err
}

func protectedActionStrings(actions []safety.ProtectedAction) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, len(actions))
	for i, action := range actions {
		out[i] = string(action)
	}
	return out
}

func protectedActionStringsFromSet(actions map[safety.ProtectedAction]struct{}) []string {
	if len(actions) == 0 {
		return nil
	}
	normalized := make([]safety.ProtectedAction, 0, len(actions))
	for action := range actions {
		normalized = append(normalized, action)
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})
	return protectedActionStrings(normalized)
}

func agentDefRoles(defs []AgentDef) []string {
	if len(defs) == 0 {
		return nil
	}
	roles := make([]string, len(defs))
	for i, def := range defs {
		roles[i] = def.Role
	}
	return roles
}

func (r *Runtime) automaticallyApproves(action safety.ProtectedAction) bool {
	if r == nil {
		return false
	}
	_, ok := r.automaticallyApprovedActions[action]
	return ok
}

func (r *Runtime) validateRegisteredBootstrapRoster() error {
	if r.bootstrapProfile != BootstrapProfileOrganicV1 {
		return nil
	}
	if len(r.membraneDefs) != 0 {
		return OrganicConfigError{Reason: "organic-v1 does not admit bootstrap membrane agents"}
	}
	expected, err := StarterAgentsForProfile(r.humanName, BootstrapProfileOrganicV1)
	if err != nil {
		return err
	}
	if len(r.defs) != len(expected) {
		return OrganicConfigError{
			Reason: fmt.Sprintf("constitutional kernel has %d agents, want exactly %d", len(r.defs), len(expected)),
		}
	}
	for i := range expected {
		got, want := r.defs[i], expected[i]
		if got.Name != want.Name || got.Role != want.Role {
			return OrganicConfigError{
				Reason: fmt.Sprintf(
					"constitutional kernel[%d] is %q/%q, want %q/%q",
					i, got.Name, got.Role, want.Name, want.Role,
				),
			}
		}
		if got.CanOperate || (got.RoleDefinition != nil && got.RoleDefinition.CanOperate) {
			return OrganicConfigError{
				Reason: fmt.Sprintf("constitutional kernel role %q has CanOperate=true", got.Role),
			}
		}
		if !reflect.DeepEqual(got, want) {
			return OrganicConfigError{
				Reason: fmt.Sprintf("constitutional kernel role %q differs from the organic-v1 definition", got.Role),
			}
		}
	}
	return nil
}
