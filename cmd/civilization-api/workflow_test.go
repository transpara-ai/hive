package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalWorkflowRequiresExactBytes(t *testing.T) {
	dir := t.TempDir()
	skill := filepath.Join(dir, "skill.md")
	lock := filepath.Join(dir, "lock.json")
	body := []byte("External workflow fixture\n")
	if err := os.WriteFile(skill, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte(fmt.Sprintf(`{"tag":"v0.1.2","skill_sha256":"%x"}`, sha256.Sum256(body))), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadRoutingContext(lock, skill)
	if err != nil || !strings.HasSuffix(got, string(body)) || !strings.Contains(got, `name=transpara-tlc, version="0.1.2"`) {
		t.Fatalf("context=%q %v", got, err)
	}
	if err := os.WriteFile(skill, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRoutingContext(lock, skill); err == nil {
		t.Fatal("mismatched skill accepted")
	}
}

func TestExternalWorkflowRequiresReleaseIdentity(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "lock.json")
	if err := os.WriteFile(lock, []byte(`{"skill_sha256":"irrelevant"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRoutingContext(lock, filepath.Join(dir, "skill.md")); err == nil || !strings.Contains(err.Error(), "release tag") {
		t.Fatalf("missing release identity: %v", err)
	}
}
