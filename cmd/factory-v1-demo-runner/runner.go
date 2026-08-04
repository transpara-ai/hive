package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

type demoRunner struct {
	stateDir string
	config   config
	commands commander
}

type effectReceipt struct {
	SchemaVersion      string              `json:"schema_version"`
	AttemptID          string              `json:"attempt_id"`
	RequestFingerprint string              `json:"request_fingerprint"`
	Stage              factoryv1.Stage     `json:"stage"`
	Result             factoryv1.RunResult `json:"result"`
	Details            map[string]string   `json:"details"`
	RecordedAt         string              `json:"recorded_at"`
}

var (
	hex64Pattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	gitHashPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	safePart       = regexp.MustCompile(`[^a-z0-9]+`)
)

func newDemoRunner(stateDir string, cfg config, commands commander) (*demoRunner, error) {
	if commands == nil {
		return nil, errors.New("command runner is required")
	}
	for _, directory := range []string{"receipts", "locks", "designs", "worktrees"} {
		if _, err := ensurePrivateContainedDir(stateDir, filepath.Join(stateDir, directory)); err != nil {
			return nil, err
		}
	}
	return &demoRunner{stateDir: stateDir, config: cfg, commands: commands}, nil
}

func (r *demoRunner) validateRequest(_ context.Context, request factoryv1.RunRequest) error {
	if request.Operation != "execute" && request.Operation != "reconcile" {
		return errors.New("runner operation must be execute or reconcile")
	}
	document, err := factoryv1.Canonicalize(request.Order)
	if err != nil {
		return err
	}
	if document.Markdown != request.OrderMarkdown || document.SHA256 != request.DocumentSHA256 {
		return errors.New("RunRequest does not bind the exact canonical FactoryOrder markdown and hash")
	}
	expectedAttempt, err := factoryv1.AttemptID(request.DocumentSHA256, request.Stage, request.Ordinal)
	if err != nil {
		return err
	}
	if expectedAttempt != request.AttemptID {
		return errors.New("RunRequest attempt identity is not the delimited tlc-v1 identity")
	}
	repository, ok := r.config.Repositories[request.Order.TargetRepository]
	if !ok {
		return fmt.Errorf("target repository %q is not configured", request.Order.TargetRepository)
	}
	requestRoot, err := filepath.Abs(request.RepositoryRoot)
	if err != nil {
		return err
	}
	requestRoot, err = filepath.EvalSymlinks(requestRoot)
	if err != nil {
		return fmt.Errorf("request repository root: %w", err)
	}
	if requestRoot != repository.Root {
		return errors.New("RunRequest repository root does not match the pinned repository root")
	}
	if !request.AuthorityScope.NonProductionOnly || request.AuthorityScope.ActorID == "" || !containsString(request.AuthorityScope.TargetRepositories, repository.Identity) {
		return errors.New("RunRequest authority does not permit this exact non-production repository")
	}
	for _, action := range requiredActions(request.Stage) {
		if !containsString(request.AuthorityScope.AllowedActions, action) {
			return fmt.Errorf("RunRequest authority does not permit required action %q", action)
		}
	}
	if request.BudgetRemaining.Exhausted {
		return errors.New("RunRequest budget is exhausted")
	}
	if request.Provider.ProviderID == "" || request.Provider.Family == "" || request.Provider.ExecutableRealpath == "" || !hex64Pattern.MatchString(request.Provider.ExecutableSHA256) || request.Provider.ModelID == "" || request.Provider.CredentialSourceID == "" {
		return errors.New("RunRequest provider binding is incomplete")
	}
	return nil
}

func requiredActions(stage factoryv1.Stage) []string {
	switch stage {
	case factoryv1.StageCFADA, factoryv1.StageIADA, factoryv1.StageIAR, factoryv1.StageCFAR:
		return []string{"governance.review.record"}
	case factoryv1.StageWriteCode:
		return []string{"repo.branch.create", "repo.commit.create"}
	case factoryv1.StageCreateDraftPR:
		return []string{"repo.pull_request.create"}
	case factoryv1.StageMarkPRReady:
		return []string{"repo.pull_request.mark_ready"}
	default:
		return nil
	}
}

func (r *demoRunner) execute(ctx context.Context, request factoryv1.RunRequest) (factoryv1.RunResult, error) {
	var result factoryv1.RunResult
	err := r.withAttemptLock(request.AttemptID, func() error {
		if receipt, found, err := r.loadReceipt(request); err != nil {
			return err
		} else if found {
			reconciled, reconcileErr := r.reconcileUnlocked(ctx, request)
			if reconcileErr != nil {
				return reconcileErr
			}
			if reconciled.EffectExists || reconciled.Conflict {
				result = reconciled.Result
				return nil
			}
			_ = receipt
		}
		stageResult, details, err := r.executeStage(ctx, request)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(stageResult.Provider, request.Provider) {
			return errors.New("internal runner result changed the configured provider binding")
		}
		if err := r.saveReceipt(request, stageResult, details); err != nil {
			return err
		}
		result = stageResult
		return nil
	})
	return result, err
}

func (r *demoRunner) reconcile(ctx context.Context, request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	var result factoryv1.ReconcileResult
	err := r.withAttemptLock(request.AttemptID, func() error {
		var err error
		result, err = r.reconcileUnlocked(ctx, request)
		return err
	})
	return result, err
}

