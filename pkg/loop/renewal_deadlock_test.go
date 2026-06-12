package loop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/graph"
	"github.com/transpara-ai/eventgraph/go/pkg/intelligence"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/hive/pkg/budget"
	"github.com/transpara-ai/hive/pkg/resources"
	"github.com/transpara-ai/work"
)

// ════════════════════════════════════════════════════════════════════════
// slice-1 v15-F1: the renewal deadlock
//
// Round v15 proved both halves of the v14-F3 contract in production — the
// reviewer parked visibly at 22:14 and a renewed budget resumes a parked
// loop (pinned below in duration_park_test.go). But the park was
// STDOUT-ONLY: no chain event, so a quiescent society gave the allocator
// no wake, and the allocator had no recheck pulse of its own. The renewer
// slept while the renewables waited — 10.5 unobserved hours.
//
// The fix, three pieces, each pinned here:
//   (a) a duration park EMITS agent.budget.exhausted on the chain, once
//       per park episode — the wake edge for the renewer (the type is
//       wake-worthy: see wake_filter pins below);
//   (b) the park is registered in the BudgetRegistry (DurationParked), and
//       an allocator-role keepalive loop joins the recheck allowlist gated
//       on "any duration-parked renewable exists" — a belt for lost wake
//       edges, delta-gated so an unchanged park set can never storm the
//       allocator's iteration budget;
//   (c) a parked-target duration RENEWAL bypasses the stabilization window
//       and cooldowns (narrow allowlist: resource=duration, action raises
//       the limit, target verifiably parked) — a renewer that wakes but
//       refuses is the same deadlock with extra steps. Everything outside
//       that shape keeps every gate (fail closed).
// ════════════════════════════════════════════════════════════════════════

