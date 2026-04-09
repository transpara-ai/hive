package loop

import "testing"

func TestIsHex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"abc123", true},
		{"ABC123", true},
		{"0123456789abcdef", true},
		{"xyz", false},
		{"abc12g", false},
		{"", true}, // empty is vacuously hex (length check happens elsewhere)
	}
	for _, tt := range tests {
		if got := isHex(tt.input); got != tt.want {
			t.Errorf("isHex(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExtractCommitHash_NoRepo(t *testing.T) {
	// Without a valid repo, no hash should be found (git rev-parse fails).
	got := extractCommitHash("Implemented feature in commit abc1234", "/nonexistent")
	if got != "" {
		t.Errorf("extractCommitHash with bad repo returned %q, want empty", got)
	}
}

func TestExtractCommitHash_RealRepo(t *testing.T) {
	// Use this repo itself — HEAD should be a valid commit.
	head := gitCommand(".", "rev-parse", "HEAD")
	if head == "" {
		t.Skip("not in a git repo")
	}

	short := head[:7]

	// Should find the hash in a summary string.
	got := extractCommitHash("Fixed bug in commit "+short, ".")
	if got == "" {
		t.Fatalf("extractCommitHash(%q) returned empty, expected full hash", short)
	}
	if got != head {
		t.Errorf("extractCommitHash returned %q, want %q", got, head)
	}

	// Should not match non-hex words.
	got = extractCommitHash("Updated the handler module", ".")
	if got != "" {
		t.Errorf("extractCommitHash with no hash returned %q, want empty", got)
	}

	// Should strip trailing punctuation.
	got = extractCommitHash("See commit "+short+".", ".")
	if got == "" {
		t.Fatal("extractCommitHash with trailing period returned empty")
	}
}
