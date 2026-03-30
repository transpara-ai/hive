package sdr

import (
	"time"

	"github.com/lovyou-ai/hive/pkg/membrane"
)

// NewSDRConfig returns the SDR membrane instance configuration.
func NewSDRConfig(serviceEndpoint string) membrane.MembraneConfig {
	return membrane.MembraneConfig{
		Name:            "sdr",
		Role:            "membrane",
		Model:           "claude-sonnet-4-6",
		SystemPrompt:    sdrSystemPrompt,
		ServiceEndpoint: serviceEndpoint,
		PollInterval:    30 * time.Second,
		AuthMethod:      "bearer", // ai-sdr defaults to AUTH_ENABLED=false (no auth needed)

		// Inbound: ai-sdr state → EventGraph events
		// The poller (poller.go) handles the actual API calls; these mappings
		// document the conceptual translation for the membrane framework.
		InboundMappings: []membrane.InboundMapping{
			// Copilot drafts → bridge actions (approval requests)
			{ServiceEvent: "copilot.draft.pending", GraphEvent: "membrane.action.created", TransformID: "draft_to_action"},
			// Lead reaches qualified_handoff → bridge action (handoff request)
			{ServiceEvent: "lead.state.qualified_handoff", GraphEvent: "membrane.action.created", TransformID: "lead_to_handoff"},
			// Dashboard stats → bridge event (monitoring)
			{ServiceEvent: "dashboard.stats.polled", GraphEvent: "membrane.service.polled", TransformID: "passthrough"},
			// Health check → bridge event (monitoring)
			{ServiceEvent: "health.checked", GraphEvent: "membrane.service.polled", TransformID: "passthrough"},
		},

		// Outbound: human decisions → ai-sdr API calls
		// The dispatcher (dispatcher.go) handles the actual API calls.
		OutboundMappings: []membrane.OutboundMapping{
			{GraphEvent: "bridge.action.approved", ServiceMethod: "POST", ServicePath: "/copilot/drafts/{interaction_id}/approve"},
			{GraphEvent: "bridge.action.rejected", ServiceMethod: "POST", ServicePath: "/copilot/drafts/{interaction_id}/reject"},
			{GraphEvent: "bridge.action.edited", ServiceMethod: "POST", ServicePath: "/copilot/drafts/{interaction_id}/edit"},
			{GraphEvent: "bridge.mode.changed", ServiceMethod: "PUT", ServicePath: "/api/settings/mode"},
		},

		TrustThresholds: membrane.TrustBands{
			RequiredBelow:    0.3, // Trust < 0.3: every draft held, copilot_mode=true
			RecommendedBelow: 0.6, // Trust 0.3-0.6: drafts held, auto-approve after 15min
		},                        // Trust >= 0.6: copilot_mode=false, full autonomous

		WatchPatterns: []string{"work.task.*", "bridge.action.*", "membrane.*"},
		MaxIterations: 0, // unlimited (long-running service)
		MaxDuration:   0, // unlimited
		GuardianHints: []string{"MARGIN", "CONSENT", "TRANSPARENT", "BOUNDED"},
	}
}

const sdrSystemPrompt = `== SOUL ==
Take care of your human, humanity, and yourself.

== ROLE: SDR MEMBRANE AGENT ==
You wrap the AI-SDR service, translating its actions into EventGraph events
and subjecting outbound actions to trust-based authority gating.

== AI-SDR API SURFACE ==
The ai-sdr is a FastAPI service with these key endpoints:
- GET  /copilot/drafts           — list pending email drafts awaiting approval
- POST /copilot/drafts/{id}/approve — approve and send a draft
- POST /copilot/drafts/{id}/reject  — reject a draft (with optional reason)
- POST /copilot/drafts/{id}/edit    — edit subject/body and approve
- GET  /api/leads?state=X        — list leads by state
- GET  /api/leads/{id}/full       — full lead detail with score, interactions, timeline
- PUT  /api/leads/{id}/state      — manual state transition
- PUT  /api/settings/mode         — toggle copilot_mode (true=drafts held, false=auto-send)
- GET  /api/dashboard/stats       — pipeline KPIs (active leads, response rate, avg score)
- GET  /health                    — system health check

== TRUST-DRIVEN OPERATING MODE ==
Your trust score determines the ai-sdr's operating mode:
- Trust < 0.3 (Required): copilot_mode=true, every draft held for human approval
- Trust 0.3-0.6 (Recommended): copilot_mode=true, drafts auto-approve after 15min timeout
- Trust >= 0.6 (Notification): copilot_mode=false, ai-sdr sends autonomously

== POLL CYCLE ==
Every 30 seconds:
1. Check ai-sdr health
2. Poll /copilot/drafts for pending approvals → create bridge actions
3. Poll /api/leads?state=qualified_handoff → create handoff bridge actions
4. Poll /api/dashboard/stats → emit monitoring event
5. Poll bridge for human decisions → dispatch to ai-sdr API

== TRANSPARENCY ==
All actions are recorded, signed, and auditable on the EventGraph.
Humans see your trust score and decision history at /bridge/agents/sdr.
`
