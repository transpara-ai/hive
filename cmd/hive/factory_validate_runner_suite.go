package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/transpara-ai/hive/pkg/hive"
)

// Local, non-mutating validation of an issue-scan runner-suite package
// (hive#262, first implementation slice of
// docs/designs/issue-scan-runner-suite-packaging-v0.1.0.md). The harness
// cross-checks a package manifest and its inert fixtures against the
// in-process issueScanRunnerContracts() document. It never executes runner
// commands and never mutates GitHub, Work, EventGraph, or runtime state.

const (
	issueScanRunnerSuiteManifestKind     = "issue_scan_runner_suite_package_manifest"
	issueScanRunnerSuiteManifestFileName = "manifest.json"
)

// issueScanRunnerSuiteForbiddenEnvMinimum is the canonical minimum every
// component's forbidden_env must declare: setting either variable overrides
// the Claude CLI subscription auth the runtime depends on, and a runner
// package may never require or silently permit them.
var issueScanRunnerSuiteForbiddenEnvMinimum = []string{
	"ANTHROPIC_API_KEY",
	"HIVE_ANTHROPIC_API_KEY",
}

var issueScanRunnerSuiteIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type issueScanRunnerSuiteManifest struct {
	Kind              string                          `json:"kind"`
	SuiteID           string                          `json:"suite_id"`
	LifecycleVersion  string                          `json:"lifecycle_version"`
	TerminalStagePath string                          `json:"terminal_stage_path"`
	ValidationCommand string                          `json:"validation_command"`
	Components        []issueScanRunnerSuiteComponent `json:"components"`
}

type issueScanRunnerSuiteComponent struct {
	ID                  string                         `json:"id"`
	Command             string                         `json:"command"`
	Argv                []string                       `json:"argv"`
	Timeout             string                         `json:"timeout"`
	StdinKind           string                         `json:"stdin_kind"`
	StdoutKind          string                         `json:"stdout_kind"`
	RequiredEnv         []string                       `json:"required_env"`
	ForbiddenEnv        []string                       `json:"forbidden_env"`
	AuthorityBoundaries []string                       `json:"authority_boundaries"`
	Fixtures            issueScanRunnerSuiteFixtureSet `json:"fixtures"`
}

type issueScanRunnerSuiteFixtureSet struct {
	Stdin  string `json:"stdin"`
	Stdout string `json:"stdout"`
}

type issueScanRunnerSuiteValidationReport struct {
	SuiteID           string `json:"suite_id"`
	LifecycleVersion  string `json:"lifecycle_version"`
	TerminalStagePath string `json:"terminal_stage_path"`
	ComponentCount    int    `json:"component_count"`
}

