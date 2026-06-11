package checkpoint

import (
	"testing"
	"time"
)

// TestRecoverAll_WarmStart_ReviewerChainStateAttached pins the v12-F1 review
// finding B1: reviewer chain state must attach in EVERY recovery mode, not
// only cold starts. The live chain fold deliberately skips the reviewer's own
// reviews (they are recorded at emission), so a warm-started reviewer without
// seeded counts forgets its verdict cap — settled work pends again and a
// capped task can be re-reviewed or re-reopened past the limit.
func TestRecoverAll_WarmStart_ReviewerChainStateAttached(t *testing.T) {
	stub := NewStubThoughtStore()
	captureCheckpointFor(t, stub, RoleReviewer, 7, "review pending completions", "")

	result, err := RecoverAll([]string{RoleReviewer}, stub, nil, 2*time.Hour)
	if err != nil {
		t.Fatalf("RecoverAll: %v", err)
	}

	rs, ok := result[RoleReviewer]
	if !ok {
		t.Fatal("reviewer not in result map")
	}
	if rs.Mode != ModeWarm {
		t.Fatalf("Mode: got %v, want warm (the cold path always attached state)", rs.Mode)
	}
	if rs.ReviewerState == nil {
		t.Fatal("warm-started reviewer must still receive chain-replayed reviewer state — without it the verdict cap resets on restart")
	}
}
