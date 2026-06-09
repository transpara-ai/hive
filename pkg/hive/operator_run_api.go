package hive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// maxRunLaunchBodyBytes caps POST /api/hive/runs request bodies at 64 KiB.
const maxRunLaunchBodyBytes = 64 * 1024

// operatorRunLaunchWriter carries the EventGraph write primitives the run
// launch endpoint needs. It records a queued launch request; it does not start
// or claim runtime execution beyond the durable request events.
type operatorRunLaunchWriter struct {
	factory *event.EventFactory
	signer  event.Signer
	human   types.ActorID
	conv    types.ConversationID
	mu      sync.Mutex
}

// WithOperatorRunLaunchWriter enables the POST /api/hive/runs route, supplying
// the factory/signer/human/conversation used to append causal launch events.
func WithOperatorRunLaunchWriter(factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID) OperatorServerOption {
	return func(o *operatorServerOptions) {
		o.runWriter = &operatorRunLaunchWriter{factory: factory, signer: signer, human: human, conv: conv}
	}
}

type operatorRunLaunchRequest struct {
	OperatorID     string                 `json:"operator_id"`
	IntakeID       string                 `json:"intake_id"`
	Title          string                 `json:"title"`
	Brief          json.RawMessage        `json:"brief"`
	Sources        []RunLaunchSource      `json:"sources"`
	Authority      RunLaunchAuthority     `json:"authority"`
	Budget         runLaunchBudgetRequest `json:"budget"`
	ModelOverrides []ModelOverrideRequest `json:"model_overrides,omitempty"`
	TargetRepos    []string               `json:"target_repos"`
}

// ModelOverrideRequest is a role-scoped model/profile override request. It is
// shared by the run-launch API and the factory-order CLI so both paths use the
// same validation and resolution guardrails.
type ModelOverrideRequest struct {
	Role                 string   `json:"role"`
	Model                string   `json:"model,omitempty"`
	Provider             string   `json:"provider,omitempty"`
	Profile              string   `json:"profile,omitempty"`
	AuthMode             string   `json:"auth_mode,omitempty"`
	PreferredTier        string   `json:"preferred_tier,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	MaxCostPerCallUSD    *float64 `json:"max_cost_per_call_usd,omitempty"`
}

type runLaunchBudgetRequest struct {
	MaxIterations *int     `json:"max_iterations"`
	MaxCostUSD    *float64 `json:"max_cost_usd"`
}

type validatedRunLaunchRequest struct {
	OperatorID     string
	IntakeID       string
	Title          string
	Brief          json.RawMessage
	Sources        []RunLaunchSource
	Authority      RunLaunchAuthority
	Budget         RunLaunchBudget
	ModelOverrides []RunLaunchModelOverride
	TargetRepos    []string
}

type operatorRunLaunchResponse struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"`
	FirstEventID string `json:"first_event_id"`
}

type runLaunchAppendResult struct {
	RunID        string
	FirstEventID types.EventID
}

func handleOperatorRunLaunch(w http.ResponseWriter, r *http.Request, s store.Store, writer *operatorRunLaunchWriter, modelSelection OperatorModelSelectionSource) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRunLaunchBodyBytes)
	var raw operatorRunLaunchRequest
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

	launch, err := validateRunLaunchRequest(raw, modelSelection)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result, err := appendRunLaunchEvents(s, writer, launch)
	if err != nil {
		http.Error(w, fmt.Sprintf("record run launch: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(operatorRunLaunchResponse{
		RunID:        result.RunID,
		Status:       "queued",
		FirstEventID: result.FirstEventID.Value(),
	})
}

