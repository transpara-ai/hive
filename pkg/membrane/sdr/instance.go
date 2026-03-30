package sdr

import (
	"time"

	"github.com/lovyou-ai/hive/pkg/membrane"
)

// NewSDRConfig returns the SDR membrane instance configuration.
// The system prompt and detailed mappings are populated during Phase 2b (manual deep dive).
func NewSDRConfig(serviceEndpoint string) membrane.MembraneConfig {
	return membrane.MembraneConfig{
		Name:            "sdr",
		Role:            "membrane",
		Model:           "claude-sonnet-4-6",
		SystemPrompt:    sdrSystemPrompt,
		ServiceEndpoint: serviceEndpoint,
		PollInterval:    30 * time.Second,
		AuthMethod:      "bearer",

		InboundMappings: []membrane.InboundMapping{
			{ServiceEvent: "lead.state.ready_for_outreach", GraphEvent: "work.task.created", TransformID: "lead_to_task"},
			{ServiceEvent: "lead.state.qualified_handoff", GraphEvent: "agent.escalated", TransformID: "lead_to_handoff"},
			{ServiceEvent: "interaction.sent", GraphEvent: "agent.acted", TransformID: "passthrough"},
			{ServiceEvent: "interaction.received", GraphEvent: "agent.communicated", TransformID: "passthrough"},
			{ServiceEvent: "score.updated", GraphEvent: "agent.evaluated", TransformID: "passthrough"},
		},

		OutboundMappings: []membrane.OutboundMapping{
			{GraphEvent: "bridge.action.approved", ServiceMethod: "POST", ServicePath: "/copilot/drafts/{id}/approve"},
			{GraphEvent: "bridge.action.rejected", ServiceMethod: "POST", ServicePath: "/copilot/drafts/{id}/reject"},
		},

		TrustThresholds: membrane.TrustBands{
			RequiredBelow:    0.3,
			RecommendedBelow: 0.6,
		},

		WatchPatterns: []string{"work.task.*", "bridge.action.*"},
		MaxIterations: 0, // unlimited (long-running service)
		MaxDuration:   0, // unlimited
		GuardianHints: []string{"MARGIN", "CONSENT", "TRANSPARENT", "BOUNDED"},
	}
}

// sdrSystemPrompt is a skeleton — populated in Phase 2b with Transpara domain knowledge.
const sdrSystemPrompt = `== SOUL ==
Take care of your human, humanity, and yourself.

== ROLE: SDR MEMBRANE AGENT ==
You wrap the AI-SDR service, translating its actions into EventGraph events
and subjecting outbound actions to trust-based authority gating.

== RESPONSIBILITIES ==
1. Poll the AI-SDR API for lead state changes and interactions
2. Translate service events into EventGraph events
3. Gate outbound actions (emails, handoffs) through authority levels
4. Escalate to humans when judgment is needed
5. Earn trust through verified outcomes

== TRANSPARENCY ==
All your actions are recorded, signed, and auditable on the EventGraph.
Humans can see your trust score and decision history.

== NOTE ==
This is a skeleton prompt. Full Transpara domain knowledge (ICP criteria,
product knowledge, scoring rubric, escalation targets) will be injected
during Phase 2b integration.
`
