package loop

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	egagent "github.com/transpara-ai/eventgraph/go/pkg/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/decision"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/hive/pkg/resources"
)

// ════════════════════════════════════════════════════════════════════════
// Reason-path provider resilience — slice-1 v13 run, finding v13-F1
//
// The implementer's first Reason call died on API Error 529 Overloaded —
// a transient upstream error whose own text says "try again in a moment" —
// and Run() treated any Reason error as fatal StopError: no retry, no log
// line, no chain event. The loop goroutine exited silently (its Result
// invisible until daemon shutdown) and the society lost its only
// CanOperate agent AND the only auto-assign re-check ticker: the run froze
// at the decomposition→assignment boundary while every operator surface
// showed healthy. v8-F2 already established the contract for the signal
// path: RAISE on-chain, then PARK. A provider failure must follow the same
// contract — bounded in-iteration retries to absorb blips, then
// raise-and-park (keepalive) or a visible terminal escalation (one-shot).
// A loop must NEVER die silently on a provider error.
// ════════════════════════════════════════════════════════════════════════

// flakyProvider scripts a sequence of Reason outcomes — error or response —
// repeating the final step once the script is exhausted.
type flakyStep struct {
	err  error
	resp string
}

type flakyProvider struct {
	steps     []flakyStep
	callCount atomic.Int32
}

func (p *flakyProvider) Name() string  { return "flaky" }
func (p *flakyProvider) Model() string { return "flaky-model" }

func (p *flakyProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	idx := int(p.callCount.Add(1)) - 1
	if idx >= len(p.steps) {
		idx = len(p.steps) - 1
	}
	s := p.steps[idx]
	if s.err != nil {
		return decision.Response{}, s.err
	}
	confidence, _ := types.NewScore(0.8)
	return decision.NewResponse(s.resp, confidence, decision.TokenUsage{InputTokens: 30, OutputTokens: 20}), nil
}

var _ intelligence.Provider = (*flakyProvider)(nil)

// err529 mirrors the exact failure shape recovered from the v13 daemon's
// memory (RunConcurrent results[8], dlv attach, 2026-06-11).
var err529 = errors.New("claude CLI reason returned error: API Error: 529 Overloaded. This is a server-side issue, usually temporary — try again in a moment.")

func countEscalations(t *testing.T, g *graph.Graph) int {
	t.Helper()
	page, err := g.Store().ByType(event.EventTypeAgentEscalated, 100, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(agent.escalated): %v", err)
	}
	return len(page.Items())
}

// fastBackoff makes retry gaps test-sized. Production defaults are tens of
// seconds; the schedule is data, not logic, so tests shrink it.
var fastBackoff = []time.Duration{time.Millisecond, time.Millisecond}

// TestReason_TransientProviderErrorRetriesWithinIteration: two 529s then a
// good response must complete the iteration as if nothing happened — no
// escalation raised, no loop exit, the blip absorbed entirely in-iteration.
// Also pins the failure-episode reset: a pre-set escalated flag clears on
// the first successful Reason.
func TestReason_TransientProviderErrorRetriesWithinIteration(t *testing.T) {
	provider := &flakyProvider{steps: []flakyStep{
		{err: err529},
		{err: err529},
		{resp: `/signal {"signal":"TASK_DONE"}`},
	}}
	agent, g := agentWithGraph(t, provider)

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
		Task:    "build a widget",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.reasonRetryBackoff = fastBackoff
	l.reasonFailureEscalated = true // stale episode flag — success must clear it

	result := l.Run(context.Background())

	if result.Reason != StopTaskDone {
		t.Fatalf("reason = %s (%s), want %s — transient provider errors must be retried, not kill the loop", result.Reason, result.Detail, StopTaskDone)
	}
	if result.Iterations != 1 {
		t.Errorf("iterations = %d, want 1 (retries happen inside the iteration)", result.Iterations)
	}
	if got := int(provider.callCount.Load()); got != 3 {
		t.Errorf("provider calls = %d, want 3 (fail, fail, succeed)", got)
	}
	if got := countEscalations(t, g); got != 0 {
		t.Errorf("escalations = %d, want 0 — an absorbed blip must not page the human", got)
	}
	if l.reasonFailureEscalated {
		t.Error("reasonFailureEscalated still set after a successful Reason; the failure episode must reset")
	}
}

