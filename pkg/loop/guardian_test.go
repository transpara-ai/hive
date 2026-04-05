package loop

import (
	"testing"
)

// --- parseApproveCommand ---

func TestParseApproveCommand_Valid(t *testing.T) {
	response := `The proposed role passes all governance checks. Soul is present, rights preserved.
/approve {"name":"code-reviewer","reason":"Soul present, rights preserved, BOUNDED and OBSERVABLE, specific watch patterns"}
/signal {"signal": "IDLE"}`

	cmd := parseApproveCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil ApproveCommand")
	}
	if cmd.Name != "code-reviewer" {
		t.Errorf("Name = %q, want %q", cmd.Name, "code-reviewer")
	}
	if cmd.Reason != "Soul present, rights preserved, BOUNDED and OBSERVABLE, specific watch patterns" {
		t.Errorf("Reason = %q, unexpected value", cmd.Reason)
	}
}

func TestParseApproveCommand_NoCommand(t *testing.T) {
	response := `No spawn proposal to evaluate. Standing by.
/signal {"signal": "IDLE"}`

	cmd := parseApproveCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

func TestParseApproveCommand_MalformedJSON(t *testing.T) {
	response := `/approve {bad json here`

	cmd := parseApproveCommand(response)
	if cmd != nil {
		t.Errorf("expected nil for malformed JSON, got %+v", cmd)
	}
}

// --- parseRejectCommand ---

func TestParseRejectCommand_Valid(t *testing.T) {
	response := `The proposed role lacks soul alignment and uses a bare wildcard watch pattern.
/reject {"name":"data-scraper","reason":"Soul statement missing from prompt; watch_patterns contains bare wildcard"}
/signal {"signal": "IDLE"}`

	cmd := parseRejectCommand(response)
	if cmd == nil {
		t.Fatal("expected non-nil RejectCommand")
	}
	if cmd.Name != "data-scraper" {
		t.Errorf("Name = %q, want %q", cmd.Name, "data-scraper")
	}
	if cmd.Reason != "Soul statement missing from prompt; watch_patterns contains bare wildcard" {
		t.Errorf("Reason = %q, unexpected value", cmd.Reason)
	}
}

func TestParseRejectCommand_NoCommand(t *testing.T) {
	response := `The proposal looks fine. Approving.
/approve {"name":"code-reviewer","reason":"All checks pass"}
/signal {"signal": "IDLE"}`

	cmd := parseRejectCommand(response)
	if cmd != nil {
		t.Errorf("expected nil, got %+v", cmd)
	}
}

func TestParseRejectCommand_MalformedJSON(t *testing.T) {
	response := `/reject {bad json here`

	cmd := parseRejectCommand(response)
	if cmd != nil {
		t.Errorf("expected nil for malformed JSON, got %+v", cmd)
	}
}
