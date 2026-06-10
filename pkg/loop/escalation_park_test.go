package loop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════
// Keepalive escalation parks — slice-1 v8 run, finding v8-F2
//
// /signal ESCALATE returned StopEscalation from Run() even for keepalive
// agents: the goroutine exited, the bus subscription dropped, and the agent
// was gone for the daemon's lifetime. On the v8 convergence run the
// implementer escalated over a TRANSIENT condition (readiness gates missing
// — the planner attached them 27 seconds later) and the society permanently
// lost its only CanOperate agent; the strategist died the same way. Spawned
// replacements cannot operate (spawned agents are CanOperate=false by
// design), so a daemon cannot self-heal the loss.
//
// The fix: a keepalive agent's escalation RAISES (agent.escalated stays on
// the chain for the human) and PARKS (re-enters waitForEvents); the next
// wake or gated re-check re-evaluates with fresh context. One-shot
// (non-keepalive) loops keep terminal escalation — their callers consume
// the StopEscalation result.
// ════════════════════════════════════════════════════════════════════════

// escalationParkLoop builds a keepalive Loop whose agent emits the scripted
// responses, wired to a real graph so the escalation event lands on a real
// chain and the wake channel behaves exactly as production.
func escalationParkLoop(t *testing.T, responses ...string) (*Loop, *mockProvider, func() int) {
	t.Helper()
	provider := newMockProvider(responses...)
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
	escalations := func() int {
		page, err := g.Store().ByType(event.EventTypeAgentEscalated, 100, types.None[types.Cursor]())
		if err != nil {
			t.Fatalf("ByType(agent.escalated): %v", err)
		}
		return len(page.Items())
	}
	return l, provider, escalations
}

// TestRun_KeepaliveEscalationParksInsteadOfExiting is the v8-F2 keystone: a
// keepalive agent that signals ESCALATE must raise the escalation event and
// then PARK — Run() must not return — and a later wake must run further
// iterations on the same live loop.
func TestRun_KeepaliveEscalationParksInsteadOfExiting(t *testing.T) {
	l, provider, escalations := escalationParkLoop(t,
		`/signal {"signal":"ESCALATE","reason":"task missing readiness gates"}`,
		`/signal {"signal":"IDLE"}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	// The escalation must be RAISED and the loop must PARK, not exit.
	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s); a keepalive escalation must park the loop, not kill the agent", r.Reason, r.Detail)
	case <-time.After(300 * time.Millisecond):
	}
	if got := escalations(); got != 1 {
		t.Fatalf("agent.escalated events on the chain = %d; want exactly 1 (raise half of raise-and-park)", got)
	}

	// A wake must resume the SAME loop: further iterations run (the dead-loop
	// bug leaves callCount at 1 forever).
	select {
	case l.wake <- struct{}{}:
	default:
		t.Fatal("wake channel unexpectedly full; the parked loop never drained it")
	}
	deadline := time.Now().Add(2 * time.Second)
	for int(provider.callCount.Load()) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := int(provider.callCount.Load()); got < 2 {
		t.Fatalf("provider calls after wake = %d; want >= 2 (the loop must be alive after escalating)", got)
	}
	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s) after the post-escalation wake; keepalive loops park on IDLE instead", r.Reason, r.Detail)
	case <-time.After(200 * time.Millisecond):
	}

	// Shutdown still terminates the loop promptly.
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

// TestRun_KeepaliveTextEscalationAlsoParks pins the text-fallback signal path
// (no /signal JSON): the same raise-and-park semantics must hold, or the fix
// covers one parser and not the behavior.
func TestRun_KeepaliveTextEscalationAlsoParks(t *testing.T) {
	l, _, escalations := escalationParkLoop(t,
		"ESCALATE: blocked on a transient condition",
		`/signal {"signal":"IDLE"}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s); a keepalive text-fallback escalation must park, not exit", r.Reason, r.Detail)
	case <-time.After(300 * time.Millisecond):
	}
	if got := escalations(); got != 1 {
		t.Fatalf("agent.escalated events on the chain = %d; want exactly 1", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

// TestRun_KeepaliveMixedHaltEscalateStaysTerminal pins the constitutional
// gate across the park (codex review of #149, finding 1): parseSignal returns
// the LAST /signal line, so a response carrying a HALT directive — textual or
// an earlier /signal JSON line — plus a final structured ESCALATE never
// reaches checkResponseText's HALT check. Parking that response would convert
// a pre-fix stop into continued execution: HALT is constitutional and must
// never be masked. A HALT-bearing escalation must stay terminal.
func TestRun_KeepaliveMixedHaltEscalateStaysTerminal(t *testing.T) {
	cases := []struct {
		name     string
		response string
	}{
		{"textual HALT + JSON ESCALATE", "HALT: constitutional violation observed\n/signal {\"signal\":\"ESCALATE\",\"reason\":\"also blocked\"}"},
		{"JSON HALT + JSON ESCALATE", "/signal {\"signal\":\"HALT\",\"reason\":\"violation\"}\nfurther narration\n/signal {\"signal\":\"ESCALATE\",\"reason\":\"also blocked\"}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, _, _ := escalationParkLoop(t, tc.response)

			done := make(chan Result, 1)
			go func() { done <- l.Run(context.Background()) }()

			select {
			case r := <-done:
				if r.Reason != StopEscalation && r.Reason != StopHalt {
					t.Fatalf("mixed HALT/ESCALATE returned %s (%s); want a terminal stop", r.Reason, r.Detail)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("Run did not return for a HALT-bearing response; the keepalive park masked a constitutional HALT")
			}
		})
	}
}

// TestRun_ParkedEscalationShutdownResult pins the shutdown shape (codex
// review of #149, finding 2): cancelling a loop parked on an escalation
// returns StopEscalation — the outstanding, unanswered escalation stays
// visible in RunConcurrent results, mirroring how the quiescence branch
// reports its wait context — with a detail that names the shutdown so it
// cannot read as a live terminal escalation.
func TestRun_ParkedEscalationShutdownResult(t *testing.T) {
	l, _, _ := escalationParkLoop(t,
		`/signal {"signal":"ESCALATE","reason":"awaiting human"}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s) before cancellation; want parked", r.Reason, r.Detail)
	case <-time.After(300 * time.Millisecond):
	}
	cancel()

	select {
	case r := <-done:
		if r.Reason != StopEscalation {
			t.Fatalf("shutdown while parked returned %s; want %s with the outstanding escalation on record", r.Reason, StopEscalation)
		}
		if !strings.Contains(r.Detail, "shutdown while parked") || !strings.Contains(r.Detail, "awaiting human") {
			t.Fatalf("shutdown detail %q; want it to name the shutdown and carry the outstanding escalation reason", r.Detail)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

// TestRun_OneShotEscalationStaysTerminal pins the boundary: a NON-keepalive
// loop keeps terminal escalation semantics — pipeline callers consume the
// StopEscalation result and must keep doing so.
func TestRun_OneShotEscalationStaysTerminal(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"ESCALATE","reason":"needs human"}`)
	agent, g := agentWithGraph(t, provider)
	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Bus:     g.Bus(),
		// Keepalive deliberately false.
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	done := make(chan Result, 1)
	go func() { done <- l.Run(context.Background()) }()

	select {
	case r := <-done:
		if r.Reason != StopEscalation {
			t.Fatalf("one-shot escalation returned %s (%s); want %s", r.Reason, r.Detail, StopEscalation)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("one-shot Run did not return on escalation; terminal semantics regressed")
	}
}
