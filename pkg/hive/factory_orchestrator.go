package hive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

const (
	TransparaAIDraftPRPolicyBundleID       = "transpara-ai-issue-scan-draft-pr-create-only-v0.1"
	TransparaAIDraftPRReceiptArtifactLabel = "transpara_ai_draft_pr_receipt"

	transparaAIDraftPRReceiptKind = "transpara_ai_draft_pr_receipt"
)

// DraftPRArtifact bundles the produced artifact's PR content with its target.
type DraftPRArtifact struct {
	Target         DraftPRTarget
	Title          string
	Body           string
	ChangedFiles   []string
	ActorRole      string
	DeciderActorID string
	DeciderRole    string
}

type TransparaAIDraftPRRun struct {
	WorkTask       work.Task
	Target         DraftPRTarget
	MutationResult work.Epic11DraftPullRequestResult
	Receipt        TransparaAIDraftPRReceipt
}

type TransparaAIDraftPRReceipt struct {
	Kind                   string   `json:"kind"`
	Repository             string   `json:"repository"`
	PRNumber               int      `json:"pr_number"`
	PRURL                  string   `json:"pr_url"`
	BaseRef                string   `json:"base_ref"`
	BaseSHA                string   `json:"base_sha"`
	HeadRef                string   `json:"head_ref"`
	HeadSHA                string   `json:"head_sha"`
	RemoteHeadSHA          string   `json:"remote_head_sha"`
	ChangedFiles           []string `json:"changed_files"`
	Draft                  bool     `json:"draft"`
	State                  string   `json:"state"`
	PolicyBundleID         string   `json:"policy_bundle_id"`
	PolicyBundleHash       string   `json:"policy_bundle_hash"`
	AuthorityNonce         string   `json:"authority_nonce"`
	AuthorityRequestID     string   `json:"authority_request_id,omitempty"`
	AuthorityDecisionRef   string   `json:"authority_decision_ref,omitempty"`
	HumanApprovalRequired  bool     `json:"human_approval_required"`
	NoMergeOrDeployClaim   bool     `json:"no_merge_or_deploy_claim"`
	ReadyForReviewRequired bool     `json:"ready_for_review_required"`
}

func TransparaAIDraftPRPolicyBundleHash() string {
	return sha256HexPrefixed([]byte(strings.Join([]string{
		TransparaAIDraftPRPolicyBundleID,
		"action=pull_request.create",
		"repository_owner=transpara-ai",
		"draft_required=true",
		"head_sha_must_match_approved_scope=true",
		"changed_files_must_be_normalized_and_non_empty=true",
		"no_merge_no_deploy_no_ready_state_claim=true",
	}, "\n")))
}

func CreateTransparaAIDraftPRFromApprovedDecision(ctx context.Context, ts *work.TaskStore, source types.ActorID, conv types.ConversationID, client work.Epic11PullRequestCreator, art DraftPRArtifact, causes ...types.EventID) (TransparaAIDraftPRRun, error) {
	if ts == nil {
		return TransparaAIDraftPRRun{}, errors.New("task store is required")
	}
	if client == nil {
		return TransparaAIDraftPRRun{}, errors.New("pull request creator is required")
	}
	if err := VerifyDraftPRContent(art.Target, art.Title, art.Body); err != nil {
		return TransparaAIDraftPRRun{}, err
	}
	target, err := normalizeTransparaAIDraftPRTarget(art.Target)
	if err != nil {
		return TransparaAIDraftPRRun{}, err
	}
	requestedFiles, err := normalizePRChangedFiles(art.ChangedFiles)
	if err != nil {
		return TransparaAIDraftPRRun{}, fmt.Errorf("changed files: %w", err)
	}
	mutation := transparaAIDraftPRMutation(target, art.Title, art.Body)
	headState, err := client.PreflightHead(ctx, mutation)
	if err != nil {
		return TransparaAIDraftPRRun{}, fmt.Errorf("preflight head: %w", err)
	}
	remoteFiles, err := validateTransparaAIDraftPRRemoteHead(target, headState)
	if err != nil {
		return TransparaAIDraftPRRun{}, fmt.Errorf("validate remote head: %w", err)
	}
	if len(requestedFiles) > 0 && !sameStringSet(requestedFiles, remoteFiles) {
		return TransparaAIDraftPRRun{}, fmt.Errorf("changed files %v do not match remote preflight files %v", requestedFiles, remoteFiles)
	}

	task, err := ts.Create(source, "Create draft PR for "+target.Repository, "Create the approved draft PR for a Transpara-AI issue-scan implementation. This task records draft-PR creation only; ready-for-review, adversarial review, Human approval, merge, and deploy remain separate gates.", causes, conv)
	if err != nil {
		return TransparaAIDraftPRRun{}, fmt.Errorf("create draft PR work task: %w", err)
	}
	taskCauses := compactEventIDs(append(append([]types.EventID(nil), causes...), task.ID))

	result, err := client.CreateDraftPullRequest(ctx, mutation)
	if err != nil {
		return TransparaAIDraftPRRun{}, fmt.Errorf("create draft PR: %w", err)
	}
	if err := validateTransparaAIDraftPRResult(target, result); err != nil {
		return TransparaAIDraftPRRun{}, err
	}

	receipt := transparaAIDraftPRReceipt(target, result, headState.HeadSHA, remoteFiles)
	body, err := transparaAIDraftPRReceiptBody(receipt)
	if err != nil {
		return TransparaAIDraftPRRun{}, err
	}
	if err := ts.AddArtifact(source, task.ID, TransparaAIDraftPRReceiptArtifactLabel, "application/json", body, taskCauses, conv); err != nil {
		return TransparaAIDraftPRRun{}, fmt.Errorf("record draft PR receipt artifact: %w", err)
	}
	if err := ts.Complete(source, task.ID, fmt.Sprintf("Created draft PR %s#%d for %s; ready-for-review and Human approval remain separate gates.", result.Repository, result.Number, result.URL), taskCauses, conv); err != nil {
		return TransparaAIDraftPRRun{}, fmt.Errorf("complete draft PR work task: %w", err)
	}

	return TransparaAIDraftPRRun{
		WorkTask:       task,
		Target:         target,
		MutationResult: result,
		Receipt:        receipt,
	}, nil
}

