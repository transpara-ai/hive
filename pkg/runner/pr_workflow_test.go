package runner

import (
	"strings"
	"testing"
	"time"
)

// TestFixTitleDedup asserts that fixTitle collapses all retry-prefix layers
// (both [hive:*] and "Fix: ") before prepending a single "Fix: ". Without full
// normalization, compounded inputs produce "Fix: Fix: Fix: …" across cycles
// and branch slugs degrade to "fix-hive-builder-fix-hive-builder-…".
func TestFixTitleDedup(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Fix: something", "Fix: something"},
		{"something", "Fix: something"},
		{"Add feature", "Fix: Add feature"},
		{"Fix: Fix: nested", "Fix: nested"},
		{"[hive:builder] Add feature", "Fix: Add feature"},
		{"[hive:builder] Fix: X", "Fix: X"},
		{"[hive:builder] Fix: [hive:builder] Fix: X", "Fix: X"},
		{"[hive:critic] [hive:builder] Fix: Fix: X", "Fix: X"},
	}
	for _, tt := range tests {
		got := fixTitle(tt.input)
		if got != tt.want {
			t.Errorf("fixTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestStripRetryPrefixes isolates the normalization helper.
func TestStripRetryPrefixes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"X", "X"},
		{"Fix: X", "X"},
		{"Fix: Fix: X", "X"},
		{"[hive:builder] X", "X"},
		{"[hive:builder] Fix: X", "X"},
		{"[hive:builder] Fix: [hive:builder] Fix: X", "X"},
		{"[hive:critic] [hive:builder] Fix: Fix: X", "X"},
		{"[hive:unterminated X", "[hive:unterminated X"},
	}
	for _, tt := range tests {
		got := stripRetryPrefixes(tt.input)
		if got != tt.want {
			t.Errorf("stripRetryPrefixes(%q) = %q, want %q", tt.input, got, tt.want)
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
		// Leading "Fix: " is a retry-cycle prefix and is collapsed by
		// normalization — see retry_prefixes_stripped_before_slug.
		got := branchSlug("Fix: something (v2)", date)
		want := "feat/20260327-something-v2"
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

	// Compounded retry prefixes must collapse before sluggification — otherwise
	// the tail (the meaningful part of the title) gets lost to 40-char
	// truncation and all retries produce near-identical branch names.
	t.Run("retry prefixes stripped before slug", func(t *testing.T) {
		compounded := "[hive:builder] Fix: [hive:builder] Fix: add validation test for landing page"
		got := branchSlug(compounded, date)
		want := "feat/20260327-add-validation-test-for-landing-page"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("single hive prefix stripped", func(t *testing.T) {
		got := branchSlug("[hive:builder] Add OAuth integration", date)
		want := "feat/20260327-add-oauth-integration"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
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
		{"[hive:builder] [hive:builder] Add KindQuestion", "Add KindQuestion"},
		{"[hive:critic] [hive:builder] Fix: compounded prefix", "Fix: compounded prefix"},
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
