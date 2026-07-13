package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The hive-lifecycle runbooks delegate hive.service posture adjudication to
// the tested verifier subcommand instead of inline shell forensics
// (FO-HIVE-267-PREFLIGHT-RUNBOOK-WIRING; verifier delivered in PR #277).
// The assertions bind to the load-bearing lines inside the runbook's bash
// fence — prose mentions of the subcommand name alone must never satisfy
// them, or deleting the fence would go undetected. The fail-closed line is
// the classification every command-level failure (missing checkout, build
// failure, verifier UNKNOWN) must reach without aborting a strict-shell
// caller.
const (
	hiveLifecycleVerifierInvocation = "cd /Transpara/transpara-ai/repos/hive && go run ./cmd/hive factory preflight-hive-unit"
	hiveLifecycleVerifierFailClosed = "treat as UNKNOWN, fail closed; if local-only was intended, STOP the unit"
)

func hiveLifecycleDialectRunbooks() []struct{ name, path string } {
	return []struct{ name, path string }{
		{name: "claude", path: filepath.Join("..", "..", ".claude", "skills", "hive-lifecycle", "SKILL.md")},
		{name: "codex", path: filepath.Join("..", "..", "skills", "hive-lifecycle", "codex", "SKILL.md")},
	}
}

func TestHiveLifecycleRunbooksInvokeTestedUnitPreflight(t *testing.T) {
	for _, tt := range hiveLifecycleDialectRunbooks() {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatalf("read %s dialect runbook: %v", tt.name, err)
			}
			content := string(raw)
			if !strings.Contains(content, hiveLifecycleVerifierInvocation) {
				t.Fatalf("%s dialect is missing the executable verifier invocation %q for hive.service posture confirmation (prose mentions do not count)", tt.name, hiveLifecycleVerifierInvocation)
			}
			if !strings.Contains(content, hiveLifecycleVerifierFailClosed) {
				t.Fatalf("%s dialect is missing the fail-closed classification %q for command-level verifier failures", tt.name, hiveLifecycleVerifierFailClosed)
			}
			for _, stale := range []string{
				"tracked as separate work",
				"tracked separately)",
			} {
				if strings.Contains(content, stale) {
					t.Fatalf("%s dialect still carries the stale verifier promise %q — the verifier exists as `hive factory preflight-hive-unit`", tt.name, stale)
				}
			}
			for _, inlineProbe := range []string{
				"grep '^TRANSPARA_API_KEY='",
				"grep '^LOVYOU_API_KEY='",
			} {
				if strings.Contains(content, inlineProbe) {
					t.Fatalf("%s dialect re-derives hive.service credential posture with inline shell probe %q — that adjudication belongs to `hive factory preflight-hive-unit`", tt.name, inlineProbe)
				}
			}
		})
	}
}

func TestHiveLifecycleClaudeDialectHomeResolvesToPhysicalFile(t *testing.T) {
	home, err := filepath.EvalSymlinks(filepath.Join("..", "..", "skills", "hive-lifecycle", "claude", "SKILL.md"))
	if err != nil {
		t.Fatalf("resolve claude dialect home: %v", err)
	}
	physical, err := filepath.EvalSymlinks(filepath.Join("..", "..", ".claude", "skills", "hive-lifecycle", "SKILL.md"))
	if err != nil {
		t.Fatalf("resolve claude dialect physical file: %v", err)
	}
	if home != physical {
		t.Fatalf("claude dialect home resolves to %q, want the physical file %q (one-physical-copy invariant, FO-HIVE-265 R2)", home, physical)
	}
}