// TestRun_DurationParkEmitsBudgetExhaustedOnChain is the v15-F1(a) keystone:
// the park that was stdout-only in round 5 must put agent.budget.exhausted
// on the chain — the wake edge the sleeping allocator never got — exactly
// once per park episode, no matter how many spurious wakes re-enter the
// parked branch.
func TestRun_DurationParkEmitsBudgetExhaustedOnChain(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	cfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
	bi := resources.NewBudgetForTest(cfg, time.Now().Add(-time.Hour))
	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Bus:            g.Bus(),
		Keepalive:      true,
		Budget:         cfg,
		BudgetInstance: bi,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	count := func() int {
		page, err := g.Store().ByType(event.EventTypeAgentBudgetExhausted, 100, types.None[types.Cursor]())
		if err != nil {
			t.Fatalf("ByType(agent.budget.exhausted): %v", err)
		}
		return len(page.Items())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	// Park, not exit.
	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s); want park", r.Reason, r.Detail)
	case <-time.After(300 * time.Millisecond):
	}

	// The raise half: exactly one agent.budget.exhausted on the chain,
	// carrying the duration resource and sourced from the parked agent.
	if got := count(); got != 1 {
		t.Fatalf("agent.budget.exhausted events on the chain = %d; want exactly 1 (the park's wake edge for the allocator)", got)
	}
	page, err := g.Store().ByType(event.EventTypeAgentBudgetExhausted, 100, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType: %v", err)
	}
	ev := page.Items()[0]
	if ev.Source() != agent.ID() {
		t.Errorf("exhausted event source = %s; want the parked agent %s", ev.Source(), agent.ID())
	}
	c, ok := ev.Content().(event.AgentBudgetExhaustedContent)
	if !ok {
		t.Fatalf("exhausted event content type = %T; want AgentBudgetExhaustedContent", ev.Content())
	}
	if c.Resource != string(resources.ResourceDuration) {
		t.Errorf("exhausted Resource = %q; want %q", c.Resource, resources.ResourceDuration)
	}

	// Spurious wakes re-enter the parked branch while still exhausted; the
	// emit must dedupe per episode — one park, one event.
	for i := 0; i < 3; i++ {
		select {
		case l.wake <- struct{}{}:
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := count(); got != 1 {
		t.Fatalf("agent.budget.exhausted after wake spam = %d; want still exactly 1 (episode dedup)", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestRun_DurationParkSetsAndClearsRegistryMarker: the park registers itself
// in the BudgetRegistry (v15-F1b) so the allocator's recheck gate and the
// renewal exemption can key on EXPLICIT parked state — and clears the marker
// on resume so a renewed agent stops counting as a renewable.
func TestRun_DurationParkSetsAndClearsRegistryMarker(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	cfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
	bi := resources.NewBudgetForTest(cfg, time.Now().Add(-time.Hour))
	reg := resources.NewBudgetRegistry()
	reg.Register(agent.Name(), bi, 100, "test-model")
	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Bus:            g.Bus(),
		Keepalive:      true,
		Budget:         cfg,
		BudgetInstance: bi,
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	// Marker must appear while parked.
	deadline := time.Now().Add(2 * time.Second)
	for len(reg.DurationParkedNames()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := reg.DurationParkedNames(); len(got) != 1 || got[0] != agent.Name() {
		t.Fatalf("DurationParkedNames while parked = %v; want [%s]", got, agent.Name())
	}

	// Renewal + wake resumes — marker must clear.
	bi.SetMaxDuration(24 * time.Hour)
	select {
	case l.wake <- struct{}{}:
	default:
	}
	deadline = time.Now().Add(2 * time.Second)
	for len(reg.DurationParkedNames()) != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := reg.DurationParkedNames(); len(got) != 0 {
		t.Fatalf("DurationParkedNames after resume = %v; want empty (a renewed agent is no longer a renewable)", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// roleAgentWithGraph builds an agent of the given role sharing a graph, for
// tests that need both a specific role and bus/store access.
func roleAgentWithGraph(t *testing.T, provider intelligence.Provider, role, name string) (*hiveagent.Agent, *graph.Graph) {
	t.Helper()
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	work.RegisterWithRegistry(g.Registry())
	a, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role:     hiveagent.Role(role),
		Name:     name,
		Graph:    g,
		Provider: provider,
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, g
}

// allocatorRecheckLoop builds an allocator-role keepalive loop with a fast
// recheck tick and a shared registry, plus a worker entry the tests can park.
func allocatorRecheckLoop(t *testing.T) (*Loop, *mockProvider, *resources.BudgetRegistry, context.CancelFunc, chan Result) {
	t.Helper()
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := roleAgentWithGraph(t, provider, "allocator", "alloc-recheck-test")
	reg := resources.NewBudgetRegistry()
	workerBudget := resources.NewBudget(resources.BudgetConfig{MaxDuration: 30 * time.Minute})
	reg.Register("worker-a", workerBudget, 100, "test-model")
	reg.Register("worker-b", workerBudget, 100, "test-model")
	l, err := New(Config{
		Agent:           agent,
		HumanID:         humanID(),
		Bus:             g.Bus(),
		Keepalive:       true,
		Budget:          resources.BudgetConfig{MaxIterations: 100},
		BudgetRegistry:  reg,
		RecheckInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Run did not return after cancellation")
		}
	})
	return l, provider, reg, cancel, done
}

// waitForCallCount polls until the provider has made at least n calls.
func waitForCallCount(t *testing.T, p *mockProvider, n int, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for int(p.callCount.Load()) < n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	return int(p.callCount.Load()) >= n
}

// waitForStableCalls waits until the provider call count stops changing for
// quiet, then returns it. Keepalive loops run a short burst of iterations
// (two consecutive quiescent passes) before blocking on the wake channel —
// tests baseline on the stabilized count rather than pinning that mechanic.
func waitForStableCalls(t *testing.T, p *mockProvider, quiet time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	last := int(p.callCount.Load())
	stableSince := time.Now()
	for time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		cur := int(p.callCount.Load())
		if cur != last {
			last = cur
			stableSince = time.Now()
			continue
		}
		if time.Since(stableSince) >= quiet {
			return last
		}
	}
	t.Fatalf("provider call count never stabilized (last=%d)", last)
	return last
}

// TestRun_AllocatorRecheckWakesOnParkedRenewables is the v15-F1(b) keystone:
// an allocator-role keepalive loop joins the recheck allowlist, gated on
// "a duration-parked renewable exists" — so a park whose wake edge was lost
// (allocator mid-iteration, or park persisted by a prior daemon) still gets
// its renewer within one recheck tick, with NO bus event at all.
func TestRun_AllocatorRecheckWakesOnParkedRenewables(t *testing.T) {
	_, provider, reg, _, _ := allocatorRecheckLoop(t)

	// Boot iterations settle first.
	base := waitForStableCalls(t, provider, 300*time.Millisecond)

	// No bus event — only the registry marker changes.
	reg.SetDurationParked("worker-a", true)
	if !waitForCallCount(t, provider, base+1, 2*time.Second) {
		t.Fatalf("allocator never woke on a parked renewable via recheck (calls=%d, want > %d) — the renewal deadlock persists", provider.callCount.Load(), base)
	}
}

// TestRun_AllocatorRecheckDeltaGateNoStorm: an UNCHANGED park set must fire
// the recheck at most once. Without the delta gate a 50ms tick would run an
// LLM iteration per tick against a society whose allocator chose not to
// renew — burning its 150-iteration budget in minutes and killing the
// renewer (iteration exhaustion is terminal). The park set must CHANGE to
// fire again.
func TestRun_AllocatorRecheckDeltaGateNoStorm(t *testing.T) {
	_, provider, reg, _, _ := allocatorRecheckLoop(t)

	base := waitForStableCalls(t, provider, 300*time.Millisecond)
	reg.SetDurationParked("worker-a", true)
	if !waitForCallCount(t, provider, base+1, 2*time.Second) {
		t.Fatalf("first park fire never happened (calls=%d)", provider.callCount.Load())
	}
	afterFire := waitForStableCalls(t, provider, 300*time.Millisecond)

	// Same park set across many ticks: zero further fires.
	time.Sleep(500 * time.Millisecond) // ~10 ticks at 50ms
	if got := int(provider.callCount.Load()); got != afterFire {
		t.Fatalf("provider calls with unchanged park set grew %d -> %d; want no growth (delta gate must prevent the recheck storm)", afterFire, got)
	}

	// A NEW park changes the set — fires again.
	reg.SetDurationParked("worker-b", true)
	if !waitForCallCount(t, provider, afterFire+1, 2*time.Second) {
		t.Fatalf("allocator did not re-fire on a NEW parked renewable (calls=%d)", provider.callCount.Load())
	}
	afterSecond := waitForStableCalls(t, provider, 300*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	if got := int(provider.callCount.Load()); got != afterSecond {
		t.Fatalf("provider calls after second fire grew %d -> %d; want no growth", afterSecond, got)
	}
}

// TestRun_AllocatorRecheckResetsWhenSetEmpties: park → fire → resume (set
// empties) → the SAME agent parking again is a NEW episode and must fire
// again. Without the empty-reset, a renew-then-repark of the same worker
// would be invisible forever.
func TestRun_AllocatorRecheckResetsWhenSetEmpties(t *testing.T) {
	_, provider, reg, _, _ := allocatorRecheckLoop(t)

	base := waitForStableCalls(t, provider, 300*time.Millisecond)
	reg.SetDurationParked("worker-a", true)
	if !waitForCallCount(t, provider, base+1, 2*time.Second) {
		t.Fatalf("first park fire never happened (calls=%d)", provider.callCount.Load())
	}
	afterFire := waitForStableCalls(t, provider, 300*time.Millisecond)

	// Renewal happens; the set empties. Give the ticker time to observe it.
	reg.SetDurationParked("worker-a", false)
	time.Sleep(300 * time.Millisecond)

	// Same worker re-parks: new episode, must fire again.
	reg.SetDurationParked("worker-a", true)
	if !waitForCallCount(t, provider, afterFire+1, 2*time.Second) {
		t.Fatalf("allocator did not fire on a re-park after the set emptied (calls=%d) — renew-then-repark would deadlock", provider.callCount.Load())
	}
}

// TestRun_NonAllocatorKeepaliveNeverFiresOnParks pins the allowlist: parked
// renewables wake ONLY the allocator role. A pure-keepalive agent (no
// operate, no review duty, not allocator) with the same registry and tick
// must stay parked on its wake channel — re-check duties are an explicit
// allowlist, not a default.
func TestRun_NonAllocatorKeepaliveNeverFiresOnParks(t *testing.T) {
	for _, role := range []string{"strategist", "builder", "sysmon"} {
		t.Run(role, func(t *testing.T) {
			provider := newMockProvider(`/signal {"signal":"IDLE"}`)
			agent, g := roleAgentWithGraph(t, provider, role, "nonalloc-"+role)
			reg := resources.NewBudgetRegistry()
			reg.Register("worker-a", resources.NewBudget(resources.BudgetConfig{MaxDuration: 30 * time.Minute}), 100, "test-model")
			l, err := New(Config{
				Agent:           agent,
				HumanID:         humanID(),
				Bus:             g.Bus(),
				Keepalive:       true,
				Budget:          resources.BudgetConfig{MaxIterations: 100},
				BudgetRegistry:  reg,
				RecheckInterval: 50 * time.Millisecond,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan Result, 1)
			go func() { done <- l.Run(ctx) }()

			base := waitForStableCalls(t, provider, 300*time.Millisecond)
			reg.SetDurationParked("worker-a", true)
			time.Sleep(500 * time.Millisecond)
			if got := int(provider.callCount.Load()); got != base {
				t.Fatalf("%s provider calls after park grew %d -> %d; want no growth (only the allocator wakes on parked renewables)", role, base, got)
			}

			cancel()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("Run did not return after cancellation")
			}
		})
	}
}

// TestNew_AllocatorGetsRecheckDefault: the allocator joins the recheck
// default alongside the implementer and reviewer — an unset interval gets
// the 30s safety net, so no call site has to remember the new duty.
func TestNew_AllocatorGetsRecheckDefault(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := roleAgentWithGraph(t, provider, "allocator", "alloc-default-test")
	l, err := New(Config{
		Agent:     agent,
		HumanID:   humanID(),
		Bus:       g.Bus(),
		Keepalive: true,
		Budget:    resources.BudgetConfig{MaxIterations: 100},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := l.config.RecheckInterval; got != 30*time.Second {
		t.Fatalf("allocator RecheckInterval default = %v; want 30s (the renewer needs its safety-net pulse)", got)
	}
}

// ────────────────────────────────────────────────────────────────────
// v15-F1(c): the renewal exemption.
//
// A renewer that wakes but REFUSES is the same deadlock with extra steps:
// the allocator's own stabilization window (default 10 iterations) blocks
// renewals in a young society; the global cooldown (1 per 5 iterations)
// blocks renewal #2 when several agents park together (v14's close parked
// EIGHT simultaneously); the per-agent cooldown (10 iterations) blocks a
// renew-then-repark. In a quiescent society the allocator's iterations
// only advance on fires, so every one of those refusals is permanent.
//
// The exemption is a narrow allowlist: resource=duration AND the action
// RAISES the limit (increase/set) AND the target is verifiably parked in
// the registry. Everything outside that shape keeps every gate, and the
// constitutional self-renewal refusal holds even for a parked allocator.
// ────────────────────────────────────────────────────────────────────

// markParked registers the target as duration-parked in the loop's registry.
func markParked(l *Loop, name string) {
	l.config.BudgetRegistry.SetDurationParked(name, true)
}

func TestValidateBudget_ParkedDurationRenewalBypassesStabilizationWindow(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	markParked(l, "implementer")

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 30, Reason: "renew parked worker", Resource: "duration"}
	if err := l.validateBudgetCommand(cmd, 1); err != nil {
		t.Fatalf("parked-target duration renewal at iteration 1 = %v; want nil (a renewer that wakes but refuses is still a deadlock)", err)
	}
}

func TestValidateBudget_ParkedDurationRenewalBypassesCooldowns(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	markParked(l, "implementer")

	// A fresh adjustment to ANOTHER agent arms the global cooldown, and a
	// fresh adjustment to the target arms the per-agent cooldown.
	l.adjustmentHistory = append(l.adjustmentHistory,
		budget.AdjustmentRecord{Agent: "sysmon", Iteration: 14, Delta: 10, Reason: "x"},
		budget.AdjustmentRecord{Agent: "implementer", Iteration: 14, Delta: 10, Reason: "x"},
	)

	cmd := &BudgetCommand{Agent: "implementer", Action: "set", Amount: 60, Reason: "renew parked worker", Resource: "duration"}
	if err := l.validateBudgetCommand(cmd, 15); err != nil {
		t.Fatalf("parked-target duration renewal under cooldown = %v; want nil (eight simultaneous parks cannot wait 5 iterations each in a quiescent society)", err)
	}
}

func TestValidateBudget_NonParkedDurationKeepsAllGates(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	// implementer NOT parked.

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 30, Reason: "x", Resource: "duration"}
	err := l.validateBudgetCommand(cmd, 5)
	if err == nil {
		t.Fatal("non-parked duration increase inside the stabilization window passed; the exemption must require a verifiably parked target (fail closed)")
	}
	if !strings.Contains(err.Error(), "stabilization") {
		t.Errorf("error = %v; want stabilization window refusal", err)
	}

	// Cooldowns too.
	l.adjustmentHistory = append(l.adjustmentHistory,
		budget.AdjustmentRecord{Agent: "implementer", Iteration: 14, Delta: 10, Reason: "x"},
	)
	if err := l.validateBudgetCommand(cmd, 15); err == nil {
		t.Fatal("non-parked duration increase under per-agent cooldown passed; want refusal")
	}
}

func TestValidateBudget_ParkedDurationDecreaseNotExempt(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	markParked(l, "implementer")

	cmd := &BudgetCommand{Agent: "implementer", Action: "decrease", Amount: 10, Reason: "x", Resource: "duration"}
	err := l.validateBudgetCommand(cmd, 5)
	if err == nil {
		t.Fatal("parked-target duration DECREASE inside the window passed; only limit-raising renewals are exempt (a decrease cannot unpark anyone)")
	}
}

func TestValidateBudget_ParkedIterationCommandNotExempt(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	markParked(l, "implementer")

	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 25, Reason: "x"}
	err := l.validateBudgetCommand(cmd, 5)
	if err == nil {
		t.Fatal("iteration-resource command for a parked agent bypassed the window; the exemption is duration-only (iterations cannot unpark anyone)")
	}
}

func TestValidateBudget_ParkedSelfRenewalStillRefused(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	self := l.agent.Name()
	l.config.BudgetRegistry.Register(self, resources.NewBudget(resources.BudgetConfig{MaxIterations: 150}), 150, "")
	markParked(l, self)

	cmd := &BudgetCommand{Agent: self, Action: "set", Amount: 600, Reason: "x", Resource: "duration"}
	err := l.validateBudgetCommand(cmd, 15)
	if err == nil {
		t.Fatal("parked allocator self-renewal passed; the constitutional refusal must hold — the exemption can never weaken it")
	}
	if !strings.Contains(err.Error(), "self-renewal refused") {
		t.Errorf("error = %v; want the self-renewal refusal", err)
	}
}

func TestValidateBudget_ParkedDurationRenewalKeepsBasicValidity(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	markParked(l, "implementer")

	// Amount must still be positive.
	cmd := &BudgetCommand{Agent: "implementer", Action: "increase", Amount: 0, Reason: "x", Resource: "duration"}
	if err := l.validateBudgetCommand(cmd, 1); err == nil {
		t.Fatal("parked-target renewal with amount 0 passed; basic validity gates must survive the exemption")
	}
	// Unknown agents still refused (and an unknown name can never be parked).
	cmd = &BudgetCommand{Agent: "ghost", Action: "increase", Amount: 30, Reason: "x", Resource: "duration"}
	if err := l.validateBudgetCommand(cmd, 1); err == nil {
		t.Fatal("renewal for unknown agent passed; want refusal")
	}
}

func TestValidateBudget_ParkedDurationSetBelowCurrentNotExempt(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	// Give the implementer an explicit duration limit so "below current" is
	// well-defined: 120 minutes.
	l.config.BudgetRegistry.Register("implementer",
		resources.NewBudget(resources.BudgetConfig{MaxIterations: 100, MaxDuration: 120 * time.Minute}), 100, "")
	markParked(l, "implementer")

	// set 60 < current 120: a decrease in set clothing — it cannot unpark
	// anyone, so it must NOT ride the renewal exemption past the window.
	cmd := &BudgetCommand{Agent: "implementer", Action: "set", Amount: 60, Reason: "x", Resource: "duration"}
	if err := l.validateBudgetCommand(cmd, 5); err == nil {
		t.Fatal("parked-target duration SET below the current limit bypassed the window; the exemption requires a limit RAISE (fail closed)")
	}
}

// TestRun_ParkWakesAllocatorWhichRenewsAndWorkerResumes replays round v15's
// deadlock end-to-end and proves it broken: a worker parks on duration
// exhaustion (the reviewer at 22:14), the park's chain event wakes a LIVE
// allocator loop on the same bus, the allocator's /budget duration renewal
// passes validation (parked-target exemption) and applies to the SHARED
// budget instance, and the worker's park poll resumes it. In round 5 this
// exact sequence stalled for 10.5 hours because the park was stdout-only.
func TestRun_ParkWakesAllocatorWhichRenewsAndWorkerResumes(t *testing.T) {
	// One graph, two agents.
	s := store.NewInMemoryStore()
	as := actor.NewInMemoryActorStore()
	g := graph.New(s, as)
	if err := g.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	work.RegisterWithRegistry(g.Registry())

	const workerName = "impl-keystone"

	workerProvider := newMockProvider(`/signal {"signal":"IDLE"}`)
	worker, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role: hiveagent.Role("reviewer"), Name: workerName, Graph: g, Provider: workerProvider,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The allocator settles on two IDLE boot iterations, then answers its
	// park wake with the renewal, then goes idle again.
	allocProvider := newMockProvider(
		`/signal {"signal":"IDLE"}`,
		`/signal {"signal":"IDLE"}`,
		`/budget {"agent":"`+workerName+`","action":"set","amount":120,"reason":"renew parked reviewer","resource":"duration"}`,
		`/signal {"signal":"IDLE"}`,
	)
	alloc, err := hiveagent.New(context.Background(), hiveagent.Config{
		Role: hiveagent.Role("allocator"), Name: "alloc-keystone", Graph: g, Provider: allocProvider,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Shared registry; the worker's budget instance is duration-exhausted.
	reg := resources.NewBudgetRegistry()
	workerCfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
	workerBudget := resources.NewBudgetForTest(workerCfg, time.Now().Add(-time.Hour))
	reg.Register(workerName, workerBudget, 100, "test-model")

	allocLoop, err := New(Config{
		Agent:           alloc,
		HumanID:         humanID(),
		Bus:             g.Bus(),
		Keepalive:       true,
		Budget:          resources.BudgetConfig{MaxIterations: 100},
		BudgetRegistry:  reg,
		RecheckInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New(allocator): %v", err)
	}
	workerLoop, err := New(Config{
		Agent:           worker,
		HumanID:         humanID(),
		Bus:             g.Bus(),
		Keepalive:       true,
		Budget:          workerCfg,
		BudgetInstance:  workerBudget,
		BudgetRegistry:  reg,
		RecheckInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New(worker): %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	allocDone := make(chan Result, 1)
	go func() { allocDone <- allocLoop.Run(ctx) }()
	// Allocator settles first so its bus subscription is live before the
	// park raises (the recheck belt covers the other ordering; this test
	// exercises the wake path deterministically).
	waitForStableCalls(t, allocProvider, 300*time.Millisecond)

	workerDone := make(chan Result, 1)
	go func() { workerDone <- workerLoop.Run(ctx) }()

	// The worker must park (zero provider calls) and then RESUME once the
	// allocator's renewal lands — the full loop, no human in the path.
	if !waitForCallCount(t, workerProvider, 1, 5*time.Second) {
		t.Fatalf("worker never resumed: the renewal deadlock persists (worker calls=%d, allocator calls=%d, parked=%v)",
			workerProvider.callCount.Load(), allocProvider.callCount.Load(), reg.DurationParkedNames())
	}

	// The chain must carry the whole story: the park's raise and the renewal.
	exhausted, err := g.Store().ByType(event.EventTypeAgentBudgetExhausted, 100, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(exhausted): %v", err)
	}
	if len(exhausted.Items()) < 1 {
		t.Errorf("agent.budget.exhausted events = %d; want >= 1 (the park must raise on-chain)", len(exhausted.Items()))
	}
	adjusted, err := g.Store().ByType(event.EventTypeAgentBudgetAdjusted, 100, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(adjusted): %v", err)
	}
	renewalSeen := false
	for _, ev := range adjusted.Items() {
		if c, ok := ev.Content().(event.AgentBudgetAdjustedContent); ok &&
			c.AgentName == workerName && c.Resource == "duration" {
			renewalSeen = true
		}
	}
	if !renewalSeen {
		t.Errorf("no agent.budget.adjusted duration renewal for %s on the chain; the renewal must be recorded", workerName)
	}

	// And the registry marker must have cleared on resume.
	if parked := reg.DurationParkedNames(); len(parked) != 0 {
		t.Errorf("DurationParkedNames after resume = %v; want empty", parked)
	}

	cancel()
	for _, ch := range []chan Result{allocDone, workerDone} {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatal("a loop did not return after cancellation")
		}
	}
}

// TestWakeFilter_BudgetExhaustedStaysWakeWorthyAndObservable pins the wake
// plumbing the whole fix rides on: agent.budget.exhausted must wake
// subscribers and appear in observations. The filters are denylists today,
// so this passes by default — the pin exists so a future allowlist
// conversion or denylist addition cannot silently drop the park's raise
// (which would resurrect the renewal deadlock with no failing test).
func TestWakeFilter_BudgetExhaustedStaysWakeWorthyAndObservable(t *testing.T) {
	if !isWakeWorthy(event.EventTypeAgentBudgetExhausted) {
		t.Error("agent.budget.exhausted is not wake-worthy; the park's raise would wake nobody and the renewal deadlock returns")
	}
	if !isObservable(event.EventTypeAgentBudgetExhausted) {
		t.Error("agent.budget.exhausted is not observable; the allocator would wake blind to WHY")
	}
	if !isWakeWorthy(event.EventTypeAgentBudgetAdjusted) {
		t.Error("agent.budget.adjusted is not wake-worthy; renewals must remain visible wake edges")
	}
}

// TestEnrichBudgetObservation_MarksParkedAgents: the allocator's budget
// table must say PARKED explicitly. The dur= column alone made the parked
// state an inference; the marker makes it a fact the LLM can act on.
func TestEnrichBudgetObservation_MarksParkedAgents(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	markParked(l, "implementer")

	obs := l.enrichBudgetObservation("base", 5)
	if !strings.Contains(obs, "PARKED(duration)") {
		t.Fatalf("allocator observation lacks PARKED(duration) marker for a parked agent:\n%s", obs)
	}
	if strings.Count(obs, "PARKED(duration)") != 1 {
		t.Fatalf("PARKED(duration) marker count = %d; want exactly 1 (only the parked agent)", strings.Count(obs, "PARKED(duration)"))
	}
}

// ────────────────────────────────────────────────────────────────────
// codex r1 nonblockers (2026-06-12), each pinned red-first.
// ────────────────────────────────────────────────────────────────────

// TestRun_DurationParkMarkerSurvivesSpuriousWakes pins codex r1 #1: a wake
// that does NOT carry a renewal must not flicker the DurationParked marker.
// The clear belongs at the proven-resume site (budget check passes) and at
// shutdown — never on the wake edge itself, where a still-exhausted loop
// would briefly read as unparked and the allocator's empty-set reset would
// treat the SAME park as a new episode (delta-gate invariant violation).
func TestRun_DurationParkMarkerSurvivesSpuriousWakes(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	cfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
	bi := resources.NewBudgetForTest(cfg, time.Now().Add(-time.Hour))
	reg := resources.NewBudgetRegistry()
	reg.Register(agent.Name(), bi, 100, "test-model")
	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Bus:            g.Bus(),
		Keepalive:      true,
		Budget:         cfg,
		BudgetInstance: bi,
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for len(reg.DurationParkedNames()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(reg.DurationParkedNames()) == 0 {
		t.Fatal("never parked")
	}

	// Hammer spurious wakes while a sampler watches for any empty window.
	sawEmpty := make(chan struct{}, 1)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				if len(reg.DurationParkedNames()) == 0 {
					select {
					case sawEmpty <- struct{}{}:
					default:
					}
					return
				}
			}
		}
	}()
	for i := 0; i < 50; i++ {
		select {
		case l.wake <- struct{}{}:
		default:
		}
		time.Sleep(2 * time.Millisecond)
	}
	close(stop)
	select {
	case <-sawEmpty:
		t.Fatal("DurationParked flickered empty on a spurious wake; the marker must clear only on proven resume or shutdown")
	default:
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestRun_DurationParkMarkerVisibleWhenRaiseLands pins codex r1 #2: the
// registry marker must be set BEFORE agent.budget.exhausted is emitted, so
// any observer the raise wakes — however the bus schedules delivery — reads
// PARKED(duration) for the source agent. Marker-after-emit leaves the
// immediate wake path racing the registry write.
func TestRun_DurationParkMarkerVisibleWhenRaiseLands(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	cfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
	bi := resources.NewBudgetForTest(cfg, time.Now().Add(-time.Hour))
	reg := resources.NewBudgetRegistry()
	reg.Register(agent.Name(), bi, 100, "test-model")

	markerAtDelivery := make(chan bool, 4)
	g.Bus().Subscribe(types.MustSubscriptionPattern("agent.budget.exhausted"), func(ev event.Event) {
		markerAtDelivery <- len(reg.DurationParkedNames()) > 0
	})

	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Bus:            g.Bus(),
		Keepalive:      true,
		Budget:         cfg,
		BudgetInstance: bi,
		BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	select {
	case visible := <-markerAtDelivery:
		if !visible {
			t.Fatal("agent.budget.exhausted delivered while DurationParked was unset; the marker must be set before the raise")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("raise never delivered")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestValidateBudget_ParkedRenewalAtCeilingNotExempt pins codex r1 #3: a
// parked target already AT the duration ceiling cannot be raised — any
// "increase", or "set" that clamps, is a guaranteed no-op that would ride
// the exemption past the timing gates, burn the allocator's fire, emit a
// zero-delta adjustment, and leave the target parked. No raise possible →
// no exemption (fail closed); the ceiling is the designed epoch bound.
func TestValidateBudget_ParkedRenewalAtCeilingNotExempt(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	ceiling := budget.LoadConfig().DurationCeilingMin
	l.config.BudgetRegistry.Register("maxed",
		resources.NewBudget(resources.BudgetConfig{MaxIterations: 100, MaxDuration: time.Duration(ceiling) * time.Minute}), 100, "")
	markParked(l, "maxed")

	for _, cmd := range []*BudgetCommand{
		{Agent: "maxed", Action: "increase", Amount: 30, Reason: "x", Resource: "duration"},
		{Agent: "maxed", Action: "set", Amount: ceiling + 100, Reason: "x", Resource: "duration"},
	} {
		if err := l.validateBudgetCommand(cmd, 1); err == nil {
			t.Errorf("%s at ceiling bypassed the timing gates; a guaranteed-clamp no-op must not be exempt", cmd.Action)
		}
	}

	// Boundary: one minute below the ceiling is still raisable — exempt.
	l.config.BudgetRegistry.Register("almost",
		resources.NewBudget(resources.BudgetConfig{MaxIterations: 100, MaxDuration: time.Duration(ceiling-1) * time.Minute}), 100, "")
	markParked(l, "almost")
	cmd := &BudgetCommand{Agent: "almost", Action: "increase", Amount: 30, Reason: "x", Resource: "duration"}
	if err := l.validateBudgetCommand(cmd, 1); err != nil {
		t.Errorf("increase one minute below ceiling = %v; want exempt (still raisable)", err)
	}
}

// ────────────────────────────────────────────────────────────────────
// codex r2 blockers (2026-06-12), each pinned red-first.
// ────────────────────────────────────────────────────────────────────

// TestRun_DurationParkMarkerNeverSurvivesLoopDeath pins codex r2 #1+#2 as a
// CLASS: whatever path a parked loop dies through — cancellation winning
// the race after a wake, a non-duration budget failure surfacing after a
// renewal, any future return — the DurationParked marker must not survive
// Run() returning. A stale marker makes the allocator renew a corpse and
// wedges the park-set signature on a dead name. The fix owns the clear at
// the result() chokepoint, so the property holds for every death path by
// construction, not per-path whack-a-mole.
func TestRun_DurationParkMarkerNeverSurvivesLoopDeath(t *testing.T) {
	t.Run("cancellation wins after a wake", func(t *testing.T) {
		provider := newMockProvider(`/signal {"signal":"IDLE"}`)
		agent, g := agentWithGraph(t, provider)
		cfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
		bi := resources.NewBudgetForTest(cfg, time.Now().Add(-time.Hour))
		reg := resources.NewBudgetRegistry()
		reg.Register(agent.Name(), bi, 100, "test-model")
		l, err := New(Config{
			Agent: agent, HumanID: humanID(), Bus: g.Bus(), Keepalive: true,
			Budget: cfg, BudgetInstance: bi, BudgetRegistry: reg,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan Result, 1)
		go func() { done <- l.Run(ctx) }()

		deadline := time.Now().Add(2 * time.Second)
		for len(reg.DurationParkedNames()) == 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if len(reg.DurationParkedNames()) == 0 {
			t.Fatal("never parked")
		}

		// Cancel and wake in a deliberate race: whichever select arm wins,
		// the loop dies (ctx.Err at loop top, or ctx.Done in the wait).
		cancel()
		for i := 0; i < 10; i++ {
			select {
			case l.wake <- struct{}{}:
			default:
			}
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not return")
		}
		if got := reg.DurationParkedNames(); len(got) != 0 {
			t.Fatalf("DurationParked = %v after Run returned; a dead loop must never read as a renewable", got)
		}
	})

	t.Run("non-duration exhaustion surfaces after renewal", func(t *testing.T) {
		provider := newMockProvider(`/signal {"signal":"IDLE"}`)
		agent, g := agentWithGraph(t, provider)
		cfg := resources.BudgetConfig{MaxIterations: 100, MaxDuration: 30 * time.Minute}
		bi := resources.NewBudgetForTest(cfg, time.Now().Add(-time.Hour))
		bi.SeedConsumed(25, 0, 0) // used=25 passes max=100 at park entry
		reg := resources.NewBudgetRegistry()
		reg.Register(agent.Name(), bi, 100, "test-model")
		l, err := New(Config{
			Agent: agent, HumanID: humanID(), Bus: g.Bus(), Keepalive: true,
			Budget: cfg, BudgetInstance: bi, BudgetRegistry: reg,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan Result, 1)
		go func() { done <- l.Run(ctx) }()

		deadline := time.Now().Add(2 * time.Second)
		for len(reg.DurationParkedNames()) == 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if len(reg.DurationParkedNames()) == 0 {
			t.Fatal("never parked")
		}

		// While parked: the duration renewal lands AND the allocator
		// decreases iterations below the seeded usage (prod-reachable via
		// /budget decrease). The resume re-check now fails on ITERATIONS —
		// the terminal, non-parking branch.
		bi.SetMaxDuration(24 * time.Hour)
		if _, _, err := reg.AdjustMaxIterations(agent.Name(), -85, 10, 500); err != nil {
			t.Fatalf("AdjustMaxIterations: %v", err)
		}
		select {
		case l.wake <- struct{}{}:
		default:
		}
		select {
		case r := <-done:
			if r.Reason != StopBudget {
				t.Fatalf("Run returned %s (%s); want StopBudget on iterations", r.Reason, r.Detail)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run did not return on non-duration exhaustion")
		}
		if got := reg.DurationParkedNames(); len(got) != 0 {
			t.Fatalf("DurationParked = %v after a non-duration death; the marker leaked through the terminal StopBudget path", got)
		}
	})
}

// TestValidateBudget_InsufficientRenewalNotExempt pins codex r2 #3
// (validate side): the exemption exists to UNPARK — a renewal whose
// post-clamp limit still trails the target's elapsed wall-clock cannot
// unpark anyone. It must not ride the exemption: it would burn the
// allocator's one fire for this park set, leave the signature unchanged,
// and resurrect the renewal deadlock with the renewer believing it acted.
func TestValidateBudget_InsufficientRenewalNotExempt(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	// Parked 10 hours past a 30m limit: elapsed ≈ 600 minutes.
	stale := resources.NewBudgetForTest(
		resources.BudgetConfig{MaxIterations: 100, MaxDuration: 30 * time.Minute},
		time.Now().Add(-10*time.Hour))
	l.config.BudgetRegistry.Register("stale-worker", stale, 100, "")
	markParked(l, "stale-worker")

	for _, tc := range []struct {
		name   string
		cmd    *BudgetCommand
		exempt bool
	}{
		{"increase 30 -> 60m, far below 600m elapsed", &BudgetCommand{Agent: "stale-worker", Action: "increase", Amount: 30, Reason: "x", Resource: "duration"}, false},
		{"set 500 < 600m elapsed", &BudgetCommand{Agent: "stale-worker", Action: "set", Amount: 500, Reason: "x", Resource: "duration"}, false},
		{"set 601 > 600m elapsed", &BudgetCommand{Agent: "stale-worker", Action: "set", Amount: 601, Reason: "x", Resource: "duration"}, true},
		{"increase 600 -> 630m > elapsed", &BudgetCommand{Agent: "stale-worker", Action: "increase", Amount: 600, Reason: "x", Resource: "duration"}, true},
		{"set 800 clamps to 720 ceiling, still > elapsed", &BudgetCommand{Agent: "stale-worker", Action: "set", Amount: 800, Reason: "x", Resource: "duration"}, true},
	} {
		err := l.validateBudgetCommand(tc.cmd, 1) // iteration 1: only the exemption passes the window
		if tc.exempt && err != nil {
			t.Errorf("%s: want exempt (sufficient renewal), got %v", tc.name, err)
		}
		if !tc.exempt && err == nil {
			t.Errorf("%s: insufficient renewal rode the exemption; it cannot unpark and must keep every timing gate", tc.name)
		}
	}
}

// TestValidateBudget_InsufficientBeyondCeilingNeverExempt: a worker parked
// longer than the ceiling allows (elapsed > 720m) can NEVER be renewed
// past its elapsed — every renewal clamps below elapsed. No command may
// ride the exemption for it; the epoch ceiling is where the society ends.
func TestValidateBudget_InsufficientBeyondCeilingNeverExempt(t *testing.T) {
	l := makeLoopWithRegistry(t, "allocator")
	ancient := resources.NewBudgetForTest(
		resources.BudgetConfig{MaxIterations: 100, MaxDuration: 30 * time.Minute},
		time.Now().Add(-13*time.Hour)) // 780m elapsed > 720m ceiling
	l.config.BudgetRegistry.Register("ancient-worker", ancient, 100, "")
	markParked(l, "ancient-worker")

	for _, cmd := range []*BudgetCommand{
		{Agent: "ancient-worker", Action: "increase", Amount: 700, Reason: "x", Resource: "duration"},
		{Agent: "ancient-worker", Action: "set", Amount: 800, Reason: "x", Resource: "duration"},
	} {
		if err := l.validateBudgetCommand(cmd, 1); err == nil {
			t.Errorf("%s for beyond-ceiling worker rode the exemption; post-clamp limit can never exceed its elapsed", cmd.Action)
		}
	}
}

// TestRun_DurationParkReRaisesOnInsufficientRenewal pins codex r2 #3
// (worker side, the belt): a renewal that lands but does NOT unpark the
// worker must trigger a fresh agent.budget.exhausted raise — the limit
// CHANGED while the park persisted, so the renewer acted on a stale
// picture and needs a new wake. Spurious wakes without a limit change must
// still not re-raise (episode dedup holds).
func TestRun_DurationParkReRaisesOnInsufficientRenewal(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	cfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
	bi := resources.NewBudgetForTest(cfg, time.Now().Add(-2*time.Hour))
	reg := resources.NewBudgetRegistry()
	reg.Register(agent.Name(), bi, 100, "test-model")
	l, err := New(Config{
		Agent: agent, HumanID: humanID(), Bus: g.Bus(), Keepalive: true,
		Budget: cfg, BudgetInstance: bi, BudgetRegistry: reg,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	count := func() int {
		page, err := g.Store().ByType(event.EventTypeAgentBudgetExhausted, 100, types.None[types.Cursor]())
		if err != nil {
			t.Fatalf("ByType: %v", err)
		}
		return len(page.Items())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := count(); got != 1 {
		t.Fatalf("initial raise count = %d; want 1", got)
	}

	// An INSUFFICIENT renewal: 2h elapsed, limit raised to only 60m. The
	// wake re-enters the park branch, the budget still fails, the limit
	// changed -> the worker must raise again.
	bi.SetMaxDuration(time.Hour)
	select {
	case l.wake <- struct{}{}:
	default:
	}
	deadline = time.Now().Add(2 * time.Second)
	for count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := count(); got != 2 {
		t.Fatalf("raise count after insufficient renewal = %d; want 2 (the renewer needs a fresh wake — without it the deadlock resurrects)", got)
	}

	// Spurious wakes with NO limit change: no further raises.
	for i := 0; i < 3; i++ {
		select {
		case l.wake <- struct{}{}:
		default:
		}
		time.Sleep(30 * time.Millisecond)
	}
	if got := count(); got != 2 {
		t.Fatalf("raise count after spurious wakes = %d; want still 2 (episode dedup must hold)", got)
	}

	// A SUFFICIENT renewal resumes the worker.
	bi.SetMaxDuration(24 * time.Hour)
	select {
	case l.wake <- struct{}{}:
	default:
	}
	if !waitForCallCount(t, provider, 1, 2*time.Second) {
		t.Fatalf("worker never resumed after sufficient renewal")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestRun_DurationParkReRaisesWithoutAnyWake pins codex r3's blocker: an
// insufficient renewal whose agent.budget.adjusted emit FAILS mutates the
// live limit but delivers no wake — and a poll that only checks for a full
// budget pass leaves the re-raise belt unreachable. The park's recheck tick
// must notice ANY unacknowledged limit change (acknowledged = at episode
// entry or on a successful raise), re-enter the park branch, and raise
// afresh — with NO wake involved at any point. The follow-up sufficient
// renewal must also resume the worker wake-free (the v14 poll contract).
func TestRun_DurationParkReRaisesWithoutAnyWake(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	cfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
	bi := resources.NewBudgetForTest(cfg, time.Now().Add(-2*time.Hour))
	reg := resources.NewBudgetRegistry()
	reg.Register(agent.Name(), bi, 100, "test-model")
	l, err := New(Config{
		Agent: agent, HumanID: humanID(), Bus: g.Bus(), Keepalive: true,
		Budget: cfg, BudgetInstance: bi, BudgetRegistry: reg,
		RecheckInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	count := func() int {
		page, err := g.Store().ByType(event.EventTypeAgentBudgetExhausted, 100, types.None[types.Cursor]())
		if err != nil {
			t.Fatalf("ByType: %v", err)
		}
		return len(page.Items())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for count() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := count(); got != 1 {
		t.Fatalf("initial raise count = %d; want 1", got)
	}

	// Insufficient renewal lands with NO event and NO wake (the failed-emit
	// shape): 2h elapsed, limit only reaches 60m.
	bi.SetMaxDuration(time.Hour)
	deadline = time.Now().Add(2 * time.Second)
	for count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := count(); got != 2 {
		t.Fatalf("raise count after wake-less insufficient renewal = %d; want 2 — the park's tick must notice an unacknowledged limit change or the deadlock resurrects through the failed-emit path", got)
	}

	// The re-raise acknowledged the new limit: ticks stay quiet now.
	time.Sleep(300 * time.Millisecond)
	if got := count(); got != 2 {
		t.Fatalf("raise count grew to %d on a stable limit; want still 2 (one raise per unacknowledged change, no tick storm)", got)
	}

	// Sufficient renewal, still wake-free: the poll resumes the worker.
	bi.SetMaxDuration(24 * time.Hour)
	if !waitForCallCount(t, provider, 1, 2*time.Second) {
		t.Fatalf("worker never resumed after wake-free sufficient renewal")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestRun_DurationParkRaceRenewalDuringRaiseStillDetected pins codex r4's
// blocker as a property: a renewal that lands WHILE the raise is being
// published (injected from the exhausted-event subscriber, the earliest
// hook an external actor has) must still be detected — the acknowledged
// limit is captured BEFORE the publish, so any change after that read is
// unacknowledged by construction and produces a follow-up raise with no
// wake required. Pre-fix the ack read the limit AFTER publishing, so a
// fast renewal inside the window was swallowed as already-acknowledged.
// (The adverse interleaving is sub-microsecond and not reliably forcible
// in-process — this is the by-construction pin, per the r1-#2 precedent.)
func TestRun_DurationParkRaceRenewalDuringRaiseStillDetected(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	cfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
	bi := resources.NewBudgetForTest(cfg, time.Now().Add(-2*time.Hour))
	reg := resources.NewBudgetRegistry()
	reg.Register(agent.Name(), bi, 100, "test-model")

	// On the FIRST raise delivery, inject an insufficient renewal with no
	// event and no wake (the failed-emit shape) — as close to "during the
	// publish" as an external actor can get.
	injected := false
	g.Bus().Subscribe(types.MustSubscriptionPattern("agent.budget.exhausted"), func(ev event.Event) {
		if !injected {
			injected = true
			bi.SetMaxDuration(time.Hour) // 60m, still below 120m elapsed
		}
	})

	l, err := New(Config{
		Agent: agent, HumanID: humanID(), Bus: g.Bus(), Keepalive: true,
		Budget: cfg, BudgetInstance: bi, BudgetRegistry: reg,
		RecheckInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	count := func() int {
		page, err := g.Store().ByType(event.EventTypeAgentBudgetExhausted, 100, types.None[types.Cursor]())
		if err != nil {
			t.Fatalf("ByType: %v", err)
		}
		return len(page.Items())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	// The injected mid-raise renewal must surface as a SECOND raise via the
	// wake-free detector.
	deadline := time.Now().Add(3 * time.Second)
	for count() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := count(); got < 2 {
		t.Fatalf("raise count after mid-raise insufficient renewal = %d; want >= 2 (a renewal landing during the publish window must not be swallowed as acknowledged)", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}
