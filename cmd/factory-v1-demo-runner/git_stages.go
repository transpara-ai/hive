package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

type pullRequestView struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	HeadRefOID  string `json:"headRefOid"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	IsDraft     bool   `json:"isDraft"`
	State       string `json:"state"`
}

type checkView struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Bucket string `json:"bucket"`
	Link   string `json:"link"`
}

func (r *demoRunner) executeWriteCode(ctx context.Context, request factoryv1.RunRequest) (factoryv1.RunResult, map[string]string, error) {
	if reconciled, err := r.reconcileWriteCode(ctx, request); err != nil {
		return factoryv1.RunResult{}, nil, err
	} else if reconciled.EffectExists || reconciled.Conflict {
		return reconciled.Result, nil, nil
	}
	repository := r.config.Repositories[request.Order.TargetRepository]
	if err := r.ensureRepositoryPins(ctx, repository); err != nil {
		return r.blocked(request, "repository_pin_failed", err.Error(), "Repair the exact repository/remote identity binding."), nil, nil
	}
	worktree, branch, err := r.ensureWorktree(ctx, request, repository)
	if err != nil {
		return r.blocked(request, "worktree_failed", err.Error(), "Resolve the deterministic branch/worktree conflict."), nil, nil
	}
	relative := evidenceRelativePath(request)
	target := filepath.Join(worktree, filepath.FromSlash(relative))
	content := []byte(renderDemoEvidence(request))
	if existing, readErr := os.ReadFile(target); readErr == nil && string(existing) != string(content) {
		return r.blocked(request, "evidence_file_conflict", "The bounded evidence path contains different bytes.", "Resolve the branch content conflict without overwriting unrelated work."), map[string]string{"branch": branch, "path": relative}, nil
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return factoryv1.RunResult{}, nil, readErr
	}
	status, err := r.git(ctx, worktree, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	if err := allowOnlyPath(status.Stdout, relative); err != nil {
		return r.blocked(request, "worktree_not_bounded", err.Error(), "Remove or preserve unrelated work before retrying."), map[string]string{"branch": branch}, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	workingValidation, err := r.runWorkingTreeValidationAt(ctx, repository, worktree)
	if err != nil {
		return r.blocked(request, "write_validation_failed", err.Error(), "Repair the bounded evidence change or named tests."), map[string]string{"branch": branch, "path": relative}, nil
	}
	status, err = r.git(ctx, worktree, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	if err := allowOnlyPath(status.Stdout, relative); err != nil {
		return r.blocked(request, "test_side_effect_not_bounded", err.Error(), "Remove test-generated or unrelated work before retrying."), map[string]string{"branch": branch}, nil
	}
	committedContent, committed := r.fileAtHead(ctx, worktree, relative)
	if !committed || committedContent != string(content) {
		if _, err := r.git(ctx, worktree, "add", "--", relative); err != nil {
			return factoryv1.RunResult{}, nil, err
		}
		commitMessage := "docs(factory-v1): add " + safeOrderID(request.Order.DocID) + " evidence"
		if _, err := r.git(ctx, worktree, "-c", "user.name="+r.config.CommitUserName, "-c", "user.email="+r.config.CommitUserEmail, "commit", "-m", commitMessage, "--", relative); err != nil {
			return r.blocked(request, "commit_failed", err.Error(), "Inspect the deterministic branch and retry without rewriting history."), map[string]string{"branch": branch}, nil
		}
	}
	head, err := r.gitOutput(ctx, worktree, "rev-parse", "HEAD")
	if err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	if !gitHashPattern.MatchString(head) {
		return factoryv1.RunResult{}, nil, errors.New("git produced an invalid implementation head")
	}
	validation, err := r.runRepositoryValidationAt(ctx, request, repository, worktree, true)
	if err != nil {
		return r.blocked(request, "committed_validation_failed", err.Error(), "Repair the exact committed diff or named tests before push."), map[string]string{"branch": branch, "head": head}, nil
	}
	if err := r.ensureRepositoryPins(ctx, repository); err != nil {
		return r.blocked(request, "pre_push_pin_failed", err.Error(), "Repair the exact push target binding; no push occurred."), map[string]string{"branch": branch, "head": head}, nil
	}
	if _, err := r.git(ctx, worktree, "push", r.config.GitRemote, "HEAD:refs/heads/"+branch); err != nil {
		return r.blocked(request, "push_failed", err.Error(), "Resolve the non-force deterministic branch push conflict."), map[string]string{"branch": branch, "head": head}, nil
	}
	remoteHead, err := r.remoteBranchHead(ctx, repository, branch)
	if err != nil {
		return r.blocked(request, "remote_head_unavailable", err.Error(), "Reconcile the deterministic branch without force-pushing."), map[string]string{"branch": branch, "head": head}, nil
	}
	if remoteHead != head {
		return r.blocked(request, "remote_head_mismatch", fmt.Sprintf("The pushed branch head %s differs from local head %s.", remoteHead, head), "Reconcile the deterministic branch without force-pushing."), map[string]string{"branch": branch, "head": head, "remote_head": remoteHead}, nil
	}
	evidence := factoryv1.Evidence{Kind: "code", Reference: "git:" + repository.Identity + "@" + head + ":" + relative, PRHeadSHA: head, SHA256: factoryv1.HashText(string(content)), Metadata: map[string]string{"branch": branch, "path": relative, "tests_passing": "true", "working_validation_sha256": workingValidation, "validation_sha256": validation}}
	return r.passed(request, evidence), map[string]string{"branch": branch, "head": head, "path": relative, "working_validation_sha256": workingValidation, "validation_sha256": validation}, nil
}

func (r *demoRunner) reconcileWriteCode(ctx context.Context, request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	repository := r.config.Repositories[request.Order.TargetRepository]
	if err := r.ensureRepositoryPins(ctx, repository); err != nil {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "repository_pin_failed", err.Error(), "Repair the exact repository/remote identity binding.")}, nil
	}
	branch := branchName(request)
	remoteHead, err := r.remoteBranchHead(ctx, repository, branch)
	if err != nil {
		if isMissingRef(err) {
			return factoryv1.ReconcileResult{EffectExists: false, Result: r.blocked(request, "branch_missing", "The deterministic remote branch does not exist.", "Execute the same reconciled attempt.")}, nil
		}
		return factoryv1.ReconcileResult{}, err
	}
	if !gitHashPattern.MatchString(remoteHead) {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "remote_head_invalid", "The deterministic remote branch has an invalid head.", "Resolve the remote branch conflict.")}, nil
	}
	if receipt, found, receiptErr := r.loadReceipt(request); receiptErr != nil {
		return factoryv1.ReconcileResult{}, receiptErr
	} else if found && receipt.Result.Status == factoryv1.RunnerPassed {
		for _, evidence := range receipt.Result.Evidence {
			if evidence.PRHeadSHA == remoteHead && evidence.Metadata["branch"] == branch {
				return factoryv1.ReconcileResult{EffectExists: true, Result: receipt.Result}, nil
			}
		}
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "receipt_head_conflict", "The remote branch head differs from the private attempt receipt.", "Resolve the branch/receipt conflict without force-pushing.")}, nil
	}
	worktree, _, err := r.ensureWorktree(ctx, request, repository)
	if err != nil {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "worktree_conflict", err.Error(), "Resolve the deterministic worktree conflict.")}, nil
	}
	localHead, err := r.gitOutput(ctx, worktree, "rev-parse", "HEAD")
	if err != nil || localHead != remoteHead {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "local_remote_head_conflict", "The deterministic local worktree differs from the remote branch head.", "Resolve the local/remote branch conflict without rewriting history.")}, nil
	}
	relative := evidenceRelativePath(request)
	content, err := os.ReadFile(filepath.Join(worktree, filepath.FromSlash(relative)))
	if err != nil || string(content) != renderDemoEvidence(request) {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "branch_content_conflict", "The remote branch does not contain the exact bounded evidence bytes.", "Resolve the branch content conflict.")}, nil
	}
	validation, err := r.runRepositoryValidationAt(ctx, request, repository, worktree, true)
	if err != nil {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "reconciled_validation_failed", err.Error(), "Repair the exact branch validation failure.")}, nil
	}
	evidence := factoryv1.Evidence{Kind: "code", Reference: "git:" + repository.Identity + "@" + remoteHead + ":" + relative, PRHeadSHA: remoteHead, SHA256: factoryv1.HashText(string(content)), Metadata: map[string]string{"branch": branch, "path": relative, "tests_passing": "true", "validation_sha256": validation}}
	return factoryv1.ReconcileResult{EffectExists: true, Result: r.passed(request, evidence)}, nil
}

func (r *demoRunner) executeCreateDraftPR(ctx context.Context, request factoryv1.RunRequest) (factoryv1.RunResult, map[string]string, error) {
	repository := r.config.Repositories[request.Order.TargetRepository]
	head, err := implementationHead(request.PriorEvidence)
	if err != nil {
		return r.blocked(request, "implementation_head_missing", err.Error(), "Restore exact write_code evidence."), nil, nil
	}
	branch := branchName(request)
	view, found, conflict, err := r.queryOpenPR(ctx, repository, branch)
	if err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	if conflict || (found && view.HeadRefOID != head) {
		return r.blocked(request, "draft_pr_conflict", "Open PR state conflicts with the deterministic branch/head.", "Resolve duplicate or mismatched pull requests."), map[string]string{"branch": branch, "head": head}, nil
	}
	if !found {
		title := "Factory v1 demo: " + oneLine(request.Order.Title)
		body := "Automated non-production Factory v1 demonstration output.\n\nOrder: `" + request.Order.DocID + "@" + request.Order.Version + "`\nDocument SHA-256: `" + request.DocumentSHA256 + "`\n\nThis PR remains at the Human Review boundary and must not be auto-merged."
		if _, err := r.gh(ctx, repository.Root, "pr", "create", "--repo", repository.Identity, "--draft", "--head", branch, "--base", repository.BaseBranch, "--title", title, "--body", body); err != nil {
			return r.blocked(request, "draft_pr_create_failed", err.Error(), "Inspect GitHub state and reconcile the deterministic head branch."), map[string]string{"branch": branch, "head": head}, nil
		}
		view, found, conflict, err = r.queryOpenPR(ctx, repository, branch)
		if err != nil || !found || conflict {
			return r.blocked(request, "draft_pr_unobservable", "The created draft PR was not uniquely observable.", "Resolve GitHub visibility or duplicate PR state."), map[string]string{"branch": branch, "head": head}, nil
		}
	}
	if !view.IsDraft || view.State != "OPEN" || view.BaseRefName != repository.BaseBranch || view.HeadRefOID != head {
		return r.blocked(request, "draft_pr_exactness_failed", "The PR is not one open draft at the exact implementation head/base.", "Resolve the PR state conflict."), map[string]string{"branch": branch, "head": head}, nil
	}
	pr := prEvidence(repository, view, head, false)
	return r.passed(request, factoryv1.Evidence{Kind: "draft_pr", Reference: view.URL, PRHeadSHA: head, PR: &pr}), map[string]string{"branch": branch, "head": head, "pr_number": strconv.Itoa(view.Number), "pr_url": view.URL}, nil
}

func (r *demoRunner) reconcileDraftPR(ctx context.Context, request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	repository := r.config.Repositories[request.Order.TargetRepository]
	head, err := implementationHead(request.PriorEvidence)
	if err != nil {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "implementation_head_missing", err.Error(), "Restore exact write_code evidence.")}, nil
	}
	view, found, conflict, err := r.queryOpenPR(ctx, repository, branchName(request))
	if err != nil {
		return factoryv1.ReconcileResult{}, err
	}
	if !found {
		return factoryv1.ReconcileResult{EffectExists: false, Result: r.blocked(request, "draft_pr_missing", "No open PR exists for the deterministic branch.", "Execute the same reconciled attempt.")}, nil
	}
	if conflict || !view.IsDraft || view.HeadRefOID != head || view.BaseRefName != repository.BaseBranch {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "draft_pr_conflict", "The open PR does not match the one exact draft PR predicate.", "Resolve GitHub PR state.")}, nil
	}
	pr := prEvidence(repository, view, head, false)
	return factoryv1.ReconcileResult{EffectExists: true, Result: r.passed(request, factoryv1.Evidence{Kind: "draft_pr", Reference: view.URL, PRHeadSHA: head, PR: &pr})}, nil
}

func (r *demoRunner) executeIAR(ctx context.Context, request factoryv1.RunRequest) (factoryv1.RunResult, map[string]string, error) {
	repository := r.config.Repositories[request.Order.TargetRepository]
	view, found, conflict, err := r.queryOpenPR(ctx, repository, branchName(request))
	if err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	if !found || conflict {
		return r.blocked(request, "iar_pr_missing", "IAR cannot identify one open exact-head PR.", "Restore the deterministic draft PR."), nil, nil
	}
	head, err := implementationHead(request.PriorEvidence)
	if err != nil || view.HeadRefOID != head {
		return r.blocked(request, "iar_head_mismatch", "IAR PR head differs from prior implementation evidence.", "Restore exact-head evidence before IAR."), nil, nil
	}
	worktree, _, err := r.ensureWorktree(ctx, request, repository)
	if err != nil {
		return r.blocked(request, "iar_worktree_failed", err.Error(), "Repair the deterministic exact-head worktree."), nil, nil
	}
	localHead, err := r.gitOutput(ctx, worktree, "rev-parse", "HEAD")
	if err != nil || localHead != head {
		return r.blocked(request, "iar_local_head_mismatch", "The deterministic worktree does not equal the PR head selected for IAR.", "Reconcile the worktree to the exact non-rewritten PR head."), nil, nil
	}
	validation, err := r.runRepositoryValidationAt(ctx, request, repository, worktree, true)
	if err != nil {
		return r.blocked(request, "iar_validation_failed", err.Error(), "Repair the exact PR head until diff and named tests pass."), map[string]string{"head": head}, nil
	}
	zero := 0
	evidence := factoryv1.Evidence{Kind: "gate", Reference: "runner-receipt:" + request.AttemptID, PRHeadSHA: head, ReviewedHeadSHA: head, BlockerCount: &zero, AuthorFamily: r.config.AuthorFamily, ReviewerFamily: r.config.AuthorFamily, SHA256: validation}
	return r.passed(request, evidence), map[string]string{"head": head, "validation_sha256": validation, "pr_number": strconv.Itoa(view.Number)}, nil
}

func (r *demoRunner) reconcileIAR(ctx context.Context, request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	receipt, found, err := r.loadReceipt(request)
	if err != nil {
		return factoryv1.ReconcileResult{}, err
	}
	if !found {
		return factoryv1.ReconcileResult{EffectExists: false, Result: r.blocked(request, "iar_receipt_missing", "No exact IAR attempt receipt exists.", "Execute the same reconciled attempt.")}, nil
	}
	if receipt.Result.Status != factoryv1.RunnerPassed {
		return factoryv1.ReconcileResult{EffectExists: true, Result: receipt.Result}, nil
	}
	head, err := reviewedGateHead(receipt.Result.Evidence, "gate")
	if err != nil {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "iar_receipt_invalid", err.Error(), "Repair the private exact-head IAR receipt.")}, nil
	}
	repository := r.config.Repositories[request.Order.TargetRepository]
	view, prFound, conflict, err := r.queryOpenPR(ctx, repository, branchName(request))
	if err != nil {
		return factoryv1.ReconcileResult{}, err
	}
	if !prFound {
		return factoryv1.ReconcileResult{EffectExists: false, Result: r.blocked(request, "iar_pr_missing", "The exact PR reviewed by IAR is no longer open.", "Restore the exact deterministic PR before retrying.")}, nil
	}
	if conflict || view.HeadRefOID != head {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "iar_head_conflict", "The current PR head differs from the IAR-reviewed head.", "Resolve the PR head conflict without rewriting review evidence.")}, nil
	}
	return factoryv1.ReconcileResult{EffectExists: true, Result: receipt.Result}, nil
}

func (r *demoRunner) executeMarkReady(ctx context.Context, request factoryv1.RunRequest) (factoryv1.RunResult, map[string]string, error) {
	repository := r.config.Repositories[request.Order.TargetRepository]
	head, err := reviewedGateHead(request.PriorEvidence, "cross_family_gate")
	if err != nil {
		return r.blocked(request, "cfar_head_missing", err.Error(), "Restore exact blocker-free CFAR evidence."), nil, nil
	}
	view, found, conflict, err := r.queryOpenPR(ctx, repository, branchName(request))
	if err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	if !found || conflict || view.HeadRefOID != head {
		return r.blocked(request, "ready_pr_head_conflict", "The open PR does not match the exact CFAR-reviewed head.", "Resolve the PR/head conflict before readying."), nil, nil
	}
	checks, checksPassing, err := r.requiredChecks(ctx, repository, view.Number)
	if err != nil || !checksPassing {
		return r.blocked(request, "required_checks_not_passing", commandErrorText(err, "Required checks are not all passing."), "Wait for or repair required checks without changing the reviewed head."), map[string]string{"head": head, "required_check_count": strconv.Itoa(len(checks))}, nil
	}
	if view.IsDraft {
		if _, err := r.gh(ctx, repository.Root, "pr", "ready", strconv.Itoa(view.Number), "--repo", repository.Identity); err != nil {
			return r.blocked(request, "pr_ready_failed", err.Error(), "Reconcile GitHub state and retry the same exact reviewed head."), nil, nil
		}
	}
	view, found, conflict, err = r.queryOpenPR(ctx, repository, branchName(request))
	if err != nil || !found || conflict || view.IsDraft || view.HeadRefOID != head {
		return r.blocked(request, "ready_pr_unverified", "The PR is not observably open and non-draft at the reviewed head.", "Repair or reconcile the PR ready transition."), nil, nil
	}
	checks, checksPassing, err = r.requiredChecks(ctx, repository, view.Number)
	if err != nil || !checksPassing {
		return r.blocked(request, "post_ready_checks_failed", commandErrorText(err, "Required checks stopped passing."), "Restore passing checks at the unchanged reviewed head."), nil, nil
	}
	pr := prEvidence(repository, view, head, true)
	return r.passed(request, factoryv1.Evidence{Kind: "ready_pr", Reference: view.URL, PRHeadSHA: head, ReviewedHeadSHA: head, PR: &pr, Metadata: map[string]string{"required_check_count": strconv.Itoa(len(checks))}}), map[string]string{"head": head, "pr_number": strconv.Itoa(view.Number), "pr_url": view.URL, "required_check_count": strconv.Itoa(len(checks))}, nil
}

func (r *demoRunner) reconcileReadyPR(ctx context.Context, request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	repository := r.config.Repositories[request.Order.TargetRepository]
	head, err := reviewedGateHead(request.PriorEvidence, "cross_family_gate")
	if err != nil {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "cfar_head_missing", err.Error(), "Restore exact CFAR evidence.")}, nil
	}
	view, found, conflict, err := r.queryOpenPR(ctx, repository, branchName(request))
	if err != nil {
		return factoryv1.ReconcileResult{}, err
	}
	if !found || view.IsDraft {
		return factoryv1.ReconcileResult{EffectExists: false, Result: r.blocked(request, "ready_effect_absent", "The exact PR is absent or still draft.", "Execute the same reconciled ready transition.")}, nil
	}
	if conflict || view.HeadRefOID != head {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "ready_head_conflict", "The ready PR differs from the CFAR-reviewed head.", "Resolve the PR/head conflict.")}, nil
	}
	checks, passing, err := r.requiredChecks(ctx, repository, view.Number)
	if err != nil || !passing {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "ready_checks_conflict", commandErrorText(err, "Required checks are not passing."), "Restore exact-head checks.")}, nil
	}
	pr := prEvidence(repository, view, head, true)
	result := r.passed(request, factoryv1.Evidence{Kind: "ready_pr", Reference: view.URL, PRHeadSHA: head, ReviewedHeadSHA: head, PR: &pr, Metadata: map[string]string{"required_check_count": strconv.Itoa(len(checks))}})
	return factoryv1.ReconcileResult{EffectExists: true, Result: result}, nil
}

func (r *demoRunner) executeHumanReview(ctx context.Context, request factoryv1.RunRequest) (factoryv1.RunResult, map[string]string, error) {
	reconciled, err := r.reconcileHumanReview(ctx, request)
	if err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	return reconciled.Result, map[string]string{"human_boundary": "true"}, nil
}

func (r *demoRunner) reconcileHumanReview(ctx context.Context, request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	repository := r.config.Repositories[request.Order.TargetRepository]
	head, err := readyHead(request.PriorEvidence)
	if err != nil {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "ready_evidence_missing", err.Error(), "Restore exact ready PR evidence.")}, nil
	}
	view, found, conflict, err := r.queryOpenPR(ctx, repository, branchName(request))
	if err != nil {
		return factoryv1.ReconcileResult{}, err
	}
	if !found || conflict || view.IsDraft || view.HeadRefOID != head {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "human_review_boundary_invalid", "The exact-head ready PR boundary is no longer valid.", "Restore the open non-draft exact-head PR and passing checks.")}, nil
	}
	checks, passing, checkErr := r.requiredChecks(ctx, repository, view.Number)
	if checkErr != nil || !passing {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "human_review_boundary_invalid", "The exact-head ready PR boundary is no longer valid.", "Restore the open non-draft exact-head PR and passing checks.")}, nil
	}
	pr := prEvidence(repository, view, head, true)
	result := factoryv1.RunResult{
		Status: factoryv1.RunnerHumanRequired, Provider: request.Provider,
		NextAction: "Human reviews the exact-head ready PR; the runner will not merge or deploy it.",
		Evidence:   []factoryv1.Evidence{{Kind: "human_review_boundary", Reference: view.URL, PRHeadSHA: head, ReviewedHeadSHA: head, PR: &pr, Metadata: map[string]string{"required_check_count": strconv.Itoa(len(checks))}}},
	}
	return factoryv1.ReconcileResult{EffectExists: true, Result: result}, nil
}

func (r *demoRunner) runRepositoryValidation(ctx context.Context, request factoryv1.RunRequest, implementation bool) (string, error) {
	repository := r.config.Repositories[request.Order.TargetRepository]
	return r.runRepositoryValidationAt(ctx, request, repository, repository.Root, implementation)
}

func (r *demoRunner) runRepositoryValidationAt(ctx context.Context, request factoryv1.RunRequest, repository repositoryConfig, directory string, implementation bool) (string, error) {
	if err := r.ensureRepositoryPins(ctx, repository); err != nil {
		return "", err
	}
	var receipts []string
	base := "refs/remotes/" + r.config.GitRemote + "/" + repository.BaseBranch
	if _, err := r.git(ctx, repository.Root, "fetch", "--no-tags", r.config.GitRemote, "+refs/heads/"+repository.BaseBranch+":"+base); err != nil {
		return "", err
	}
	diffRange := base + "..." + base
	if implementation {
		diffRange = base + "...HEAD"
	}
	result, err := r.git(ctx, directory, "diff", "--check", diffRange)
	if err != nil {
		return "", err
	}
	receipts = append(receipts, "git diff --check\n"+result.Stdout)
	if implementation {
		files, err := r.gitOutput(ctx, directory, "diff", "--name-only", diffRange)
		if err != nil {
			return "", err
		}
		expected := evidenceRelativePath(request)
		if strings.TrimSpace(files) != expected {
			return "", fmt.Errorf("bounded diff contains %q, want only %q", strings.TrimSpace(files), expected)
		}
		receipts = append(receipts, "git diff --name-only\n"+files)
	}
	for _, command := range repository.TestCommands {
		executable, err := resolveTestExecutable(command[0], r.config.GitExecutable, r.config.GHExecutable)
		if err != nil {
			return "", err
		}
		result, err := r.commands.Run(ctx, directory, executable, command[1:]...)
		if err != nil {
			return "", err
		}
		receipts = append(receipts, strings.Join(command, " ")+"\n"+result.Stdout+"\n"+result.Stderr)
	}
	status, err := r.gitOutput(ctx, directory, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	if status != "" {
		return "", fmt.Errorf("named validation left the repository dirty: %q", status)
	}
	sum := sha256.Sum256([]byte(strings.Join(receipts, "\n---\n")))
	return hex.EncodeToString(sum[:]), nil
}

func (r *demoRunner) runWorkingTreeValidationAt(ctx context.Context, repository repositoryConfig, directory string) (string, error) {
	var receipts []string
	result, err := r.git(ctx, directory, "diff", "--check")
	if err != nil {
		return "", err
	}
	receipts = append(receipts, "git diff --check\n"+result.Stdout)
	for _, command := range repository.TestCommands {
		executable, err := resolveTestExecutable(command[0], r.config.GitExecutable, r.config.GHExecutable)
		if err != nil {
			return "", err
		}
		result, err := r.commands.Run(ctx, directory, executable, command[1:]...)
		if err != nil {
			return "", err
		}
		receipts = append(receipts, strings.Join(command, " ")+"\n"+result.Stdout+"\n"+result.Stderr)
	}
	sum := sha256.Sum256([]byte(strings.Join(receipts, "\n---\n")))
	return hex.EncodeToString(sum[:]), nil
}

func (r *demoRunner) fileAtHead(ctx context.Context, directory, relative string) (string, bool) {
	result, err := r.git(ctx, directory, "show", "HEAD:"+relative)
	if err != nil {
		return "", false
	}
	return result.Stdout, true
}

func resolveTestExecutable(name, gitExecutable, ghExecutable string) (string, error) {
	if name == "git" || name == filepath.Base(gitExecutable) {
		return gitExecutable, nil
	}
	if name == "gh" || name == filepath.Base(ghExecutable) {
		return "", errors.New("named validation commands may not mutate GitHub")
	}
	return exec.LookPath(name)
}

func (r *demoRunner) ensureRepositoryPins(ctx context.Context, repository repositoryConfig) error {
	root, err := r.gitOutput(ctx, repository.Root, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil || resolvedRoot != repository.Root {
		return errors.New("git repository root differs from pinned configuration")
	}
	for _, args := range [][]string{{"remote", "get-url", "--all", r.config.GitRemote}, {"remote", "get-url", "--push", "--all", r.config.GitRemote}} {
		urlOutput, err := r.gitOutput(ctx, repository.Root, args...)
		if err != nil {
			return err
		}
		urls := strings.Fields(urlOutput)
		if len(urls) != 1 || normalizeGitHubURL(urls[0]) != normalizeGitHubURL(repository.RemoteURL) {
			return errors.New("git remote URL differs from the pinned GitHub target")
		}
	}
	identity, err := r.ghOutput(ctx, repository.Root, "repo", "view", repository.Identity, "--json", "nameWithOwner", "--jq", ".nameWithOwner")
	if err != nil {
		return err
	}
	if identity != repository.Identity {
		return fmt.Errorf("authenticated GitHub repository identity %q differs from %q", identity, repository.Identity)
	}
	return nil
}

func (r *demoRunner) ensureWorktree(ctx context.Context, request factoryv1.RunRequest, repository repositoryConfig) (string, string, error) {
	branch := branchName(request)
	if branch == repository.BaseBranch {
		return "", "", errors.New("deterministic branch unexpectedly equals the protected base branch")
	}
	worktree := filepath.Join(r.stateDir, "worktrees", safeOrderID(request.Order.DocID)+"-"+request.DocumentSHA256[:12])
	base := "refs/remotes/" + r.config.GitRemote + "/" + repository.BaseBranch
	if _, err := r.git(ctx, repository.Root, "fetch", "--no-tags", r.config.GitRemote, "+refs/heads/"+repository.BaseBranch+":"+base); err != nil {
		return "", "", err
	}
	if info, err := os.Stat(worktree); err == nil && info.IsDir() {
		current, branchErr := r.gitOutput(ctx, worktree, "branch", "--show-current")
		if branchErr != nil || current != branch {
			return "", "", errors.New("existing deterministic worktree is on a different branch")
		}
		return worktree, branch, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	if _, err := r.git(ctx, repository.Root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		if _, err := r.git(ctx, repository.Root, "worktree", "add", worktree, branch); err != nil {
			return "", "", err
		}
		return worktree, branch, nil
	}
	remoteHead, remoteErr := r.remoteBranchHead(ctx, repository, branch)
	if remoteErr == nil && remoteHead != "" {
		remoteRef := "refs/remotes/" + r.config.GitRemote + "/" + branch
		if _, err := r.git(ctx, repository.Root, "fetch", "--no-tags", r.config.GitRemote, "+refs/heads/"+branch+":"+remoteRef); err != nil {
			return "", "", err
		}
		if _, err := r.git(ctx, repository.Root, "worktree", "add", "-b", branch, worktree, remoteRef); err != nil {
			return "", "", err
		}
		return worktree, branch, nil
	}
	if remoteErr != nil && !isMissingRef(remoteErr) {
		return "", "", remoteErr
	}
	if _, err := r.git(ctx, repository.Root, "worktree", "add", "-b", branch, worktree, base); err != nil {
		return "", "", err
	}
	return worktree, branch, nil
}

func (r *demoRunner) remoteBranchHead(ctx context.Context, repository repositoryConfig, branch string) (string, error) {
	result, err := r.git(ctx, repository.Root, "ls-remote", "--exit-code", "--heads", r.config.GitRemote, "refs/heads/"+branch)
	if err != nil {
		return "", fmt.Errorf("missing remote ref: %w", err)
	}
	fields := strings.Fields(result.Stdout)
	if len(fields) != 2 || fields[1] != "refs/heads/"+branch {
		return "", errors.New("remote branch query returned conflicting output")
	}
	return fields[0], nil
}

func (r *demoRunner) queryOpenPR(ctx context.Context, repository repositoryConfig, branch string) (pullRequestView, bool, bool, error) {
	if err := r.ensureRepositoryPins(ctx, repository); err != nil {
		return pullRequestView{}, false, false, err
	}
	result, err := r.gh(ctx, repository.Root, "pr", "list", "--repo", repository.Identity, "--head", branch, "--state", "open", "--json", "number,url,headRefOid,headRefName,baseRefName,isDraft,state")
	if err != nil {
		return pullRequestView{}, false, false, err
	}
	var views []pullRequestView
	if err := json.Unmarshal([]byte(result.Stdout), &views); err != nil {
		return pullRequestView{}, false, false, err
	}
	if len(views) == 0 {
		return pullRequestView{}, false, false, nil
	}
	if len(views) != 1 {
		return pullRequestView{}, true, true, nil
	}
	return views[0], true, false, nil
}

func (r *demoRunner) requiredChecks(ctx context.Context, repository repositoryConfig, number int) ([]checkView, bool, error) {
	policyResult, err := r.gh(ctx, repository.Root, "api", "repos/"+repository.Identity+"/branches/"+repository.BaseBranch+"/protection/required_status_checks")
	if err != nil {
		return nil, false, fmt.Errorf("read required-check policy: %w", err)
	}
	var policy struct {
		Contexts []string `json:"contexts"`
	}
	if err := json.Unmarshal([]byte(policyResult.Stdout), &policy); err != nil {
		return nil, false, fmt.Errorf("decode required-check policy: %w", err)
	}
	contexts := uniqueNonEmpty(policy.Contexts)
	if len(contexts) == 0 {
		return nil, false, errors.New("required-check policy is empty")
	}

	rollupResult, err := r.gh(ctx, repository.Root, "pr", "view", strconv.Itoa(number), "--repo", repository.Identity, "--json", "statusCheckRollup")
	if err != nil {
		return nil, false, fmt.Errorf("read exact-head check rollup: %w", err)
	}
	var rollup struct {
		StatusCheckRollup []struct {
			Name       string `json:"name"`
			Context    string `json:"context"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			State      string `json:"state"`
			DetailsURL string `json:"detailsUrl"`
			TargetURL  string `json:"targetUrl"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal([]byte(rollupResult.Stdout), &rollup); err != nil {
		return nil, false, fmt.Errorf("decode exact-head check rollup: %w", err)
	}
	reported := make(map[string]checkView, len(rollup.StatusCheckRollup))
	for _, item := range rollup.StatusCheckRollup {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = strings.TrimSpace(item.Context)
		}
		if name == "" {
			continue
		}
		state := strings.ToUpper(strings.TrimSpace(item.State))
		status := strings.ToUpper(strings.TrimSpace(item.Status))
		conclusion := strings.ToUpper(strings.TrimSpace(item.Conclusion))
		bucket := "pending"
		if state == "SUCCESS" || (status == "COMPLETED" && conclusion == "SUCCESS") {
			bucket = "pass"
		} else if state != "PENDING" && state != "EXPECTED" && status == "COMPLETED" {
			bucket = "fail"
		}
		link := strings.TrimSpace(item.DetailsURL)
		if link == "" {
			link = strings.TrimSpace(item.TargetURL)
		}
		reported[name] = checkView{Name: name, State: valueOr(conclusion, state, status), Bucket: bucket, Link: link}
	}
	checks := make([]checkView, 0, len(contexts))
	passing := true
	for _, contextName := range contexts {
		check, ok := reported[contextName]
		if !ok {
			check = checkView{Name: contextName, State: "EXPECTED", Bucket: "pending"}
		}
		if check.Bucket != "pass" {
			passing = false
		}
		checks = append(checks, check)
	}
	return checks, passing, nil
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func valueOr(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (r *demoRunner) git(ctx context.Context, dir string, args ...string) (commandResult, error) {
	return r.commands.Run(ctx, dir, r.config.GitExecutable, args...)
}

func (r *demoRunner) gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	result, err := r.git(ctx, dir, args...)
	return strings.TrimSpace(result.Stdout), err
}

func (r *demoRunner) gh(ctx context.Context, dir string, args ...string) (commandResult, error) {
	return r.commands.Run(ctx, dir, r.config.GHExecutable, args...)
}

func (r *demoRunner) ghOutput(ctx context.Context, dir string, args ...string) (string, error) {
	result, err := r.gh(ctx, dir, args...)
	return strings.TrimSpace(result.Stdout), err
}

func branchName(request factoryv1.RunRequest) string {
	return "factory-v1/" + safeOrderID(request.Order.DocID) + "-" + request.DocumentSHA256[:12]
}

func evidenceRelativePath(request factoryv1.RunRequest) string {
	return "factory-v1-demo/" + safeOrderID(request.Order.DocID) + ".md"
}

func renderDemoEvidence(request factoryv1.RunRequest) string {
	return "# Factory v1 demonstration evidence: " + oneLine(request.Order.Title) + "\n\n" +
		"- FactoryOrder: `" + request.Order.DocID + "@" + request.Order.Version + "`\n" +
		"- Document SHA-256: `" + request.DocumentSHA256 + "`\n" +
		"- Intake channel: `" + string(request.Order.Channel) + "`\n" +
		"- Target repository: `" + request.Order.TargetRepository + "`\n\n" +
		"This bounded, non-production file is the implementation effect for the Factory v1 acceptance demonstration. Its pull request remains unmerged at Human Review.\n"
}

func allowOnlyPath(status, allowed string) error {
	for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 4 || strings.TrimSpace(line[3:]) != allowed {
			return fmt.Errorf("worktree contains an unrelated status entry %q", line)
		}
	}
	return nil
}

func implementationHead(evidence []factoryv1.Evidence) (string, error) {
	for i := len(evidence) - 1; i >= 0; i-- {
		if gitHashPattern.MatchString(evidence[i].PRHeadSHA) {
			return evidence[i].PRHeadSHA, nil
		}
		if evidence[i].PR != nil && gitHashPattern.MatchString(evidence[i].PR.HeadSHA) {
			return evidence[i].PR.HeadSHA, nil
		}
	}
	return "", errors.New("prior evidence does not contain an implementation head")
}

func reviewedGateHead(evidence []factoryv1.Evidence, kind string) (string, error) {
	for i := len(evidence) - 1; i >= 0; i-- {
		item := evidence[i]
		if item.Kind == kind && item.BlockerCount != nil && *item.BlockerCount == 0 && gitHashPattern.MatchString(item.PRHeadSHA) && item.PRHeadSHA == item.ReviewedHeadSHA {
			return item.PRHeadSHA, nil
		}
	}
	return "", errors.New("prior evidence lacks an exact blocker-free reviewed gate head")
}

func readyHead(evidence []factoryv1.Evidence) (string, error) {
	for i := len(evidence) - 1; i >= 0; i-- {
		item := evidence[i]
		if item.Kind == "ready_pr" && item.PR != nil && item.PR.Open && !item.PR.Draft && item.PR.ChecksPassing && item.PR.HeadSHA == item.PR.ReviewedHeadSHA {
			return item.PR.HeadSHA, nil
		}
	}
	return "", errors.New("prior evidence lacks an exact-head ready PR")
}

func prEvidence(repository repositoryConfig, view pullRequestView, reviewedHead string, checksPassing bool) factoryv1.PREvidence {
	if !checksPassing {
		reviewedHead = ""
	}
	return factoryv1.PREvidence{Repository: repository.Identity, Number: view.Number, URL: view.URL, HeadSHA: view.HeadRefOID, ReviewedHeadSHA: reviewedHead, Open: view.State == "OPEN", Draft: view.IsDraft, ChecksPassing: checksPassing}
}

func isMissingRef(err error) bool {
	return err != nil && strings.Contains(err.Error(), "missing remote ref")
}

func commandErrorText(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}
