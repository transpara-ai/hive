package hive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/transpara-ai/hive/pkg/loop"
	"github.com/transpara-ai/work"
)

type issueScanWorkspaceResolution struct {
	RepoPath              string
	ContainmentWatchRoots []string
}

type issueScanImplementationWorkspaceContext struct {
	Kind          string                     `json:"kind"`
	SelectedIssue issueScanBriefIssuePayload `json:"selected_issue"`
	TargetRepos   []string                   `json:"target_repos"`
}

func (r *Runtime) taskWorkspaceProviderFor(def AgentDef) loop.TaskWorkspaceProviderFunc {
	return func(_ context.Context, task work.Task, _ string) (loop.TaskWorkspaceProviderResult, error) {
		if r == nil || !def.CanOperate {
			return loop.TaskWorkspaceProviderResult{}, nil
		}
		targetRepo, ok, err := r.issueScanImplementationTaskTargetRepo(task)
		if err != nil || !ok {
			return loop.TaskWorkspaceProviderResult{}, err
		}
		resolved, err := r.resolveIssueScanWorkspaceForRepo(targetRepo)
		if err != nil {
			return loop.TaskWorkspaceProviderResult{}, err
		}
		return loop.TaskWorkspaceProviderResult{
			Applied:               true,
			RepoPath:              resolved.RepoPath,
			ContainmentWatchRoots: resolved.ContainmentWatchRoots,
		}, nil
	}
}

func (r *Runtime) issueScanImplementationTaskTargetRepo(task work.Task) (string, bool, error) {
	if !isIssueScanImplementationWorkTask(task) {
		return "", false, nil
	}
	if r == nil || r.tasks == nil {
		return "", true, fmt.Errorf("runtime task store is required to resolve issue-scan implementation workspace")
	}
	artifacts, err := r.tasks.ListArtifacts(task.ID)
	if err != nil {
		return "", true, fmt.Errorf("list issue-scan implementation task artifacts: %w", err)
	}
	for i := len(artifacts) - 1; i >= 0; i-- {
		artifact := artifacts[i]
		if strings.TrimSpace(artifact.Label) != IssueScanImplementationTaskContextArtifactLabel {
			continue
		}
		var payload issueScanImplementationWorkspaceContext
		if err := json.Unmarshal([]byte(strings.TrimSpace(artifact.Body)), &payload); err != nil {
			return "", true, fmt.Errorf("decode issue-scan implementation context artifact %s: %w", artifact.ID.Value(), err)
		}
		if strings.TrimSpace(payload.Kind) != issueScanImplementationTaskContextArtifactKind {
			return "", true, fmt.Errorf("issue-scan implementation context artifact %s has kind %q, want %q", artifact.ID.Value(), payload.Kind, issueScanImplementationTaskContextArtifactKind)
		}
		targetRepo := strings.TrimSpace(payload.SelectedIssue.Repo)
		if targetRepo == "" && len(payload.TargetRepos) > 0 {
			targetRepo = strings.TrimSpace(payload.TargetRepos[0])
		}
		targetRepo = strings.ToLower(strings.TrimSpace(targetRepo))
		if !ValidTransparaAIRepo(targetRepo) {
			return "", true, fmt.Errorf("issue-scan implementation task %s targets invalid Transpara-AI repo %q", task.ID.Value(), targetRepo)
		}
		return targetRepo, true, nil
	}
	return "", true, fmt.Errorf("issue-scan implementation task %s has no %q artifact; refusing to infer workspace from task text", task.ID.Value(), IssueScanImplementationTaskContextArtifactLabel)
}

func isIssueScanImplementationWorkTask(task work.Task) bool {
	factoryOrderID := strings.TrimSpace(task.FactoryOrderID)
	if factoryOrderID == "" {
		return false
	}
	return strings.TrimSpace(task.CanonicalTaskID) == issueScanImplementationTaskCanonicalID(factoryOrderID)
}