// validateIssueScanRunnerSuitePackage validates the package rooted at dir and
// fails closed: any unreadable, unknown, missing, or mismatched declaration is
// an error, never a skip.
func validateIssueScanRunnerSuitePackage(dir string) (issueScanRunnerSuiteValidationReport, error) {
	document := issueScanRunnerContracts()
	manifest, err := loadIssueScanRunnerSuiteManifest(filepath.Join(dir, issueScanRunnerSuiteManifestFileName))
	if err != nil {
		return issueScanRunnerSuiteValidationReport{}, err
	}

	var problems []error
	report := issueScanRunnerSuiteValidationReport{
		SuiteID:           manifest.SuiteID,
		LifecycleVersion:  manifest.LifecycleVersion,
		TerminalStagePath: manifest.TerminalStagePath,
		ComponentCount:    len(manifest.Components),
	}

	if manifest.Kind != issueScanRunnerSuiteManifestKind {
		problems = append(problems, fmt.Errorf("manifest kind %q must be %q", manifest.Kind, issueScanRunnerSuiteManifestKind))
	}
	if !issueScanRunnerSuiteIDPattern.MatchString(manifest.SuiteID) {
		problems = append(problems, fmt.Errorf("suite_id %q is missing or not lowercase-kebab", manifest.SuiteID))
	}
	if manifest.LifecycleVersion != document.LifecycleVersion {
		problems = append(problems, fmt.Errorf("lifecycle_version %q does not match contracts document %q", manifest.LifecycleVersion, document.LifecycleVersion))
	}
	if strings.TrimSpace(manifest.ValidationCommand) == "" {
		problems = append(problems, errors.New("validation_command is required"))
	}

	requiredComponents, err := issueScanRunnerSuiteRequiredComponents(document, manifest.TerminalStagePath)
	if err != nil {
		problems = append(problems, err)
	}

	if len(manifest.Components) == 0 {
		problems = append(problems, errors.New("components must not be empty"))
	}

	contractsByID := make(map[string]issueScanRunnerContract, len(document.ExternalRunnerContracts))
	for _, contract := range document.ExternalRunnerContracts {
		contractsByID[contract.ID] = contract
	}

	seen := make(map[string]bool, len(manifest.Components))
	for _, component := range manifest.Components {
		if seen[component.ID] {
			problems = append(problems, fmt.Errorf("duplicate component id %q", component.ID))
			continue
		}
		seen[component.ID] = true
		contract, known := contractsByID[component.ID]
		if !known {
			problems = append(problems, fmt.Errorf("unknown component id %q (not an external runner contract)", component.ID))
			continue
		}
		if requiredComponents != nil && !requiredComponents[component.ID] {
			problems = append(problems, fmt.Errorf("component %q is not required by terminal_stage_path %q (mutually exclusive terminal adapters stay out of one package)", component.ID, manifest.TerminalStagePath))
			continue
		}
		problems = append(problems, validateIssueScanRunnerSuiteComponent(dir, document, contract, component)...)
	}
	for id := range requiredComponents {
		if !seen[id] {
			problems = append(problems, fmt.Errorf("missing required component %q for terminal_stage_path %q", id, manifest.TerminalStagePath))
		}
	}

	if joined := errors.Join(problems...); joined != nil {
		return issueScanRunnerSuiteValidationReport{}, joined
	}
	return report, nil
}

func loadIssueScanRunnerSuiteManifest(path string) (issueScanRunnerSuiteManifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return issueScanRunnerSuiteManifest{}, fmt.Errorf("read %s: %w", issueScanRunnerSuiteManifestFileName, err)
	}
	var manifest issueScanRunnerSuiteManifest
	if err := strictDecodeJSON(body, &manifest); err != nil {
		return issueScanRunnerSuiteManifest{}, fmt.Errorf("parse %s: %w", issueScanRunnerSuiteManifestFileName, err)
	}
	return manifest, nil
}

// issueScanRunnerSuiteRequiredComponents derives, from the contracts document
// alone, exactly which external runner components the declared terminal stage
// path requires: the full-chain daemon runner flags minus the path's
// mutual exclusions, plus the path's own flags.
func issueScanRunnerSuiteRequiredComponents(document issueScanRunnerContractsDocument, terminalPathID string) (map[string]bool, error) {
	var terminalPath *issueScanTerminalPath
	knownPaths := make([]string, 0, len(document.TerminalStagePaths))
	for i := range document.TerminalStagePaths {
		knownPaths = append(knownPaths, document.TerminalStagePaths[i].ID)
		if document.TerminalStagePaths[i].ID == terminalPathID {
			terminalPath = &document.TerminalStagePaths[i]
		}
	}
	if terminalPath == nil {
		return nil, fmt.Errorf("unknown terminal_stage_path %q (want one of %s)", terminalPathID, strings.Join(knownPaths, "|"))
	}
	allowedFlags := make(map[string]bool, len(document.FullChainDaemonFlags)+len(terminalPath.Flags))
	for _, flagName := range document.FullChainDaemonFlags {
		allowedFlags[flagName] = true
	}
	for _, flagName := range terminalPath.MutuallyExclusiveWith {
		delete(allowedFlags, flagName)
	}
	for _, flagName := range terminalPath.Flags {
		allowedFlags[flagName] = true
	}
	required := make(map[string]bool)
	for _, contract := range document.ExternalRunnerContracts {
		if contract.DaemonFlag != "" && allowedFlags[contract.DaemonFlag] {
			required[contract.ID] = true
		}
	}
	if len(required) == 0 {
		return nil, fmt.Errorf("terminal_stage_path %q derives no required external components; refusing to validate an empty contract", terminalPathID)
	}
	return required, nil
}

