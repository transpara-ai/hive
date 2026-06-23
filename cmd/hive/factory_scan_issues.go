package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/hive/pkg/registry"
)

func cmdFactoryScanIssues(args []string) error {
	fs := flag.NewFlagSet("factory scan-issues", flag.ContinueOnError)
	human := fs.String("human", "", "Operator name (required)")
	operatorID := fs.String("operator-id", "", "Operator id for queued run metadata (default: derived from --human)")
	storeDSN := fs.String("store", "", "Store DSN (postgres://... or empty for in-memory)")
	repoPath := fs.String("repo-path", "", "Path to repo for dispatch Operate context (default: current dir)")
	limit := fs.Int("limit", 10, "Maximum open issues to read per repo")
	maxIterations := fs.Int("max-iterations", 30, "Queued run iteration budget")
	maxCostUSD := fs.Float64("max-cost-usd", 25, "Queued run cost budget in USD")
	dispatch := fs.Bool("dispatch", false, "Immediately dispatch queued run into a FactoryOrder task")
	useRegistry := fs.Bool("registry", false, "Scan every Transpara-AI GitHub repo in repos.json when --repo is omitted")
	repos := repeatedStringFlag{}
	labels := repeatedStringFlag{}
	fs.Var(&repos, "repo", "Transpara-AI repo slug to scan, e.g. transpara-ai/hive (repeatable; required unless --registry is set)")
	fs.Var(&labels, "label", "GitHub issue label filter passed to gh issue list (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *human == "" {
		return fmt.Errorf("--human is required")
	}
	if *limit <= 0 {
		return fmt.Errorf("--limit must be greater than zero")
	}
	if *maxIterations <= 0 {
		return fmt.Errorf("--max-iterations must be greater than zero")
	}
	if *maxCostUSD < 0 {
		return fmt.Errorf("--max-cost-usd must be zero or greater")
	}
	registryPath := ""
	if len(repos) == 0 && *useRegistry {
		var err error
		registryPath, err = issueScanRegistryPath()
		if err != nil {
			return err
		}
	}
	normalizedRepos, err := resolveIssueScanRepos(repos, *useRegistry, registryPath)
	if err != nil {
		return err
	}
	opID := strings.TrimSpace(*operatorID)
	if opID == "" {
		opID = hive.IssueScanOperatorID(*human)
	} else if !safeIssueScanOperatorID(opID) {
		return fmt.Errorf("--operator-id is unsafe")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	issues, err := scanGitHubIssues(ctx, normalizedRepos, *limit, labels)
	if err != nil {
		return err
	}
	if len(issues) == 0 {
		return fmt.Errorf("no open GitHub issues found for %s", strings.Join(normalizedRepos, ", "))
	}

	var (
		rt *hive.Runtime
		fc *factoryContext
	)
	if *dispatch {
		rt, fc, err = openFactoryRuntime(ctx, *storeDSN, *human, *repoPath)
	} else {
		fc, err = openFactoryContext(ctx, *storeDSN, *human)
	}
	if err != nil {
		return err
	}
	defer fc.close()

	conv := factoryOrderConversation(hive.IssueScanIntakeID(issues[0]))
	queued, err := hive.QueueIssueScanRunLaunch(fc.store, fc.factory, fc.signer, fc.humanID, conv, hive.IssueScanRunLaunchRequest{
		OperatorID: opID,
		Issues:     issues,
		Authority: hive.RunLaunchAuthority{
			InitialLevel: event.AuthorityLevelRequired,
			Scope:        "transpara-ai issue scan to ready-for-Human PR; no merge or deploy",
			PolicyRef:    hive.IssueScanDefaultPolicyRef,
			Rationale:    "Civilization scanned Transpara-AI GitHub issues and selected one for governed factory execution.",
		},
		Budget: hive.RunLaunchBudget{MaxIterations: *maxIterations, MaxCostUSD: *maxCostUSD},
	}, nil)
	if err != nil {
		return fmt.Errorf("queue issue-scan run launch: %w", err)
	}

	fmt.Printf("queued issue-scan run %s for %s#%d (first event %s)\n", queued.RunID, queued.Selected.Repo, queued.Selected.Number, queued.FirstEventID)
	if *dispatch {
		result, err := rt.DispatchQueuedRunLaunch(queued.RunID)
		if err != nil {
			return fmt.Errorf("dispatch queued run launches: %w", err)
		}
		fmt.Printf("dispatch scanned=%d dispatched=%d already_dispatched=%d failed=%d\n", result.Scanned, result.Dispatched, result.AlreadyDispatched, result.Failed)
		if len(result.DispatchedOrderIDs) > 0 {
			fmt.Printf("factory order(s): %s\n", strings.Join(result.DispatchedOrderIDs, ", "))
		}
	}
	return nil
}

func resolveIssueScanRepos(values []string, useRegistry bool, registryPath string) ([]string, error) {
	if len(values) > 0 {
		return normalizeIssueScanRepos(values)
	}
	if !useRegistry {
		return nil, fmt.Errorf("--repo is required unless --registry is set")
	}
	return issueScanReposFromRegistry(registryPath)
}

func issueScanRegistryPath() (string, error) {
	hiveDir := findHiveDir()
	if _, err := os.Stat(filepath.Join(hiveDir, "agents")); err != nil {
		return "", fmt.Errorf("locate hive repo for --registry: agents directory not found from %s", hiveDir)
	}
	goMod, err := os.ReadFile(filepath.Join(hiveDir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("locate hive repo for --registry: read go.mod: %w", err)
	}
	if !issueScanGoModDeclaresHiveModule(goMod) {
		return "", fmt.Errorf("locate hive repo for --registry: %s is not the Hive repo root", hiveDir)
	}
	return filepath.Join(hiveDir, "repos.json"), nil
}

func issueScanGoModDeclaresHiveModule(goMod []byte) bool {
	for _, line := range strings.Split(string(goMod), "\n") {
		if strings.TrimSpace(line) == "module github.com/transpara-ai/hive" {
			return true
		}
	}
	return false
}

func issueScanReposFromRegistry(registryPath string) ([]string, error) {
	registryPath = strings.TrimSpace(registryPath)
	if registryPath == "" {
		return nil, fmt.Errorf("repos.json path is required")
	}
	reg, err := registry.Load(registryPath)
	if err != nil {
		return nil, fmt.Errorf("load issue-scan repo registry: %w", err)
	}
	out := make([]string, 0, len(reg.Repos))
	seen := map[string]struct{}{}
	for _, repo := range reg.Repos {
		slug := issueScanRepoSlugFromRegistryRepo(repo)
		if !hive.ValidTransparaAIRepo(slug) {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		out = append(out, slug)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("repos.json contains no scannable Transpara-AI GitHub repos")
	}
	return out, nil
}

func issueScanRepoSlugFromRegistryRepo(repo registry.Repo) string {
	raw := strings.TrimSpace(repo.URL)
	if raw == "" && strings.TrimSpace(repo.Name) != "" {
		raw = "transpara-ai/" + strings.TrimSpace(repo.Name)
	}
	raw = strings.ToLower(raw)
	raw = strings.TrimRight(raw, "/")
	raw = strings.TrimSuffix(raw, ".git")
	raw = strings.TrimPrefix(raw, "ssh://git@")
	raw = strings.TrimPrefix(raw, "git@")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "github.com:")
	raw = strings.TrimPrefix(raw, "github.com/")
	raw = strings.Trim(raw, "/")
	raw = strings.TrimSuffix(raw, ".git")
	return raw
}

func normalizeIssueScanRepos(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for i, raw := range values {
		repo := strings.ToLower(strings.TrimSpace(raw))
		if !hive.ValidTransparaAIRepo(repo) {
			return nil, fmt.Errorf("--repo[%d] must be a transpara-ai owner/repo slug", i)
		}
		if _, ok := seen[repo]; ok {
			continue
		}
		seen[repo] = struct{}{}
		out = append(out, repo)
	}
	return out, nil
}

func safeIssueScanOperatorID(value string) bool {
	if value == "" || len(value) > 128 || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func scanGitHubIssues(ctx context.Context, repos []string, limit int, labels []string) ([]hive.GitHubIssueCandidate, error) {
	var out []hive.GitHubIssueCandidate
	for _, repo := range repos {
		issues, err := scanGitHubRepoIssues(ctx, repo, limit, labels)
		if err != nil {
			return nil, err
		}
		out = append(out, issues...)
	}
	return out, nil
}

func scanGitHubRepoIssues(ctx context.Context, repo string, limit int, labels []string) ([]hive.GitHubIssueCandidate, error) {
	args := []string{"issue", "list", "--repo", repo, "--state", "open", "--limit", fmt.Sprintf("%d", limit), "--json", "number,title,url,body,labels"}
	for _, label := range labels {
		if trimmed := strings.TrimSpace(label); trimmed != "" {
			args = append(args, "--label", trimmed)
		}
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh issue list %s: %v: %s", repo, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh issue list %s: %w", repo, err)
	}
	var raw []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		URL    string `json:"url"`
		Body   string `json:"body"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("decode gh issue list %s: %w", repo, err)
	}
	issues := make([]hive.GitHubIssueCandidate, 0, len(raw))
	for _, issue := range raw {
		labels := make([]string, 0, len(issue.Labels))
		for _, label := range issue.Labels {
			labels = append(labels, label.Name)
		}
		issues = append(issues, hive.GitHubIssueCandidate{
			Repo:   repo,
			Number: issue.Number,
			Title:  issue.Title,
			URL:    issue.URL,
			Body:   issue.Body,
			Labels: labels,
		})
	}
	return issues, nil
}
