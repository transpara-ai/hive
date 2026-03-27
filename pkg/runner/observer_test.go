package runner

import (
	"strings"
	"testing"
)

func TestBuildPart2Instruction_NoAPIKey(t *testing.T) {
	result := buildPart2Instruction("hive", "")
	if !strings.Contains(result, "Skipped") {
		t.Errorf("expected skip message when apiKey is empty, got: %q", result)
	}
	if strings.Contains(result, "Authorization: Bearer") {
		t.Errorf("should not contain curl auth command when apiKey is empty, got: %q", result)
	}
}

func TestBuildPart2Instruction_WithAPIKey(t *testing.T) {
	result := buildPart2Instruction("hive", "lv_testkey")
	if strings.Contains(result, "Skipped") {
		t.Errorf("should not contain skip message when apiKey is set, got: %q", result)
	}
	if !strings.Contains(result, "lv_testkey") {
		t.Errorf("expected API key in output, got: %q", result)
	}
	if !strings.Contains(result, "hive") {
		t.Errorf("expected space slug in output, got: %q", result)
	}
	if !strings.Contains(result, "Authorization: Bearer") {
		t.Errorf("expected curl auth command when apiKey is set, got: %q", result)
	}
}

func TestBuildOutputInstruction_NoAPIKey(t *testing.T) {
	result := buildOutputInstruction("hive", "")
	if !strings.Contains(result, "TASK_TITLE:") {
		t.Errorf("expected text task format when apiKey is empty, got: %q", result)
	}
	if strings.Contains(result, "Authorization: Bearer") {
		t.Errorf("should not contain curl auth command when apiKey is empty, got: %q", result)
	}
}

func TestBuildOutputInstruction_WithAPIKey(t *testing.T) {
	result := buildOutputInstruction("hive", "lv_testkey")
	if strings.Contains(result, "TASK_TITLE:") {
		t.Errorf("should not contain text task format when apiKey is set, got: %q", result)
	}
	if !strings.Contains(result, "lv_testkey") {
		t.Errorf("expected API key in output, got: %q", result)
	}
	if !strings.Contains(result, "hive") {
		t.Errorf("expected space slug in output, got: %q", result)
	}
	if !strings.Contains(result, "Authorization: Bearer") {
		t.Errorf("expected curl auth command when apiKey is set, got: %q", result)
	}
}
