package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
)

func eventsToResult(events []event.Event) ToolCallResult {
	if len(events) == 0 {
		return TextResult("No events found.")
	}
	var items []map[string]any
	for _, ev := range events {
		items = append(items, eventMap(ev))
	}
	data, _ := json.MarshalIndent(items, "", "  ")
	return TextResult(string(data))
}

func eventToResult(ev event.Event) ToolCallResult {
	data, _ := json.MarshalIndent(eventMap(ev), "", "  ")
	return TextResult(string(data))
}

func eventMap(ev event.Event) map[string]any {
	causes := make([]string, len(ev.Causes()))
	for i, c := range ev.Causes() {
		causes[i] = c.Value()
	}
	return map[string]any{
		"id":              ev.ID().Value(),
		"type":            ev.Type().Value(),
		"source":          ev.Source().Value(),
		"conversation_id": ev.ConversationID().Value(),
		"timestamp":       ev.Timestamp().String(),
		"causes":          causes,
		"content":         fmt.Sprintf("%v", ev.Content()),
	}
}

func actorToResult(a actor.IActor) ToolCallResult {
	data, _ := json.MarshalIndent(actorMap(a), "", "  ")
	return TextResult(string(data))
}

func actorsToResult(actors []actor.IActor) ToolCallResult {
	if len(actors) == 0 {
		return TextResult("No actors found.")
	}
	var items []map[string]any
	for _, a := range actors {
		items = append(items, actorMap(a))
	}
	data, _ := json.MarshalIndent(items, "", "  ")
	return TextResult(string(data))
}

func actorMap(a actor.IActor) map[string]any {
	return map[string]any{
		"id":           a.ID().Value(),
		"display_name": a.DisplayName(),
		"type":         string(a.Type()),
		"status":       string(a.Status()),
		"metadata":     a.Metadata(),
	}
}

func trustToResult(metrics event.TrustMetrics) ToolCallResult {
	info := map[string]any{
		"actor":      metrics.Actor().Value(),
		"overall":    metrics.Overall().Value(),
		"confidence": metrics.Confidence().Value(),
		"trend":      metrics.Trend().Value(),
		"decay_rate": metrics.DecayRate().Value(),
	}
	if len(metrics.ByDomain()) > 0 {
		domains := make(map[string]float64)
		for d, s := range metrics.ByDomain() {
			domains[d.Value()] = s.Value()
		}
		info["domains"] = domains
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	return TextResult(string(data))
}
