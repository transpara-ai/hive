package main

import (
	"context"
	"crypto/sha1" // Git object identity is SHA-1 in the configured v1 repositories.
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

type reviewerArtifact struct {
	SchemaVersion  string                    `json:"schema_version"`
	Gate           string                    `json:"gate"`
	OrderID        string                    `json:"order_id"`
	DocumentSHA256 string                    `json:"document_sha256"`
	DesignBlobSHA  string                    `json:"design_blob_sha,omitempty"`
	PRHeadSHA      string                    `json:"pr_head_sha,omitempty"`
	BlockerCount   int                       `json:"blocker_count"`
	AuthorFamily   string                    `json:"author_family"`
	ReviewerFamily string                    `json:"reviewer_family"`
	Provider       factoryv1.ProviderBinding `json:"provider"`
	Reference      string                    `json:"reference"`
}

func (r *demoRunner) executeDesign(request factoryv1.RunRequest) (factoryv1.RunResult, map[string]string, error) {
	path := r.designPath(request)
	content := []byte(renderDesign(request))
	if existing, err := os.ReadFile(path); err == nil {
		if !reflect.DeepEqual(existing, content) {
			return r.blocked(request, "design_conflict", "The deterministic private design path contains different bytes.", "Resolve the conflicting design artifact."), map[string]string{"design_path": path}, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return factoryv1.RunResult{}, nil, err
	} else if err := writePrivateFile(path, content); err != nil {
		return factoryv1.RunResult{}, nil, err
	}
	blob := gitBlobSHA(content)
	result := r.passed(request, factoryv1.Evidence{Kind: "design", Reference: path, DesignBlobSHA: blob, SHA256: factoryv1.HashText(string(content))})
	return result, map[string]string{"design_path": path, "design_blob_sha": blob}, nil
}

func (r *demoRunner) reconcileDesign(request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	path := r.designPath(request)
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return factoryv1.ReconcileResult{EffectExists: false, Result: r.blocked(request, "design_missing", "The exact private design artifact does not exist.", "Execute the same attempt to create it.")}, nil
	}
	if err != nil {
		return factoryv1.ReconcileResult{}, err
	}
	if string(content) != renderDesign(request) {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "design_conflict", "The private design bytes conflict with the deterministic design.", "Resolve the design conflict.")}, nil
	}
	result := r.passed(request, factoryv1.Evidence{Kind: "design", Reference: path, DesignBlobSHA: gitBlobSHA(content), SHA256: factoryv1.HashText(string(content))})
	return factoryv1.ReconcileResult{EffectExists: true, Result: result}, nil
}

func (r *demoRunner) executeIADA(ctx context.Context, request factoryv1.RunRequest) (factoryv1.RunResult, map[string]string, error) {
	design, err := r.requireDesign(request)
	if err != nil {
		return r.blocked(request, "design_missing", err.Error(), "Produce the exact deterministic design artifact first."), nil, nil
	}
	validation, err := r.runRepositoryValidation(ctx, request, false)
	if err != nil {
		return r.blocked(request, "iada_validation_failed", err.Error(), "Repair the bounded design or repository validation failure."), map[string]string{"design_blob_sha": design}, nil
	}
	zero := 0
	evidence := factoryv1.Evidence{
		Kind: "gate", Reference: "runner-receipt:" + request.AttemptID, DesignBlobSHA: design,
		BlockerCount: &zero, AuthorFamily: r.config.AuthorFamily, ReviewerFamily: r.config.AuthorFamily,
		SHA256: validation,
	}
	return r.passed(request, evidence), map[string]string{"design_blob_sha": design, "validation_sha256": validation}, nil
}

func (r *demoRunner) reconcileIADA(request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	receipt, found, err := r.loadReceipt(request)
	if err != nil {
		return factoryv1.ReconcileResult{}, err
	}
	if !found {
		return factoryv1.ReconcileResult{EffectExists: false, Result: r.blocked(request, "iada_receipt_missing", "No exact IADA attempt receipt exists.", "Execute the same reconciled attempt.")}, nil
	}
	if receipt.Result.Status != factoryv1.RunnerPassed {
		return factoryv1.ReconcileResult{EffectExists: true, Result: receipt.Result}, nil
	}
	design, err := r.requireDesign(request)
	if err != nil {
		return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "iada_design_conflict", err.Error(), "Restore the exact IADA-reviewed design bytes.")}, nil
	}
	for _, evidence := range receipt.Result.Evidence {
		if evidence.Kind == "gate" && evidence.DesignBlobSHA == design && evidence.BlockerCount != nil && *evidence.BlockerCount == 0 {
			return factoryv1.ReconcileResult{EffectExists: true, Result: receipt.Result}, nil
		}
	}
	return factoryv1.ReconcileResult{Conflict: true, Result: r.blocked(request, "iada_receipt_conflict", "The IADA receipt does not bind the current exact design.", "Repair the exact design/receipt conflict.")}, nil
}

