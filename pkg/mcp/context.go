package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// ContextBuilder builds context text to inject before agent prompts.
// This gives agents awareness of identity, actors, trust, and recent events
// without requiring them to use MCP tools for basic orientation.
type ContextBuilder struct {
	store  store.Store
	actors actor.IActorStore
	trust  *trust.DefaultTrustModel
}

// NewContextBuilder creates a context builder.
func NewContextBuilder(s store.Store, actors actor.IActorStore, trustModel *trust.DefaultTrustModel) *ContextBuilder {
	return &ContextBuilder{
		store:  s,
		actors: actors,
		trust:  trustModel,
	}
}

// Build generates context text for injection before an agent's prompt.
// Includes: own identity, human operator, other actors, trust scores,
// and recent events.
func (b *ContextBuilder) Build(agentID, humanID types.ActorID) string {
	var sections []string

	// 1. Own identity
	if self, err := b.actors.Get(agentID); err == nil {
		info := map[string]any{
			"id":           self.ID().Value(),
			"display_name": self.DisplayName(),
			"type":         string(self.Type()),
			"status":       string(self.Status()),
		}
		if metrics, err := b.trust.Score(nil, self); err == nil {
			info["trust_score"] = metrics.Overall().Value()
			info["confidence"] = metrics.Confidence().Value()
			info["trend"] = metrics.Trend().Value()
		}
		data, _ := json.MarshalIndent(info, "  ", "  ")
		sections = append(sections, fmt.Sprintf("## Your Identity\n  %s", string(data)))
	}

	// 2. Human operator
	if human, err := b.actors.Get(humanID); err == nil {
		sections = append(sections, fmt.Sprintf("## Human Operator\n  %s (%s)",
			human.DisplayName(), human.ID().Value()))
	}

	// 3. Other actors
	if page, err := b.actors.List(actor.ActorFilter{Limit: 20}); err == nil {
		var lines []string
		for _, a := range page.Items() {
			if a.ID() == agentID || a.ID() == humanID {
				continue
			}
			trust := "unknown"
			if metrics, err := b.trust.Score(nil, a); err == nil {
				trust = fmt.Sprintf("%.2f", metrics.Overall().Value())
			}
			lines = append(lines, fmt.Sprintf("  - %s (%s) type=%s status=%s trust=%s",
				a.DisplayName(), a.ID().Value(), a.Type(), a.Status(), trust))
		}
		if len(lines) > 0 {
			sections = append(sections, "## Other Actors\n"+strings.Join(lines, "\n"))
		}
	}

	// 4. Recent events (last 10)
	if page, err := b.store.Recent(10, types.None[types.Cursor]()); err == nil && len(page.Items()) > 0 {
		var lines []string
		for _, ev := range page.Items() {
			lines = append(lines, fmt.Sprintf("  - [%s] %s by %s",
				ev.Type().Value(), ev.ID().Value(), ev.Source().Value()))
		}
		sections = append(sections, "## Recent Events\n"+strings.Join(lines, "\n"))
	}

	if len(sections) == 0 {
		return ""
	}

	return "# Hive Context\n\n" + strings.Join(sections, "\n\n") + "\n"
}