func validateRunLaunchRequest(raw operatorRunLaunchRequest, modelSelection OperatorModelSelectionSource) (validatedRunLaunchRequest, error) {
	launch := validatedRunLaunchRequest{
		OperatorID:  strings.TrimSpace(raw.OperatorID),
		IntakeID:    strings.TrimSpace(raw.IntakeID),
		Title:       strings.TrimSpace(raw.Title),
		Brief:       append(json.RawMessage(nil), bytes.TrimSpace(raw.Brief)...),
		Sources:     normalizeRunLaunchSources(raw.Sources),
		Authority:   normalizeRunLaunchAuthority(raw.Authority),
		TargetRepos: normalizeRunLaunchRepos(raw.TargetRepos),
	}

	if err := validateRunLaunchIDField("operator_id", launch.OperatorID); err != nil {
		return validatedRunLaunchRequest{}, err
	}
	if err := validateRunLaunchIDField("intake_id", launch.IntakeID); err != nil {
		return validatedRunLaunchRequest{}, err
	}
	if launch.Title == "" {
		return validatedRunLaunchRequest{}, fmt.Errorf("title is required")
	}
	if hasControlRune(launch.Title) {
		return validatedRunLaunchRequest{}, fmt.Errorf("title contains control characters")
	}
	if len(launch.Brief) == 0 {
		return validatedRunLaunchRequest{}, fmt.Errorf("brief is required")
	}
	if !bytes.HasPrefix(launch.Brief, []byte("{")) {
		return validatedRunLaunchRequest{}, fmt.Errorf("brief must be a JSON object")
	}
	var briefObject map[string]any
	if err := json.Unmarshal(launch.Brief, &briefObject); err != nil {
		return validatedRunLaunchRequest{}, fmt.Errorf("brief must be valid JSON: %v", err)
	}
	if len(launch.Sources) == 0 {
		return validatedRunLaunchRequest{}, fmt.Errorf("sources is required")
	}
	for i, source := range launch.Sources {
		if source.Type == "" {
			return validatedRunLaunchRequest{}, fmt.Errorf("sources[%d].type is required", i)
		}
		if source.Ref == "" {
			return validatedRunLaunchRequest{}, fmt.Errorf("sources[%d].ref is required", i)
		}
		if hasControlRune(source.ID) || hasControlRune(source.Type) || hasControlRune(source.Ref) || hasControlRune(source.Title) {
			return validatedRunLaunchRequest{}, fmt.Errorf("sources[%d] contains control characters", i)
		}
	}
	level, ok := canonicalRunLaunchAuthorityLevel(launch.Authority.InitialLevel)
	if !ok {
		return validatedRunLaunchRequest{}, fmt.Errorf("authority.initial_level must be Required, Recommended, or Notification")
	}
	launch.Authority.InitialLevel = level
	if hasControlRune(launch.Authority.Scope) || hasControlRune(launch.Authority.PolicyRef) || hasControlRune(launch.Authority.Rationale) {
		return validatedRunLaunchRequest{}, fmt.Errorf("authority contains control characters")
	}
	if raw.Budget.MaxIterations == nil {
		return validatedRunLaunchRequest{}, fmt.Errorf("budget.max_iterations is required")
	}
	if raw.Budget.MaxCostUSD == nil {
		return validatedRunLaunchRequest{}, fmt.Errorf("budget.max_cost_usd is required")
	}
	launch.Budget = RunLaunchBudget{MaxIterations: *raw.Budget.MaxIterations, MaxCostUSD: *raw.Budget.MaxCostUSD}
	if launch.Budget.MaxIterations <= 0 {
		return validatedRunLaunchRequest{}, fmt.Errorf("budget.max_iterations must be greater than zero")
	}
	if launch.Budget.MaxCostUSD < 0 {
		return validatedRunLaunchRequest{}, fmt.Errorf("budget.max_cost_usd must be zero or greater")
	}
	overrides, err := ValidateModelOverrides(raw.ModelOverrides, modelSelection)
	if err != nil {
		return validatedRunLaunchRequest{}, err
	}
	launch.ModelOverrides = overrides
	if len(launch.TargetRepos) == 0 {
		return validatedRunLaunchRequest{}, fmt.Errorf("target_repos is required")
	}
	for i, repo := range launch.TargetRepos {
		if !validTargetRepo(repo) {
			return validatedRunLaunchRequest{}, fmt.Errorf("target_repos[%d] must be a safe owner/repo name", i)
		}
	}

	return launch, nil
}

