package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

type issueScanRunnerContractsDocument struct {
	Kind                      string                    `json:"kind"`
	LifecycleVersion          string                    `json:"lifecycle_version"`
	Purpose                   string                    `json:"purpose"`
	FullChainDaemonFlags      []string                  `json:"full_chain_daemon_flags"`
	NamedProgressFlags        []string                  `json:"named_progress_flags"`
	TerminalStagePaths        []issueScanTerminalPath   `json:"terminal_stage_paths"`
	GovernanceBoundaries      []string                  `json:"governance_boundaries"`
	ExternalRunnerContracts   []issueScanRunnerContract `json:"external_runner_contracts"`
	ManagedBoundaryContracts  []issueScanRunnerContract `json:"managed_boundary_contracts,omitempty"`
	InternalFinalizerContract *issueScanRunnerContract  `json:"internal_finalizer_contract,omitempty"`
	OperatorNotes             []string                  `json:"operator_notes"`
}

type issueScanRunnerContract struct {
	ID                   string   `json:"id"`
	Stage                string   `json:"stage"`
	DaemonFlag           string   `json:"daemon_flag,omitempty"`
	ProgressFlag         string   `json:"progress_flag,omitempty"`
	StandaloneCommand    string   `json:"standalone_command,omitempty"`
	StdinContextKind     string   `json:"stdin_context_kind"`
	StdinContextType     string   `json:"stdin_context_type"`
	StdoutContractType   string   `json:"stdout_contract_type"`
	StdoutRequiredFields []string `json:"stdout_required_fields"`
	Preconditions        []string `json:"preconditions,omitempty"`
	RecordedArtifacts    []string `json:"recorded_artifacts,omitempty"`
	ValidationBoundaries []string `json:"validation_boundaries"`
	AuthorityBoundaries  []string `json:"authority_boundaries"`
}

type issueScanTerminalPath struct {
	ID                    string   `json:"id"`
	Description           string   `json:"description"`
	Flags                 []string `json:"flags"`
	MutuallyExclusiveWith []string `json:"mutually_exclusive_with,omitempty"`
}

func cmdFactoryIssueScanRunnerContracts(args []string) error {
	fs := flag.NewFlagSet("factory issue-scan-runner-contracts", flag.ContinueOnError)
	format := fs.String("format", "json", "Output format (json)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *format != "json" {
		return fmt.Errorf("--format %q is not supported (want json)", *format)
	}
	body, err := issueScanRunnerContractsJSON()
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(body, '\n'))
	return err
}

func issueScanRunnerContractsJSON() ([]byte, error) {
	return json.MarshalIndent(issueScanRunnerContracts(), "", "  ")
}

