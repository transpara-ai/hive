package loop

import (
	"strings"
	"testing"
)

func TestCheckpointContextFromResponse(t *testing.T) {
	response := `I completed the OpenBrain checkpoint refactor because warm restart needs rationale.

NEXT: Run focused tests and then the hive verifier.

/signal {"signal":"TASK_DONE"}`

	ctx := checkpointContextFromResponse(response)
	if ctx.Intent != "I completed the OpenBrain checkpoint refactor because warm restart needs rationale." {
		t.Fatalf("Intent = %q", ctx.Intent)
	}
	if ctx.Next != "Run focused tests and then the hive verifier." {
		t.Fatalf("Next = %q", ctx.Next)
	}
	if strings.Contains(ctx.Context, "/signal") {
		t.Fatalf("Context should omit signal line: %q", ctx.Context)
	}
	if !strings.Contains(ctx.Context, "warm restart needs rationale") {
		t.Fatalf("Context missing response body: %q", ctx.Context)
	}
}

func TestCheckpointContextFromResponse_Truncates(t *testing.T) {
	response := strings.Repeat("x", 2000)

	ctx := checkpointContextFromResponse(response)
	if len(ctx.Context) != 1600 {
		t.Fatalf("Context length = %d, want 1600", len(ctx.Context))
	}
	if !strings.HasSuffix(ctx.Context, "...") {
		t.Fatalf("Context should end with ellipsis")
	}
	if len(ctx.Intent) != 280 {
		t.Fatalf("Intent length = %d, want 280", len(ctx.Intent))
	}
}
