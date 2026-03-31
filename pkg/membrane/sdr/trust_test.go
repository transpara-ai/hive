package sdr

import "testing"

func TestTrustEvidenceClassifications(t *testing.T) {
	tests := []struct {
		name      string
		fn        func(string) TrustEvidence
		wantType  string
		wantDir   EvidenceDirection
		wantWeight EvidenceWeight
	}{
		{
			name:      "EmailDelivered is positive normal",
			fn:        EmailDelivered,
			wantType:  "email.delivered",
			wantDir:   EvidencePositive,
			wantWeight: WeightNormal,
		},
		{
			name:      "ProspectReplied is positive normal",
			fn:        ProspectReplied,
			wantType:  "prospect.replied",
			wantDir:   EvidencePositive,
			wantWeight: WeightNormal,
		},
		{
			name:      "HandoffAccepted is positive normal",
			fn:        HandoffAccepted,
			wantType:  "handoff.accepted",
			wantDir:   EvidencePositive,
			wantWeight: WeightNormal,
		},
		{
			name:      "DraftApprovedClean is positive normal",
			fn:        DraftApprovedClean,
			wantType:  "draft.approved_clean",
			wantDir:   EvidencePositive,
			wantWeight: WeightNormal,
		},
		{
			name:      "DraftRejected is negative normal",
			fn:        DraftRejected,
			wantType:  "draft.rejected",
			wantDir:   EvidenceNegative,
			wantWeight: WeightNormal,
		},
		{
			name:      "DraftEdited is negative mild",
			fn:        DraftEdited,
			wantType:  "draft.edited",
			wantDir:   EvidenceNegative,
			wantWeight: WeightMild,
		},
		{
			name:      "ProspectUnsubscribed is negative strong",
			fn:        ProspectUnsubscribed,
			wantType:  "prospect.unsubscribed",
			wantDir:   EvidenceNegative,
			wantWeight: WeightStrong,
		},
		{
			name:      "DeadLetterTask is negative normal",
			fn:        DeadLetterTask,
			wantType:  "task.dead_letter",
			wantDir:   EvidenceNegative,
			wantWeight: WeightNormal,
		},
		{
			name:      "ServiceUnreachable is negative normal",
			fn:        ServiceUnreachable,
			wantType:  "service.unreachable",
			wantDir:   EvidenceNegative,
			wantWeight: WeightNormal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := tt.fn("sdr")

			if ev.EvidenceType != tt.wantType {
				t.Errorf("EvidenceType = %q, want %q", ev.EvidenceType, tt.wantType)
			}
			if ev.Direction != tt.wantDir {
				t.Errorf("Direction = %v, want %v", ev.Direction, tt.wantDir)
			}
			if ev.Weight != tt.wantWeight {
				t.Errorf("Weight = %v, want %v", ev.Weight, tt.wantWeight)
			}
			if ev.AgentName != "sdr" {
				t.Errorf("AgentName = %q, want %q", ev.AgentName, "sdr")
			}
			if ev.Description == "" {
				t.Error("Description should not be empty")
			}
			if ev.Timestamp.IsZero() {
				t.Error("Timestamp should not be zero")
			}
		})
	}
}

func TestEvidenceDirectionString(t *testing.T) {
	tests := []struct {
		dir  EvidenceDirection
		want string
	}{
		{EvidencePositive, "positive"},
		{EvidenceNegative, "negative"},
		{EvidenceDirection(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.dir.String(); got != tt.want {
			t.Errorf("EvidenceDirection(%d).String() = %q, want %q", tt.dir, got, tt.want)
		}
	}
}

func TestEvidenceWeightString(t *testing.T) {
	tests := []struct {
		w    EvidenceWeight
		want string
	}{
		{WeightNormal, "normal"},
		{WeightMild, "mild"},
		{WeightStrong, "strong"},
		{EvidenceWeight(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.w.String(); got != tt.want {
			t.Errorf("EvidenceWeight(%d).String() = %q, want %q", tt.w, got, tt.want)
		}
	}
}
