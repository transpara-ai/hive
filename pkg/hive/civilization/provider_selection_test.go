package civilization

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type selectionProbe struct {
	requests []ProviderRequest
	err      error
}

func (p *selectionProbe) Run(_ context.Context, request ProviderRequest) (ProviderResult, error) {
	p.requests = append(p.requests, request)
	return ProviderResult{Status: "passed"}, p.err
}

func TestPeerProviderResolutionAndNoFallback(t *testing.T) {
	codex, claude := &selectionProbe{}, &selectionProbe{}
	router := &ProviderRouter{DefaultProvider: "codex", Hosts: map[string]ProviderHost{"codex": {Provider: codex, DefaultModel: "codex-default"}, "claude": {Provider: claude, DefaultModel: "claude-default"}}}
	for _, tc := range []struct {
		requested     ExecutionSelection
		model, source string
	}{
		{ExecutionSelection{Provider: "claude", Model: "explicit", ReasoningEffort: "high"}, "explicit", "invocation"},
		{ExecutionSelection{Provider: "claude"}, "claude-default", "configured_provider_default"},
		{ExecutionSelection{}, "codex-default", "configured_provider_default"},
	} {
		result, err := router.Run(context.Background(), ProviderRequest{Selection: tc.requested})
		if err != nil {
			t.Fatal(err)
		}
		if result.Execution.Requested != tc.requested || result.Execution.Effective.Model != tc.model || result.Execution.ModelSource != tc.source {
			t.Fatalf("evidence = %+v", result.Execution)
		}
	}
	router.Hosts["claude"] = ProviderHost{Provider: claude}
	result, err := router.Run(context.Background(), ProviderRequest{Selection: ExecutionSelection{Provider: "claude"}})
	if err != nil || result.Execution.Effective.Model != "" || result.Execution.ModelSource != "provider_default" {
		t.Fatalf("omission = %+v %v", result, err)
	}
	claude.err = errors.New("model unavailable for this account")
	before := len(codex.requests)
	_, err = router.Run(context.Background(), ProviderRequest{Selection: ExecutionSelection{Provider: "claude", Model: "unavailable"}})
	if err == nil || !strings.Contains(err.Error(), "unavailable") || len(codex.requests) != before {
		t.Fatal("unavailable model silently substituted")
	}
	for _, selection := range []ExecutionSelection{{Provider: "other"}, {Model: "--model=bad"}, {Model: "two words"}, {ReasoningEffort: "invented"}} {
		if _, err := router.Resolve(selection); err == nil {
			t.Fatalf("accepted %+v", selection)
		}
	}
	delete(router.Hosts, "claude")
	if _, err := router.Resolve(ExecutionSelection{Provider: "claude"}); err == nil {
		t.Fatal("missing host accepted")
	}
}

func TestCodexModelOmissionOverrideAndReceiptBinding(t *testing.T) {
	provider, argsPath := testCodexProvider(t)
	provider.config.Model = ""
	provider.config.ReceiptDirectory = t.TempDir()
	t.Setenv("FAKE_RESULT", `{"status":"passed","summary":"ok","changed_files":[],"checks":[],"next_action":"inspect"}`)
	request := ProviderRequest{Operation: OperationReview, AttemptID: strings.Repeat("a", 64), RepositoryRoot: testRepository(t), Prompt: "Review"}
	if _, err := provider.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(argsPath)
	if strings.Contains(string(raw), "--model") {
		t.Fatal("omitted model was passed")
	}
	request.Selection = ExecutionSelection{Provider: "codex", Model: "another", ReasoningEffort: "high"}
	if _, err := provider.Run(context.Background(), request); err == nil || !strings.Contains(err.Error(), "exact provider request") {
		t.Fatalf("receipt crossed selections: %v", err)
	}
	request.AttemptID = strings.Repeat("b", 64)
	if _, err := provider.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(argsPath)
	if !strings.Contains(string(raw), "--model\nanother\n") || !strings.Contains(string(raw), `model_reasoning_effort="high"`) {
		t.Fatalf("args = %s", raw)
	}
}

func testClaude(t *testing.T) (*ClaudeCLI, string) {
	t.Helper()
	root := t.TempDir()
	executable := filepath.Join(root, "claude-fixture")
	args := filepath.Join(root, "args")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FAKE_ARGS\"\ncat >/dev/null\nprintf '%s' \"$FAKE_RESULT\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(root, "settings.json")
	if err := os.WriteFile(settings, []byte(`{"sandbox":{"enabled":true,"failIfUnavailable":true,"allowUnsandboxedCommands":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	digest, _ := digestFile(executable)
	settingsDigest, _ := digestFile(settings)
	t.Setenv("FAKE_ARGS", args)
	provider, err := NewClaudeCLI(ClaudeCLIConfig{Executable: executable, ExecutableSHA256: digest, SettingsFile: settings, SettingsSHA256: settingsDigest, Timeout: time.Second, OutputLimitBytes: 64000, EnvironmentKeys: []string{"FAKE_ARGS", "FAKE_RESULT"}, ReceiptDirectory: filepath.Join(root, "receipts")})
	if err != nil {
		t.Fatal(err)
	}
	return provider, args
}

func TestClaudePeerCLISelectionIsolationAndReplay(t *testing.T) {
	provider, argsPath := testClaude(t)
	t.Setenv("FAKE_RESULT", `{"is_error":false,"structured_output":{"status":"passed","summary":"review passed","changed_files":[],"checks":[],"next_action":"inspect"},"modelUsage":{"reported-model":{}}}`)
	request := ProviderRequest{Operation: OperationReview, AttemptID: strings.Repeat("c", 64), RepositoryRoot: testRepository(t), Prompt: "Review", Selection: ExecutionSelection{Provider: "claude", ReasoningEffort: "high"}}
	result, err := provider.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Execution.Effective.Model != "reported-model" {
		t.Fatalf("missing observed model: %+v", result.Execution)
	}
	raw, _ := os.ReadFile(argsPath)
	args := string(raw)
	for _, arg := range []string{"--restricted\n", "--permission-mode\ndontAsk\n", "--strict-mcp-config\n", "--effort\nhigh\n", "--tools\nRead,Glob,Grep\n"} {
		if !strings.Contains(args, arg) {
			t.Fatalf("missing %q in %s", arg, args)
		}
	}
	for _, arg := range []string{"--fallback-model", "--advisor", "fable", "bypassPermissions", "--model\n", "Bash"} {
		if strings.Contains(args, arg) {
			t.Fatalf("unexpected %q", arg)
		}
	}
	os.Remove(argsPath)
	if _, err := provider.Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(argsPath); !os.IsNotExist(err) {
		t.Fatal("receipt replay executed provider")
	}
	request.AttemptID = strings.Repeat("d", 64)
	request.Selection.Model = "exact-model"
	t.Setenv("FAKE_RESULT", `{"is_error":true,"result":"model unavailable"}`)
	if _, err := provider.Run(context.Background(), request); err == nil || !strings.Contains(err.Error(), "model unavailable") {
		t.Fatalf("model error=%v", err)
	}
	if err := os.WriteFile(provider.config.SettingsFile, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Run(context.Background(), request); err == nil {
		t.Fatal("changed settings accepted")
	}
}
