package hive

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	IssueScanSourceIssueMarkerSchemaVersion = "issue_scan_source_issue_marker_v0.1"

	IssueScanFactoryStatusLabelAcquired      = "factory:acquired"
	IssueScanFactoryStatusLabelParked        = "factory:parked"
	IssueScanFactoryStatusLabelReadyForHuman = "factory:ready-for-human"
	IssueScanFactoryStatusLabelCompleted     = "factory:completed"
	IssueScanFactoryStatusLabelAbandoned     = "factory:abandoned"
	IssueScanFactoryStatusLabelSuperseded    = "factory:superseded"

	issueScanSourceIssueMarkerTruthBoundary = "GitHub labels/comments are human-visible projections only. Canonical execution and stage state live in Work; durable provenance, blocker, lineage, and authority truth live in EventGraph. Do not parse this comment as workflow state or authority."
	issueScanSourceIssueMarkerAuthority     = "This marker does not authorize protected actions, issue closure, PR creation, merge, deploy, production EventGraph writes, Hive wake/start/re-enable, Test 001 GREEN, value allocation, or autonomy increase."
)

// IssueScanSourceIssueMarkerActivationMode controls whether the Runtime-owned
// marker bridge may cross from projection planning into a client-side apply.
type IssueScanSourceIssueMarkerActivationMode string

const (
	IssueScanSourceIssueMarkerActivationDryRunOnly           IssueScanSourceIssueMarkerActivationMode = "dry_run"
	IssueScanSourceIssueMarkerActivationMockedImplementation IssueScanSourceIssueMarkerActivationMode = "mocked_implementation"
	IssueScanSourceIssueMarkerActivationLive                 IssueScanSourceIssueMarkerActivationMode = "live_activation"
)

type IssueScanSourceIssueMarkerClientKind string

const (
	IssueScanSourceIssueMarkerMockClient       IssueScanSourceIssueMarkerClientKind = "mock"
	IssueScanSourceIssueMarkerLiveGitHubClient IssueScanSourceIssueMarkerClientKind = "live_github"
)

// IssueScanSourceIssueMarkerIssueScope is an exact source issue allowed by a
// marker activation packet. Wildcard source issue mutation is intentionally not
// modeled here.
type IssueScanSourceIssueMarkerIssueScope struct {
	Repo   string
	Number int
}

// IssueScanSourceIssueMarkerActivation is the explicit authority packet shape
// for applying marker projections. The zero value is dry-run-only. This PR
// supports mocked implementation evidence only; live GitHub activation remains
// fail-closed until a future authority packet and client are added.
type IssueScanSourceIssueMarkerActivation struct {
	Mode                IssueScanSourceIssueMarkerActivationMode
	AuthorityRef        string
	Actor               string
	Environment         string
	CredentialSource    string
	AllowedIssues       []IssueScanSourceIssueMarkerIssueScope
	AllowComments       bool
	AllowLabels         bool
	StopConditions      []string
	LiveEvidenceAllowed bool
}

// IssueScanSourceIssueMarkerTransition names the semantic source-issue marker
// transition. These values are intentionally narrower than the Work/EventGraph
// lifecycle: only human-visible source issue status changes get a marker.
type IssueScanSourceIssueMarkerTransition string

const (
	IssueScanSourceIssueMarkerAcquired      IssueScanSourceIssueMarkerTransition = "acquired"
	IssueScanSourceIssueMarkerParked        IssueScanSourceIssueMarkerTransition = "parked"
	IssueScanSourceIssueMarkerHumanAction   IssueScanSourceIssueMarkerTransition = "human_action"
	IssueScanSourceIssueMarkerReadyForHuman IssueScanSourceIssueMarkerTransition = "ready_for_human"
	IssueScanSourceIssueMarkerCompleted     IssueScanSourceIssueMarkerTransition = "completed"
	IssueScanSourceIssueMarkerAbandoned     IssueScanSourceIssueMarkerTransition = "abandoned"
	IssueScanSourceIssueMarkerSuperseded    IssueScanSourceIssueMarkerTransition = "superseded"
)

