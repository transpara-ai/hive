package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestRunbookDialectsNameNoLegacyVariable guards against a legacy LOVYOU_*
// environment-variable name creeping back into either hive-lifecycle skill
// dialect after the rename (FO R1/R3, packet AC-5). Standalone at the design
// base bf3f126 (no hive#283 runbook-consistency test present to extend).
func TestRunbookDialectsNameNoLegacyVariable(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path to locate the repo root")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	dialects := []string{
		filepath.Join(repoRoot, "skills", "hive-lifecycle", "codex", "SKILL.md"),
		filepath.Join(repoRoot, ".claude", "skills", "hive-lifecycle", "SKILL.md"),
	}
	for _, path := range dialects {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read skill dialect %s: %v", path, err)
		}
		if bytes.Contains(data, []byte("LOVYOU_")) {
			t.Errorf("%s still contains a legacy LOVYOU_ variable name", path)
		}
	}
}
