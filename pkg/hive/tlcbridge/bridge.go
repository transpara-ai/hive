// Package tlcbridge validates the thin TLC change brief at Hive's ingress.
//
// TLC owns route selection and the short brief. Hive adds source identity,
// repository-effect containment, and an idempotency key used by its existing
// durable EventGraph/Work dispatch. This package deliberately owns no second
// workflow state machine and enables no runtime route by itself.
package tlcbridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const BriefSchemaVersion = "tlc-change-brief/v1"

type Route string

const (
	RouteRoutine  Route = "Routine"
	RouteDesigned Route = "Designed"
	RouteCritical Route = "Critical"
)

type Brief struct {
	Outcome     string   `json:"outcome"`
	Scope       []string `json:"scope"`
	NonGoals    []string `json:"non_goals"`
	Assumptions []string `json:"assumptions"`
	Constraints []string `json:"constraints"`
	Tests       []string `json:"tests"`
	NextAction  string   `json:"next_action"`
}

type Clarification struct {
	Rounds    int      `json:"rounds"`
	Questions []string `json:"questions"`
}

type CriticalObservation struct {
	Trigger  string `json:"trigger"`
	Evidence string `json:"evidence"`
}

type Critical struct {
	Observations            []CriticalObservation `json:"observations"`
	SeparateEffectAuthority bool                  `json:"separate_effect_authority"`
}

type Fable struct {
	SelectedByHuman       bool   `json:"selected_by_human"`
	Effort                string `json:"effort"`
	MaximumCollaborations int    `json:"maximum_collaborations"`
	AdvisoryOnly          bool   `json:"advisory_only"`
	ReviewCredit          bool   `json:"review_credit"`
}

type ChangeBrief struct {
	SchemaVersion string         `json:"schema_version"`
	Route         Route          `json:"route"`
	Brief         Brief          `json:"brief"`
	Clarification *Clarification `json:"clarification,omitempty"`
	Critical      *Critical      `json:"critical,omitempty"`
	Fable         *Fable         `json:"fable,omitempty"`
}

type SourceKind string

const (
	SourceHuman SourceKind = "human"
	SourceIssue SourceKind = "issue"
)

// Source is Hive-private provenance supplied independently of the TLC brief.
type Source struct {
	Kind       SourceKind
	Identity   string
	Repository string
}

// RepositoryEffects is constructed from Source.Repository. There is no
// caller-supplied override, so an Issue in RepoX can target only RepoX.
type RepositoryEffects struct {
	WorktreeRepository    string
	PullRequestRepository string
}

// BoundRequest is ready for Hive's existing durable dispatcher. The TLC brief
// contains no source chain, retry, worktree, provider, or effect state.
type BoundRequest struct {
	Source         Source
	Change         ChangeBrief
	Effects        RepositoryEffects
	IdempotencyKey string
}

var (
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	criticalTriggers  = map[string]struct{}{
		"credentials_auth_crypto_secrets":           {},
		"destructive_irreversible":                  {},
		"safety_financial_legal_regulatory_privacy": {},
		"production_runtime_enforcement":            {},
		"governance_review_approval_repo_settings":  {},
		"major_trust_or_security_architecture":      {},
		"material_residual_risk_acceptance":         {},
	}
)

// Bind validates TLC's complete public object, then adds only Hive-owned
// source and effect-routing state. It performs no persistence or effects.
func Bind(source Source, raw []byte) (BoundRequest, error) {
	source.Identity = strings.TrimSpace(source.Identity)
	source.Repository = strings.TrimSpace(source.Repository)
	if err := validateSource(source); err != nil {
		return BoundRequest{}, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return BoundRequest{}, errors.New("TLC change brief is empty")
	}
	var change ChangeBrief
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&change); err != nil {
		return BoundRequest{}, fmt.Errorf("decode TLC change brief: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return BoundRequest{}, err
	}
	if err := validateChange(change); err != nil {
		return BoundRequest{}, err
	}
	canonical, err := json.Marshal(change)
	if err != nil {
		return BoundRequest{}, fmt.Errorf("canonicalize TLC change brief: %w", err)
	}
	digestInput := strings.Join(
		[]string{string(source.Kind), source.Identity, source.Repository, string(canonical)},
		"\x00",
	)
	digest := sha256.Sum256([]byte(digestInput))
	return BoundRequest{
		Source: source,
		Change: change,
		Effects: RepositoryEffects{
			WorktreeRepository:    source.Repository,
			PullRequestRepository: source.Repository,
		},
		IdempotencyKey: "tlc-brief-" + hex.EncodeToString(digest[:]),
	}, nil
}