// IssueScanSourceIssueMarkerInput is a pure planning input. It must not call
// GitHub, mutate Work, or write EventGraph truth.
type IssueScanSourceIssueMarkerInput struct {
	Transition     IssueScanSourceIssueMarkerTransition
	Issue          GitHubIssueCandidate
	RunID          string
	FactoryOrderID string
	StageID        string
	StageState     string
	ActorRole      string
	WorkRefs       []string
	EventGraphRefs []string
	EvidenceRefs   []string
	GeneratedAt    time.Time
}

// IssueScanSourceIssueMarkerPlan is the complete source GitHub issue mutation
// payload a bridge can apply after its own authority and GitHub client checks.
// It is a projection plan only; it is not itself a GitHub write.
type IssueScanSourceIssueMarkerPlan struct {
	Repo              string
	IssueNumber       int
	IssueURL          string
	Transition        IssueScanSourceIssueMarkerTransition
	IdempotencyKey    string
	AddLabels         []string
	RemoveLabels      []string
	CommentBody       string
	CanonicalRefs     []string
	TruthBoundary     string
	AuthorityBoundary string
}

// IssueScanSourceIssueMarkerClient is the minimum GitHub issue mutation surface
// needed to apply a planned marker. Implementations should make RemoveLabels
// tolerant of labels that are already absent. ListCommentBodies must return all
// existing comment bodies for the issue, across all pages, so hidden
// idempotency keys cannot be missed.
type IssueScanSourceIssueMarkerClient interface {
	SourceIssueMarkerClientKind() IssueScanSourceIssueMarkerClientKind
	AddLabels(ctx context.Context, repo string, number int, labels []string) error
	RemoveLabels(ctx context.Context, repo string, number int, labels []string) error
	ListCommentBodies(ctx context.Context, repo string, number int) ([]string, error)
	CreateComment(ctx context.Context, repo string, number int, body string) error
}

type issueScanSourceIssueMarkerMockClient interface {
	IssueScanSourceIssueMarkerClient
	issueScanSourceIssueMarkerMockClient()
}

type IssueScanSourceIssueMarkerApplyResult struct {
	LabelsAdded    []string
	LabelsRemoved  []string
	CommentCreated bool
	CommentSkipped bool
}

// IssueScanSourceIssueMarkerBridgeResult records one runtime bridge pass. A
// dry-run result proves Hive could derive a valid projection plan without
// mutating GitHub. An applied result means an explicitly configured client
// accepted the same plan at the GitHub mutation boundary.
type IssueScanSourceIssueMarkerBridgeResult struct {
	RunID             string
	FactoryOrderID    string
	Repository        string
	IssueNumber       int
	Transition        IssueScanSourceIssueMarkerTransition
	StageID           string
	IdempotencyKey    string
	ActivationMode    IssueScanSourceIssueMarkerActivationMode
	AuthorityRef      string
	ClientKind        IssueScanSourceIssueMarkerClientKind
	Refused           bool
	RefusalReason     string
	DryRun            bool
	Applied           bool
	CommentCreated    bool
	CommentSkipped    bool
	LabelsAdded       []string
	LabelsRemoved     []string
	CanonicalRefs     []string
	WorkRefs          []string
	EventGraphRefs    []string
	EvidenceRefs      []string
	TruthBoundary     string
	AuthorityBoundary string
}

