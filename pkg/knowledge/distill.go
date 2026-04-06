package knowledge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// EventEmitter is the interface for emitting signed events to the chain.
// Implemented by the runtime using the system actor's signer and factory.
type EventEmitter interface {
	Emit(eventType types.EventType, content event.EventContent) error
}

// Distiller runs background detectors that analyse the event chain
// and emit distilled knowledge insights.
type Distiller struct {
	store    store.Store
	ks       KnowledgeStore
	emitter  EventEmitter
	interval time.Duration
	known    map[string]bool // dedup: insight IDs already emitted
	mu       sync.Mutex
}

// NewDistiller creates a distiller that analyses the chain on the given interval.
func NewDistiller(s store.Store, ks KnowledgeStore, emitter EventEmitter, interval time.Duration) *Distiller {
	return &Distiller{
		store:    s,
		ks:       ks,
		emitter:  emitter,
		interval: interval,
		known:    make(map[string]bool),
	}
}

// Run starts the distiller loop. Blocks until ctx is cancelled.
func (d *Distiller) Run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.runAllDetectors()
		}
	}
}

func (d *Distiller) runAllDetectors() {
	insights := d.detectHealthCorrelations()
	insights = append(insights, d.detectBudgetEffectiveness()...)

	d.mu.Lock()
	defer d.mu.Unlock()

	for _, ins := range insights {
		if d.known[ins.InsightID] {
			continue
		}
		// Emit to chain.
		content := event.KnowledgeInsightContent{
			InsightID:     ins.InsightID,
			Domain:        ins.Domain,
			Summary:       ins.Summary,
			RelevantRoles: ins.RelevantRoles,
			Confidence:    ins.Confidence,
			EvidenceCount: ins.EvidenceCount,
			Source:        ins.Source,
			TTL:           ttlFromExpiry(ins.RecordedAt, ins.ExpiresAt),
		}
		if err := d.emitter.Emit(event.EventTypeKnowledgeInsightRecorded, content); err != nil {
			continue
		}
		// Record in knowledge store for immediate availability.
		_ = d.ks.Record(ins)
		d.known[ins.InsightID] = true
	}
}

// ttlFromExpiry converts an absolute expiry to TTL in hours.
// Returns 0 for zero ExpiresAt (never expires).
func ttlFromExpiry(recorded, expires time.Time) int {
	if expires.IsZero() {
		return 0
	}
	hours := int(expires.Sub(recorded).Hours())
	if hours < 1 {
		return 1
	}
	return hours
}

// ────────────────────────────────────────────────────────────────────
// Detector 1: Health Correlations
// ────────────────────────────────────────────────────────────────────

// detectHealthCorrelations identifies agents whose activity correlates
// with health severity events (warning/critical).
func (d *Distiller) detectHealthCorrelations() []KnowledgeInsight {
	healthType := event.EventTypeHealthReport
	page, err := d.store.ByType(healthType, 200, types.None[types.Cursor]())
	if err != nil {
		return nil
	}
	items := page.Items()
	if len(items) == 0 {
		return nil
	}

	// Count which source actors appear near severity events.
	agentCorrelation := make(map[string]int)
	totalScanned := 0

	for _, ev := range items {
		hc, ok := ev.Content().(event.HealthReportContent)
		if !ok {
			continue
		}
		// Skip healthy reports.
		if hc.Overall.Value() >= 1.0 {
			continue
		}
		totalScanned++

		// Look at events near this health report using Since to anchor
		// to the report's position in the chain, not the current head.
		nearbyPage, err := d.store.Since(ev.ID(), 20)
		if err != nil {
			continue
		}
		for _, nearby := range nearbyPage.Items() {
			src := nearby.Source().Value()
			if src != "" {
				agentCorrelation[src]++
			}
		}
	}

	if totalScanned == 0 {
		return nil
	}

	var insights []KnowledgeInsight
	dateStr := time.Now().Format("2006-01-02")

	for agentSrc, count := range agentCorrelation {
		if count < 3 {
			continue
		}
		confidence := 0.5 + float64(count)*0.1
		if confidence > 0.95 {
			confidence = 0.95
		}
		recordedAt := time.Now()
		insights = append(insights, KnowledgeInsight{
			InsightID:     fmt.Sprintf("health-corr-%s-%s", agentSrc, dateStr),
			Domain:        DomainHealth,
			Summary:       fmt.Sprintf("Agent %s activity correlates with %d health severity events", agentSrc, count),
			RelevantRoles: []string{"sysmon", "allocator"},
			Confidence:    confidence,
			EvidenceCount: totalScanned,
			Source:        SourceDistillerPrefix + "health-correlation",
			RecordedAt:    recordedAt,
			ExpiresAt:     recordedAt.Add(168 * time.Hour), // 7 days
			Active:        true,
		})
	}

	return insights
}

