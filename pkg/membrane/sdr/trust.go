package sdr

import "time"

// EvidenceDirection indicates whether evidence moves trust up or down.
type EvidenceDirection int

const (
	// EvidencePositive increases trust score.
	EvidencePositive EvidenceDirection = iota
	// EvidenceNegative decreases trust score.
	EvidenceNegative
)

func (d EvidenceDirection) String() string {
	switch d {
	case EvidencePositive:
		return "positive"
	case EvidenceNegative:
		return "negative"
	default:
		return "unknown"
	}
}

// EvidenceWeight controls how strongly evidence affects the trust score.
type EvidenceWeight int

const (
	WeightNormal EvidenceWeight = iota
	WeightMild
	WeightStrong
)

func (w EvidenceWeight) String() string {
	switch w {
	case WeightNormal:
		return "normal"
	case WeightMild:
		return "mild"
	case WeightStrong:
		return "strong"
	default:
		return "unknown"
	}
}

// TrustEvidence wraps an evidence event with its expected direction and weight.
type TrustEvidence struct {
	EvidenceType string            `json:"evidence_type"`
	Direction    EvidenceDirection  `json:"direction"`
	Weight       EvidenceWeight    `json:"weight"`
	AgentName    string            `json:"agent_name"`
	Description  string            `json:"description"`
	Timestamp    time.Time         `json:"timestamp"`
}

// SDR-specific evidence classification functions.
// Each returns a TrustEvidence with the appropriate direction and weight.

// EmailDelivered returns positive evidence: an email was successfully delivered.
func EmailDelivered(agentName string) TrustEvidence {
	return TrustEvidence{
		EvidenceType: "email.delivered",
		Direction:    EvidencePositive,
		Weight:       WeightNormal,
		AgentName:    agentName,
		Description:  "email successfully delivered to prospect",
		Timestamp:    time.Now(),
	}
}

// ProspectReplied returns positive evidence: a prospect replied to outreach.
func ProspectReplied(agentName string) TrustEvidence {
	return TrustEvidence{
		EvidenceType: "prospect.replied",
		Direction:    EvidencePositive,
		Weight:       WeightNormal,
		AgentName:    agentName,
		Description:  "prospect replied to outreach email",
		Timestamp:    time.Now(),
	}
}

// HandoffAccepted returns positive evidence: a qualified lead handoff was accepted.
func HandoffAccepted(agentName string) TrustEvidence {
	return TrustEvidence{
		EvidenceType: "handoff.accepted",
		Direction:    EvidencePositive,
		Weight:       WeightNormal,
		AgentName:    agentName,
		Description:  "qualified lead handoff accepted by sales",
		Timestamp:    time.Now(),
	}
}

// DraftApprovedClean returns positive evidence: a draft was approved without edits.
func DraftApprovedClean(agentName string) TrustEvidence {
	return TrustEvidence{
		EvidenceType: "draft.approved_clean",
		Direction:    EvidencePositive,
		Weight:       WeightNormal,
		AgentName:    agentName,
		Description:  "draft approved without edits",
		Timestamp:    time.Now(),
	}
}

// DraftRejected returns negative evidence: a draft was rejected by a human.
func DraftRejected(agentName string) TrustEvidence {
	return TrustEvidence{
		EvidenceType: "draft.rejected",
		Direction:    EvidenceNegative,
		Weight:       WeightNormal,
		AgentName:    agentName,
		Description:  "draft rejected by human reviewer",
		Timestamp:    time.Now(),
	}
}

// DraftEdited returns mild negative evidence: a human had to edit the draft before approving.
func DraftEdited(agentName string) TrustEvidence {
	return TrustEvidence{
		EvidenceType: "draft.edited",
		Direction:    EvidenceNegative,
		Weight:       WeightMild,
		AgentName:    agentName,
		Description:  "draft required human edits before approval",
		Timestamp:    time.Now(),
	}
}

// ProspectUnsubscribed returns strong negative evidence: a prospect unsubscribed.
func ProspectUnsubscribed(agentName string) TrustEvidence {
	return TrustEvidence{
		EvidenceType: "prospect.unsubscribed",
		Direction:    EvidenceNegative,
		Weight:       WeightStrong,
		AgentName:    agentName,
		Description:  "prospect unsubscribed from outreach",
		Timestamp:    time.Now(),
	}
}

// DeadLetterTask returns negative evidence: a task could not be processed.
func DeadLetterTask(agentName string) TrustEvidence {
	return TrustEvidence{
		EvidenceType: "task.dead_letter",
		Direction:    EvidenceNegative,
		Weight:       WeightNormal,
		AgentName:    agentName,
		Description:  "task moved to dead letter queue",
		Timestamp:    time.Now(),
	}
}

// ServiceUnreachable returns negative evidence: the wrapped service could not be reached.
func ServiceUnreachable(agentName string) TrustEvidence {
	return TrustEvidence{
		EvidenceType: "service.unreachable",
		Direction:    EvidenceNegative,
		Weight:       WeightNormal,
		AgentName:    agentName,
		Description:  "wrapped service unreachable",
		Timestamp:    time.Now(),
	}
}
