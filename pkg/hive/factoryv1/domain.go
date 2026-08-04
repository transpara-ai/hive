// Package factoryv1 implements the durable, adapter-neutral core of the
// Civilization and Dark Factory v1 control loop.
package factoryv1

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	SchemaVersion = "factory-v1"
	TLCVersion    = "tlc-v1"
)

type Channel string

const (
	ChannelIssueScan      Channel = "issue_scan"
	ChannelHumanIdea      Channel = "human_idea"
	ChannelCompletedOrder Channel = "completed_factory_order"
)

func (c Channel) valid() bool {
	switch c {
	case ChannelIssueScan, ChannelHumanIdea, ChannelCompletedOrder:
		return true
	default:
		return false
	}
}

type Requirement struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
	Rationale string `json:"rationale"`
}

type AcceptanceCriterion struct {
	ID                 string `json:"id"`
	Statement          string `json:"statement"`
	VerificationMethod string `json:"verification_method"`
	RiskClass          string `json:"risk_class"`
}

type SourceReference struct {
	Kind     string `json:"kind"`
	Identity string `json:"identity"`
	URI      string `json:"uri,omitempty"`
	SHA256   string `json:"sha256"`
}

type AuthorityScope struct {
	ActorID            string   `json:"actor_id"`
	AllowedActions     []string `json:"allowed_actions"`
	TargetRepositories []string `json:"target_repositories"`
	NonProductionOnly  bool     `json:"non_production_only"`
}

type BudgetLimit struct {
	MaxAttempts   int   `json:"max_attempts"`
	MaxTokens     int64 `json:"max_tokens"`
	MaxCostMicros int64 `json:"max_cost_micros"`
}

type FactoryOrder struct {
	DocID              string                `json:"doc_id"`
	Version            string                `json:"version"`
	Status             string                `json:"status"`
	Title              string                `json:"title"`
	Channel            Channel               `json:"channel"`
	TargetRepository   string                `json:"target_repository"`
	SourceReferences   []SourceReference     `json:"source_references"`
	Requirements       []Requirement         `json:"requirements"`
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptance_criteria"`
	TestPlan           []string              `json:"test_plan"`
	Constraints        []string              `json:"constraints"`
	NonGoals           []string              `json:"non_goals"`
	ExpectedOutputs    []string              `json:"expected_outputs"`
	Authority          AuthorityScope        `json:"authority_scope"`
	Budget             BudgetLimit           `json:"budget"`
}

type CanonicalDocument struct {
	Order    FactoryOrder `json:"order"`
	Markdown string       `json:"markdown"`
	SHA256   string       `json:"sha256"`
}