// BridgeIssueScanSourceIssueMarker is the single Runtime-owned bridge from
// canonical issue-scan state into source GitHub issue marker projection. It
// plans on every call, but it applies only when an explicit mocked activation
// packet and marker client are configured. Missing or live activation remains
// dry-run/refused so adding a client cannot silently enable GitHub mutation.
func (r *Runtime) BridgeIssueScanSourceIssueMarker(ctx context.Context, input IssueScanSourceIssueMarkerInput) (IssueScanSourceIssueMarkerBridgeResult, error) {
	plan, err := PlanIssueScanSourceIssueMarker(input)
	if err != nil {
		return IssueScanSourceIssueMarkerBridgeResult{}, err
	}
	decision := r.issueScanSourceIssueMarkerActivationDecision(plan)
	result := IssueScanSourceIssueMarkerBridgeResult{
		RunID:             strings.TrimSpace(input.RunID),
		FactoryOrderID:    strings.TrimSpace(input.FactoryOrderID),
		Repository:        plan.Repo,
		IssueNumber:       plan.IssueNumber,
		Transition:        plan.Transition,
		StageID:           strings.TrimSpace(input.StageID),
		IdempotencyKey:    plan.IdempotencyKey,
		ActivationMode:    decision.Mode,
		AuthorityRef:      decision.AuthorityRef,
		ClientKind:        decision.ClientKind,
		Refused:           !decision.Apply,
		RefusalReason:     decision.RefusalReason,
		DryRun:            !decision.Apply,
		CanonicalRefs:     append([]string(nil), plan.CanonicalRefs...),
		WorkRefs:          compactStrings(input.WorkRefs),
		EventGraphRefs:    compactStrings(input.EventGraphRefs),
		EvidenceRefs:      compactStrings(input.EvidenceRefs),
		TruthBoundary:     plan.TruthBoundary,
		AuthorityBoundary: plan.AuthorityBoundary,
	}
	if !decision.Apply {
		return result, nil
	}
	applied, err := ApplyIssueScanSourceIssueMarker(ctx, decision.Client, plan)
	if err != nil {
		return result, err
	}
	result.Refused = false
	result.RefusalReason = ""
	result.DryRun = false
	result.Applied = true
	result.CommentCreated = applied.CommentCreated
	result.CommentSkipped = applied.CommentSkipped
	result.LabelsAdded = append([]string(nil), applied.LabelsAdded...)
	result.LabelsRemoved = append([]string(nil), applied.LabelsRemoved...)
	return result, nil
}

type issueScanSourceIssueMarkerActivationDecision struct {
	Apply         bool
	Mode          IssueScanSourceIssueMarkerActivationMode
	AuthorityRef  string
	Client        IssueScanSourceIssueMarkerClient
	ClientKind    IssueScanSourceIssueMarkerClientKind
	RefusalReason string
}

func (r *Runtime) issueScanSourceIssueMarkerActivationDecision(plan IssueScanSourceIssueMarkerPlan) issueScanSourceIssueMarkerActivationDecision {
	decision := issueScanSourceIssueMarkerActivationDecision{
		Mode:          IssueScanSourceIssueMarkerActivationDryRunOnly,
		RefusalReason: "source issue marker activation is dry-run-only",
	}
	if r == nil {
		decision.RefusalReason = "runtime is required for source issue marker activation"
		return decision
	}
	activation := normalizeIssueScanSourceIssueMarkerActivation(r.issueScanSourceIssueMarkerActivation)
	decision.Mode = activation.Mode
	decision.AuthorityRef = activation.AuthorityRef
	if r.issueScanSourceIssueMarkerClient != nil {
		decision.Client = r.issueScanSourceIssueMarkerClient
		decision.ClientKind = r.issueScanSourceIssueMarkerClient.SourceIssueMarkerClientKind()
	}
	if activation.Mode == IssueScanSourceIssueMarkerActivationDryRunOnly {
		return decision
	}
	if activation.Mode == IssueScanSourceIssueMarkerActivationLive {
		decision.RefusalReason = "live source issue marker activation is not implemented or authorized"
		return decision
	}
	if activation.Mode != IssueScanSourceIssueMarkerActivationMockedImplementation {
		decision.RefusalReason = fmt.Sprintf("unknown source issue marker activation mode %q", activation.Mode)
		return decision
	}
	if strings.TrimSpace(activation.AuthorityRef) == "" {
		decision.RefusalReason = "source issue marker mocked activation requires authority_ref"
		return decision
	}
	if strings.TrimSpace(activation.Actor) == "" {
		decision.RefusalReason = "source issue marker mocked activation requires actor"
		return decision
	}
	if strings.TrimSpace(activation.Environment) == "" {
		decision.RefusalReason = "source issue marker mocked activation requires environment"
		return decision
	}
	if credential := strings.TrimSpace(activation.CredentialSource); !strings.EqualFold(credential, "none") {
		decision.RefusalReason = "source issue marker mocked activation requires credential_source none"
		return decision
	}
	if activation.LiveEvidenceAllowed {
		decision.RefusalReason = "source issue marker mocked activation cannot allow live evidence"
		return decision
	}
	if len(activation.StopConditions) == 0 {
		decision.RefusalReason = "source issue marker mocked activation requires stop conditions"
		return decision
	}
	if !issueScanSourceIssueMarkerPlanAllowedByScope(plan, activation.AllowedIssues) {
		decision.RefusalReason = "source issue marker target is outside activation scope"
		return decision
	}
	if strings.TrimSpace(plan.CommentBody) != "" && !activation.AllowComments {
		decision.RefusalReason = "source issue marker comment mutation is not allowed by activation"
		return decision
	}
	if (len(plan.AddLabels) > 0 || len(plan.RemoveLabels) > 0) && !activation.AllowLabels {
		decision.RefusalReason = "source issue marker label mutation is not allowed by activation"
		return decision
	}
	if decision.Client == nil {
		decision.RefusalReason = "source issue marker client is not configured"
		return decision
	}
	if decision.ClientKind != IssueScanSourceIssueMarkerMockClient {
		decision.RefusalReason = "source issue marker mocked activation requires a mock client"
		return decision
	}
	if _, ok := decision.Client.(issueScanSourceIssueMarkerMockClient); !ok {
		decision.RefusalReason = "source issue marker mocked activation requires package-local mock client evidence"
		return decision
	}
	decision.Apply = true
	decision.RefusalReason = ""
	return decision
}

