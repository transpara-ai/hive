package hive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// maxModelRolePolicyBodyBytes caps POST /api/hive/model-selection/role-policy
// request bodies at 16 KiB; one request updates one role policy.
const maxModelRolePolicyBodyBytes = 16 * 1024

type operatorModelRolePolicyWriter struct {
	factory *event.EventFactory
	signer  event.Signer
	human   types.ActorID
	conv    types.ConversationID
	mu      sync.Mutex
}

// WithOperatorModelRolePolicyWriter enables the Hive-owned model-policy write
// route. Site may call this route, but only Hive validates and records policy.
func WithOperatorModelRolePolicyWriter(factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID) OperatorServerOption {
	return func(o *operatorServerOptions) {
		o.modelPolicyWriter = &operatorModelRolePolicyWriter{factory: factory, signer: signer, human: human, conv: conv}
	}
}

type operatorModelRolePolicyRequest struct {
	OperatorID           string   `json:"operator_id,omitempty"`
	Reason               string   `json:"reason,omitempty"`
	Role                 string   `json:"role"`
	Model                string   `json:"model,omitempty"`
	Provider             string   `json:"provider,omitempty"`
	Profile              string   `json:"profile,omitempty"`
	AuthMode             string   `json:"auth_mode,omitempty"`
	PreferredTier        string   `json:"preferred_tier,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	MaxCostPerCallUSD    *float64 `json:"max_cost_per_call_usd,omitempty"`
}

type operatorModelRolePolicyResponse struct {
	EventID          string `json:"event_id"`
	Status           string `json:"status"`
	Role             string `json:"role"`
	ResolvedModel    string `json:"resolved_model"`
	ResolvedProvider string `json:"resolved_provider"`
	AuthMode         string `json:"auth_mode"`
}

func handleOperatorModelRolePolicyUpdate(w http.ResponseWriter, r *http.Request, s store.Store, writer *operatorModelRolePolicyWriter, modelSelection OperatorModelSelectionSource) {
	r.Body = http.MaxBytesReader(w, r.Body, maxModelRolePolicyBodyBytes)
	var raw operatorModelRolePolicyRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "request body too large", http.StatusBadRequest)
			return
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	content, err := validateModelRolePolicyRequest(raw, modelSelection)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	eventID, err := appendModelRolePolicyUpdate(s, writer, content)
	if err != nil {
		http.Error(w, fmt.Sprintf("record model role policy: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(operatorModelRolePolicyResponse{
		EventID:          eventID.Value(),
		Status:           "recorded",
		Role:             content.Role,
		ResolvedModel:    content.ResolvedModel,
		ResolvedProvider: content.ResolvedProvider,
		AuthMode:         content.AuthMode,
	})
}

func validateModelRolePolicyRequest(raw operatorModelRolePolicyRequest, modelSelection OperatorModelSelectionSource) (ModelRolePolicyUpdatedContent, error) {
	operatorID := strings.TrimSpace(raw.OperatorID)
	reason := strings.TrimSpace(raw.Reason)
	if hasControlRune(operatorID) || hasControlRune(reason) {
		return ModelRolePolicyUpdatedContent{}, fmt.Errorf("operator_id or reason contains control characters")
	}
	validated, err := ValidateModelOverrides([]ModelOverrideRequest{{
		Role:                 raw.Role,
		Model:                raw.Model,
		Provider:             raw.Provider,
		Profile:              raw.Profile,
		AuthMode:             raw.AuthMode,
		PreferredTier:        raw.PreferredTier,
		RequiredCapabilities: append([]string(nil), raw.RequiredCapabilities...),
		MaxCostPerCallUSD:    cloneRunLaunchFloat64Ptr(raw.MaxCostPerCallUSD),
	}}, modelSelection)
	if err != nil {
		return ModelRolePolicyUpdatedContent{}, err
	}
	if len(validated) != 1 {
		return ModelRolePolicyUpdatedContent{}, fmt.Errorf("model role policy validation produced %d records, want 1", len(validated))
	}
	override := validated[0]
	return ModelRolePolicyUpdatedContent{
		Role:                 override.Role,
		Model:                override.Model,
		Provider:             override.Provider,
		Profile:              override.Profile,
		RequestedAuthMode:    override.RequestedAuthMode,
		PreferredTier:        override.PreferredTier,
		RequiredCapabilities: append([]string(nil), override.RequiredCapabilities...),
		MaxCostPerCallUSD:    cloneRunLaunchFloat64Ptr(override.MaxCostPerCallUSD),
		ResolvedModel:        override.ResolvedModel,
		ResolvedProvider:     override.ResolvedProvider,
		AuthMode:             override.AuthMode,
		OperatorID:           operatorID,
		Reason:               reason,
	}, nil
}

func appendModelRolePolicyUpdate(s store.Store, writer *operatorModelRolePolicyWriter, content ModelRolePolicyUpdatedContent) (types.EventID, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	head, err := s.Head()
	if err != nil {
		return types.EventID{}, fmt.Errorf("load chain head: %w", err)
	}
	if head.IsNone() {
		return types.EventID{}, fmt.Errorf("event graph is not bootstrapped")
	}
	ev, err := writer.factory.Create(EventTypeModelRolePolicyUpdated, writer.human, content, []types.EventID{head.Unwrap().ID()}, writer.conv, s, writer.signer)
	if err != nil {
		return types.EventID{}, err
	}
	stored, err := s.Append(ev)
	if err != nil {
		return types.EventID{}, err
	}
	return stored.ID(), nil
}

func applyModelRolePolicyUpdates(p *OperatorProjection, s store.Store, config OperatorModelSelectionConfig, limit int) OperatorModelSelectionConfig {
	updates := latestModelRolePolicyUpdates(p, s, limit)
	if len(updates) == 0 {
		return config
	}
	if len(config.RolePolicies) == 0 {
		config.RolePolicies = updates
		return config
	}
	merged := make(map[string]OperatorModelRolePolicy, len(config.RolePolicies)+len(updates))
	for role, policy := range config.RolePolicies {
		merged[role] = policy
	}
	for role, policy := range updates {
		merged[role] = policy
	}
	config.RolePolicies = merged
	return config
}

func modelSelectionSourceWithRolePolicyUpdates(s store.Store, source OperatorModelSelectionSource, limit int) OperatorModelSelectionSource {
	return func() OperatorModelSelectionConfig {
		config := DefaultOperatorModelSelectionConfig(types.Now().Value())
		if source != nil {
			config = source()
		}
		p := OperatorProjection{}
		config = applyModelRolePolicyUpdates(&p, s, normalizeOperatorModelSelectionConfig(config), limit)
		if len(p.Errors) > 0 {
			config.RolePolicyError = strings.Join(p.Errors, "; ")
		}
		return config
	}
}

func latestModelRolePolicyUpdates(p *OperatorProjection, s store.Store, limit int) map[string]OperatorModelRolePolicy {
	events := readProjectionEvents(p, s, EventTypeModelRolePolicyUpdated, limit)
	out := make(map[string]OperatorModelRolePolicy)
	for _, pe := range events {
		role, policy, ok, err := modelRolePolicyUpdateFromEvent(pe.event)
		if err != nil {
			p.Errors = append(p.Errors, err.Error())
			continue
		}
		if !ok {
			continue
		}
		// Store pagination is newest-first by append/chain position. First valid
		// policy per role wins; wall-clock timestamps are not policy authority.
		if _, exists := out[role]; exists {
			continue
		}
		out[role] = policy
	}
	return out
}

func latestModelRolePolicyUpdateForRole(s store.Store, role string, limit int) (OperatorModelRolePolicy, bool, error) {
	canonicalRole, _, ok := canonicalStarterRole(role)
	if !ok || s == nil {
		return OperatorModelRolePolicy{}, false, nil
	}
	if limit <= 0 {
		limit = defaultOperatorProjectionLimit
	}
	page, err := s.ByType(EventTypeModelRolePolicyUpdated, limit, types.None[types.Cursor]())
	if err != nil {
		return OperatorModelRolePolicy{}, false, fmt.Errorf("read %s: %w", EventTypeModelRolePolicyUpdated.Value(), err)
	}
	var latest OperatorModelRolePolicy
	found := false
	for _, item := range page.Items() {
		eventRole, policy, ok, err := modelRolePolicyUpdateFromEvent(item)
		if err != nil {
			if eventRole == canonicalRole {
				return OperatorModelRolePolicy{}, false, err
			}
			continue
		}
		if !ok || eventRole != canonicalRole {
			continue
		}
		// ByType is newest-first by append/chain position, so the first matching
		// role is the active policy even if wall-clock timestamps moved backward.
		latest = policy
		found = true
		break
	}
	return latest, found, nil
}

func modelRolePolicyUpdateFromEvent(ev event.Event) (string, OperatorModelRolePolicy, bool, error) {
	content, ok := ev.Content().(ModelRolePolicyUpdatedContent)
	if !ok {
		return "", OperatorModelRolePolicy{}, false, errors.New(contentTypeError(ev, "ModelRolePolicyUpdatedContent"))
	}
	rawRole := strings.TrimSpace(content.Role)
	role, _, ok := canonicalStarterRole(rawRole)
	if !ok {
		return rawRole, OperatorModelRolePolicy{}, false, fmt.Errorf("model policy event %s has unknown role %q", ev.ID().Value(), content.Role)
	}
	return role, OperatorModelRolePolicy{
		Policy:            modelRolePolicyFromUpdate(content),
		RequestedAuthMode: strings.TrimSpace(content.RequestedAuthMode),
		ResolvedModel:     strings.TrimSpace(content.ResolvedModel),
		ResolvedProvider:  strings.TrimSpace(content.ResolvedProvider),
		AuthMode:          strings.TrimSpace(content.AuthMode),
		EventID:           ev.ID().Value(),
		UpdatedAt:         ev.Timestamp().Value(),
	}, true, nil
}

func modelRolePolicyFromUpdate(content ModelRolePolicyUpdatedContent) *modelconfig.RoleModelPolicy {
	return &modelconfig.RoleModelPolicy{
		Model:                strings.TrimSpace(content.Model),
		Provider:             strings.TrimSpace(content.Provider),
		Profile:              strings.TrimSpace(content.Profile),
		PreferredTier:        modelconfig.ModelTier(strings.TrimSpace(content.PreferredTier)),
		RequiredCapabilities: modelCapabilitiesFromStrings(content.RequiredCapabilities),
		MaxCostPerCallUSD:    cloneRunLaunchFloat64Ptr(content.MaxCostPerCallUSD),
	}
}

func modelCapabilitiesFromStrings(values []string) []modelconfig.Capability {
	if len(values) == 0 {
		return nil
	}
	out := make([]modelconfig.Capability, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, modelconfig.Capability(value))
		}
	}
	return out
}