var (
	docIDPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	repoPattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	hexPattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	gitHashPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

type ValidationError struct {
	Fields []string `json:"fields"`
}

func (e *ValidationError) Error() string {
	return "invalid FactoryOrder: " + strings.Join(e.Fields, "; ")
}

func ValidateFactoryOrder(order FactoryOrder) error {
	var fields []string
	if !docIDPattern.MatchString(order.DocID) {
		fields = append(fields, "doc_id must be a stable identifier")
	}
	if !versionPattern.MatchString(order.Version) {
		fields = append(fields, "version must be semantic x.y.z")
	}
	if order.Status != "approved" {
		fields = append(fields, "status must be approved")
	}
	if strings.TrimSpace(order.Title) == "" {
		fields = append(fields, "title is required")
	}
	if !order.Channel.valid() {
		fields = append(fields, "channel is not in the v1 allowlist")
	}
	if !repoPattern.MatchString(order.TargetRepository) {
		fields = append(fields, "target_repository must be owner/repository")
	}
	if len(order.SourceReferences) == 0 {
		fields = append(fields, "at least one immutable source reference is required")
	}
	for i, source := range order.SourceReferences {
		prefix := fmt.Sprintf("source_references[%d]", i)
		if strings.TrimSpace(source.Kind) == "" || strings.TrimSpace(source.Identity) == "" {
			fields = append(fields, prefix+" kind and identity are required")
		}
		if !hexPattern.MatchString(source.SHA256) {
			fields = append(fields, prefix+" sha256 must be 64 lowercase hexadecimal characters")
		}
	}
	if len(order.Requirements) == 0 {
		fields = append(fields, "at least one requirement is required")
	}
	validateUniqueRequirements(order.Requirements, &fields)
	if len(order.AcceptanceCriteria) == 0 {
		fields = append(fields, "at least one acceptance criterion is required")
	}
	validateUniqueAcceptance(order.AcceptanceCriteria, &fields)
	if len(order.TestPlan) == 0 {
		fields = append(fields, "test_plan is required")
	}
	if len(order.ExpectedOutputs) == 0 {
		fields = append(fields, "expected_outputs is required")
	}
	if strings.TrimSpace(order.Authority.ActorID) == "" || len(order.Authority.AllowedActions) == 0 {
		fields = append(fields, "authority_scope actor_id and allowed_actions are required")
	}
	if !order.Authority.NonProductionOnly {
		fields = append(fields, "v1 authority_scope must be non-production-only")
	}
	if !contains(order.Authority.TargetRepositories, order.TargetRepository) {
		fields = append(fields, "authority_scope must include target_repository")
	}
	if order.Budget.MaxAttempts <= 0 {
		fields = append(fields, "budget.max_attempts must be positive")
	}
	if order.Budget.MaxTokens < 0 || order.Budget.MaxCostMicros < 0 {
		fields = append(fields, "budget token and cost limits cannot be negative")
	}
	if len(fields) != 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func validateUniqueRequirements(requirements []Requirement, fields *[]string) {
	seen := make(map[string]struct{}, len(requirements))
	for i, requirement := range requirements {
		if strings.TrimSpace(requirement.ID) == "" || strings.TrimSpace(requirement.Statement) == "" || strings.TrimSpace(requirement.Rationale) == "" {
			*fields = append(*fields, fmt.Sprintf("requirements[%d] id, statement, and rationale are required", i))
		}
		if _, exists := seen[requirement.ID]; exists {
			*fields = append(*fields, "requirement IDs must be unique")
		}
		seen[requirement.ID] = struct{}{}
	}
}

func validateUniqueAcceptance(criteria []AcceptanceCriterion, fields *[]string) {
	seen := make(map[string]struct{}, len(criteria))
	for i, criterion := range criteria {
		if strings.TrimSpace(criterion.ID) == "" || strings.TrimSpace(criterion.Statement) == "" || strings.TrimSpace(criterion.VerificationMethod) == "" || strings.TrimSpace(criterion.RiskClass) == "" {
			*fields = append(*fields, fmt.Sprintf("acceptance_criteria[%d] fields are required", i))
		}
		if _, exists := seen[criterion.ID]; exists {
			*fields = append(*fields, "acceptance criterion IDs must be unique")
		}
		seen[criterion.ID] = struct{}{}
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func Canonicalize(order FactoryOrder) (CanonicalDocument, error) {
	if err := ValidateFactoryOrder(order); err != nil {
		return CanonicalDocument{}, err
	}
	markdown := renderFactoryOrder(order)
	sum := sha256.Sum256([]byte(markdown))
	return CanonicalDocument{Order: order, Markdown: markdown, SHA256: hex.EncodeToString(sum[:])}, nil
}

func DecodeFactoryOrder(raw []byte) (FactoryOrder, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var order FactoryOrder
	if err := decoder.Decode(&order); err != nil {
		return FactoryOrder{}, fmt.Errorf("decode FactoryOrder: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return FactoryOrder{}, errors.New("decode FactoryOrder: trailing JSON value")
		}
		return FactoryOrder{}, fmt.Errorf("decode FactoryOrder trailing data: %w", err)
	}
	if err := ValidateFactoryOrder(order); err != nil {
		return FactoryOrder{}, err
	}
	return order, nil
}

func renderFactoryOrder(order FactoryOrder) string {
	var b strings.Builder
	line := func(value string) {
		b.WriteString(strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n")))
		b.WriteByte('\n')
	}
	line("---")
	line("doc_id: " + order.DocID)
	line("version: " + order.Version)
	line("status: " + order.Status)
	line("title: " + order.Title)
	line("channel: " + string(order.Channel))
	line("target_repository: " + order.TargetRepository)
	line("---")
	line("")
	line("# " + order.Title)
	line("")
	line("## Immutable source references")
	for _, source := range order.SourceReferences {
		line(fmt.Sprintf("- %s | %s | %s | sha256:%s", source.Kind, source.Identity, source.URI, source.SHA256))
	}
	renderRequirements(&b, order.Requirements)
	renderAcceptance(&b, order.AcceptanceCriteria)
	renderList(&b, "Test plan", order.TestPlan)
	renderList(&b, "Constraints", order.Constraints)
	renderList(&b, "Non-goals", order.NonGoals)
	renderList(&b, "Expected outputs", order.ExpectedOutputs)
	line = func(value string) {
		b.WriteString(strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n")))
		b.WriteByte('\n')
	}
	line("")
	line("## Authority scope and budget")
	line("- actor_id: " + order.Authority.ActorID)
	line("- non_production_only: " + fmt.Sprintf("%t", order.Authority.NonProductionOnly))
	for _, action := range order.Authority.AllowedActions {
		line("- allowed_action: " + action)
	}
	for _, repository := range order.Authority.TargetRepositories {
		line("- target_repository: " + repository)
	}
	line(fmt.Sprintf("- max_attempts: %d", order.Budget.MaxAttempts))
	line(fmt.Sprintf("- max_tokens: %d", order.Budget.MaxTokens))
	line(fmt.Sprintf("- max_cost_micros: %d", order.Budget.MaxCostMicros))
	return strings.ReplaceAll(b.String(), "\r\n", "\n")
}

func renderRequirements(b *strings.Builder, requirements []Requirement) {
	b.WriteString("\n## Requirements\n")
	for _, requirement := range requirements {
		fmt.Fprintf(b, "\n### %s\n%s\n\nRationale: %s\n", strings.TrimSpace(requirement.ID), strings.TrimSpace(requirement.Statement), strings.TrimSpace(requirement.Rationale))
	}
}

func renderAcceptance(b *strings.Builder, criteria []AcceptanceCriterion) {
	b.WriteString("\n## Acceptance criteria\n")
	for _, criterion := range criteria {
		fmt.Fprintf(b, "\n### %s\n%s\n\nVerification: %s\n\nRisk: %s\n", strings.TrimSpace(criterion.ID), strings.TrimSpace(criterion.Statement), strings.TrimSpace(criterion.VerificationMethod), strings.TrimSpace(criterion.RiskClass))
	}
}

func renderList(b *strings.Builder, heading string, values []string) {
	fmt.Fprintf(b, "\n## %s\n", heading)
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n")))
	}
}

func HashText(value string) string {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(value, "\r\n", "\n")))
	return hex.EncodeToString(sum[:])
}

func SortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

type Clock interface {
	Now() time.Time
}

type WallClock struct{}

func (WallClock) Now() time.Time { return time.Now().UTC() }