// PlanIssueScanSourceIssueMarker returns the idempotent label/comment payload
// for a source GitHub issue marker. The returned plan keeps cc:* labels out of
// operational status and points humans back to canonical Work/EventGraph refs.
func PlanIssueScanSourceIssueMarker(input IssueScanSourceIssueMarkerInput) (IssueScanSourceIssueMarkerPlan, error) {
	transition := input.Transition
	statusLabel, err := transition.factoryStatusLabel()
	if err != nil {
		return IssueScanSourceIssueMarkerPlan{}, err
	}
	issue := normalizeIssueScanMarkerIssue(input.Issue)
	if err := validateIssueScanMarkerIssue(issue); err != nil {
		return IssueScanSourceIssueMarkerPlan{}, err
	}
	runID := strings.TrimSpace(input.RunID)
	if runID == "" {
		return IssueScanSourceIssueMarkerPlan{}, fmt.Errorf("run_id is required")
	}
	factoryOrderID := strings.TrimSpace(input.FactoryOrderID)
	if factoryOrderID == "" {
		return IssueScanSourceIssueMarkerPlan{}, fmt.Errorf("factory_order_id is required")
	}
	stageID := strings.TrimSpace(input.StageID)
	idempotencyKey := issueScanSourceIssueMarkerKey(transition, issue, runID, factoryOrderID, stageID)
	canonicalRefs := issueScanSourceIssueMarkerRefs(input, issue, runID, factoryOrderID)
	plan := IssueScanSourceIssueMarkerPlan{
		Repo:              issue.Repo,
		IssueNumber:       issue.Number,
		IssueURL:          issue.URL,
		Transition:        transition,
		IdempotencyKey:    idempotencyKey,
		AddLabels:         []string{statusLabel},
		RemoveLabels:      issueScanSourceIssueMarkerRemoveLabels(statusLabel),
		CanonicalRefs:     canonicalRefs,
		TruthBoundary:     issueScanSourceIssueMarkerTruthBoundary,
		AuthorityBoundary: issueScanSourceIssueMarkerAuthority,
	}
	plan.CommentBody = issueScanSourceIssueMarkerComment(input, issue, plan)
	return plan, nil
}