func validateIssueScanRunnerSuiteComponent(dir string, document issueScanRunnerContractsDocument, contract issueScanRunnerContract, component issueScanRunnerSuiteComponent) []error {
	var problems []error
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("component %q: %w", component.ID, fmt.Errorf(format, args...)))
	}

	if strings.TrimSpace(component.Command) == "" {
		fail("command placeholder is required")
	} else if !filepath.IsLocal(component.Command) {
		fail("command %q is not package-local", component.Command)
	}
	// nil means the field was omitted (or JSON null) — an absent declaration
	// is not the same fail-closed statement as an explicit empty list.
	if component.Argv == nil {
		fail("argv is required (declare [] for no arguments)")
	}
	for _, arg := range component.Argv {
		if strings.TrimSpace(arg) == "" {
			fail("argv entries must be non-empty")
			break
		}
	}
	timeout, err := time.ParseDuration(component.Timeout)
	if err != nil {
		fail("timeout %q is not a duration: %v", component.Timeout, err)
	} else if timeout <= 0 {
		fail("timeout %q must be positive", component.Timeout)
	}
	if component.StdinKind != contract.StdinContextKind {
		fail("stdin kind %q does not match contract %q", component.StdinKind, contract.StdinContextKind)
	}
	if component.StdoutKind != contract.StdoutContractType {
		fail("stdout kind %q does not match contract %q", component.StdoutKind, contract.StdoutContractType)
	}
	problems = append(problems, validateIssueScanRunnerSuiteEnv(component)...)
	if len(component.AuthorityBoundaries) == 0 {
		fail("authority_boundaries must not be empty")
	}
	for _, boundary := range component.AuthorityBoundaries {
		if strings.TrimSpace(boundary) == "" {
			fail("authority_boundaries entries must be non-empty")
			break
		}
	}
	// Exact set match against the contract: a dropped boundary hides a limit
	// and an added one could grant authority, so neither direction is open to
	// package authors. Operational notes belong in the package README.
	declared := make(map[string]bool, len(component.AuthorityBoundaries))
	for _, boundary := range component.AuthorityBoundaries {
		declared[boundary] = true
	}
	contractBoundaries := make(map[string]bool, len(contract.AuthorityBoundaries))
	for _, boundary := range contract.AuthorityBoundaries {
		contractBoundaries[boundary] = true
		if !declared[boundary] {
			fail("authority_boundaries must include contract boundary %q", boundary)
		}
	}
	for _, boundary := range component.AuthorityBoundaries {
		if !contractBoundaries[boundary] {
			fail("authority_boundaries entry %q is not a contract boundary (exact match required)", boundary)
		}
	}
	problems = append(problems, validateIssueScanRunnerSuiteFixtures(dir, document, contract, component)...)
	return problems
}

func validateIssueScanRunnerSuiteEnv(component issueScanRunnerSuiteComponent) []error {
	var problems []error
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("component %q: %w", component.ID, fmt.Errorf(format, args...)))
	}
	if component.RequiredEnv == nil {
		fail("required_env is required (declare [] for no required variables)")
	}
	forbidden := make(map[string]bool, len(component.ForbiddenEnv))
	for _, name := range component.ForbiddenEnv {
		if strings.TrimSpace(name) == "" {
			fail("forbidden_env entries must be non-empty")
			continue
		}
		forbidden[name] = true
	}
	for _, name := range issueScanRunnerSuiteForbiddenEnvMinimum {
		if !forbidden[name] {
			fail("forbidden_env must include the canonical minimum %s", strings.Join(issueScanRunnerSuiteForbiddenEnvMinimum, ", "))
			break
		}
	}
	for _, name := range component.RequiredEnv {
		if strings.TrimSpace(name) == "" {
			fail("required_env entries must be non-empty")
			continue
		}
		if forbidden[name] {
			fail("required env var %q is also forbidden", name)
		}
	}
	return problems
}

