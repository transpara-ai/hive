package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/pipeline"
)

// pipelineEventTypes are the event types to query, in display order.
var pipelineEventTypes = []types.EventType{
	pipeline.EventTypeRunStarted,
	pipeline.EventTypeRunCompleted,
	pipeline.EventTypePhaseStarted,
	pipeline.EventTypePhaseCompleted,
	pipeline.EventTypeAgentSpawned,
	pipeline.EventTypeProgress,
	pipeline.EventTypeOutput,
	pipeline.EventTypeWarning,
	pipeline.EventTypeTelemetry,
}

// queryPipelineEvents reads all pipeline events from the store and prints them.
func queryPipelineEvents(s store.Store, filter string) error {
	// Register pipeline content unmarshalers so we can deserialize from Postgres.
	pipeline.RegisterEventTypes()

	var events []event.Event

	for _, et := range pipelineEventTypes {
		if filter != "" && !strings.Contains(et.Value(), filter) {
			continue
		}
		cursor := types.None[types.Cursor]()
		for {
			page, err := s.ByType(et, 100, cursor)
			if err != nil {
				return fmt.Errorf("query %s: %w", et.Value(), err)
			}
			events = append(events, page.Items()...)
			if !page.HasMore() {
				break
			}
			cursor = page.Cursor()
		}
	}

	if len(events) == 0 {
		fmt.Fprintln(os.Stderr, "No pipeline events found.")
		return nil
	}

	// Sort by timestamp.
	sortEventsByTimestamp(events)

	// Group by conversation ID (pipeline run).
	var currentConv string
	for _, ev := range events {
		conv := ev.ConversationID().Value()
		if conv != currentConv {
			if currentConv != "" {
				fmt.Println()
			}
			fmt.Printf("═══ Run %s ═══\n", truncateConvID(conv))
			currentConv = conv
		}
		printEvent(ev)
	}

	return nil
}

func printEvent(ev event.Event) {
	ts := ev.Timestamp().Value().Format("15:04:05")
	typeName := ev.Type().Value()
	short := strings.TrimPrefix(typeName, "pipeline.")

	content := ev.Content()
	if content == nil {
		fmt.Printf("[%s] %-18s (no content)\n", ts, short)
		return
	}

	switch c := content.(type) {
	case pipeline.RunStartedContent:
		fmt.Printf("[%s] %-18s mode=%s desc=%s\n", ts, short, c.Mode, truncateStr(c.Description, 60))
	case pipeline.RunCompletedContent:
		line := fmt.Sprintf("[%s] %-18s events=%d agents=%d cost=$%.2f", ts, short, c.EventCount, c.AgentCount, c.TotalCostUSD)
		if c.PRURL != "" {
			line += fmt.Sprintf(" pr=%s merged=%v", c.PRURL, c.Merged)
		}
		if c.FailedPhase != "" {
			line += fmt.Sprintf(" FAILED=%s reason=%s", c.FailedPhase, truncateStr(c.FailReason, 60))
		}
		fmt.Println(line)
	case pipeline.PhaseStartedContent:
		if c.Round > 0 {
			fmt.Printf("[%s] %-18s %s (round %d)\n", ts, short, c.Phase, c.Round)
		} else {
			fmt.Printf("[%s] %-18s %s\n", ts, short, c.Phase)
		}
	case pipeline.PhaseCompletedContent:
		dur := fmt.Sprintf("%dms", c.DurationMs)
		if c.DurationMs > 1000 {
			dur = fmt.Sprintf("%.1fs", float64(c.DurationMs)/1000)
		}
		fmt.Printf("[%s] %-18s %s (%s)\n", ts, short, c.Phase, dur)
	case pipeline.AgentSpawnedContent:
		fmt.Printf("[%s] %-18s %s (%s) %s\n", ts, short, c.Role, c.Model, c.ActorID)
	case pipeline.ProgressContent:
		msg := c.Message
		if c.Phase != "" {
			msg = c.Phase + ": " + msg
		}
		fmt.Printf("[%s] %-18s %s\n", ts, short, truncateStr(msg, 100))
	case pipeline.OutputContent:
		preview := truncateStr(strings.ReplaceAll(c.Content, "\n", " "), 100)
		fmt.Printf("[%s] %-18s [%s/%s] %s\n", ts, short, c.Role, c.Kind, preview)
	case pipeline.WarningContent:
		msg := c.Message
		if c.Phase != "" {
			msg = c.Phase + ": " + msg
		}
		fmt.Printf("[%s] %-18s %s\n", ts, "WARNING", truncateStr(msg, 100))
	case pipeline.TelemetryContent:
		fmt.Printf("[%s] %-18s %s (%s) tokens=%d cost=$%.4f\n", ts, short, c.Role, c.Model, c.TotalTokens, c.CostUSD)
	default:
		fmt.Printf("[%s] %-18s %T\n", ts, short, content)
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func truncateConvID(conv string) string {
	if len(conv) > 20 {
		return conv[:20] + "..."
	}
	return conv
}

func sortEventsByTimestamp(events []event.Event) {
	// Simple insertion sort — event counts are small (hundreds, not millions).
	for i := 1; i < len(events); i++ {
		for j := i; j > 0 && events[j].Timestamp().Value().Before(events[j-1].Timestamp().Value()); j-- {
			events[j], events[j-1] = events[j-1], events[j]
		}
	}
}
