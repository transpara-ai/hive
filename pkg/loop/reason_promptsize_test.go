package loop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// v14-F1 observability: when reason exhaustion raises, the escalation must
// carry the prompt size — the first discriminator between prompt-bloat and
// provider hangs, and the only one available when the killed calls leave no
// transcripts (--no-session-persistence).
func TestReasonExhaustionEscalationNamesPromptSize(t *testing.T) {
	provider := &flakyProvider{steps: []flakyStep{{err: err529}}}
	agent, g := agentWithGraph(t, provider)

	l, err := New(Config{
		Agent:     agent,
		HumanID:   humanID(),
		Bus:       g.Bus(),
		Keepalive: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.reasonRetryBackoff = fastBackoff

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for countEscalations(t, g) < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := countEscalations(t, g); got != 1 {
		t.Fatalf("escalations = %d; want 1", got)
	}

	page, err := g.Store().ByType(event.EventTypeAgentEscalated, 10, types.None[types.Cursor]())
	if err != nil || len(page.Items()) != 1 {
		t.Fatalf("ByType(agent.escalated) = %d items (%v); want 1", len(page.Items()), err)
	}
	content, ok := page.Items()[0].Content().(event.AgentEscalatedContent)
	if !ok {
		t.Fatal("agent.escalated content has wrong type")
	}
	if !strings.Contains(content.Reason, "prompt_chars=") {
		t.Fatalf("escalation reason %q must carry prompt_chars= (v14-F1 observability)", content.Reason)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}
