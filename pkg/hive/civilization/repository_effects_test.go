package civilization

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

func TestGitHubEffectsRequiresSeparateAuthorityForEnabledEffects(t *testing.T) {
	config, _ := testGitHubEffectsConfig(t)
	config.PublishEnabled = true
	if _, err := NewGitHubEffects(config); err == nil || !strings.Contains(err.Error(), "publication") {
		t.Fatalf("publication authority error = %v", err)
	}
	config.PublishEnabled = false
	config.AutoMergeEnabled = true
	if _, err := NewGitHubEffects(config); err == nil || !strings.Contains(err.Error(), "auto-merge") {
		t.Fatalf("auto-merge authority error = %v", err)
	}
}

func TestGitHubEffectsRejectsRepositoryRootMappedToAnotherName(t *testing.T) {
	config, _ := testGitHubEffectsConfig(t)
	spec := config.Repositories["transpara-ai/hive"]
	otherRoot := filepath.Join(t.TempDir(), "site")
	if err := os.Rename(spec.Root, otherRoot); err != nil {
		t.Fatal(err)
	}
	spec.Root = otherRoot
	config.Repositories["transpara-ai/hive"] = spec
	if _, err := NewGitHubEffects(config); err == nil || !strings.Contains(err.Error(), "root must resolve") {
		t.Fatalf("repository mapping error = %v", err)
	}
}