func (r *demoRunner) executeCrossFamilyGate(request factoryv1.RunRequest, gate string) (factoryv1.RunResult, map[string]string, error) {
	artifact, path, err := r.loadReviewerArtifact(request, gate)
	if errors.Is(err, os.ErrNotExist) {
		result := r.blocked(request, "reviewer_artifact_missing", "The pre-created exact cross-family reviewer artifact is unavailable.", "Create the independent reviewer artifact at the reported private path, then resolve this intervention.")
		result.Evidence[0].Reference = path
		return result, map[string]string{"reviewer_artifact_path": path}, nil
	}
	if err != nil {
		result := r.blocked(request, "reviewer_artifact_invalid", err.Error(), "Replace the artifact with an exact independent blocker-free review.")
		result.Evidence[0].Reference = path
		return result, map[string]string{"reviewer_artifact_path": path}, nil
	}
	zero := 0
	evidence := factoryv1.Evidence{
		Kind: "cross_family_gate", Reference: artifact.Reference, DesignBlobSHA: artifact.DesignBlobSHA,
		PRHeadSHA: artifact.PRHeadSHA, ReviewedHeadSHA: artifact.PRHeadSHA, BlockerCount: &zero,
		AuthorFamily: artifact.AuthorFamily, ReviewerFamily: artifact.ReviewerFamily, Provider: &artifact.Provider,
	}
	hash, _ := hashFile(path)
	evidence.SHA256 = hash
	return r.passed(request, evidence), map[string]string{"reviewer_artifact_path": path, "reviewer_artifact_sha256": hash}, nil
}

func (r *demoRunner) reconcileCrossFamilyGate(request factoryv1.RunRequest, gate string) (factoryv1.ReconcileResult, error) {
	result, _, err := r.executeCrossFamilyGate(request, gate)
	if err != nil {
		return factoryv1.ReconcileResult{}, err
	}
	if result.Status == factoryv1.RunnerPassed {
		return factoryv1.ReconcileResult{EffectExists: true, Result: result}, nil
	}
	return factoryv1.ReconcileResult{EffectExists: false, Result: result}, nil
}

func (r *demoRunner) executeHumanDesignReview(request factoryv1.RunRequest) (factoryv1.RunResult, map[string]string, error) {
	key := approvalKey(request.Order.DocID, request.Order.Version, request.DocumentSHA256)
	receipt, ok := r.config.StandingApprovals[key]
	if !ok {
		return r.blocked(request, "standing_approval_missing", "No configured standing approval binds this exact order tuple.", "Obtain and configure a fresh scoped Human approval."), nil, nil
	}
	document, _ := factoryv1.Canonicalize(request.Order)
	if err := factoryv1.ValidateApprovalReceipt(document, receipt); err != nil {
		return r.blocked(request, "standing_approval_invalid", err.Error(), "Correct the configured exact Human approval receipt."), nil, nil
	}
	result := r.passed(request, factoryv1.Evidence{Kind: "human_approval", Reference: receipt.ApprovalSourceEventID, Approval: &receipt})
	return result, map[string]string{"approval_key": key}, nil
}

