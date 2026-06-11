package loop

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/resources"
)

// ════════════════════════════════════════════════════════════════════════
// slice-1 v14-F3(b): duration exhaustion parks keepalive loops
//
// Round v14's resume epoch ran past the 30-minute MaxDuration default and
// all eight non-implementer agents exited simultaneously — the society has
// a built-in lifespan nobody chose. A keepalive loop with a bus now PARKS
// on duration exhaustion (raise-and-park family: v8-F2 escalations,
// v13-F1 reason failures) and resumes when the allocator renews its limit.
//
// The park is an ALLOWLIST of exactly one resource: duration. Iterations,
// tokens, cost, and any future budget resource keep the terminal stop —
// the default branch exits (fail closed).
// ════════════════════════════════════════════════════════════════════════

// durationParkLoop builds a keepalive Loop whose budget is already
// duration-exhausted at entry (aged start, 30m limit).
func durationParkLoop(t *testing.T, keepalive bool, cfg resources.BudgetConfig, age time.Duration, responses ...string) (*Loop, *mockProvider, *resources.Budget) {
	t.Helper()
	provider := newMockProvider(responses...)
	agent, g := agentWithGraph(t, provider)
	bi := resources.NewBudgetForTest(cfg, time.Now().Add(-age))
	l, err := New(Config{
		Agent:          agent,
		HumanID:        humanID(),
		Bus:            g.Bus(),
		Keepalive:      keepalive,
		Budget:         cfg,
		BudgetInstance: bi,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l, provider, bi
}

// TestRun_KeepaliveDurationExhaustionParksAndRenewalResumes is the keystone:
// a duration-exhausted keepalive loop must NOT return — it parks before
// burning any provider call — and an allocator renewal plus a wake must
// resume iteration on the same live loop.
func TestRun_KeepaliveDurationExhaustionParksAndRenewalResumes(t *testing.T) {
	l, provider, bi := durationParkLoop(t, true,
		resources.BudgetConfig{MaxDuration: 30 * time.Minute}, time.Hour,
		`/signal {"signal":"IDLE"}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	// Must park, not exit — and parked means ZERO provider calls.
	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s); duration exhaustion on a keepalive loop must park, not kill the agent", r.Reason, r.Detail)
	case <-time.After(300 * time.Millisecond):
	}
	if got := int(provider.callCount.Load()); got != 0 {
		t.Fatalf("provider calls while duration-parked = %d; want 0 (the park must precede the iteration)", got)
	}

	// Renewal (the allocator's SetMaxDuration write-through) + wake resumes.
	bi.SetMaxDuration(24 * time.Hour)
	select {
	case l.wake <- struct{}{}:
	default:
		t.Fatal("wake channel unexpectedly full; the parked loop never drained it")
	}
	deadline := time.Now().Add(2 * time.Second)
	for int(provider.callCount.Load()) < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := int(provider.callCount.Load()); got < 1 {
		t.Fatalf("provider calls after renewal+wake = %d; want >= 1 (the loop must resume on the same goroutine)", got)
	}

	// Shutdown still terminates promptly.
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

// TestRun_KeepaliveDurationParkRenewalResumesWithoutWake: a renewal must
// resume the parked loop even when NO bus event matches the agent's
// subscriptions — the renewal event itself may not (only some agents watch
// budget.*), and a society whose workers are all parked generates no other
// traffic. The park polls the in-memory budget on the recheck tick; a wake
// that never comes must not park a renewed agent forever (the v13/v14
// silent-wait class, again).
func TestRun_KeepaliveDurationParkRenewalResumesWithoutWake(t *testing.T) {
	provider := newMockProvider(`/signal {"signal":"IDLE"}`)
	agent, g := agentWithGraph(t, provider)
	cfg := resources.BudgetConfig{MaxDuration: 30 * time.Minute}
	bi := resources.NewBudgetForTest(cfg, time.Now().Add(-time.Hour))
	l, err := New(Config{
		Agent:           agent,
		HumanID:         humanID(),
		Bus:             g.Bus(),
		Keepalive:       true,
		Budget:          cfg,
		BudgetInstance:  bi,
		RecheckInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s); want park", r.Reason, r.Detail)
	case <-time.After(200 * time.Millisecond):
	}

	// Renew WITHOUT any wake: only the recheck tick can observe this.
	bi.SetMaxDuration(24 * time.Hour)
	deadline := time.Now().Add(2 * time.Second)
	for int(provider.callCount.Load()) < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := int(provider.callCount.Load()); got < 1 {
		t.Fatalf("provider calls after no-wake renewal = %d; want >= 1 — a renewed agent must not stay parked waiting for a wake that never comes", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// TestRun_KeepaliveDurationParkShutdownNamesThePark: cancellation while
// parked must return a named StopBudget result, not read as a live stop.
func TestRun_KeepaliveDurationParkShutdownNamesThePark(t *testing.T) {
	l, _, _ := durationParkLoop(t, true,
		resources.BudgetConfig{MaxDuration: 30 * time.Minute}, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() { done <- l.Run(ctx) }()

	select {
	case r := <-done:
		t.Fatalf("Run returned %s (%s) before cancellation; want park", r.Reason, r.Detail)
	case <-time.After(300 * time.Millisecond):
	}
	cancel()
	select {
	case r := <-done:
		if r.Reason != StopBudget {
			t.Errorf("shutdown result = %s; want %s", r.Reason, StopBudget)
		}
		if !strings.Contains(r.Detail, "shutdown while parked on budget exhaustion") {
			t.Errorf("shutdown detail %q must name the parked-on-budget state", r.Detail)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

// TestRun_NonKeepaliveDurationExhaustionStillStops: one-shot loops keep the
// terminal StopBudget — parking without a wake source would hang forever.
func TestRun_NonKeepaliveDurationExhaustionStillStops(t *testing.T) {
	l, _, _ := durationParkLoop(t, false,
		resources.BudgetConfig{MaxDuration: 30 * time.Minute}, time.Hour)

	done := make(chan Result, 1)
	go func() { done <- l.Run(context.Background()) }()
	select {
	case r := <-done:
		if r.Reason != StopBudget {
			t.Fatalf("non-keepalive duration exhaustion = %s (%s); want %s", r.Reason, r.Detail, StopBudget)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return for a non-keepalive duration-exhausted loop")
	}
}

// TestRun_KeepaliveNonDurationBudgetExhaustionStillStops sweeps the budget
// resource domain: every resource EXCEPT duration must keep the terminal
// stop on a keepalive loop. This is the allowlist proof — if a future
// resource is added to Budget.Check, it exits unless someone explicitly
// proves parking is safe for it.
func TestRun_KeepaliveNonDurationBudgetExhaustionStillStops(t *testing.T) {
	cases := []struct {
		name    string
		cfg     resources.BudgetConfig
		consume func(b *resources.Budget)
	}{
		{"iterations", resources.BudgetConfig{MaxIterations: 1}, func(b *resources.Budget) { b.Record(0, 0) }},
		{"tokens", resources.BudgetConfig{MaxTokens: 1}, func(b *resources.Budget) { b.Record(2, 0) }},
		{"cost", resources.BudgetConfig{MaxCostUSD: 0.01}, func(b *resources.Budget) { b.Record(0, 0.02) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, _, bi := durationParkLoop(t, true, tc.cfg, 0)
			tc.consume(bi)

			done := make(chan Result, 1)
			go func() { done <- l.Run(context.Background()) }()
			select {
			case r := <-done:
				if r.Reason != StopBudget {
					t.Fatalf("%s exhaustion = %s (%s); want terminal %s — only duration parks", tc.name, r.Reason, r.Detail, StopBudget)
				}
				if !strings.Contains(r.Detail, tc.name) {
					t.Errorf("detail %q must name the exhausted resource %q", r.Detail, tc.name)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("Run did not return for keepalive %s exhaustion; parking is allowlisted to duration only", tc.name)
			}
		})
	}
}

func TestFormatBudgetPark(t *testing.T) {
	err := (&resources.BudgetExceededError{Resource: resources.ResourceDuration, Used: "31m0s", Limit: "30m0s"})
	line := formatBudgetPark("implementer", err)
	for _, want := range []string{"[implementer]", "duration", "parked pending renewal/wake", "31m0s"} {
		if !strings.Contains(line, want) {
			t.Fatalf("formatBudgetPark = %q; missing %q", line, want)
		}
	}
}

func TestFormatReasonPromptSize(t *testing.T) {
	line := formatReasonPromptSize("implementer", 18342, 7)
	for _, want := range []string{"[implementer]", "prompt_chars=18342", "iteration 7"} {
		if !strings.Contains(line, want) {
			t.Fatalf("formatReasonPromptSize = %q; missing %q", line, want)
		}
	}
}