func (r *demoRunner) executeStage(ctx context.Context, request factoryv1.RunRequest) (factoryv1.RunResult, map[string]string, error) {
	switch request.Stage {
	case factoryv1.StageIngestWork:
		return r.passed(request, factoryv1.Evidence{Kind: "accepted_source", Reference: request.Order.SourceReferences[0].Identity, SHA256: request.Order.SourceReferences[0].SHA256}), nil, nil
	case factoryv1.StageCraftFactoryOrder:
		return r.passed(request, factoryv1.Evidence{Kind: "canonical_factory_order", Reference: "sha256:" + request.DocumentSHA256, SHA256: request.DocumentSHA256}), nil, nil
	case factoryv1.StageDesign:
		return r.executeDesign(request)
	case factoryv1.StageIADA:
		return r.executeIADA(ctx, request)
	case factoryv1.StageCFADA:
		return r.executeCrossFamilyGate(request, "cfada")
	case factoryv1.StageHumanDesignReview:
		return r.executeHumanDesignReview(request)
	case factoryv1.StageWriteCode:
		return r.executeWriteCode(ctx, request)
	case factoryv1.StageCreateDraftPR:
		return r.executeCreateDraftPR(ctx, request)
	case factoryv1.StageIAR:
		return r.executeIAR(ctx, request)
	case factoryv1.StageCFAR:
		return r.executeCrossFamilyGate(request, "cfar")
	case factoryv1.StageMarkPRReady:
		return r.executeMarkReady(ctx, request)
	case factoryv1.StageHumanReview:
		return r.executeHumanReview(ctx, request)
	default:
		return factoryv1.RunResult{}, nil, fmt.Errorf("unsupported stage %q", request.Stage)
	}
}

func (r *demoRunner) reconcileUnlocked(ctx context.Context, request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	switch request.Stage {
	case factoryv1.StageDesign:
		return r.reconcileDesign(request)
	case factoryv1.StageIADA:
		return r.reconcileIADA(request)
	case factoryv1.StageCFADA:
		return r.reconcileCrossFamilyGate(request, "cfada")
	case factoryv1.StageWriteCode:
		return r.reconcileWriteCode(ctx, request)
	case factoryv1.StageCreateDraftPR:
		return r.reconcileDraftPR(ctx, request)
	case factoryv1.StageIAR:
		return r.reconcileIAR(ctx, request)
	case factoryv1.StageCFAR:
		return r.reconcileCrossFamilyGate(request, "cfar")
	case factoryv1.StageMarkPRReady:
		return r.reconcileReadyPR(ctx, request)
	case factoryv1.StageHumanReview:
		return r.reconcileHumanReview(ctx, request)
	default:
		receipt, found, err := r.loadReceipt(request)
		if err != nil {
			return factoryv1.ReconcileResult{}, err
		}
		if !found {
			return factoryv1.ReconcileResult{EffectExists: false, Result: r.blocked(request, "receipt_missing", "No exact attempt receipt exists.", "Execute the same reconciled attempt.")}, nil
		}
		return factoryv1.ReconcileResult{EffectExists: true, Result: receipt.Result}, nil
	}
}

func (r *demoRunner) passed(request factoryv1.RunRequest, evidence ...factoryv1.Evidence) factoryv1.RunResult {
	return factoryv1.RunResult{Status: factoryv1.RunnerPassed, Evidence: evidence, Provider: request.Provider}
}

func (r *demoRunner) blocked(request factoryv1.RunRequest, kind, blocker, nextAction string) factoryv1.RunResult {
	return factoryv1.RunResult{
		Status: factoryv1.RunnerBlocked, Provider: request.Provider, Blocker: blocker, NextAction: nextAction,
		Evidence: []factoryv1.Evidence{{Kind: kind, Reference: "runner:" + request.AttemptID}},
	}
}

func (r *demoRunner) withAttemptLock(attemptID string, action func() error) error {
	if !hex64Pattern.MatchString(attemptID) {
		return errors.New("attempt lock requires a valid attempt identity")
	}
	path := filepath.Join(r.stateDir, "locks", attemptID+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN) //nolint:errcheck
	return action()
}

func (r *demoRunner) receiptPath(attemptID string) string {
	return filepath.Join(r.stateDir, "receipts", attemptID+".json")
}

func (r *demoRunner) saveReceipt(request factoryv1.RunRequest, result factoryv1.RunResult, details map[string]string) error {
	receipt := effectReceipt{
		SchemaVersion: runnerSchemaVersion, AttemptID: request.AttemptID, RequestFingerprint: requestFingerprint(request),
		Stage: request.Stage, Result: result, Details: details, RecordedAt: nowUTC().Format(timeLayout),
	}
	return writePrivateJSON(r.receiptPath(request.AttemptID), receipt)
}

func (r *demoRunner) loadReceipt(request factoryv1.RunRequest) (effectReceipt, bool, error) {
	path := r.receiptPath(request.AttemptID)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return effectReceipt{}, false, nil
	}
	if err != nil {
		return effectReceipt{}, false, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return effectReceipt{}, false, errors.New("attempt receipt is not a private regular file")
	}
	var receipt effectReceipt
	if err := readStrictJSON(path, &receipt); err != nil {
		return effectReceipt{}, false, err
	}
	if receipt.SchemaVersion != runnerSchemaVersion || receipt.AttemptID != request.AttemptID || receipt.Stage != request.Stage || receipt.RequestFingerprint != requestFingerprint(request) {
		return effectReceipt{}, false, errors.New("attempt receipt conflicts with the exact request")
	}
	return receipt, true, nil
}

func requestFingerprint(request factoryv1.RunRequest) string {
	request.Operation = ""
	raw, _ := json.Marshal(request)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func writePrivateJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary) // scoped, private atomic-write temporary
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func readStrictJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

const timeLayout = "2006-01-02T15:04:05.000000000Z"

func safeOrderID(value string) string {
	value = strings.ToLower(value)
	value = strings.Trim(safePart.ReplaceAllString(value, "-"), "-")
	if len(value) > 64 {
		value = value[:64]
	}
	if value == "" {
		return "order"
	}
	return value
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func hashFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
