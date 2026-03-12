package pipeline

import (
	"fmt"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// emit appends a pipeline event to the graph. Best-effort — errors are
// swallowed because output events are observability, not control flow.
func (p *Pipeline) emit(eventType types.EventType, content event.EventContent) {
	head, err := p.store.Head()
	if err != nil || !head.IsSome() {
		return
	}
	causeID := head.Unwrap().ID()
	ev, err := p.factory.Create(eventType, p.humanID, content, []types.EventID{causeID}, p.convID, p.store, p.signer)
	if err != nil {
		return
	}
	_, _ = p.store.Append(ev)
}

// --- Convenience methods ---

func (p *Pipeline) emitRunStarted(mode string, description string) {
	p.emit(EventTypeRunStarted, RunStartedContent{
		Mode:        mode,
		Description: description,
	})
}

func (p *Pipeline) emitRunCompleted(mode string, eventCount, agentCount int, dur time.Duration, prURL string, merged bool, failedPhase, failReason string, totalCost float64) {
	p.emit(EventTypeRunCompleted, RunCompletedContent{
		Mode:         mode,
		EventCount:   eventCount,
		AgentCount:   agentCount,
		DurationMs:   dur.Milliseconds(),
		PRURL:        prURL,
		Merged:       merged,
		FailedPhase:  failedPhase,
		FailReason:   failReason,
		TotalCostUSD: totalCost,
	})
}

func (p *Pipeline) emitPhaseStarted(phase Phase, round int) {
	p.emit(EventTypePhaseStarted, PhaseStartedContent{
		Phase: string(phase),
		Round: round,
	})
}

func (p *Pipeline) emitPhaseCompleted(phase Phase, dur time.Duration, round int) {
	p.emit(EventTypePhaseCompleted, PhaseCompletedContent{
		Phase:      string(phase),
		DurationMs: dur.Milliseconds(),
		Round:      round,
	})
}

func (p *Pipeline) emitAgentSpawned(role, actorID, model string) {
	p.emit(EventTypeAgentSpawned, AgentSpawnedContent{
		Role:    role,
		ActorID: actorID,
		Model:   model,
	})
}

func (p *Pipeline) emitProgress(phase Phase, format string, args ...any) {
	p.emit(EventTypeProgress, ProgressContent{
		Phase:   string(phase),
		Message: fmt.Sprintf(format, args...),
	})
}

func (p *Pipeline) emitOutput(role, kind, content string) {
	p.emit(EventTypeOutput, OutputContent{
		Role:    role,
		Kind:    kind,
		Content: content,
	})
}

func (p *Pipeline) emitWarning(phase Phase, format string, args ...any) {
	p.emit(EventTypeWarning, WarningContent{
		Phase:   string(phase),
		Message: fmt.Sprintf(format, args...),
	})
}

func (p *Pipeline) emitTelemetryEntry(role, model string, input, output, total, cacheRead, cacheWrite int, cost float64) {
	p.emit(EventTypeTelemetry, TelemetryContent{
		Role:             role,
		Model:            model,
		InputTokens:      input,
		OutputTokens:     output,
		TotalTokens:      total,
		CacheReadTokens:  cacheRead,
		CacheWriteTokens: cacheWrite,
		CostUSD:          cost,
	})
}
