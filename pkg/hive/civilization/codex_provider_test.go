package civilization

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return root
}

func testCodexExecutable(t *testing.T) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex-fixture")
	script := `#!/bin/sh
set -eu
out=''
: > "$FAKE_ARGS"
while [ "$#" -gt 0 ]; do
  printf '%s\n' "$1" >> "$FAKE_ARGS"
  if [ "$1" = '--output-last-message' ]; then
    shift
    out=$1
    printf '%s\n' "$1" >> "$FAKE_ARGS"
  fi
  shift
done
[ -n "$out" ]
printf '%s' "$FAKE_RESULT" > "$out"
printf 'fixture complete\n'
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	_, digest, err := resolvePinnedExecutable(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, digest
}

func testCodexRequirements(t *testing.T) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "requirements.toml")
	if err := os.WriteFile(path, []byte("[permissions.filesystem]\ndeny_read = [\"/run/secrets/**\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, err := digestFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, digest
}

func testCodexProvider(t *testing.T) (*CodexCLI, string) {
	t.Helper()
	executable, digest := testCodexExecutable(t)
	requirements, requirementsDigest := testCodexRequirements(t)
	argsPath := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_ARGS", argsPath)
	provider, err := NewCodexCLI(CodexCLIConfig{
		Executable: executable, ExecutableSHA256: digest, Model: "test-codex",
		ManagedRequirementsFile: requirements, ManagedRequirementsSHA256: requirementsDigest,
		Timeout: time.Second, OutputLimitBytes: 64 * 1024,
		EnvironmentKeys: []string{"FAKE_ARGS", "FAKE_RESULT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider, argsPath
}

func TestCodexCLIRouteUsesReadOnlyStructuredExecution(t *testing.T) {
	provider, argsPath := testCodexProvider(t)
	t.Setenv("FAKE_RESULT", `{
  "status":"passed","summary":"routed","tlc_envelope":{"schema_version":"tlc-envelope/v1"},
  "changed_files":[],"checks":[],"next_action":"implement"
}`)
	result, err := provider.Run(context.Background(), ProviderRequest{
		Operation: OperationRoute, RepositoryRoot: testRepository(t), Prompt: "Use TLC to route this request.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "passed" || len(result.TLCEnvelope) == 0 {
		t.Fatalf("result = %+v", result)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	joined := string(args)
	for _, required := range []string{"exec\n", "--ephemeral\n", "--strict-config\n", "--output-schema\n", "--sandbox\nread-only\n"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("args missing %q:\n%s", required, joined)
		}
	}
	if strings.Contains(joined, "dangerously-bypass") || strings.Contains(joined, "--approve-for-me") {
		t.Fatalf("read-only route contained unsafe args:\n%s", joined)
	}
}

func TestCodexCLIImplementUsesWorkspaceSandbox(t *testing.T) {
	provider, argsPath := testCodexProvider(t)
	t.Setenv("FAKE_RESULT", `{
  "status":"passed","summary":"implemented","changed_files":["README.md"],
  "checks":[{"name":"test","status":"passed","summary":"ok"}],"next_action":"review"
}`)
	result, err := provider.Run(context.Background(), ProviderRequest{
		Operation: OperationImplement, RepositoryRoot: testRepository(t), Prompt: "Implement the brief.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "passed" || len(result.ChangedFiles) != 1 {
		t.Fatalf("result = %+v", result)
	}
	args, _ := os.ReadFile(argsPath)
	joined := string(args)
	if !strings.Contains(joined, "--sandbox\nworkspace-write\n") || !strings.Contains(joined, "--approve-for-me\n") {
		t.Fatalf("implement args lack bounded write posture:\n%s", joined)
	}
	if strings.Contains(joined, "dangerously-bypass") {
		t.Fatalf("implement args contained dangerous bypass:\n%s", joined)
	}
}

func TestCodexCLIRejectsExecutableReplacement(t *testing.T) {
	executable, digest := testCodexExecutable(t)
	requirements, requirementsDigest := testCodexRequirements(t)
	provider, err := NewCodexCLI(CodexCLIConfig{
		Executable: executable, ExecutableSHA256: digest, Model: "test-codex",
		ManagedRequirementsFile: requirements, ManagedRequirementsSHA256: requirementsDigest,
		Timeout: time.Second, OutputLimitBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err = provider.Run(context.Background(), ProviderRequest{
		Operation: OperationReview, RepositoryRoot: testRepository(t), Prompt: "Review.",
	})
	if err == nil || !strings.Contains(err.Error(), "changed after provider initialization") {
		t.Fatalf("err = %v", err)
	}
}

func TestCodexCLIRejectsManagedRequirementsReplacement(t *testing.T) {
	executable, digest := testCodexExecutable(t)
	requirements, requirementsDigest := testCodexRequirements(t)
	provider, err := NewCodexCLI(CodexCLIConfig{
		Executable: executable, ExecutableSHA256: digest, Model: "test-codex",
		ManagedRequirementsFile: requirements, ManagedRequirementsSHA256: requirementsDigest,
		Timeout: time.Second, OutputLimitBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(requirements, []byte("[permissions.filesystem]\ndeny_read = []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = provider.Run(context.Background(), ProviderRequest{
		Operation: OperationReview, RepositoryRoot: testRepository(t), Prompt: "Review.",
	})
	if err == nil || !strings.Contains(err.Error(), "managed requirements changed") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeProviderResultFailsClosed(t *testing.T) {
	for _, raw := range []string{
		`{"status":"passed","summary":"ok","changed_files":[],"checks":[],"next_action":"done","unknown":true}`,
		`{"status":"blocked","summary":"no","changed_files":[],"checks":[],"next_action":"repair"}`,
	} {
		if _, err := decodeProviderResult([]byte(raw)); err == nil {
			t.Fatalf("accepted invalid result: %s", raw)
		}
	}
}

func TestCodexCLIReplaysExactAttemptFromDurableReceipt(t *testing.T) {
	executable, digest := testCodexExecutable(t)
	requirements, requirementsDigest := testCodexRequirements(t)
	argsPath := filepath.Join(t.TempDir(), "args")
	receipts := filepath.Join(t.TempDir(), "receipts")
	t.Setenv("FAKE_ARGS", argsPath)
	t.Setenv("FAKE_RESULT", `{"status":"passed","summary":"first","changed_files":["README.md"],"checks":[{"name":"test","status":"passed","summary":"ok"}],"next_action":"review"}`)
	provider, err := NewCodexCLI(CodexCLIConfig{
		Executable: executable, ExecutableSHA256: digest, Model: "test-codex",
		ManagedRequirementsFile: requirements, ManagedRequirementsSHA256: requirementsDigest,
		Timeout: time.Second, OutputLimitBytes: 64 * 1024, ReceiptDirectory: receipts,
		EnvironmentKeys: []string{"FAKE_ARGS", "FAKE_RESULT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := ProviderRequest{
		Operation: OperationImplement, AttemptID: strings.Repeat("a", 64),
		RepositoryRoot: testRepository(t), Prompt: "Implement once.",
	}
	first, err := provider.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(argsPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_RESULT", `{"status":"passed","summary":"second","changed_files":["wrong"],"checks":[],"next_action":"wrong"}`)
	second, err := provider.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Summary != first.Summary || second.Summary != "first" {
		t.Fatalf("receipt replay = %+v", second)
	}
	if _, err := os.Stat(argsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("provider executed despite receipt, stat error = %v", err)
	}
}