func validateSource(source Source) error {
	if source.Kind != SourceHuman && source.Kind != SourceIssue {
		return fmt.Errorf("unsupported source kind %q", source.Kind)
	}
	if strings.TrimSpace(source.Identity) == "" {
		return errors.New("source identity is required")
	}
	if !repositoryPattern.MatchString(source.Repository) {
		return fmt.Errorf("source repository %q must be owner/repo", source.Repository)
	}
	parts := strings.Split(source.Repository, "/")
	for _, part := range parts {
		if part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return fmt.Errorf("source repository %q is unsafe", source.Repository)
		}
	}
	return nil
}

func validateChange(change ChangeBrief) error {
	if change.SchemaVersion != BriefSchemaVersion {
		return fmt.Errorf("unsupported TLC change brief schema %q", change.SchemaVersion)
	}
	if err := validateBrief(change.Brief); err != nil {
		return err
	}
	if change.Clarification != nil {
		if change.Clarification.Rounds != 1 {
			return errors.New("clarification must contain exactly one grouped round")
		}
		if err := validateStrings("clarification.questions", change.Clarification.Questions, true); err != nil {
			return err
		}
	}
	if change.Fable != nil {
		if change.Route == RouteRoutine {
			return errors.New("Fable is unavailable for Routine work")
		}
		if !change.Fable.SelectedByHuman || change.Fable.Effort != "maximum" ||
			change.Fable.MaximumCollaborations != 1 || !change.Fable.AdvisoryOnly || change.Fable.ReviewCredit {
			return errors.New("Fable must be Human-selected, Maximum effort, bounded to one, advisory, and without review credit")
		}
	}

	switch change.Route {
	case RouteRoutine:
		if change.Critical != nil {
			return errors.New("Routine brief contains Critical data")
		}
	case RouteDesigned:
		if change.Critical != nil {
			return errors.New("Designed brief contains Critical data")
		}
	case RouteCritical:
		if change.Critical == nil {
			return errors.New("Critical brief has no Critical observations")
		}
		if err := validateCritical(*change.Critical); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported TLC route %q", change.Route)
	}
	return nil
}

func validateBrief(brief Brief) error {
	if strings.TrimSpace(brief.Outcome) == "" || strings.TrimSpace(brief.NextAction) == "" {
		return errors.New("brief outcome and next action are required")
	}
	for name, values := range map[string][]string{
		"brief.scope":       brief.Scope,
		"brief.non_goals":   brief.NonGoals,
		"brief.assumptions": brief.Assumptions,
		"brief.constraints": brief.Constraints,
		"brief.tests":       brief.Tests,
	} {
		if err := validateStrings(name, values, name == "brief.tests"); err != nil {
			return err
		}
	}
	return nil
}

func validateCritical(critical Critical) error {
	if !critical.SeparateEffectAuthority || len(critical.Observations) == 0 {
		return errors.New("Critical brief requires observations and separate effect authority")
	}
	seen := make(map[string]struct{}, len(critical.Observations))
	for _, observation := range critical.Observations {
		if _, ok := criticalTriggers[observation.Trigger]; !ok {
			return fmt.Errorf("unsupported Critical trigger %q", observation.Trigger)
		}
		if _, duplicate := seen[observation.Trigger]; duplicate {
			return fmt.Errorf("duplicate Critical trigger %q", observation.Trigger)
		}
		seen[observation.Trigger] = struct{}{}
		if strings.TrimSpace(observation.Evidence) == "" {
			return fmt.Errorf("Critical trigger %q has no evidence", observation.Trigger)
		}
	}
	return nil
}

func validateStrings(name string, values []string, requireItem bool) error {
	if values == nil {
		return fmt.Errorf("%s must be an array", name)
	}
	if requireItem && len(values) == 0 {
		return fmt.Errorf("%s must contain at least one item", name)
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("%s contains an empty item", name)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s contains a duplicate item", name)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("TLC change brief contains multiple JSON values")
		}
		return fmt.Errorf("decode TLC change brief trailer: %w", err)
	}
	return nil
}
