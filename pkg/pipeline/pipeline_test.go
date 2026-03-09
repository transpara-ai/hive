package pipeline

import "testing"

func TestContainsAlert(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Everything looks fine", false},
		{"HALT: policy violation detected", true},
		{"ALERT: trust anomaly in builder agent", true},
		{"Found a VIOLATION of soul values", true},
		{"QUARANTINE agent builder_01", true},
		{"Minor alert about formatting", true},
		{"The code is clean", false},
		{"", false},
		{"halt operations immediately", true},
	}

	for _, tt := range tests {
		got := containsAlert(tt.input)
		if got != tt.want {
			t.Errorf("containsAlert(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