// ValidateModelOverrides validates and resolves role-scoped model override
// requests through the active Hive model-selection source.
func ValidateModelOverrides(raw []ModelOverrideRequest, modelSelection OperatorModelSelectionSource) ([]RunLaunchModelOverride, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	config := DefaultOperatorModelSelectionConfig(time.Time{})
	if modelSelection != nil {
		config = normalizeOperatorModelSelectionConfig(modelSelection())
	}
	resolver := config.Resolver
	roles := StarterRoleDefinitions()
	seen := make(map[string]struct{}, len(raw))
	out := make([]RunLaunchModelOverride, 0, len(raw))
	for i, override := range raw {
		role := strings.TrimSpace(override.Role)
		if role == "" {
			return nil, fmt.Errorf("model_overrides[%d].role is required", i)
		}
		if hasControlRune(role) {
			return nil, fmt.Errorf("model_overrides[%d].role contains control characters", i)
		}
		roleDef, ok := roles[role]
		if !ok {
			return nil, fmt.Errorf("model_overrides[%d].role %q is not a starter civic role", i, role)
		}
		if _, duplicate := seen[role]; duplicate {
			return nil, fmt.Errorf("model_overrides[%d].role %q is duplicated", i, role)
		}
		seen[role] = struct{}{}

		policy, recorded, err := runLaunchOverridePolicy(i, override)
		if err != nil {
			return nil, err
		}
		resolved, err := resolver.Resolve(modelconfig.ResolutionInput{
			Role:         role,
			Policy:       roleDef.ModelPolicy,
			TaskOverride: policy,
			CanOperate:   roleDef.CanOperate,
		})
		if err != nil {
			return nil, fmt.Errorf("model_overrides[%d] for role %q is unsafe: %v", i, role, err)
		}
		if err := validateRunLaunchOverrideResolvedConfig(i, role, recorded.RequestedAuthMode, resolved); err != nil {
			return nil, err
		}
		recorded.Role = role
		recorded.ResolvedModel = resolved.Model
		recorded.ResolvedProvider = resolved.Provider
		recorded.AuthMode = string(resolved.AuthMode)
		out = append(out, recorded)
	}
	return out, nil
}

func runLaunchOverridePolicy(index int, override ModelOverrideRequest) (*modelconfig.RoleModelPolicy, RunLaunchModelOverride, error) {
	model := strings.TrimSpace(override.Model)
	provider := strings.TrimSpace(override.Provider)
	profile := strings.TrimSpace(override.Profile)
	authMode := strings.TrimSpace(override.AuthMode)
	preferredTier := strings.TrimSpace(override.PreferredTier)
	caps := trimRunLaunchStrings(override.RequiredCapabilities)
	if hasControlRune(model) || hasControlRune(provider) || hasControlRune(profile) || hasControlRune(authMode) || hasControlRune(preferredTier) {
		return nil, RunLaunchModelOverride{}, fmt.Errorf("model_overrides[%d] contains control characters", index)
	}
	if authMode != "" && authMode != string(modelconfig.AuthSubscription) && authMode != string(modelconfig.AuthAPIKey) && authMode != string(modelconfig.AuthLocal) {
		return nil, RunLaunchModelOverride{}, fmt.Errorf("model_overrides[%d].auth_mode must be %q, %q, or %q", index, modelconfig.AuthSubscription, modelconfig.AuthAPIKey, modelconfig.AuthLocal)
	}
	if len(caps) != len(override.RequiredCapabilities) {
		return nil, RunLaunchModelOverride{}, fmt.Errorf("model_overrides[%d].required_capabilities contains empty values", index)
	}
	hasOverride := model != "" || provider != "" || profile != "" || authMode != "" || preferredTier != "" || len(caps) > 0 || override.MaxCostPerCallUSD != nil
	if !hasOverride {
		return nil, RunLaunchModelOverride{}, fmt.Errorf("model_overrides[%d] must set model, profile, provider, auth_mode, preferred_tier, required_capabilities, or max_cost_per_call_usd", index)
	}
	if override.MaxCostPerCallUSD != nil && *override.MaxCostPerCallUSD < 0 {
		return nil, RunLaunchModelOverride{}, fmt.Errorf("model_overrides[%d].max_cost_per_call_usd must be zero or greater", index)
	}
	capabilities := make([]modelconfig.Capability, 0, len(caps))
	for _, cap := range caps {
		if hasControlRune(cap) {
			return nil, RunLaunchModelOverride{}, fmt.Errorf("model_overrides[%d].required_capabilities contains control characters", index)
		}
		capabilities = append(capabilities, modelconfig.Capability(cap))
	}
	policy := &modelconfig.RoleModelPolicy{
		Model:                model,
		Provider:             provider,
		Profile:              profile,
		PreferredTier:        modelconfig.ModelTier(preferredTier),
		RequiredCapabilities: capabilities,
		MaxCostPerCallUSD:    override.MaxCostPerCallUSD,
	}
	recorded := RunLaunchModelOverride{
		Model:                model,
		Provider:             provider,
		Profile:              profile,
		RequestedAuthMode:    authMode,
		PreferredTier:        preferredTier,
		RequiredCapabilities: caps,
		MaxCostPerCallUSD:    override.MaxCostPerCallUSD,
	}
	return policy, recorded, nil
}