func validateIssueScanRunnerSuiteFixtures(dir string, document issueScanRunnerContractsDocument, contract issueScanRunnerContract, component issueScanRunnerSuiteComponent) []error {
	var problems []error
	stdinBody, errs := readIssueScanRunnerSuiteFixture(dir, component.ID, component.Fixtures.Stdin, "fixtures.stdin")
	problems = append(problems, errs...)
	stdoutBody, errs := readIssueScanRunnerSuiteFixture(dir, component.ID, component.Fixtures.Stdout, "fixtures.stdout")
	problems = append(problems, errs...)

	if stdinBody != nil {
		problems = append(problems, validateIssueScanRunnerSuiteStdinFixture(document, contract, component, stdinBody)...)
	}
	if stdoutBody != nil {
		problems = append(problems, validateIssueScanRunnerSuiteStdoutFixture(contract, component, stdoutBody)...)
	}
	if stdinBody != nil && stdoutBody != nil {
		problems = append(problems, validateIssueScanRunnerSuiteFixturePair(component, stdinBody, stdoutBody)...)
	}
	return problems
}

// validateIssueScanRunnerSuiteFixturePair enforces the deterministic
// stdin↔stdout relations the runtime validates when recording runner results,
// so a package cannot certify an expected-output pair the runtime rejects:
// a blocker-repair commit must differ from the previously reviewed commit,
// and review receipts must cite the exact reviewed head (the finalizer
// rejects moved heads, so the ready head equals the Operate commit).
// Stage-role and implementation outputs have no such stdin-derived relation;
// per-side validation still applies to them. Decode errors return no pair
// problems here because the per-side strict decodes already report them.
func validateIssueScanRunnerSuiteFixturePair(component issueScanRunnerSuiteComponent, stdinBody, stdoutBody []byte) []error {
	fail := func(format string, args ...any) []error {
		return []error{fmt.Errorf("component %q fixture pair: %w", component.ID, fmt.Errorf(format, args...))}
	}
	switch component.ID {
	case "blocker_repair_runner":
		var context hive.IssueScanBlockerRepairRunnerContext
		if err := json.Unmarshal(stdinBody, &context); err != nil {
			return nil
		}
		var result hive.IssueScanBlockerRepairRunnerResult
		if err := json.Unmarshal(stdoutBody, &result); err != nil {
			return nil
		}
		commit, err := hive.IssueScanOperateResultBodyCommit(result.OperateResultBody)
		if err != nil {
			return nil
		}
		previous := strings.TrimSpace(context.PreviousOperateCommit)
		if previous == "" {
			return fail("stdin previous_operate_commit is required")
		}
		if strings.EqualFold(commit, previous) {
			return fail("stdout Operate commit %q must differ from stdin previous_operate_commit (the runtime rejects unchanged repair commits, comparing case-insensitively)", commit)
		}
	case "adversarial_review_runner":
		var context hive.IssueScanAdversarialReviewContext
		if err := json.Unmarshal(stdinBody, &context); err != nil {
			return nil
		}
		var receipt hive.IssueScanAdversarialReviewReceipt
		if err := json.Unmarshal(stdoutBody, &receipt); err != nil {
			return nil
		}
		operateCommit := strings.TrimSpace(context.OperateCommit)
		if operateCommit == "" {
			return fail("stdin operate_commit is required")
		}
		if !strings.EqualFold(strings.TrimSpace(receipt.ReviewedHeadSHA), operateCommit) {
			return fail("stdout reviewed_head_sha %q must match stdin operate_commit %q", receipt.ReviewedHeadSHA, operateCommit)
		}
	case "ready_state_review_runner":
		var context hive.IssueScanReadyStateReviewContext
		if err := json.Unmarshal(stdinBody, &context); err != nil {
			return nil
		}
		var receipt hive.IssueScanReadyStateReviewReceipt
		if err := json.Unmarshal(stdoutBody, &receipt); err != nil {
			return nil
		}
		operateCommit := strings.TrimSpace(context.OperateCommit)
		if operateCommit == "" {
			return fail("stdin operate_commit is required")
		}
		if !strings.EqualFold(strings.TrimSpace(receipt.ReviewedHeadSHA), operateCommit) {
			return fail("stdout reviewed_head_sha %q must match stdin operate_commit %q", receipt.ReviewedHeadSHA, operateCommit)
		}
	}
	return nil
}