func TestGitHubEffectsPrepareUsesExactBaseAndPublishDefaultsOff(t *testing.T) {
	config, logPath := testGitHubEffectsConfig(t)
	effects, err := NewGitHubEffects(config)
	if err != nil {
		t.Fatal(err)
	}
	bound := testBoundRequest(t, "transpara-ai/hive")
	workspace, err := effects.Prepare(context.Background(), "work-aaaaaaaaaaaaaaaaaaaaaaaa", bound)
	if err != nil {
		t.Fatal(err)
	}
	if !gitSHA1Pattern.MatchString(workspace.BaseSHA) || workspace.Branch != "civilization/work-aaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("workspace = %+v", workspace)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root, "change.txt"), []byte("bounded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := effects.CaptureImplementation(context.Background(), "work-aaaaaaaaaaaaaaaaaaaaaaaa", bound, workspace, passingImplementation("change.txt"))
	if err != nil || !sha256Pattern.MatchString(digest) {
		t.Fatalf("implementation digest = %q, err = %v", digest, err)
	}
	matches, err := effects.ImplementationMatches(context.Background(), "work-aaaaaaaaaaaaaaaaaaaaaaaa", bound, workspace, passingImplementation("change.txt"), digest)
	if err != nil || !matches {
		t.Fatalf("implementation match = %t, err = %v", matches, err)
	}
	if err := os.Remove(filepath.Join(workspace.Root, "change.txt")); err != nil {
		t.Fatal(err)
	}
	matches, err = effects.ImplementationMatches(context.Background(), "work-aaaaaaaaaaaaaaaaaaaaaaaa", bound, workspace, passingImplementation("change.txt"), digest)
	if err != nil || matches {
		t.Fatalf("missing implementation match = %t, err = %v", matches, err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root, "change.txt"), []byte("bounded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = effects.Publish(context.Background(), "work-aaaaaaaaaaaaaaaaaaaaaaaa", bound, workspace,
		passingImplementation("change.txt"), digest, passingReview())
	if err == nil || !strings.Contains(err.Error(), "publication is disabled") {
		t.Fatalf("publish error = %v", err)
	}
	if raw, readErr := os.ReadFile(logPath); readErr == nil && len(raw) > 0 {
		t.Fatalf("GitHub CLI was invoked while publication disabled: %s", raw)
	}
}

func TestGitHubEffectsPublishesOnlyIndependentlyVerifiedExactDiff(t *testing.T) {
	config, logPath := testGitHubEffectsConfig(t)
	config.PublishEnabled = true
	config.PublishAuthority = "human:test:publish"
	effects, err := NewGitHubEffects(config)
	if err != nil {
		t.Fatal(err)
	}
	bound := testBoundRequest(t, "transpara-ai/hive")
	workID := "work-bbbbbbbbbbbbbbbbbbbbbbbb"
	workspace, err := effects.Prepare(context.Background(), workID, bound)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_WORKTREE", workspace.Root)
	if err := os.WriteFile(filepath.Join(workspace.Root, "change.txt"), []byte("bounded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := effects.CaptureImplementation(context.Background(), workID, bound, workspace, passingImplementation("change.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := effects.CaptureImplementation(context.Background(), workID, bound, workspace, passingImplementation("change.txt", "/run/secrets/token")); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("unsafe changed-file report error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root, "change.txt"), []byte("tampered after review\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := effects.Publish(context.Background(), workID, bound, workspace, passingImplementation("change.txt"), digest, passingReview()); err == nil || !strings.Contains(err.Error(), "reviewed digest") {
		t.Fatalf("same-file content drift error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace.Root, "change.txt"), []byte("bounded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := effects.Publish(context.Background(), workID, bound, workspace, passingImplementation("wrong.txt"), digest, passingReview()); err == nil || !strings.Contains(err.Error(), "matches") {
		t.Fatalf("mismatched report error = %v", err)
	}
	if raw, readErr := os.ReadFile(logPath); readErr == nil && len(raw) > 0 {
		t.Fatalf("GitHub CLI ran before exact diff validation: %s", raw)
	}

	published, err := effects.Publish(context.Background(), workID, bound, workspace, passingImplementation("change.txt"), digest, passingReview())
	if err != nil {
		t.Fatal(err)
	}
	if published.HeadSHA == "" || published.HeadSHA != published.ReviewedHeadSHA || published.HeadSHA != published.ValidatedHeadSHA || !published.CreatedByCivilization {
		t.Fatalf("published = %+v", published)
	}
	remoteHead := gitTestOutput(t, config.Repositories["transpara-ai/hive"].Root, "ls-remote", "origin", "refs/heads/"+workspace.Branch)
	if !strings.HasPrefix(remoteHead, published.HeadSHA) {
		t.Fatalf("remote head = %q, want %s", remoteHead, published.HeadSHA)
	}
}

func TestGitHubEffectsAutoMergeUsesExactHeadWithoutAdmin(t *testing.T) {
	config, logPath := testGitHubEffectsConfig(t)
	config.AutoMergeEnabled = true
	config.AutoMergeAuthority = "human:test:auto-merge"
	effects, err := NewGitHubEffects(config)
	if err != nil {
		t.Fatal(err)
	}
	head := strings.Repeat("1", 40)
	t.Setenv("FAKE_HEAD", head)
	pr := PullRequest{
		Repository: "transpara-ai/hive", Number: 42, URL: "https://github.com/transpara-ai/hive/pull/42",
		HeadSHA: head, ReviewedHeadSHA: head, ValidatedHeadSHA: head, Open: true, ChecksPassing: true, ChecksState: "passed",
		ChangedFiles: []string{"README.md"}, ChangedFilesComplete: true, CreatedByCivilization: true,
	}
	if err := effects.EnableAutoMerge(context.Background(), pr, head); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	if !strings.Contains(args, "--match-head-commit\n"+head+"\n") || !strings.Contains(args, "--auto\n") || strings.Contains(args, "--admin\n") {
		t.Fatalf("auto-merge args:\n%s", args)
	}
}

func TestGitHubEffectsDistinguishesPendingAndFailedRequiredChecks(t *testing.T) {
	config, _ := testGitHubEffectsConfig(t)
	effects, err := NewGitHubEffects(config)
	if err != nil {
		t.Fatal(err)
	}
	head := strings.Repeat("2", 40)
	t.Setenv("FAKE_HEAD", head)
	input := PullRequest{Repository: "transpara-ai/hive", Number: 42, HeadSHA: head, ReviewedHeadSHA: head, ValidatedHeadSHA: head, CreatedByCivilization: true}

	t.Setenv("FAKE_CHECK_BUCKET", "pending")
	pending, err := effects.ObservePullRequest(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if pending.ChecksState != "pending" || pending.ChecksPassing {
		t.Fatalf("pending = %+v", pending)
	}

	t.Setenv("FAKE_CHECK_BUCKET", "fail")
	failed, err := effects.ObservePullRequest(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if failed.ChecksState != "failed" || failed.ChecksPassing {
		t.Fatalf("failed = %+v", failed)
	}

	t.Setenv("FAKE_CHECK_BUCKET", "pass")
	t.Setenv("FAKE_CHANGED_FILES_COUNT", "2")
	truncated, err := effects.ObservePullRequest(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if truncated.ChangedFilesComplete || pullRequestReady(truncated) {
		t.Fatalf("truncated file observation was accepted: %+v", truncated)
	}
}

func TestVerificationSandboxDropsAmbientCredentials(t *testing.T) {
	config, _ := testGitHubEffectsConfig(t)
	effects, err := NewGitHubEffects(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_TOKEN", "must-not-cross-boundary")
	t.Setenv("GITHUB_TOKEN", "must-not-cross-boundary")
	t.Setenv("OPENAI_API_KEY", "must-not-cross-boundary")
	t.Setenv("SSH_AUTH_SOCK", "/must/not/cross")
	root := config.Repositories["transpara-ai/hive"].Root
	check := `test -z "${GH_TOKEN+x}" && test -z "${GITHUB_TOKEN+x}" && test -z "${OPENAI_API_KEY+x}" && test -z "${SSH_AUTH_SOCK+x}" && test "$CODEX_HOME" = "` + config.VerificationCodexHome + `" && test "$GOPROXY" = off`
	if _, err := effects.verificationRun(context.Background(), root, "/bin/sh", "-c", check); err != nil {
		t.Fatal(err)
	}
}

func testGitHubEffectsConfig(t *testing.T) (GitHubEffectsConfig, string) {
	t.Helper()
	repository := testRepositoryWithOrigin(t)
	gitPath, gitDigest, err := resolvePinnedExecutable("git")
	if err != nil {
		t.Fatal(err)
	}
	ghPath := filepath.Join(t.TempDir(), "gh-fixture")
	logPath := filepath.Join(t.TempDir(), "gh.log")
	script := `#!/bin/sh
set -eu
for arg in "$@"; do printf '%s\n' "$arg" >> "$FAKE_GH_LOG"; done
if [ "$1" = pr ] && [ "$2" = list ]; then
  head=${FAKE_HEAD:-$(git -C "$FAKE_WORKTREE" rev-parse HEAD)}
  printf '[{"number":42,"url":"https://github.com/transpara-ai/hive/pull/42","state":"OPEN","isDraft":false,"headRefOid":"%s","body":"<!-- civilization:v1 work_id=work-bbbbbbbbbbbbbbbbbbbbbbbb -->","changedFiles":1,"files":[{"path":"change.txt"}]}]' "$head"
elif [ "$1" = pr ] && [ "$2" = view ]; then
  head=${FAKE_HEAD:-$(git -C "$FAKE_WORKTREE" rev-parse HEAD)}
  count=${FAKE_CHANGED_FILES_COUNT:-1}
  printf '{"number":42,"url":"https://github.com/transpara-ai/hive/pull/42","state":"OPEN","isDraft":false,"headRefOid":"%s","body":"<!-- civilization:v1 -->","changedFiles":%s,"files":[{"path":"README.md"}]}' "$head" "$count"
elif [ "$1" = pr ] && [ "$2" = checks ]; then
  bucket=${FAKE_CHECK_BUCKET:-pass}
  printf '[{"bucket":"%s","name":"CI","state":"STATUS"}]' "$bucket"
  if [ "$bucket" = pending ]; then exit 8; fi
  if [ "$bucket" = fail ]; then exit 1; fi
elif [ "$1" = pr ] && [ "$2" = merge ]; then
  printf 'queued\n'
else
  exit 41
fi
`
	if err := os.WriteFile(ghPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	_, ghDigest, err := resolvePinnedExecutable(ghPath)
	if err != nil {
		t.Fatal(err)
	}
	sandboxPath := filepath.Join(t.TempDir(), "codex-fixture")
	sandboxScript := `#!/bin/sh
set -eu
test "$1" = sandbox
test "$2" = -P
test "$3" = civilization-verification
test "$4" = -C
root=$5
shift 5
cd "$root"
exec "$@"
`
	if err := os.WriteFile(sandboxPath, []byte(sandboxScript), 0o700); err != nil {
		t.Fatal(err)
	}
	_, sandboxDigest, err := resolvePinnedExecutable(sandboxPath)
	if err != nil {
		t.Fatal(err)
	}
	verificationHome := t.TempDir()
	verificationConfig := filepath.Join(verificationHome, "config.toml")
	if err := os.WriteFile(verificationConfig, []byte("fixture = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	verificationConfigDigest, err := digestFile(verificationConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_GH_LOG", logPath)
	return GitHubEffectsConfig{
		Repositories: map[string]RepositorySpec{
			"transpara-ai/hive": {
				Root: repository, VerificationCommands: []VerificationCommand{{Name: "true", Args: []string{"/bin/true"}}},
			},
		},
		WorktreeRoot: filepath.Join(t.TempDir(), "worktrees"), GitExecutable: gitPath, GitSHA256: gitDigest,
		GitHubExecutable: ghPath, GitHubSHA256: ghDigest, EnvironmentKeys: []string{"PATH", "HOME", "FAKE_GH_LOG", "FAKE_WORKTREE", "FAKE_HEAD", "FAKE_CHECK_BUCKET", "FAKE_CHANGED_FILES_COUNT"},
		VerificationSandboxExecutable: sandboxPath, VerificationSandboxSHA256: sandboxDigest,
		VerificationSandboxProfile: verificationSandboxProfile, VerificationCodexHome: verificationHome,
		VerificationConfigSHA256: verificationConfigDigest, VerificationScratchRoot: filepath.Join(t.TempDir(), "scratch"),
		VerificationModuleCache: t.TempDir(),
		OutputLimitBytes:        64 * 1024, CommitUserName: "Civilization", CommitUserEmail: "civilization@transpara.ai",
	}, logPath
}

func testRepositoryWithOrigin(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "origin.git")
	repository := filepath.Join(t.TempDir(), "hive")
	gitTestRun(t, "", "init", "--bare", "--initial-branch=main", bare)
	gitTestRun(t, "", "init", "--initial-branch=main", repository)
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitTestRun(t, repository, "add", "README.md")
	gitTestRun(t, repository, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "base")
	gitTestRun(t, repository, "remote", "add", "origin", bare)
	gitTestRun(t, repository, "push", "-u", "origin", "main")
	return repository
}

func gitTestRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func gitTestOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func testBoundRequest(t *testing.T, repository string) tlcbridge.BoundRequest {
	t.Helper()
	bound, err := tlcbridge.Bind(tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "human:test", Repository: repository}, []byte(`{
  "schema_version":"tlc-envelope/v1",
  "workflow":{"name":"transpara-tlc","version":"0.1.1"},
  "route":"Routine",
  "brief":{"outcome":"Make the bounded change","scope":["change.txt"],"non_goals":[],"assumptions":[],"constraints":[],"tests":["true"],"next_action":"Implement"},
  "route_owned_data":{"preserved":true}
}`))
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func passingImplementation(files ...string) ProviderResult {
	return ProviderResult{Status: "passed", Summary: "implemented", ChangedFiles: files, Checks: []CheckResult{{Name: "true", Status: "passed", Summary: "passed"}}, NextAction: "review"}
}

func passingReview() ProviderResult {
	return ProviderResult{Status: "passed", Summary: "reviewed", ChangedFiles: []string{}, Checks: []CheckResult{}, Review: &ReviewResult{Status: "passed", Summary: "passed", Findings: []string{}}, NextAction: "publish"}
}
