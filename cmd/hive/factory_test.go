package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpara-ai/hive/pkg/hive"
	hiveregistry "github.com/transpara-ai/hive/pkg/registry"
)

// TestFactoryVerbIsRegistered asserts the router knows the "factory" verb and,
// when invoked with no subcommand, returns a usage error that names it.
func TestFactoryVerbIsRegistered(t *testing.T) {
	err := routeAndDispatch([]string{"factory"})
	if err == nil || !strings.Contains(err.Error(), "factory") {
		t.Fatalf("expected factory usage error, got %v", err)
	}
}

// TestFactoryOrderRequiresHuman asserts the --human guard fires BEFORE any side
// effect: the spec path does not exist, so if validation ran in the wrong order
// we would see a file/store/loop error instead of a missing-human error.
func TestFactoryOrderRequiresHuman(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "order", "--spec", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryScanIssuesRequiresHumanBeforeGitHub(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "scan-issues", "--repo", "transpara-ai/hive"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestFactoryScanIssuesRequiresRepoBeforeGitHub(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "scan-issues", "--human", "Michael"})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("expected missing --repo error, got %v", err)
	}
}

func TestResolveIssueScanReposLoadsTransparaAIReposFromRegistry(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "repos.json")
	if err := os.WriteFile(registryPath, []byte(`{
		"repos": [
			{"name": "site", "url": "https://github.com/transpara-ai/site"},
			{"name": "hive", "url": "git@github.com:transpara-ai/hive.git"},
			{"name": "hive-copy", "url": "https://github.com/transpara-ai/hive.git"},
			{"name": "outside", "url": "https://github.com/example/outside"},
			{"name": "work"}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write repos.json: %v", err)
	}

	repos, err := resolveIssueScanRepos(nil, true, registryPath)
	if err != nil {
		t.Fatalf("resolveIssueScanRepos: %v", err)
	}
	got := strings.Join(repos, ",")
	want := "transpara-ai/site,transpara-ai/hive,transpara-ai/work"
	if got != want {
		t.Fatalf("repos = %q, want %q", got, want)
	}
}

func TestResolveIssueScanReposPrefersExplicitReposOverRegistry(t *testing.T) {
	repos, err := resolveIssueScanRepos([]string{"transpara-ai/hive", "transpara-ai/hive"}, true, "/does/not/exist")
	if err != nil {
		t.Fatalf("resolveIssueScanRepos explicit: %v", err)
	}
	if got := strings.Join(repos, ","); got != "transpara-ai/hive" {
		t.Fatalf("repos = %q, want explicit hive only", got)
	}
}

func TestResolveIssueScanReposRequiresRepoOrRegistry(t *testing.T) {
	_, err := resolveIssueScanRepos(nil, false, "")
	if err == nil || !strings.Contains(err.Error(), "--registry") {
		t.Fatalf("expected repo or registry error, got %v", err)
	}
}

func TestIssueScanRepoSlugFromRegistryRepoNormalizesGitHubURL(t *testing.T) {
	tests := map[string]string{
		"https://github.com/transpara-ai/site":           "transpara-ai/site",
		"HTTPS://github.com/transpara-ai/site.git/":      "transpara-ai/site",
		"https://github.com/transpara-ai/site.git/.git":  "transpara-ai/site",
		"https://github.com/transpara-ai/site.git/.git/": "transpara-ai/site",
		"git@github.com:transpara-ai/hive.git":           "transpara-ai/hive",
		"ssh://git@github.com/transpara-ai/work":         "transpara-ai/work",
		"http://github.com/transpara-ai/agent.git":       "transpara-ai/agent",
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got := issueScanRepoSlugFromRegistryRepo(registryRepoForTest(raw))
			if got != want {
				t.Fatalf("slug = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveIssueScanReposRejectsEmptyRegistry(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "repos.json")
	if err := os.WriteFile(registryPath, []byte(`{
		"repos": [
			{"name": "outside", "url": "https://github.com/example/outside"},
			{"name": "subpath", "url": "https://github.com/transpara-ai/hive/tree/main"}
		]
	}`), 0o600); err != nil {
		t.Fatalf("write repos.json: %v", err)
	}

	_, err := resolveIssueScanRepos(nil, true, registryPath)
	if err == nil || !strings.Contains(err.Error(), "no scannable Transpara-AI GitHub repos") {
		t.Fatalf("expected empty registry error, got %v", err)
	}
}

func TestResolveIssueScanReposFailsClosedWhenRegistryMissing(t *testing.T) {
	_, err := resolveIssueScanRepos(nil, true, filepath.Join(t.TempDir(), "missing-repos.json"))
	if err == nil || !strings.Contains(err.Error(), "load issue-scan repo registry") {
		t.Fatalf("expected registry load error, got %v", err)
	}
}

func TestResolveIssueScanReposFailsClosedWhenRegistryMalformed(t *testing.T) {
	dir := t.TempDir()
	registryPath := filepath.Join(dir, "repos.json")
	if err := os.WriteFile(registryPath, []byte(`{"repos": [`), 0o600); err != nil {
		t.Fatalf("write repos.json: %v", err)
	}

	_, err := resolveIssueScanRepos(nil, true, registryPath)
	if err == nil || !strings.Contains(err.Error(), "load issue-scan repo registry") {
		t.Fatalf("expected registry parse error, got %v", err)
	}
}

func TestNormalizeIssueScanReposRejectsOutsideOrg(t *testing.T) {
	_, err := normalizeIssueScanRepos([]string{"example/hive"})
	if err == nil || !strings.Contains(err.Error(), "transpara-ai") {
		t.Fatalf("expected transpara-ai repo rejection, got %v", err)
	}
}

func TestNormalizeIssueScanReposRejectsUnsafeRepoBeforeGitHub(t *testing.T) {
	_, err := normalizeIssueScanRepos([]string{"transpara-ai/../hive"})
	if err == nil || !strings.Contains(err.Error(), "transpara-ai") {
		t.Fatalf("expected unsafe repo rejection, got %v", err)
	}
}

func TestNormalizeIssueScanReposRejectsMultiSegmentSlug(t *testing.T) {
	_, err := normalizeIssueScanRepos([]string{"transpara-ai/hive/tree/main"})
	if err == nil || !strings.Contains(err.Error(), "transpara-ai") {
		t.Fatalf("expected multi-segment repo rejection, got %v", err)
	}
}

func TestSafeIssueScanOperatorIDRejectsWhitespace(t *testing.T) {
	if safeIssueScanOperatorID("operator michael") {
		t.Fatal("operator id with whitespace was accepted")
	}
}

func registryRepoForTest(url string) hiveregistry.Repo {
	return hiveregistry.Repo{URL: url}
}

func TestParseFactoryOrderModelOverrideFlags(t *testing.T) {
	flags := factoryOrderModelOverrideFlags{}
	if err := flags.models.Set("guardian=api-sonnet"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := flags.authModes.Set("guardian=api-key"); err != nil {
		t.Fatalf("set auth-mode: %v", err)
	}
	if err := flags.requiredCapabilities.Set("guardian=reasoning,coding"); err != nil {
		t.Fatalf("set required capability: %v", err)
	}
	if err := flags.maxCosts.Set("guardian=0.25"); err != nil {
		t.Fatalf("set max cost: %v", err)
	}

	overrides, err := parseFactoryOrderModelOverrideFlags(flags)
	if err != nil {
		t.Fatalf("parseFactoryOrderModelOverrideFlags: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want one", overrides)
	}
	override := overrides[0]
	if override.Role != "guardian" || override.Model != "api-sonnet" || override.AuthMode != "api-key" {
		t.Fatalf("override = %+v, want guardian api-sonnet api-key", override)
	}
	if got := strings.Join(override.RequiredCapabilities, ","); got != "reasoning,coding" {
		t.Fatalf("required capabilities = %q, want reasoning,coding", got)
	}
	if override.MaxCostPerCallUSD == nil || *override.MaxCostPerCallUSD != 0.25 {
		t.Fatalf("max cost = %v, want 0.25", override.MaxCostPerCallUSD)
	}
}

func TestParseFactoryOrderModelOverrideFlagsRejectsDuplicateScalar(t *testing.T) {
	flags := factoryOrderModelOverrideFlags{}
	_ = flags.models.Set("guardian=sonnet")
	_ = flags.models.Set("guardian=opus")

	_, err := parseFactoryOrderModelOverrideFlags(flags)
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("expected duplicate scalar error, got %v", err)
	}
}

func TestValidateFactoryOrderModelOverridesPreservesCanOperateGuardrail(t *testing.T) {
	_, err := validateFactoryOrderModelOverrides("", []hive.ModelOverrideRequest{
		{Role: "implementer", Model: "api-sonnet", AuthMode: "api-key"},
	})
	if err == nil || !strings.Contains(err.Error(), "CanOperate") {
		t.Fatalf("expected CanOperate guardrail error, got %v", err)
	}
}

func TestValidateFactoryOrderModelOverridesAcceptsAuthModeOnlyOverride(t *testing.T) {
	overrides, err := validateFactoryOrderModelOverrides("", []hive.ModelOverrideRequest{
		{Role: "guardian", AuthMode: "subscription"},
	})
	if err != nil {
		t.Fatalf("validateFactoryOrderModelOverrides: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want one", overrides)
	}
	override := overrides[0]
	if override.Role != "guardian" || override.RequestedAuthMode != "subscription" || override.AuthMode != "subscription" {
		t.Fatalf("override = %+v, want guardian subscription auth-mode", override)
	}
}

// TestFactoryUnknownSubverb asserts an unrecognized subcommand is reported.
func TestFactoryUnknownSubverb(t *testing.T) {
	err := cmdFactory([]string{"frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got %v", err)
	}
}

// TestFactoryRequestPRRequiresRepo asserts request-pr validates required flags
// before opening a store or touching GitHub. With no --repo the command must
// fail fast with a flag-name error.
func TestFactoryRequestPRRequiresRepo(t *testing.T) {
	err := cmdFactory([]string{"request-pr"})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("expected missing --repo error, got %v", err)
	}
}

// TestFactoryRequestPRRequiresFlags asserts that request-pr validates all
// required flags before any store open or network access. requireFlags lists
// --repo first, so calling request-pr with NO flags must produce an error
// mentioning "repo" — the very first missing required flag.
func TestFactoryRequestPRRequiresFlags(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "request-pr"})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("expected missing-flag error mentioning repo, got %v", err)
	}
}

// TestFactoryCreatePRRequiresRequest asserts create-pr validates --request
// before opening a store or calling GitHub.
func TestFactoryCreatePRRequiresRequest(t *testing.T) {
	err := cmdFactory([]string{"create-pr"})
	if err == nil || !strings.Contains(err.Error(), "request") {
		t.Fatalf("expected missing --request error, got %v", err)
	}
}