// ApplyIssueScanSourceIssueMarker applies a planned source issue marker through
// an explicit client boundary. It never derives workflow truth from GitHub; it
// only uses comments to avoid duplicate projection comments on replay.
func ApplyIssueScanSourceIssueMarker(ctx context.Context, client IssueScanSourceIssueMarkerClient, plan IssueScanSourceIssueMarkerPlan) (IssueScanSourceIssueMarkerApplyResult, error) {
	var result IssueScanSourceIssueMarkerApplyResult
	if client == nil {
		return result, fmt.Errorf("issue marker client is required")
	}
	if err := validateIssueScanSourceIssueMarkerPlan(plan); err != nil {
		return result, err
	}
	bodies, err := client.ListCommentBodies(ctx, plan.Repo, plan.IssueNumber)
	if err != nil {
		return result, fmt.Errorf("list source issue marker comments: %w", err)
	}
	if IssueScanSourceIssueMarkerCommentExists(bodies, plan) {
		result.CommentSkipped = true
		return result, nil
	}
	if len(plan.RemoveLabels) > 0 {
		if err := client.RemoveLabels(ctx, plan.Repo, plan.IssueNumber, plan.RemoveLabels); err != nil {
			return result, fmt.Errorf("remove source issue marker labels: %w", err)
		}
		result.LabelsRemoved = append([]string(nil), plan.RemoveLabels...)
	}
	if len(plan.AddLabels) > 0 {
		if err := client.AddLabels(ctx, plan.Repo, plan.IssueNumber, plan.AddLabels); err != nil {
			return result, fmt.Errorf("add source issue marker labels: %w", err)
		}
		result.LabelsAdded = append([]string(nil), plan.AddLabels...)
	}
	if err := client.CreateComment(ctx, plan.Repo, plan.IssueNumber, plan.CommentBody); err != nil {
		return result, fmt.Errorf("create source issue marker comment: %w", err)
	}
	result.CommentCreated = true
	return result, nil
}

// IssueScanSourceIssueMarkerCommentExists reports whether a source issue already
// contains the planned marker. Bridges should use this before appending comments
// so replay does not create duplicate source-issue noise.
func IssueScanSourceIssueMarkerCommentExists(commentBodies []string, plan IssueScanSourceIssueMarkerPlan) bool {
	key := strings.TrimSpace(plan.IdempotencyKey)
	if key == "" {
		return false
	}
	needle := issueScanSourceIssueMarkerHiddenKey(key)
	for _, body := range commentBodies {
		if strings.Contains(body, needle) {
			return true
		}
	}
	return false
}

func validateIssueScanSourceIssueMarkerPlan(plan IssueScanSourceIssueMarkerPlan) error {
	switch {
	case !ValidTransparaAIRepo(strings.TrimSpace(plan.Repo)):
		return fmt.Errorf("plan.repo must be a transpara-ai owner/repo name")
	case plan.IssueNumber <= 0:
		return fmt.Errorf("plan.issue_number must be greater than zero")
	case strings.TrimSpace(plan.IdempotencyKey) == "":
		return fmt.Errorf("plan.idempotency_key is required")
	case strings.TrimSpace(plan.CommentBody) == "":
		return fmt.Errorf("plan.comment_body is required")
	case len(plan.AddLabels) == 0:
		return fmt.Errorf("plan.add_labels is required")
	default:
		return nil
	}
}

func normalizeIssueScanSourceIssueMarkerActivation(activation IssueScanSourceIssueMarkerActivation) IssueScanSourceIssueMarkerActivation {
	activation.Mode = IssueScanSourceIssueMarkerActivationMode(strings.TrimSpace(string(activation.Mode)))
	if activation.Mode == "" {
		activation.Mode = IssueScanSourceIssueMarkerActivationDryRunOnly
	}
	activation.AuthorityRef = strings.TrimSpace(activation.AuthorityRef)
	activation.Actor = strings.TrimSpace(activation.Actor)
	activation.Environment = strings.TrimSpace(activation.Environment)
	activation.CredentialSource = strings.TrimSpace(activation.CredentialSource)
	activation.StopConditions = compactStrings(activation.StopConditions)
	allowed := make([]IssueScanSourceIssueMarkerIssueScope, 0, len(activation.AllowedIssues))
	for _, issue := range activation.AllowedIssues {
		repo := strings.ToLower(strings.TrimSpace(issue.Repo))
		if repo == "" || issue.Number <= 0 {
			continue
		}
		allowed = append(allowed, IssueScanSourceIssueMarkerIssueScope{Repo: repo, Number: issue.Number})
	}
	activation.AllowedIssues = allowed
	return activation
}