func normalizeTransparaAIDraftPRTarget(target DraftPRTarget) (DraftPRTarget, error) {
	target.Repository = strings.ToLower(strings.TrimSpace(target.Repository))
	if !ValidTransparaAIRepo(target.Repository) {
		return DraftPRTarget{}, fmt.Errorf("repository %q is not a Transpara-AI repo", target.Repository)
	}
	for field, value := range map[string]string{
		"base_ref":           target.BaseRef,
		"base_sha":           target.BaseSHA,
		"head_ref":           target.HeadRef,
		"head_sha":           target.HeadSHA,
		"title_hash":         target.TitleHash,
		"body_hash":          target.BodyHash,
		"policy_bundle_id":   target.PolicyBundleID,
		"policy_bundle_hash": target.PolicyBundleHash,
		"single_use_nonce":   target.SingleUseNonce,
	} {
		if strings.TrimSpace(value) == "" {
			return DraftPRTarget{}, fmt.Errorf("%s is required", field)
		}
	}
	return target, nil
}

func transparaAIDraftPRMutation(target DraftPRTarget, title, body string) work.Epic11DraftPullRequestMutation {
	return work.Epic11DraftPullRequestMutation{
		Repository:          target.Repository,
		BaseRef:             strings.TrimSpace(target.BaseRef),
		BaseSHA:             strings.TrimSpace(target.BaseSHA),
		HeadRef:             strings.TrimSpace(target.HeadRef),
		HeadSHA:             strings.TrimSpace(target.HeadSHA),
		Title:               title,
		Body:                body,
		TitleHash:           sha256HexPrefixed([]byte(title)),
		BodyHash:            sha256HexPrefixed([]byte(body)),
		Draft:               true,
		MaintainerCanModify: true,
	}
}

func validateTransparaAIDraftPRRemoteHead(target DraftPRTarget, state work.Epic11RemoteHeadState) ([]string, error) {
	if strings.TrimSpace(state.HeadSHA) == "" {
		return nil, errors.New("remote head SHA is empty")
	}
	if strings.TrimSpace(state.HeadSHA) != strings.TrimSpace(target.HeadSHA) {
		return nil, fmt.Errorf("remote head SHA %q does not match approved head SHA %q", state.HeadSHA, target.HeadSHA)
	}
	files, err := normalizePRChangedFiles(state.ChangedFiles)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errors.New("remote head diff is empty")
	}
	return files, nil
}

func validateTransparaAIDraftPRResult(target DraftPRTarget, result work.Epic11DraftPullRequestResult) error {
	if !strings.EqualFold(strings.TrimSpace(result.Repository), target.Repository) {
		return fmt.Errorf("created PR repository %q does not match approved repository %q", result.Repository, target.Repository)
	}
	if result.Number <= 0 {
		return errors.New("created PR number is empty")
	}
	if !strings.Contains(strings.ToLower(strings.TrimSpace(result.URL)), "github.com/"+target.Repository+"/pull/") {
		return fmt.Errorf("created PR URL %q does not match repository %q", result.URL, target.Repository)
	}
	for field, gotWant := range map[string][2]string{
		"base_ref": {result.BaseRef, target.BaseRef},
		"base_sha": {result.BaseSHA, target.BaseSHA},
		"head_ref": {result.HeadRef, target.HeadRef},
		"head_sha": {result.HeadSHA, target.HeadSHA},
	} {
		if strings.TrimSpace(gotWant[0]) != strings.TrimSpace(gotWant[1]) {
			return fmt.Errorf("created PR %s %q does not match approved %q", field, gotWant[0], gotWant[1])
		}
	}
	if !result.Draft {
		return errors.New("created PR is not draft")
	}
	if !strings.EqualFold(strings.TrimSpace(result.State), "open") {
		return fmt.Errorf("created PR state %q is not open", result.State)
	}
	return nil
}