func validateRunLaunchOverrideResolvedConfig(index int, role string, requested string, resolved modelconfig.ResolvedConfig) error {
	if resolved.Provider != "" && resolved.Entry.Provider != "" && resolved.Provider != resolved.Entry.Provider {
		return fmt.Errorf("model_overrides[%d] for role %q resolved provider %q but model %q belongs to provider %q", index, role, resolved.Provider, resolved.Model, resolved.Entry.Provider)
	}
	if requested != "" && requested != string(resolved.AuthMode) {
		return fmt.Errorf("model_overrides[%d] for role %q requested auth_mode %q but resolved auth_mode %q", index, role, requested, resolved.AuthMode)
	}
	if resolved.AuthMode == modelconfig.AuthAPIKey && requested != string(modelconfig.AuthAPIKey) {
		return fmt.Errorf("model_overrides[%d] for role %q resolves to auth_mode %q; set auth_mode to %q to opt in to metered API-key models", index, role, resolved.AuthMode, modelconfig.AuthAPIKey)
	}
	return nil
}

func trimRunLaunchStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return out
		}
		out = append(out, trimmed)
	}
	return out
}

func normalizeRunLaunchSources(sources []RunLaunchSource) []RunLaunchSource {
	if len(sources) == 0 {
		return nil
	}
	out := make([]RunLaunchSource, 0, len(sources))
	for _, source := range sources {
		out = append(out, RunLaunchSource{
			ID:    strings.TrimSpace(source.ID),
			Type:  strings.TrimSpace(source.Type),
			Ref:   strings.TrimSpace(source.Ref),
			Title: strings.TrimSpace(source.Title),
		})
	}
	return out
}

func normalizeRunLaunchAuthority(authority RunLaunchAuthority) RunLaunchAuthority {
	return RunLaunchAuthority{
		InitialLevel: event.AuthorityLevel(strings.TrimSpace(string(authority.InitialLevel))),
		Scope:        strings.TrimSpace(authority.Scope),
		PolicyRef:    strings.TrimSpace(authority.PolicyRef),
		Rationale:    strings.TrimSpace(authority.Rationale),
	}
}

func normalizeRunLaunchRepos(repos []string) []string {
	if len(repos) == 0 {
		return nil
	}
	out := make([]string, 0, len(repos))
	for _, repo := range repos {
		out = append(out, strings.TrimSpace(repo))
	}
	return out
}

func validateRunLaunchIDField(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > 128 || hasControlRune(value) || strings.ContainsAny(value, " \t\r\n") {
		return fmt.Errorf("%s is unsafe", name)
	}
	return nil
}

func canonicalRunLaunchAuthorityLevel(level event.AuthorityLevel) (event.AuthorityLevel, bool) {
	switch {
	case strings.EqualFold(string(level), string(event.AuthorityLevelRequired)):
		return event.AuthorityLevelRequired, true
	case strings.EqualFold(string(level), string(event.AuthorityLevelRecommended)):
		return event.AuthorityLevelRecommended, true
	case strings.EqualFold(string(level), string(event.AuthorityLevelNotification)):
		return event.AuthorityLevelNotification, true
	default:
		return "", false
	}
}

func validTargetRepo(repo string) bool {
	if repo == "" || strings.Count(repo, "/") != 1 || strings.ContainsAny(repo, " \t\r\n") || hasControlRune(repo) {
		return false
	}
	parts := strings.Split(repo, "/")
	return validRepoComponent(parts[0]) && validRepoComponent(parts[1])
}