// ────────────────────────────────────────────────────────────────────
// Detector 2: Budget Effectiveness
// ────────────────────────────────────────────────────────────────────

// detectBudgetEffectiveness tracks whether budget increases lead to
// proportional output increases. Identifies diminishing returns.
func (d *Distiller) detectBudgetEffectiveness() []KnowledgeInsight {
	budgetType := event.EventTypeAgentBudgetAdjusted
	page, err := d.store.ByType(budgetType, 100, types.None[types.Cursor]())
	if err != nil {
		return nil
	}
	items := page.Items()
	if len(items) == 0 {
		return nil
	}

	// Group adjustments by target agent ID (not name — IDENTITY invariant).
	type adjustment struct {
		agentID   types.ActorID
		agentName string // display only
		delta     int
		timestamp time.Time
	}
	byAgent := make(map[string][]adjustment) // keyed by ActorID.Value()

	for _, ev := range items {
		bc, ok := ev.Content().(event.AgentBudgetAdjustedContent)
		if !ok {
			continue
		}
		if bc.Action != "increase" {
			continue
		}
		adj := adjustment{
			agentID:   bc.AgentID,
			agentName: bc.AgentName,
			delta:     bc.Delta,
			timestamp: ev.Timestamp().Value(),
		}
		byAgent[bc.AgentID.Value()] = append(byAgent[bc.AgentID.Value()], adj)
	}

	var insights []KnowledgeInsight
	dateStr := time.Now().Format("2006-01-02")

	for agentIDStr, adjustments := range byAgent {
		if len(adjustments) < 3 {
			continue
		}

		// Use display name from the first adjustment for human-readable summary.
		displayName := adjustments[0].agentName

		// Check output around each adjustment.
		lowEffectCount := 0
		for _, adj := range adjustments {
			beforeCount := d.countEventsAround(adj.agentID, adj.timestamp, -30*time.Minute)
			afterCount := d.countEventsAround(adj.agentID, adj.timestamp, 30*time.Minute)

			// Less than 20% output increase → low effectiveness.
			if beforeCount > 0 {
				increase := float64(afterCount-beforeCount) / float64(beforeCount)
				if increase < 0.2 {
					lowEffectCount++
				}
			} else if afterCount == 0 {
				// No output before or after → low effectiveness.
				lowEffectCount++
			}
		}

		if lowEffectCount < 3 {
			continue
		}

		confidence := 0.5 + float64(lowEffectCount)*0.1
		if confidence > 0.95 {
			confidence = 0.95
		}
		recordedAt := time.Now()
		insights = append(insights, KnowledgeInsight{
			InsightID:     fmt.Sprintf("budget-eff-%s-%s", agentIDStr, dateStr),
			Domain:        DomainBudget,
			Summary:       fmt.Sprintf("Budget increases for %s show diminishing returns (%d/%d adjustments with <20%% output increase)", displayName, lowEffectCount, len(adjustments)),
			RelevantRoles: []string{"allocator", "cto"},
			Confidence:    confidence,
			EvidenceCount: len(adjustments),
			Source:        SourceDistillerPrefix + "budget-effectiveness",
			RecordedAt:    recordedAt,
			ExpiresAt:     recordedAt.Add(336 * time.Hour), // 14 days
			Active:        true,
		})
	}

	return insights
}

// countEventsAround counts events from a source actor in a time window
// relative to a reference timestamp. Positive offset = after, negative = before.
func (d *Distiller) countEventsAround(source types.ActorID, ref time.Time, offset time.Duration) int {
	page, err := d.store.BySource(source, 50, types.None[types.Cursor]())
	if err != nil {
		return 0
	}

	var start, end time.Time
	if offset >= 0 {
		start = ref
		end = ref.Add(offset)
	} else {
		start = ref.Add(offset)
		end = ref
	}

	count := 0
	for _, ev := range page.Items() {
		t := ev.Timestamp().Value()
		if !t.Before(start) && !t.After(end) {
			count++
		}
	}
	return count
}