func normalizePRChangedFiles(files []string) ([]string, error) {
	out := make([]string, 0, len(files))
	for _, raw := range files {
		file := strings.TrimSpace(raw)
		if file == "" {
			return nil, errors.New("changed file path is empty")
		}
		if strings.Contains(file, "\\") || strings.HasPrefix(file, "/") {
			return nil, fmt.Errorf("changed file %q must be repository-relative slash path", raw)
		}
		clean := path.Clean(file)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("changed file %q escapes repository root", raw)
		}
		if clean != file {
			return nil, fmt.Errorf("changed file %q must be normalized", raw)
		}
		out = append(out, clean)
	}
	sort.Strings(out)
	return compactStrings(out), nil
}

func sameStringSet(a, b []string) bool {
	a = compactStrings(append([]string(nil), a...))
	b = compactStrings(append([]string(nil), b...))
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func transparaAIDraftPRReceipt(target DraftPRTarget, result work.Epic11DraftPullRequestResult, remoteHead string, changedFiles []string) TransparaAIDraftPRReceipt {
	return TransparaAIDraftPRReceipt{
		Kind:                   transparaAIDraftPRReceiptKind,
		Repository:             target.Repository,
		PRNumber:               result.Number,
		PRURL:                  strings.TrimSpace(result.URL),
		BaseRef:                strings.TrimSpace(target.BaseRef),
		BaseSHA:                strings.TrimSpace(target.BaseSHA),
		HeadRef:                strings.TrimSpace(target.HeadRef),
		HeadSHA:                strings.TrimSpace(target.HeadSHA),
		RemoteHeadSHA:          strings.TrimSpace(remoteHead),
		ChangedFiles:           append([]string(nil), changedFiles...),
		Draft:                  result.Draft,
		State:                  strings.ToLower(strings.TrimSpace(result.State)),
		PolicyBundleID:         strings.TrimSpace(target.PolicyBundleID),
		PolicyBundleHash:       strings.TrimSpace(target.PolicyBundleHash),
		AuthorityNonce:         strings.TrimSpace(target.SingleUseNonce),
		HumanApprovalRequired:  true,
		NoMergeOrDeployClaim:   true,
		ReadyForReviewRequired: true,
	}
}

func transparaAIDraftPRReceiptBody(receipt TransparaAIDraftPRReceipt) (string, error) {
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal draft PR receipt: %w", err)
	}
	return string(encoded), nil
}

// CreateDraftPRFromApprovedDecision builds Epic 11 options from an approved
// draft-PR decision and runs the work creator. hive orchestrates; work performs
// the (real) GitHub mutation through the supplied client.
// causes are the causal event IDs from the current chain head; pass nil when
// no prior causes are needed (the task store will reject an empty chain if the
// underlying store requires at least one cause).
func CreateDraftPRFromApprovedDecision(ctx context.Context, ts *work.TaskStore, source types.ActorID, conv types.ConversationID, client work.Epic11PullRequestCreator, art DraftPRArtifact, causes ...types.EventID) (work.Epic11DocsDraftPRRun, error) {
	dir, err := epic11WorkingDir()
	if err != nil {
		return work.Epic11DocsDraftPRRun{}, err
	}
	opts := work.BuildEpic11DocsDraftPROptions(work.Epic11OptionsInput{
		Source:         source,
		ConversationID: conv,
		Causes:         causes,
		WorkingDir:     dir,
		Client:         client,
		Target: work.Epic11DraftPullRequestTarget{
			Repository:             art.Target.Repository,
			BaseRef:                art.Target.BaseRef,
			BaseSHA:                art.Target.BaseSHA,
			HeadRef:                art.Target.HeadRef,
			HeadSHA:                art.Target.HeadSHA,
			HeadExistsOnOrigin:     true,
			Title:                  art.Title,
			Body:                   art.Body,
			ChangedFiles:           art.ChangedFiles,
			ValidationEvidenceRefs: []string{"make verify"},
			Draft:                  true,
			MaintainerCanModify:    true,
			RollbackInstructions:   "Manual rollback only: human may close the draft PR after a separately authorized mutation.",
		},
		ActorRole:      art.ActorRole,
		DeciderActorID: art.DeciderActorID,
		DeciderRole:    art.DeciderRole,
		SingleUseNonce: art.Target.SingleUseNonce,
	})
	return work.RunEpic11DocsDraftPRLiveMutation(ctx, ts, opts)
}

// epic11WorkingDir returns a fresh per-run writable directory for Epic 11
// evidence files. Returns an error if the OS cannot create the directory.
func epic11WorkingDir() (string, error) {
	dir, err := os.MkdirTemp("", "epic11-*")
	if err != nil {
		return "", err
	}
	return dir, nil
}