// TestRun_KeepaliveReasonExhaustionEscalatesAndParks is the v13-F1 keystone:
// a keepalive agent whose Reason calls exhaust their retry budget must raise
// agent.escalated ON-CHAIN exactly once per failure episode and PARK — Run()
// must not return — and a later wake must retry on the same live loop
// without re-escalating while the episode persists.
func TestRun_KeepaliveReasonExhaustionEscalatesAndParks(t *testing.T) {
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

	// The exhaustion must RAISE exactly one escalation...
	deadline := time.Now().Add(2 * time.Second)
	for countEscalations(t, g) < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := countEscalations(t, g); got != 1 {
		t.Fatalf("escalations after exhaustion = %d, want 1 (raise half of raise-and-park)", got)
	}
	// ...and PARK, not exit.
	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s); reason exhaustion on a keepalive loop must park, not kill the agent", r.Reason, r.Detail)
	case <-time.After(300 * time.Millisecond):
	}

	// A wake must retry on the SAME loop (a dead loop leaves callCount fixed)...
	callsBeforeWake := int(provider.callCount.Load())
	select {
	case l.wake <- struct{}{}:
	default:
		t.Fatal("wake channel unexpectedly full; the parked loop never drained it")
	}
	deadline = time.Now().Add(2 * time.Second)
	for int(provider.callCount.Load()) <= callsBeforeWake && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := int(provider.callCount.Load()); got <= callsBeforeWake {
		t.Fatalf("provider calls after wake = %d (was %d); the parked loop must retry on wake", got, callsBeforeWake)
	}
	// ...without exiting and WITHOUT a second escalation for the same episode.
	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s) after the post-park wake; the loop must park again", r.Reason, r.Detail)
	case <-time.After(300 * time.Millisecond):
	}
	if got := countEscalations(t, g); got != 1 {
		t.Fatalf("escalations after second exhaustion = %d, want still 1 (one raise per failure episode)", got)
	}

	// Shutdown terminates promptly with a visible, named result.
	cancel()
	select {
	case r := <-done:
		if r.Reason != StopEscalation {
			t.Errorf("shutdown result = %s, want %s", r.Reason, StopEscalation)
		}
		if !strings.Contains(r.Detail, "parked on reason failure") {
			t.Errorf("shutdown detail %q must name the parked-on-reason-failure state", r.Detail)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

// TestRun_NonKeepaliveReasonExhaustionStopsVisiblyWithEscalation: one-shot
// loops keep a terminal stop on exhaustion, but it must be the VISIBLE kind —
// escalation raised on-chain, StopEscalation (not the silent StopError that
// buried v13-F1), detail naming the attempts and carrying the provider error.
func TestRun_NonKeepaliveReasonExhaustionStopsVisiblyWithEscalation(t *testing.T) {
	provider := &flakyProvider{steps: []flakyStep{{err: err529}}}
	agent, g := agentWithGraph(t, provider)

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
		Task:    "build a widget",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.reasonRetryBackoff = fastBackoff

	result := l.Run(context.Background())

	if result.Reason != StopEscalation {
		t.Fatalf("reason = %s (%s), want %s — exhaustion must stop VISIBLY", result.Reason, result.Detail, StopEscalation)
	}
	if !strings.Contains(result.Detail, "reason failed after 3 attempts") {
		t.Errorf("detail %q must name the attempt count", result.Detail)
	}
	if !strings.Contains(result.Detail, "529") {
		t.Errorf("detail %q must carry the provider error", result.Detail)
	}
	if got := countEscalations(t, g); got != 1 {
		t.Errorf("escalations = %d, want 1 — the exhaustion must reach the chain", got)
	}
}

// TestReasonRetry_AbortsPromptlyOnShutdown: a retry backoff must not hold the
// daemon hostage — context cancellation during the wait returns promptly with
// the cancellation named, not after the full backoff.
func TestReasonRetry_AbortsPromptlyOnShutdown(t *testing.T) {
	provider := &flakyProvider{steps: []flakyStep{{err: err529}}}
	agent, _ := agentWithGraph(t, provider)

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
		Task:    "build a widget",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.reasonRetryBackoff = []time.Duration{10 * time.Second} // one long gap

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	// Wait until the first attempt has failed and the backoff wait begun.
	deadline := time.Now().Add(2 * time.Second)
	for int(provider.callCount.Load()) < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case r := <-done:
		if r.Reason != StopCancelled {
			t.Errorf("reason = %s (%s), want %s", r.Reason, r.Detail, StopCancelled)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Run did not return promptly after cancellation mid-backoff; the retry wait must be context-aware")
	}
}

// TestReasonFailure_InvalidTransitionStaysTerminal (codex r1 finding 1): a
// state-machine refusal is authority, not weather. A suspended or retired
// agent must not be revived by the retry loop (the round-1 ResetToIdle
// would have bypassed a Guardian suspension) and must not page the human as
// a provider outage. Pre-v13-F1 terminal semantics stand — now visible via
// the RunConcurrent obituary instead of silent.
func TestReasonFailure_InvalidTransitionStaysTerminal(t *testing.T) {
	provider := &flakyProvider{steps: []flakyStep{
		{err: errors.New("invalid transition: cannot transition from Suspended to Processing")},
	}}
	agent, g := agentWithGraph(t, provider)

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
		Task:    "build a widget",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.reasonRetryBackoff = fastBackoff

	result := l.Run(context.Background())

	if result.Reason != StopError {
		t.Fatalf("reason = %s (%s), want %s — state-machine refusals keep terminal semantics", result.Reason, result.Detail, StopError)
	}
	if !strings.Contains(result.Detail, "invalid transition") {
		t.Errorf("detail %q must carry the transition error", result.Detail)
	}
	if got := int(provider.callCount.Load()); got != 1 {
		t.Errorf("provider calls = %d, want 1 — an invalid transition must not be retried (revival risk)", got)
	}
	if got := countEscalations(t, g); got != 0 {
		t.Errorf("escalations = %d, want 0 — suspension is not weather", got)
	}
}

// escalationRejectingStore fails exactly the agent.escalated append —
// surgical injection for the raise-failure path (codex r1 finding 2). The
// agent's own state bookkeeping still records; only the escalation write
// dies, mirroring a chain write failing mid-outage.
type escalationRejectingStore struct {
	store.Store
}

func (s escalationRejectingStore) Append(ev event.Event) (event.Event, error) {
	if ev.Type() == event.EventTypeAgentEscalated {
		return event.Event{}, errors.New("injected: escalation append rejected")
	}
	return s.Store.Append(ev)
}

// TestReasonFailure_FailedRaiseDoesNotSetEpisodeFlag (codex r1 finding 2): if
// the on-chain raise itself fails, the episode flag must NOT set — otherwise
// every future exhaustion in the episode is suppressed and no agent.escalated
// ever reaches the chain. A failed raise must stay raisable.
func TestReasonFailure_FailedRaiseDoesNotSetEpisodeFlag(t *testing.T) {
	provider := &flakyProvider{steps: []flakyStep{{err: err529}}}
	s := escalationRejectingStore{store.NewInMemoryStore()}
	as := actor.NewInMemoryActorStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	agent, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role("builder"),
		Name:     "raise-fail-test",
		Graph:    g,
		Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
		Task:    "build a widget",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.reasonRetryBackoff = fastBackoff

	result := l.Run(context.Background())

	if result.Reason != StopEscalation {
		t.Fatalf("reason = %s (%s), want %s — the stop classification stands even when the raise write fails", result.Reason, result.Detail, StopEscalation)
	}
	if l.reasonFailureEscalated {
		t.Error("reasonFailureEscalated set despite a FAILED chain raise; future episodes would be silently suppressed")
	}
	if got := countEscalations(t, g); got != 0 {
		t.Errorf("escalations = %d, want 0 (the append was rejected)", got)
	}
}

// strandingStore rejects the next agent.state.changed append once armed —
// modeling the OBSERVABLE rollback that strands an agent in Processing when
// its error-cleanup transition (Processing → Idle) cannot record
// (transitionLocked rolls the state back; codex r2 finding 1).
type strandingStore struct {
	store.Store
	arm *atomic.Bool
}

func (s strandingStore) Append(ev event.Event) (event.Event, error) {
	if s.arm.Load() && ev.Type() == event.EventTypeAgentStateChanged {
		s.arm.Store(false)
		return event.Event{}, errors.New("injected: state-cleanup append rejected")
	}
	return s.Store.Append(ev)
}

// strandingProvider fails its first call AND arms the store at that moment,
// so the very next state.changed append — the Reason error-cleanup
// transition — is the one rejected. Second call succeeds.
type strandingProvider struct {
	arm   *atomic.Bool
	calls atomic.Int32
}

func (p *strandingProvider) Name() string  { return "stranding" }
func (p *strandingProvider) Model() string { return "stranding-model" }

func (p *strandingProvider) Reason(_ context.Context, _ string, _ []event.Event) (decision.Response, error) {
	if p.calls.Add(1) == 1 {
		p.arm.Store(true)
		return decision.Response{}, err529
	}
	confidence, _ := types.NewScore(0.8)
	return decision.NewResponse(`/signal {"signal":"TASK_DONE"}`, confidence, decision.TokenUsage{InputTokens: 30, OutputTokens: 20}), nil
}

var _ intelligence.Provider = (*strandingProvider)(nil)

// TestReason_StrandedProcessingCleanupRecovers is codex r2 finding 1's exact
// repro: a 529 whose error-cleanup transition FAILS to record leaves the
// agent stranded in Processing (OBSERVABLE rollback). The retry loop must
// heal exactly that shape — gated reset, never an authority override — and
// complete the iteration. Without the heal, the retry dies on
// Processing → Processing and a retryable store hiccup becomes a terminal
// loop death.
func TestReason_StrandedProcessingCleanupRecovers(t *testing.T) {
	var arm atomic.Bool
	provider := &strandingProvider{arm: &arm}
	s := strandingStore{Store: store.NewInMemoryStore(), arm: &arm}
	as := actor.NewInMemoryActorStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	agent, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role("builder"),
		Name:     "stranded-cleanup-test",
		Graph:    g,
		Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
		Task:    "build a widget",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.reasonRetryBackoff = fastBackoff

	result := l.Run(context.Background())

	if result.Reason != StopTaskDone {
		t.Fatalf("reason = %s (%s), want %s — a stranded cleanup is a stranded state, not authority; the retry must heal it", result.Reason, result.Detail, StopTaskDone)
	}
	if got := int(provider.calls.Load()); got != 2 {
		t.Errorf("provider calls = %d, want 2 (fail+strand, then heal+succeed)", got)
	}
	if got := countEscalations(t, g); got != 0 {
		t.Errorf("escalations = %d, want 0 — the blip was absorbed", got)
	}
}

// TestRun_SuspendedAgentStaysSuspendedAndTerminal pins the end-to-end
// authority shape codex r2 finding 2 asked for: a REAL Guardian-suspended
// agent (not a provider-injected error string) refuses at its first
// state transition, the loop exits terminally, and — the load-bearing
// assertion — the agent is STILL suspended afterwards: no recovery path
// in the loop may revive it.
func TestRun_SuspendedAgentStaysSuspendedAndTerminal(t *testing.T) {
	provider := &flakyProvider{steps: []flakyStep{{resp: `/signal {"signal":"IDLE"}`}}}
	agent, g := agentWithGraph(t, provider)
	if err := agent.Suspend(); err != nil {
		t.Fatal(err)
	}

	l, err := New(Config{
		Agent:   agent,
		HumanID: humanID(),
		Budget:  resources.BudgetConfig{MaxIterations: 10},
		Task:    "build a widget",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.reasonRetryBackoff = fastBackoff

	result := l.Run(context.Background())

	if result.Reason != StopError {
		t.Fatalf("reason = %s (%s), want %s — a suspended agent's loop exits terminally", result.Reason, result.Detail, StopError)
	}
	if !strings.Contains(result.Detail, "invalid transition") {
		t.Errorf("detail %q must carry the refusal", result.Detail)
	}
	if got := agent.State(); got != egagent.StateSuspended {
		t.Fatalf("agent state = %s, want Suspended — nothing in the loop may revive a Guardian suspension", got)
	}
	if got := int(provider.callCount.Load()); got != 0 {
		t.Errorf("provider calls = %d, want 0 — the refusal happens before any provider call", got)
	}
	if got := countEscalations(t, g); got != 0 {
		t.Errorf("escalations = %d, want 0", got)
	}
}

// TestFormatLoopExit pins the immediate per-agent exit line RunConcurrent
// prints the moment any loop returns (v13-F1: a Result buried until daemon
// shutdown made a dead agent invisible to the operator and the sentinel).
func TestFormatLoopExit(t *testing.T) {
	line := formatLoopExit(AgentResult{
		Role: "implementer",
		Name: "implementer",
		Result: Result{
			Reason:     StopError,
			Iterations: 1,
			Detail:     "reason: API Error: 529 Overloaded",
		},
	})
	for _, want := range []string{"implementer", string(StopError), "1", "529"} {
		if !strings.Contains(line, want) {
			t.Errorf("exit line %q missing %q", line, want)
		}
	}
}