func readIssueScanRunnerSuiteFixture(dir, componentID, fixturePath, field string) ([]byte, []error) {
	if strings.TrimSpace(fixturePath) == "" {
		return nil, []error{fmt.Errorf("component %q: %s is required", componentID, field)}
	}
	if !filepath.IsLocal(fixturePath) {
		return nil, []error{fmt.Errorf("component %q: %s path %q is not package-local", componentID, field, fixturePath)}
	}
	resolved, err := resolveWithinIssueScanRunnerSuiteRoot(dir, fixturePath)
	if err != nil {
		return nil, []error{fmt.Errorf("component %q: %s: %w", componentID, field, err)}
	}
	body, err := os.ReadFile(resolved)
	if err != nil {
		return nil, []error{fmt.Errorf("component %q: read fixture: %w", componentID, err)}
	}
	return body, nil
}

// resolveWithinIssueScanRunnerSuiteRoot resolves symlinks in both the package
// root and the fixture path and fails closed unless the resolved fixture stays
// beneath the resolved root — filepath.IsLocal is lexical only, so a
// package-local-looking symlink could otherwise escape the package.
func resolveWithinIssueScanRunnerSuiteRoot(dir, fixturePath string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("resolve package root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(dir, fixturePath))
	if err != nil {
		return "", fmt.Errorf("resolve fixture path: %w", err)
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q resolves outside the package root", fixturePath)
	}
	return resolved, nil
}

func validateIssueScanRunnerSuiteStdinFixture(document issueScanRunnerContractsDocument, contract issueScanRunnerContract, component issueScanRunnerSuiteComponent, body []byte) []error {
	var problems []error
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("component %q stdin fixture %s: %w", component.ID, component.Fixtures.Stdin, fmt.Errorf(format, args...)))
	}
	var generic map[string]any
	if err := json.Unmarshal(body, &generic); err != nil {
		fail("parse fixture: %v", err)
		return problems
	}
	if generic == nil {
		fail("fixture is JSON null")
		return problems
	}
	kind, _ := generic["kind"].(string)
	if kind != contract.StdinContextKind {
		fail("kind %q does not match contract stdin context kind %q", kind, contract.StdinContextKind)
	}
	lifecycle, _ := generic["lifecycle_version"].(string)
	if lifecycle != document.LifecycleVersion {
		fail("lifecycle_version %q does not match contracts document %q", lifecycle, document.LifecycleVersion)
	}
	if err := strictDecodeIssueScanRunnerSuiteFixture(component.ID, issueScanRunnerSuiteFixtureStdin, body); err != nil {
		fail("fixture does not strictly decode into %s: %v", contract.StdinContextType, err)
	}
	return problems
}

func validateIssueScanRunnerSuiteStdoutFixture(contract issueScanRunnerContract, component issueScanRunnerSuiteComponent, body []byte) []error {
	var problems []error
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("component %q stdout fixture %s: %w", component.ID, component.Fixtures.Stdout, fmt.Errorf(format, args...)))
	}
	var generic map[string]any
	if err := json.Unmarshal(body, &generic); err != nil {
		fail("parse fixture: %v", err)
		return problems
	}
	if generic == nil {
		fail("fixture is JSON null")
		return problems
	}
	if err := strictDecodeIssueScanRunnerSuiteFixture(component.ID, issueScanRunnerSuiteFixtureStdout, body); err != nil {
		fail("fixture does not satisfy %s: %v", contract.StdoutContractType, err)
	}
	for _, spec := range contract.StdoutRequiredFields {
		if err := checkIssueScanStdoutRequiredField(generic, spec); err != nil {
			fail("%v", err)
		}
	}
	return problems
}

type issueScanRunnerSuiteFixtureSide int

const (
	issueScanRunnerSuiteFixtureStdin issueScanRunnerSuiteFixtureSide = iota
	issueScanRunnerSuiteFixtureStdout
)

