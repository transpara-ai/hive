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
const hiveLifecycleVerifierInvocation = "factory preflight-hive-unit"

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
				t.Fatalf("%s dialect never invokes the tested verifier %q for hive.service posture confirmation", tt.name, hiveLifecycleVerifierInvocation)
			}
			for _, stale := range []string{
				"tracked as separate work",
				"tracked separately)",
			} {
				if strings.Contains(content, stale) {
					t.Fatalf("%s dialect still carries the stale verifier promise %q — the verifier exists as `hive %s`", tt.name, stale, hiveLifecycleVerifierInvocation)
				}
			}
			if strings.Contains(content, "grep '^LOVYOU_API_KEY='") {
				t.Fatalf("%s dialect re-derives hive.service credential posture with an inline shell probe — that adjudication belongs to `hive %s`", tt.name, hiveLifecycleVerifierInvocation)
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