func (r *demoRunner) loadReviewerArtifact(request factoryv1.RunRequest, gate string) (reviewerArtifact, string, error) {
	path := r.reviewerArtifactPath(request, gate)
	info, err := os.Lstat(path)
	if err != nil {
		return reviewerArtifact{}, path, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return reviewerArtifact{}, path, errors.New("reviewer artifact must be a private mode-0600 regular file")
	}
	var artifact reviewerArtifact
	if err := readStrictJSON(path, &artifact); err != nil {
		return reviewerArtifact{}, path, err
	}
	if artifact.SchemaVersion != runnerSchemaVersion || artifact.Gate != gate || artifact.OrderID != request.Order.DocID || artifact.DocumentSHA256 != request.DocumentSHA256 || artifact.BlockerCount != 0 || artifact.AuthorFamily != r.config.AuthorFamily || artifact.ReviewerFamily == "" || artifact.ReviewerFamily == artifact.AuthorFamily || artifact.ReviewerFamily != request.Provider.Family || !reflect.DeepEqual(artifact.Provider, request.Provider) || artifact.Reference == "" {
		return reviewerArtifact{}, path, errors.New("reviewer artifact metadata, independence, provider, or blocker count does not match the exact request")
	}
	switch gate {
	case "cfada":
		design, designErr := r.requireDesign(request)
		if designErr != nil || artifact.DesignBlobSHA != design {
			return reviewerArtifact{}, path, errors.New("CFADA artifact does not bind the exact deterministic design blob")
		}
	case "cfar":
		head, headErr := exactReviewedHead(request.PriorEvidence)
		if headErr != nil || artifact.PRHeadSHA != head {
			return reviewerArtifact{}, path, errors.New("CFAR artifact does not bind the exact current PR head")
		}
	default:
		return reviewerArtifact{}, path, errors.New("unsupported cross-family gate")
	}
	return artifact, path, nil
}

func (r *demoRunner) designPath(request factoryv1.RunRequest) string {
	return filepath.Join(r.stateDir, "designs", safeOrderID(request.Order.DocID)+"-"+request.DocumentSHA256[:12]+".md")
}

func (r *demoRunner) reviewerArtifactPath(request factoryv1.RunRequest, gate string) string {
	return filepath.Join(r.config.ReviewerEvidenceDir, safeOrderID(request.Order.DocID)+"-"+request.DocumentSHA256[:12]+"-"+gate+".json")
}

func (r *demoRunner) requireDesign(request factoryv1.RunRequest) (string, error) {
	content, err := os.ReadFile(r.designPath(request))
	if err != nil {
		return "", err
	}
	if string(content) != renderDesign(request) {
		return "", errors.New("deterministic design content conflicts")
	}
	return gitBlobSHA(content), nil
}

func renderDesign(request factoryv1.RunRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Factory v1 demonstration design: %s\n\n", oneLine(request.Order.Title))
	fmt.Fprintf(&b, "- Order: `%s@%s`\n", request.Order.DocID, request.Order.Version)
	fmt.Fprintf(&b, "- Document SHA-256: `%s`\n", request.DocumentSHA256)
	fmt.Fprintf(&b, "- Target repository: `%s`\n", request.Order.TargetRepository)
	fmt.Fprintf(&b, "- Bounded output: `docs/factory-v1-demo/%s.md`\n\n", safeOrderID(request.Order.DocID))
	b.WriteString("## Requirements covered\n\n")
	for _, requirement := range request.Order.Requirements {
		fmt.Fprintf(&b, "- `%s`: %s\n", oneLine(requirement.ID), oneLine(requirement.Statement))
	}
	b.WriteString("\n## Acceptance covered\n\n")
	for _, criterion := range request.Order.AcceptanceCriteria {
		fmt.Fprintf(&b, "- `%s`: %s — verify with %s\n", oneLine(criterion.ID), oneLine(criterion.Statement), oneLine(criterion.VerificationMethod))
	}
	b.WriteString("\n## Decision\n\nCreate one deterministic evidence markdown file on a non-default branch, validate it with the configured named tests, and stop at an exact-head ready pull request for Human Review. No merge, deployment, default-branch write, or unrelated file change is authorized.\n")
	return b.String()
}

func gitBlobSHA(content []byte) string {
	header := []byte(fmt.Sprintf("blob %d\x00", len(content)))
	sum := sha1.Sum(append(header, content...))
	return hex.EncodeToString(sum[:])
}

func writePrivateFile(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func exactReviewedHead(evidence []factoryv1.Evidence) (string, error) {
	for i := len(evidence) - 1; i >= 0; i-- {
		item := evidence[i]
		if gitHashPattern.MatchString(item.PRHeadSHA) && item.PRHeadSHA == item.ReviewedHeadSHA {
			return item.PRHeadSHA, nil
		}
		if item.PR != nil && gitHashPattern.MatchString(item.PR.HeadSHA) {
			return item.PR.HeadSHA, nil
		}
	}
	return "", errors.New("prior evidence has no exact implementation or reviewed PR head")
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// GitHub-mutating stages are implemented in git_stages.go.
