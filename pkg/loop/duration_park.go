package loop

import "fmt"

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

// formatBudgetPark renders the one-line park notice for a duration-exhausted
// keepalive loop. The sentinel wake patterns match on "parked pending".
func formatBudgetPark(agentName string, err error) string {
	return fmt.Sprintf("[%s] budget exhausted (duration): parked pending renewal/wake — %v", agentName, err)
}

// formatReasonPromptSize renders the per-call prompt-size line (v14-F1
// observability: three 10-minute reason kills left no record of what was
// actually sent; prompt size is the first discriminator between
// prompt-bloat and provider hangs).
func formatReasonPromptSize(agentName string, chars, iteration int) string {
	return fmt.Sprintf("[%s] reason prompt_chars=%d (iteration %d)", agentName, chars, iteration)
}
