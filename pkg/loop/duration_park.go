package loop

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// v14-F3(b): keepalive agents carried a 30-minute MaxDuration default, so
// every society epoch self-terminated at 30 minutes — eight simultaneous
// budget obituaries on the first epoch that ever lived that long. Duration
// exhaustion on a keepalive loop with a bus now PARKS (the same
// raise-and-park contract as v8-F2 escalations and v13-F1 reason failures)
// instead of exiting; the allocator renews the limit on-chain (v14-F3c) and
// the parked loop resumes on its next wake or gated re-check. Every OTHER
// budget resource — iterations, tokens, cost, and any future addition —
// keeps the terminal stop: parking is the explicitly-proven branch, exit is
// the default (fail closed).

// waitForBudgetRenewal blocks a duration-parked loop until a wake arrives,
// the budget passes again, or the context ends. Returns true to resume,
// false on cancellation.
//
// The bare wake channel is NOT sufficient here: only some agents subscribe
// to budget.* — for the rest, the renewal event never matches their
// patterns, and a society whose workers are all parked generates no other
// traffic. That is the v13/v14 silent-wait class (a wake that never comes)
// at the renewal boundary, so the park polls the IN-MEMORY budget on the
// recheck tick — zero LLM cost, runs only while parked, and cannot
// re-ignite the wakeup storm the per-iteration timers were removed to kill:
// its only action is resuming an agent someone explicitly renewed.
func (l *Loop) waitForBudgetRenewal(ctx context.Context) bool {
	interval := l.config.RecheckInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-l.wake:
			return true
		case <-ticker.C:
			if l.budget.Check() == nil {
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
}

// formatBudgetPark renders the one-line park notice for a duration-exhausted
// keepalive loop. The sentinel wake patterns match on "parked pending".
func formatBudgetPark(agentName string, err error) string {
	return fmt.Sprintf("[%s] budget exhausted (duration): parked pending renewal/wake — %v", agentName, err)
}

// hasParkedRenewables is the allocator recheck's gate (v15-F1b): true when
// the set of duration-parked agents is NON-EMPTY and DIFFERENT from the set
// at the last fire. Keying on the explicit DurationParked marker — never on
// derived "elapsed past limit", which turns permanently true for exited
// loops — and delta-gating on the set signature makes the gate storm-proof:
//   - allocator declines to renew → set unchanged → no refire;
//   - a NEW agent parks → signature changes → fire;
//   - renew-then-repark of the same agent → the empty set observed between
//     episodes resets the signature → fire.
// Only called from the Run() goroutine.
func (l *Loop) hasParkedRenewables() bool {
	reg := l.config.BudgetRegistry
	if reg == nil {
		return false
	}
	sig := strings.Join(reg.DurationParkedNames(), ",")
	if sig == "" {
		l.allocParkSig = ""
		return false
	}
	if sig == l.allocParkSig {
		return false
	}
	l.allocParkSig = sig
	return true
}

// formatReasonPromptSize renders the per-call prompt-size line (v14-F1
// observability: three 10-minute reason kills left no record of what was
// actually sent; prompt size is the first discriminator between
// prompt-bloat and provider hangs).
func formatReasonPromptSize(agentName string, chars, iteration int) string {
	return fmt.Sprintf("[%s] reason prompt_chars=%d (iteration %d)", agentName, chars, iteration)
}