func issueScanRunnerContracts() issueScanRunnerContractsDocument {
	return issueScanRunnerContractsDocument{
		Kind:             "issue_scan_runner_contracts",
		LifecycleVersion: "civilization_issue_to_human_ready_pr_v0.9",
		Purpose:          "Operator-readable contract for wiring external issue-scan runners into the governed Civilization lifecycle.",
		FullChainDaemonFlags: []string{
			"--issue-scan-require-full-chain",
			"--issue-scan-interval",
			"--issue-scan-repo",
			"--issue-scan-registry",
			"--repo-workspace-root",
			"--issue-scan-stage-role-runner",
			"--issue-scan-implementation-runner",
			"--issue-scan-review-runner",
			"--issue-scan-blocker-repair-runner",
			"--issue-scan-draft-pr-request",
			"--issue-scan-draft-pr-create",
			"--issue-scan-ready-pr-mark-ready",
			"--issue-scan-ready-pr-review-runner",
		},
		NamedProgressFlags: []string{
			"--run-configured-runners",
			"--run",
			"--repo-workspace-root",
			"--issue-scan-stage-role-runner",
			"--issue-scan-implementation-runner",
			"--issue-scan-review-runner",
			"--issue-scan-blocker-repair-runner",
			"--issue-scan-draft-pr-request",
			"--issue-scan-draft-pr-create",
			"--issue-scan-ready-pr-mark-ready",
			"--issue-scan-ready-pr-review-runner",
		},
		TerminalStagePaths: []issueScanTerminalPath{
			{
				ID:          "managed_ready_pr_finalizer",
				Description: "Hive marks the already approved draft PR ready, runs the configured exact-head ready-state review, and records ready-for-Human evidence.",
				Flags: []string{
					"--issue-scan-draft-pr-request",
					"--issue-scan-draft-pr-create",
					"--issue-scan-ready-pr-mark-ready",
					"--issue-scan-ready-pr-review-runner",
				},
				MutuallyExclusiveWith: []string{"--issue-scan-ready-pr-runner"},
			},
			{
				ID:          "external_ready_pr_evidence_runner",
				Description: "An external runner supplies both the draft PR receipt and ready-for-Human PR evidence for runtime validation and recording.",
				Flags: []string{
					"--issue-scan-ready-pr-runner",
				},
				MutuallyExclusiveWith: []string{"--issue-scan-ready-pr-mark-ready", "--issue-scan-ready-pr-review-runner"},
			},
		},
		GovernanceBoundaries: []string{
			"Runners receive JSON on stdin and must return exactly one JSON object on stdout.",
			"Runners may write diagnostics to stderr; stderr is included in runner failure errors.",
			"The runtime validates returned packets against the queued run, FactoryOrder, selected Transpara-AI repo, exact implementation commit, lifecycle stage, and Human-approval boundaries before recording evidence.",
			"Draft PR creation, ready-state mutation, Human approval, merge, deploy, and production migration are separate governed authorities.",
		},
		ExternalRunnerContracts: []issueScanRunnerContract{
			{
				ID:                 "stage_role_output_runner",
				Stage:              "research/debate/select/design planning stages",
				DaemonFlag:         "--issue-scan-stage-role-runner",
				ProgressFlag:       "--issue-scan-stage-role-runner",
				StandaloneCommand:  "hive factory run-issue-scan-stage-role-output",
				StdinContextKind:   "issue_scan_stage_role_output_runner_context",
				StdinContextType:   "hive.IssueScanStageRoleOutputRunnerContext",
				StdoutContractType: "hive.IssueScanStageRoleOutputRunnerResult",
				StdoutRequiredFields: []string{
					"role_outputs[]",
					"role_outputs[].role",
					"role_outputs[].summary",
					"role_outputs[].outputs[]",
				},
				RecordedArtifacts: []string{
					"issue_scan_stage_role_output",
					"issue_scan_stage_runtime_evidence when all required stage evidence is present",
				},
				ValidationBoundaries: []string{
					"role outputs must match the current stage and required civic roles",
					"output keys must satisfy the stage evidence contract before auto-completion",
				},
				AuthorityBoundaries: []string{
					"does not implement code",
					"does not run adversarial review",
					"does not create, mark ready, approve, merge, or deploy a PR",
				},
			},
			{
				ID:                 "implementation_runner",
				Stage:              "implementation",
				DaemonFlag:         "--issue-scan-implementation-runner",
				ProgressFlag:       "--issue-scan-implementation-runner",
				StandaloneCommand:  "hive factory run-issue-scan-implementation",
				StdinContextKind:   "issue_scan_implementation_runner_context",
				StdinContextType:   "hive.IssueScanImplementationRunnerContext",
				StdoutContractType: "hive.IssueScanImplementationRunnerResult",
				StdoutRequiredFields: []string{
					"operate_result_body",
					"completion_summary",
				},
				RecordedArtifacts: []string{
					"Operate result artifact on the implementation task",
					"implementation task completion event",
				},
				ValidationBoundaries: []string{
					"operate_result_body must be valid Work Operate result JSON",
					"completion_summary is required",
					"returned branch and commit are parsed from the Operate result before review can run",
				},
				AuthorityBoundaries: []string{
					"may modify the target worktree/branch only within the supplied repo context",
					"does not create, mark ready, approve, merge, or deploy a PR",
				},
			},
			{
				ID:                 "adversarial_review_runner",
				Stage:              "run_adversarial_review",
				DaemonFlag:         "--issue-scan-review-runner",
				ProgressFlag:       "--issue-scan-review-runner",
				StandaloneCommand:  "hive factory run-issue-scan-review",
				StdinContextKind:   "issue_scan_adversarial_review_context",
				StdinContextType:   "hive.IssueScanAdversarialReviewContext",
				StdoutContractType: "hive.IssueScanAdversarialReviewReceipt",
				StdoutRequiredFields: []string{
					"review_ref",
					"reviewed_head_sha",
					"verdict",
					"summary",
					"confidence",
				},
				RecordedArtifacts: []string{
					"issue_scan_adversarial_review_receipt",
					"code.review.submitted event",
				},
				ValidationBoundaries: []string{
					"reviewed_head_sha must match the implementation Operate commit",
					"verdict drives blocker repair versus terminal PR readiness",
				},
				AuthorityBoundaries: []string{
					"does not repair blockers",
					"does not mark PRs ready",
					"does not approve, merge, or deploy",
				},
			},
			{
				ID:                 "blocker_repair_runner",
				Stage:              "repair_blockers",
				DaemonFlag:         "--issue-scan-blocker-repair-runner",
				ProgressFlag:       "--issue-scan-blocker-repair-runner",
				StandaloneCommand:  "hive factory run-issue-scan-blocker-repair",
				StdinContextKind:   "issue_scan_blocker_repair_runner_context",
				StdinContextType:   "hive.IssueScanBlockerRepairRunnerContext",
				StdoutContractType: "hive.IssueScanBlockerRepairRunnerResult",
				StdoutRequiredFields: []string{
					"operate_result_body",
					"completion_summary",
				},
				RecordedArtifacts: []string{
					"Operate result artifact on the reopened implementation task",
					"implementation task completion event",
				},
				ValidationBoundaries: []string{
					"operate_result_body must be valid Work Operate result JSON",
					"returned commit must differ from the previous reviewed commit",
					"review is rerun after blocker repair before PR readiness can advance",
				},
				AuthorityBoundaries: []string{
					"may modify the target worktree/branch only within the supplied repo context",
					"does not create, mark ready, approve, merge, or deploy a PR",
				},
			},
			{
				ID:                 "ready_pr_evidence_runner",
				Stage:              "ready_for_human_pr",
				DaemonFlag:         "--issue-scan-ready-pr-runner",
				ProgressFlag:       "--issue-scan-ready-pr-runner",
				StandaloneCommand:  "hive factory run-issue-scan-ready-pr",
				StdinContextKind:   "issue_scan_ready_pr_runner_context",
				StdinContextType:   "hive.IssueScanReadyPRRunnerContext",
				StdoutContractType: "hive.IssueScanReadyPRRunnerResult",
				StdoutRequiredFields: []string{
					"draft_pr_receipt",
					"draft_pr_receipt.kind=transpara_ai_draft_pr_receipt",
					"ready_pr_evidence",
					"ready_pr_evidence.kind=issue_scan_ready_pr_evidence",
				},
				RecordedArtifacts: []string{
					"transpara_ai_draft_pr_receipt",
					"issue_scan_ready_pr_evidence",
				},
				ValidationBoundaries: []string{
					"draft PR receipt must match the selected Transpara-AI repo and implementation commit",
					"ready PR evidence must prove non-draft ready-for-review state, successful CI, passing ready-state review, and Human approval still required",
				},
				AuthorityBoundaries: []string{
					"generic ready PR runner cannot be combined with --issue-scan-ready-pr-mark-ready",
					"does not approve, merge, deploy, or perform production migrations",
				},
			},
			{
				ID:                 "ready_state_review_runner",
				Stage:              "ready_for_human_pr finalizer review",
				DaemonFlag:         "--issue-scan-ready-pr-review-runner",
				ProgressFlag:       "--issue-scan-ready-pr-review-runner",
				StdinContextKind:   "issue_scan_ready_state_review_context",
				StdinContextType:   "hive.IssueScanReadyStateReviewContext",
				StdoutContractType: "hive.IssueScanReadyStateReviewReceipt",
				StdoutRequiredFields: []string{
					"review_ref",
					"reviewed_head_sha",
					"status",
				},
				RecordedArtifacts: []string{
					"embedded in issue_scan_ready_pr_evidence.ready_state_review_ref/status",
				},
				ValidationBoundaries: []string{
					"reviewed_head_sha must match the exact PR head after marking ready",
					"status must pass before ready-for-Human evidence can complete the terminal stage",
				},
				AuthorityBoundaries: []string{
					"used only with --issue-scan-ready-pr-mark-ready",
					"does not approve, merge, deploy, or perform production migrations",
				},
			},
		},
		ManagedBoundaryContracts: []issueScanRunnerContract{
			{
				ID:                 "draft_pr_authority_requester",
				Stage:              "draft_pr_authority_request",
				DaemonFlag:         "--issue-scan-draft-pr-request",
				ProgressFlag:       "--issue-scan-draft-pr-request",
				StandaloneCommand:  "hive factory request-issue-scan-pr",
				StdinContextKind:   "issue_scan_draft_pr_authority_request_runner_context",
				StdinContextType:   "hive.IssueScanDraftPRAuthorityRequestRunnerContext",
				StdoutContractType: "hive.IssueScanDraftPRAuthorityRequestRunnerResult",
				StdoutRequiredFields: []string{
					"base_ref",
					"base_sha",
					"nonce",
				},
				Preconditions: []string{
					"zero-blocker evidence is complete",
					"ready-for-Human stage has no draft PR receipt yet",
					"matching authority request is not already recorded",
				},
				RecordedArtifacts: []string{
					"authority.request.recorded for pull_request.create",
				},
				ValidationBoundaries: []string{
					"base_sha is resolved from the configured repository base ref",
					"head_sha is pinned to the implementation Operate commit",
					"request is held for Human approval by default",
				},
				AuthorityBoundaries: []string{
					"raises an approval request only",
					"does not create, mark ready, approve, merge, or deploy a PR",
				},
			},
			{
				ID:                 "draft_pr_creation_runner",
				Stage:              "draft_pr_creation",
				DaemonFlag:         "--issue-scan-draft-pr-create",
				ProgressFlag:       "--issue-scan-draft-pr-create",
				StandaloneCommand:  "hive factory create-issue-scan-draft-pr",
				StdinContextKind:   "issue_scan_draft_pr_creation_runner_context",
				StdinContextType:   "hive.IssueScanDraftPRCreationRunnerContext",
				StdoutContractType: "hive.IssueScanDraftPRCreationResult",
				StdoutRequiredFields: []string{
					"request_id",
					"draft_pr_receipt",
					"head_sha",
				},
				Preconditions: []string{
					"recorded Human approval exists for the draft PR authority request",
					"approved scope matches the run-derived repo/base/head/title/body/policy/nonce target",
					"ready-for-Human stage has no draft PR receipt yet",
				},
				RecordedArtifacts: []string{
					"issue_scan_draft_pr_creation_reservation",
					"transpara_ai_draft_pr_receipt",
				},
				ValidationBoundaries: []string{
					"requires GITHUB_TOKEN",
					"verifies title/body hashes against the recorded Human-approved scope",
					"refuses duplicate creation after reservation without receipt",
				},
				AuthorityBoundaries: []string{
					"creates a draft PR only after Human approval",
					"does not mark ready, approve, merge, deploy, or perform production migrations",
				},
			},
		},
		InternalFinalizerContract: &issueScanRunnerContract{
			ID:                 "ready_pr_finalizer",
			Stage:              "ready_for_human_pr",
			DaemonFlag:         "--issue-scan-ready-pr-mark-ready",
			ProgressFlag:       "--issue-scan-ready-pr-mark-ready",
			StdinContextKind:   "issue_scan_ready_pr_runner_context",
			StdinContextType:   "hive.IssueScanReadyPRRunnerContext",
			StdoutContractType: "hive.IssueScanReadyPRRunnerResult",
			StdoutRequiredFields: []string{
				"draft_pr_receipt",
				"ready_pr_evidence",
			},
			Preconditions: []string{
				"transpara_ai_draft_pr_receipt artifact already recorded",
				"recorded approved human-decided unexpired pull_request.mark_ready decision exactly matches the run-derived repository, PR number, and head (a draft-PR creation approval never authorizes readying; non-human decisions neither authorize nor shadow human ones; a past finite expires_at refuses)",
				"the approval's single-use nonce has no recorded consumption anywhere in the store (mark_ready_approval_consumed artifact, cross-run, full pagination); consumption is recorded durably before the mutation with append-then-verify-winner claim ordering",
				"no issue_scan_ready_pr_blocked artifact is recorded on the ready stage (blocked evidence is terminal until explicit human remediation; the check pages through all artifact events)",
				"ready_state_review_runner returns passing exact-head receipt",
			},
			RecordedArtifacts: []string{
				"issue_scan_ready_pr_evidence",
			},
			ValidationBoundaries: []string{
				"requires GITHUB_TOKEN",
				"marks only the approved draft PR ready for review",
				"fetches live PR state and rejects moved heads or unsafe final evidence",
			},
			AuthorityBoundaries: []string{
				"does not approve, merge, deploy, or perform production migrations",
				"requires --issue-scan-ready-pr-review-runner",
				"re-drafts after a failed ready-state review only when the recorded mark-ready approval carries re_draft_on_failure; otherwise records blocked evidence and stops",
				"treats a mark-ready client failure as a possible mutation unless the client proves the PR un-mutated; unproven failures record blocked evidence",
				"reports a re-draft successful only when the returned live state proves the same PR is draft again",
				"re-draft reads only the pull-request endpoint (never commit-status or check-runs), so a CI-endpoint outage cannot prevent returning the PR to draft",
			},
		},
		OperatorNotes: []string{
			"Use --issue-scan-require-full-chain on daemon startup when the intended posture is autonomous issue-scan to ready-for-Human PR.",
			"Full-chain daemon startup requires either --issue-scan-repo or --issue-scan-registry as the issue source.",
			"Full-chain daemon startup resolves every configured external runner executable before entering the daemon loop.",
			"Named configured progress resolves every supplied external runner executable before opening the runtime or invoking runners.",
			"Use hive factory progress-issue-scan --run-configured-runners --run <id> for a bounded named-run rehearsal before daemonizing the same runner chain.",
			"Use hive factory issue-scan-runner-contexts --run <id> to build/check which runner context is ready for a stored issue-scan run before invoking an external command; context building may dispatch/scaffold the queued run.",
			"Standalone run-issue-scan-* commands resolve --runner before opening the runtime or building runner context.",
			"Use the standalone run-issue-scan-* commands to debug a single runner against a stored run without bypassing runtime validation.",
			"The full_chain_daemon_flags and named_progress_flags arrays show the managed ready-PR finalizer posture; terminal_stage_paths lists the mutually-exclusive generic ready-PR evidence runner alternative.",
		},
	}
}