func issueScanSourceIssueMarkerPlanAllowedByScope(plan IssueScanSourceIssueMarkerPlan, allowed []IssueScanSourceIssueMarkerIssueScope) bool {
	if len(allowed) == 0 {
		return false
	}
	repo := strings.ToLower(strings.TrimSpace(plan.Repo))
	for _, issue := range allowed {
		if strings.EqualFold(strings.TrimSpace(issue.Repo), repo) && issue.Number == plan.IssueNumber {
			return true
		}
	}
	return false
}

func (t IssueScanSourceIssueMarkerTransition) factoryStatusLabel() (string, error) {
	switch t {
	case IssueScanSourceIssueMarkerAcquired:
		return IssueScanFactoryStatusLabelAcquired, nil
	case IssueScanSourceIssueMarkerParked, IssueScanSourceIssueMarkerHumanAction:
		return IssueScanFactoryStatusLabelParked, nil
	case IssueScanSourceIssueMarkerReadyForHuman:
		return IssueScanFactoryStatusLabelReadyForHuman, nil
	case IssueScanSourceIssueMarkerCompleted:
		return IssueScanFactoryStatusLabelCompleted, nil
	case IssueScanSourceIssueMarkerAbandoned:
		return IssueScanFactoryStatusLabelAbandoned, nil
	case IssueScanSourceIssueMarkerSuperseded:
		return IssueScanFactoryStatusLabelSuperseded, nil
	default:
		return "", fmt.Errorf("unknown issue-scan source marker transition %q", t)
	}
}

func normalizeIssueScanMarkerIssue(issue GitHubIssueCandidate) GitHubIssueCandidate {
	issue.Repo = strings.ToLower(strings.TrimSpace(issue.Repo))
	issue.Title = strings.TrimSpace(issue.Title)
	issue.URL = strings.TrimSpace(issue.URL)
	issue.State = strings.ToLower(strings.TrimSpace(issue.State))
	issue.StateReason = strings.ToLower(strings.TrimSpace(issue.StateReason))
	if issue.State == "" {
		issue.State = "open"
	}
	if issue.URL == "" && issue.Repo != "" && issue.Number > 0 {
		issue.URL = fmt.Sprintf("https://github.com/%s/issues/%d", issue.Repo, issue.Number)
	}
	return issue
}

func validateIssueScanMarkerIssue(issue GitHubIssueCandidate) error {
	switch {
	case !ValidTransparaAIRepo(issue.Repo):
		return fmt.Errorf("issue.repo must be a transpara-ai owner/repo name")
	case issue.Number <= 0:
		return fmt.Errorf("issue.number must be greater than zero")
	case issue.Title == "":
		return fmt.Errorf("issue.title is required")
	case hasControlRune(issue.Title) || hasControlRune(issue.URL):
		return fmt.Errorf("issue contains control characters")
	default:
		return nil
	}
}

func issueScanSourceIssueMarkerKey(transition IssueScanSourceIssueMarkerTransition, issue GitHubIssueCandidate, runID, factoryOrderID, stageID string) string {
	parts := []string{
		"hive",
		IssueScanSourceIssueMarkerSchemaVersion,
		string(transition),
		safeRunLaunchID(issue.Repo),
		strconv.Itoa(issue.Number),
		safeRunLaunchID(runID),
		safeRunLaunchID(factoryOrderID),
	}
	if stageID != "" {
		parts = append(parts, safeRunLaunchID(stageID))
	}
	return strings.Join(parts, ":")
}