func (r *Runtime) resolveIssueScanWorkspaceForRepo(targetRepo string) (issueScanWorkspaceResolution, error) {
	targetRepo = strings.ToLower(strings.TrimSpace(targetRepo))
	if !ValidTransparaAIRepo(targetRepo) {
		return issueScanWorkspaceResolution{}, fmt.Errorf("issue-scan target repo %q is not a registered Transpara-AI repo", targetRepo)
	}
	repoName := strings.TrimPrefix(targetRepo, "transpara-ai/")
	if repoName == "" || repoName == targetRepo {
		return issueScanWorkspaceResolution{}, fmt.Errorf("issue-scan target repo %q has no checkout name", targetRepo)
	}

	candidates := r.issueScanWorkspaceCandidates(repoName)
	var (
		repoPathMismatch string
		invalid          []string
	)
	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			invalid = append(invalid, fmt.Sprintf("%q: %v", path, err))
			continue
		}
		slug, ok, err := transparaAIRepoSlugForPath(path)
		if err != nil {
			invalid = append(invalid, fmt.Sprintf("%q: %v", path, err))
			continue
		}
		if !ok {
			invalid = append(invalid, fmt.Sprintf("%q: remote origin is not a Transpara-AI repo", path))
			continue
		}
		if strings.EqualFold(slug, targetRepo) {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return issueScanWorkspaceResolution{}, fmt.Errorf("resolve issue-scan workspace %q: %w", path, err)
			}
			return issueScanWorkspaceResolution{
				RepoPath:              absPath,
				ContainmentWatchRoots: r.issueScanWorkspaceContainmentRoots(absPath),
			}, nil
		}
		if strings.TrimSpace(r.repoPath) != "" && cleanSamePath(path, r.repoPath) {
			repoPathMismatch = slug
		}
	}
	if repoPathMismatch != "" && strings.TrimSpace(r.repoWorkspaceRoot) == "" {
		return issueScanWorkspaceResolution{}, fmt.Errorf("configured repo path %q resolves to %q but issue-scan target repo is %q; refusing wrong-repo implementation task", r.repoPath, repoPathMismatch, targetRepo)
	}
	if len(invalid) > 0 {
		return issueScanWorkspaceResolution{}, fmt.Errorf("could not verify issue-scan workspace for %q: %s", targetRepo, strings.Join(invalid, "; "))
	}
	return issueScanWorkspaceResolution{}, fmt.Errorf("no verified checkout found for issue-scan target repo %q (repo path %q, repo workspace root %q)", targetRepo, strings.TrimSpace(r.repoPath), strings.TrimSpace(r.repoWorkspaceRoot))
}

func (r *Runtime) issueScanWorkspaceCandidates(repoName string) []string {
	var candidates []string
	seen := map[string]bool{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		seen[clean] = true
		candidates = append(candidates, path)
	}
	add(r.repoPath)
	if root := strings.TrimSpace(r.repoWorkspaceRoot); root != "" {
		add(filepath.Join(root, repoName))
	}
	if repoPath := strings.TrimSpace(r.repoPath); repoPath != "" {
		add(filepath.Join(filepath.Dir(repoPath), repoName))
	}
	return candidates
}

func (r *Runtime) issueScanWorkspaceContainmentRoots(repoPath string) []string {
	absRepo, err := filepath.Abs(repoPath)
	if err != nil {
		return []string{filepath.Dir(repoPath)}
	}
	if root := strings.TrimSpace(r.repoWorkspaceRoot); root != "" {
		if absRoot, err := filepath.Abs(root); err == nil && pathWithinRoot(absRepo, absRoot) {
			return []string{absRoot}
		}
	}
	return []string{filepath.Dir(absRepo)}
}

func cleanSamePath(a, b string) bool {
	absA, errA := filepath.Abs(strings.TrimSpace(a))
	absB, errB := filepath.Abs(strings.TrimSpace(b))
	if errA != nil || errB != nil {
		return filepath.Clean(strings.TrimSpace(a)) == filepath.Clean(strings.TrimSpace(b))
	}
	return absA == absB
}

func pathWithinRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}
