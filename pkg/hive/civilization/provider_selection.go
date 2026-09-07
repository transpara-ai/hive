package civilization

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// ExecutionSelection is caller-owned. Model and effort are independent and
// optional; a model is always interpreted by the selected provider.
type ExecutionSelection struct {
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

type ExecutionEvidence struct {
	Requested   ExecutionSelection `json:"requested"`
	Effective   ExecutionSelection `json:"effective"`
	ModelSource string             `json:"model_source"`
}

type ProviderHost struct {
	Provider     Provider
	DefaultModel string
}

// ProviderRouter never retries through a different host or model. Account
// availability is authoritative at the provider; failures return unchanged.
type ProviderRouter struct {
	DefaultProvider string
	Hosts           map[string]ProviderHost
}

var modelNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`)

func validateSelection(selection ExecutionSelection) error {
	if selection.Provider != "" && selection.Provider != "codex" && selection.Provider != "claude" {
		return fmt.Errorf("invalid provider %q: choose codex or claude", selection.Provider)
	}
	if selection.Model != "" && !modelNamePattern.MatchString(selection.Model) {
		return errors.New("invalid model: use a provider model identifier without spaces or flags")
	}
	switch selection.ReasoningEffort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return fmt.Errorf("invalid reasoning effort %q", selection.ReasoningEffort)
	}
	return nil
}

func (p *ProviderRouter) Resolve(requested ExecutionSelection) (ExecutionEvidence, error) {
	if err := validateSelection(requested); err != nil {
		return ExecutionEvidence{}, err
	}
	effective := requested
	if effective.Provider == "" {
		effective.Provider = p.DefaultProvider
	}
	host, found := p.Hosts[effective.Provider]
	if !found || host.Provider == nil {
		return ExecutionEvidence{}, fmt.Errorf("provider %q is unavailable; configure that host before retrying", effective.Provider)
	}
	source := "invocation"
	if effective.Model == "" {
		effective.Model = host.DefaultModel
		source = "configured_provider_default"
		if effective.Model == "" {
			source = "provider_default"
		}
	}
	if err := validateSelection(effective); err != nil {
		return ExecutionEvidence{}, err
	}
	return ExecutionEvidence{Requested: requested, Effective: effective, ModelSource: source}, nil
}

func (p *ProviderRouter) Run(ctx context.Context, request ProviderRequest) (ProviderResult, error) {
	evidence, err := p.Resolve(request.Selection)
	if err != nil {
		return ProviderResult{Execution: &ExecutionEvidence{Requested: request.Selection, ModelSource: "unresolved"}}, err
	}
	request.Selection = evidence.Effective
	result, err := p.Hosts[evidence.Effective.Provider].Provider.Run(ctx, request)
	if err != nil {
		return ProviderResult{Execution: &evidence}, fmt.Errorf("%s execution failed (model %q): %w", evidence.Effective.Provider, evidence.Effective.Model, err)
	}
	// Adapters may observe a provider-selected model. Do not invent one when
	// native output does not report it.
	if result.Execution != nil && result.Execution.Effective.Model != "" {
		observed := result.Execution.Effective.Model
		if evidence.Effective.Model != "" && observed != evidence.Effective.Model {
			return ProviderResult{Execution: &evidence}, errors.New("provider returned a different model than requested")
		}
		evidence.Effective.Model = observed
	}
	result.Execution = &evidence
	return result, nil
}