// strictDecodeIssueScanRunnerSuiteFixture decodes a fixture into the concrete
// pkg/hive contract type for the component. The mapping is an explicit
// allowlist; an id without a mapping is an error, never a skip.
func strictDecodeIssueScanRunnerSuiteFixture(componentID string, side issueScanRunnerSuiteFixtureSide, body []byte) error {
	var target any
	switch componentID {
	case "stage_role_output_runner":
		if side == issueScanRunnerSuiteFixtureStdin {
			target = &hive.IssueScanStageRoleOutputRunnerContext{}
		} else {
			target = &hive.IssueScanStageRoleOutputRunnerResult{}
		}
	case "implementation_runner":
		if side == issueScanRunnerSuiteFixtureStdin {
			target = &hive.IssueScanImplementationRunnerContext{}
		} else {
			result := &hive.IssueScanImplementationRunnerResult{}
			if err := strictDecodeJSON(body, result); err != nil {
				return err
			}
			return validateIssueScanRunnerSuiteOperateBody(result.OperateResultBody)
		}
	case "adversarial_review_runner":
		if side == issueScanRunnerSuiteFixtureStdin {
			target = &hive.IssueScanAdversarialReviewContext{}
		} else {
			receipt := &hive.IssueScanAdversarialReviewReceipt{}
			if err := strictDecodeJSON(body, receipt); err != nil {
				return err
			}
			if err := hive.ValidateIssueScanAdversarialReviewReceiptShape(*receipt); err != nil {
				return fmt.Errorf("receipt shape the runtime would reject: %w", err)
			}
			return nil
		}
	case "blocker_repair_runner":
		if side == issueScanRunnerSuiteFixtureStdin {
			target = &hive.IssueScanBlockerRepairRunnerContext{}
		} else {
			result := &hive.IssueScanBlockerRepairRunnerResult{}
			if err := strictDecodeJSON(body, result); err != nil {
				return err
			}
			return validateIssueScanRunnerSuiteOperateBody(result.OperateResultBody)
		}
	case "ready_state_review_runner":
		if side == issueScanRunnerSuiteFixtureStdin {
			target = &hive.IssueScanReadyStateReviewContext{}
		} else {
			receipt := &hive.IssueScanReadyStateReviewReceipt{}
			if err := strictDecodeJSON(body, receipt); err != nil {
				return err
			}
			if err := hive.ValidateIssueScanReadyStateReviewReceiptShape(*receipt); err != nil {
				return fmt.Errorf("receipt shape the runtime would reject: %w", err)
			}
			return nil
		}
	case "ready_pr_evidence_runner":
		if side == issueScanRunnerSuiteFixtureStdin {
			target = &hive.IssueScanReadyPRRunnerContext{}
		} else {
			target = &hive.IssueScanReadyPRRunnerResult{}
		}
	default:
		return fmt.Errorf("no fixture type mapping for component %q", componentID)
	}
	return strictDecodeJSON(body, target)
}

// validateIssueScanRunnerSuiteOperateBody runs the exact runtime Operate
// parser over a fixture's operate_result_body so a package cannot certify an
// expected stdout the runtime would reject.
func validateIssueScanRunnerSuiteOperateBody(body string) error {
	if _, err := hive.IssueScanOperateResultBodyCommit(body); err != nil {
		return fmt.Errorf("operate_result_body is not a valid Operate result: %w", err)
	}
	return nil
}

func strictDecodeJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	// Decoder.More() reports false for stray closing delimiters, so require a
	// clean io.EOF from a second decode instead.
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after JSON document")
	}
	return nil
}

type issueScanStdoutFieldSegment struct {
	name   string
	array  bool
	equals *string
}

type issueScanStdoutFieldSpec struct {
	segments []issueScanStdoutFieldSegment
}

var issueScanStdoutFieldNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// parseIssueScanStdoutRequiredFieldSpec parses the contract mini-grammar for
// stdout_required_fields: dot-separated segments, each `name` or `name[]`,
// with an optional `=value` on the final segment only. Anything outside the
// grammar is an error so a spec can never be silently skipped. Known
// limitation: `=value` values must not contain dots (none do today; the
// contract-wide parseability test fails loudly if one ever does).
func parseIssueScanStdoutRequiredFieldSpec(spec string) (issueScanStdoutFieldSpec, error) {
	if strings.TrimSpace(spec) == "" {
		return issueScanStdoutFieldSpec{}, errors.New("empty stdout required-field spec")
	}
	rawSegments := strings.Split(spec, ".")
	segments := make([]issueScanStdoutFieldSegment, 0, len(rawSegments))
	for i, raw := range rawSegments {
		last := i == len(rawSegments)-1
		segment := issueScanStdoutFieldSegment{}
		name := raw
		if before, value, found := strings.Cut(raw, "="); found {
			if !last {
				return issueScanStdoutFieldSpec{}, fmt.Errorf("stdout required-field spec %q: %q uses = outside the final segment", spec, raw)
			}
			if value == "" {
				return issueScanStdoutFieldSpec{}, fmt.Errorf("stdout required-field spec %q: empty comparison value", spec)
			}
			name = before
			segment.equals = &value
		}
		if trimmed, found := strings.CutSuffix(name, "[]"); found {
			if segment.equals != nil {
				return issueScanStdoutFieldSpec{}, fmt.Errorf("stdout required-field spec %q: [] and = cannot combine", spec)
			}
			name = trimmed
			segment.array = true
		}
		if !issueScanStdoutFieldNamePattern.MatchString(name) {
			return issueScanStdoutFieldSpec{}, fmt.Errorf("stdout required-field spec %q: segment %q is outside the checker grammar", spec, raw)
		}
		segment.name = name
		segments = append(segments, segment)
	}
	return issueScanStdoutFieldSpec{segments: segments}, nil
}

func checkIssueScanStdoutRequiredField(doc map[string]any, spec string) error {
	parsed, err := parseIssueScanStdoutRequiredFieldSpec(spec)
	if err != nil {
		return err
	}
	return checkIssueScanStdoutFieldSegments(doc, parsed.segments, spec)
}

func checkIssueScanStdoutFieldSegments(node map[string]any, segments []issueScanStdoutFieldSegment, spec string) error {
	segment := segments[0]
	rest := segments[1:]
	value, present := node[segment.name]
	if !present || value == nil {
		return fmt.Errorf("required stdout field %q is missing or null (spec %q)", segment.name, spec)
	}
	if segment.array {
		elements, ok := value.([]any)
		if !ok {
			return fmt.Errorf("required stdout field %q is not an array (spec %q)", segment.name, spec)
		}
		if len(elements) == 0 {
			return fmt.Errorf("required stdout field %q is an empty array (spec %q)", segment.name, spec)
		}
		for _, element := range elements {
			if element == nil {
				return fmt.Errorf("required stdout field %q contains a null element (spec %q)", segment.name, spec)
			}
			if len(rest) == 0 {
				continue
			}
			object, ok := element.(map[string]any)
			if !ok {
				return fmt.Errorf("required stdout field %q elements are not objects (spec %q)", segment.name, spec)
			}
			if err := checkIssueScanStdoutFieldSegments(object, rest, spec); err != nil {
				return err
			}
		}
		return nil
	}
	if segment.equals != nil {
		text, ok := value.(string)
		if !ok || text != *segment.equals {
			return fmt.Errorf("required stdout field %q must equal %q (spec %q)", segment.name, *segment.equals, spec)
		}
		return nil
	}
	if len(rest) > 0 {
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("required stdout field %q is not an object (spec %q)", segment.name, spec)
		}
		return checkIssueScanStdoutFieldSegments(object, rest, spec)
	}
	if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
		return fmt.Errorf("required stdout field %q is empty (spec %q)", segment.name, spec)
	}
	return nil
}

func cmdFactoryValidateIssueScanRunnerSuite(args []string) error {
	fs := flag.NewFlagSet("factory validate-issue-scan-runner-suite", flag.ContinueOnError)
	packageDir := fs.String("package", "", "Path to the runner-suite package directory (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if strings.TrimSpace(*packageDir) == "" {
		return fmt.Errorf("%w: --package is required", errUsage)
	}
	report, err := validateIssueScanRunnerSuitePackage(*packageDir)
	if err != nil {
		return fmt.Errorf("issue-scan runner suite package %s invalid:\n%w", *packageDir, err)
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(body, '\n'))
	return err
}
