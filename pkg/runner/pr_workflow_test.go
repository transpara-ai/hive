package runner

import (
	"strings"
	"testing"
	"time"
)

// TestFixTitleDedup asserts that fixTitle never produces a "Fix: Fix: …" prefix.
func TestFixTitleDedup(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Fix: something", "Fix: something"},    // already prefixed — no double
		{"something", "Fix: something"},         // plain subject gets prefix
		{"Add feature", "Fix: Add feature"},     // plain subject gets prefix
		{"Fix: Fix: nested", "Fix: Fix: nested"}, // starts with Fix: — returned unchanged
	}
	for _, tt := range tests {
		got := fixTitle(tt.input)
		if got != tt.want {
			t.Errorf("fixTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestBranchSlug asserts format, special-char stripping, and 40-char truncation.
func TestBranchSlug(t *testing.T) {
	date := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	t.Run("basic format", func(t *testing.T) {
		got := branchSlug("Add OAuth integration", date)
		want := "feat/20260327-add-oauth-integration"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("special chars stripped", func(t *testing.T) {
		got := branchSlug("Fix: something (v2)", date)
		want := "feat/20260327-fix-something-v2"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("slug truncated at 40 chars", func(t *testing.T) {
		long := "This is a very long task title that should definitely be truncated"
		got := branchSlug(long, date)
		prefix := "feat/20260327-"
		if !strings.HasPrefix(got, prefix) {
			t.Fatalf("expected prefix %q, got %q", prefix, got)
		}
		slug := got[len(prefix):]
		if len(slug) > 40 {
			t.Errorf("slug portion %q exceeds 40 chars (len=%d)", slug, len(slug))
		}
	})
}

// TestPRTitleFromSubject asserts that the [hive:builder] prefix is stripped.
func TestPRTitleFromSubject(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"[hive:builder] Add OAuth integration", "Add OAuth integration"},
		{"[hive:builder]  Fix: something  ", "Fix: something"},
		{"no prefix here", "no prefix here"},
	}
	for _, tt := range tests {
		got := prTitleFromSubject(tt.input)
		if got != tt.want {
			t.Errorf("prTitleFromSubject(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestPRModeToggle asserts that PRMode=false skips branch creation
// (buildBranchName returns "", so no git checkout -b is needed).
func TestPRModeToggle(t *testing.T) {
	t.Run("PRMode=false skips branch", func(t *testing.T) {
		cfg := Config{PRMode: false}
		got := buildBranchName(cfg, "Add feature")
		if got != "" {
			t.Errorf("expected empty branch name when PRMode=false, got %q", got)
		}
	})

	t.Run("PRMode=true returns branch name", func(t *testing.T) {
		cfg := Config{PRMode: true}
		got := buildBranchName(cfg, "Add feature")
		if got == "" {
			t.Error("expected non-empty branch name when PRMode=true")
		}
		if !strings.HasPrefix(got, "feat/") {
			t.Errorf("branch name %q should start with feat/", got)
		}
	})
}