func validRepoComponent(component string) bool {
	if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".") || strings.Contains(component, "..") {
		return false
	}
	for _, r := range component {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func hasControlRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func appendRunLaunchEvents(s store.Store, writer *operatorRunLaunchWriter, launch validatedRunLaunchRequest) (runLaunchAppendResult, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	runID, err := newOperatorRunID()
	if err != nil {
		return runLaunchAppendResult{}, fmt.Errorf("create run id: %w", err)
	}
	conv := runLaunchConversationID(runID, writer.conv)

	head, err := s.Head()
	if err != nil {
		return runLaunchAppendResult{}, fmt.Errorf("load chain head: %w", err)
	}
	if head.IsNone() {
		return runLaunchAppendResult{}, fmt.Errorf("event graph is not bootstrapped")
	}

	sourceContent := SourceIngestedContent{
		RunID:      runID,
		IntakeID:   launch.IntakeID,
		OperatorID: launch.OperatorID,
		Title:      launch.Title,
		Sources:    append([]RunLaunchSource(nil), launch.Sources...),
	}
	source, err := createAndAppendRunLaunchEvent(s, writer, EventTypeSourceIngested, sourceContent, []types.EventID{head.Unwrap().ID()}, conv)
	if err != nil {
		return runLaunchAppendResult{}, fmt.Errorf("append source.ingested: %w", err)
	}

	briefContent := BriefDerivedContent{
		RunID:         runID,
		IntakeID:      launch.IntakeID,
		OperatorID:    launch.OperatorID,
		Title:         launch.Title,
		Brief:         append(json.RawMessage(nil), launch.Brief...),
		SourceEventID: source.ID(),
	}
	brief, err := createAndAppendRunLaunchEvent(s, writer, EventTypeBriefDerived, briefContent, []types.EventID{source.ID()}, conv)
	if err != nil {
		return runLaunchAppendResult{}, fmt.Errorf("append brief.derived: %w", err)
	}

	requestContent := FactoryRunRequestedContent{
		RunID:          runID,
		IntakeID:       launch.IntakeID,
		OperatorID:     launch.OperatorID,
		Title:          launch.Title,
		Status:         "queued",
		Authority:      launch.Authority,
		Budget:         launch.Budget,
		ModelOverrides: append([]RunLaunchModelOverride(nil), launch.ModelOverrides...),
		TargetRepos:    append([]string(nil), launch.TargetRepos...),
		SourceEventID:  source.ID(),
		BriefEventID:   brief.ID(),
		Sources:        append([]RunLaunchSource(nil), launch.Sources...),
		Brief:          append(json.RawMessage(nil), launch.Brief...),
	}
	if _, err := createAndAppendRunLaunchEvent(s, writer, EventTypeFactoryRunRequested, requestContent, []types.EventID{source.ID(), brief.ID()}, conv); err != nil {
		return runLaunchAppendResult{}, fmt.Errorf("append factory.run.requested: %w", err)
	}

	return runLaunchAppendResult{RunID: runID, FirstEventID: source.ID()}, nil
}

func createAndAppendRunLaunchEvent(s store.Store, writer *operatorRunLaunchWriter, eventType types.EventType, content event.EventContent, causes []types.EventID, conv types.ConversationID) (event.Event, error) {
	ev, err := writer.factory.Create(eventType, writer.human, content, causes, conv, s, writer.signer)
	if err != nil {
		return event.Event{}, err
	}
	stored, err := s.Append(ev)
	if err != nil {
		return event.Event{}, err
	}
	return stored, nil
}

func newOperatorRunID() (string, error) {
	id, err := types.NewEventIDFromNew()
	if err != nil {
		return "", err
	}
	return "run_" + strings.ReplaceAll(id.Value(), "-", ""), nil
}

func runLaunchConversationID(runID string, fallback types.ConversationID) types.ConversationID {
	if runID == "" {
		return fallback
	}
	sum := sha256.Sum256([]byte("hive-run:" + runID))
	return types.MustConversationID("conv_hive_run_" + hex.EncodeToString(sum[:16]))
}