func issueScanSourceIssueMarkerRefs(input IssueScanSourceIssueMarkerInput, issue GitHubIssueCandidate, runID, factoryOrderID string) []string {
	refs := []string{
		issue.URL,
		"run_id:" + runID,
		"factory_order_id:" + factoryOrderID,
	}
	if stageID := strings.TrimSpace(input.StageID); stageID != "" {
		refs = append(refs, "stage_id:"+stageID)
	}
	refs = append(refs, input.WorkRefs...)
	refs = append(refs, input.EventGraphRefs...)
	refs = append(refs, input.EvidenceRefs...)
	return compactStrings(refs)
}

func issueScanSourceIssueMarkerIssueFromBrief(issue issueScanBriefIssuePayload) GitHubIssueCandidate {
	return GitHubIssueCandidate{
		Repo:        issue.Repo,
		Number:      issue.Number,
		Title:       issue.Title,
		URL:         issue.URL,
		State:       issue.State,
		StateReason: issue.StateReason,
		Labels:      append([]string(nil), issue.Labels...),
		Body:        issue.Body,
	}
}

func issueScanSourceIssueMarkerRemoveLabels(addLabel string) []string {
	labels := []string{
		IssueScanFactoryStatusLabelAcquired,
		IssueScanFactoryStatusLabelParked,
		IssueScanFactoryStatusLabelReadyForHuman,
		IssueScanFactoryStatusLabelCompleted,
		IssueScanFactoryStatusLabelAbandoned,
		IssueScanFactoryStatusLabelSuperseded,
	}
	out := make([]string, 0, len(labels)-1)
	for _, label := range labels {
		if label != addLabel {
			out = append(out, label)
		}
	}
	return out
}

func issueScanSourceIssueMarkerComment(input IssueScanSourceIssueMarkerInput, issue GitHubIssueCandidate, plan IssueScanSourceIssueMarkerPlan) string {
	var b strings.Builder
	b.WriteString(issueScanSourceIssueMarkerHiddenKey(plan.IdempotencyKey))
	b.WriteString("\n")
	fmt.Fprintf(&b, "### Factory issue-scan marker: %s\n\n", plan.Transition)
	fmt.Fprintf(&b, "The Civilization factory projected `%s` for `%s#%d`.\n\n", plan.Transition, issue.Repo, issue.Number)
	fmt.Fprintf(&b, "- source_issue: `%s#%d`\n", issue.Repo, issue.Number)
	fmt.Fprintf(&b, "- run_id: `%s`\n", strings.TrimSpace(input.RunID))
	fmt.Fprintf(&b, "- factory_order_id: `%s`\n", strings.TrimSpace(input.FactoryOrderID))
	if stageID := strings.TrimSpace(input.StageID); stageID != "" {
		fmt.Fprintf(&b, "- stage_id: `%s`\n", stageID)
	}
	if stageState := strings.TrimSpace(input.StageState); stageState != "" {
		fmt.Fprintf(&b, "- stage_state: `%s`\n", stageState)
	}
	if actorRole := strings.TrimSpace(input.ActorRole); actorRole != "" {
		fmt.Fprintf(&b, "- actor_role: `%s`\n", actorRole)
	}
	if !input.GeneratedAt.IsZero() {
		fmt.Fprintf(&b, "- generated_at: `%s`\n", input.GeneratedAt.UTC().Format(time.RFC3339))
	}
	if len(plan.CanonicalRefs) > 0 {
		b.WriteString("\nCanonical refs:\n")
		for _, ref := range plan.CanonicalRefs {
			fmt.Fprintf(&b, "- `%s`\n", ref)
		}
	}
	b.WriteString("\nTruth boundary: ")
	b.WriteString(plan.TruthBoundary)
	b.WriteString("\n\nAuthority boundary: ")
	b.WriteString(plan.AuthorityBoundary)
	b.WriteString("\n")
	return b.String()
}

func issueScanSourceIssueMarkerHiddenKey(key string) string {
	return "<!-- " + strings.TrimSpace(key) + " -->"
}
