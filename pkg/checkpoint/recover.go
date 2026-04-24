package checkpoint

import (
	"log"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/store"
)

// RecoveryMode indicates whether an agent started fresh or resumed from a checkpoint.
type RecoveryMode int

const (
	// ModeCold indicates no usable checkpoint was found — the agent starts fresh.
	ModeCold RecoveryMode = iota
	// ModeWarm indicates a recent checkpoint was found — the agent resumes state.
	ModeWarm
)

// String returns a human-readable label for the recovery mode.
func (m RecoveryMode) String() string {
	switch m {
	case ModeWarm:
		return "warm"
	default:
		return "cold"
	}
}

// RecoveryState holds the recovered state for a single agent at boot.
// Fields are zero/nil for cold starts — callers must check Mode.
type RecoveryState struct {
	Role          string
	Mode          RecoveryMode
	Iteration     int
	Intent        string // empty for cold start
	HiveSummary   string // empty if unavailable
	CurrentTaskID string // empty if no task
	BudgetState   *BudgetState
	CTOState      *CTORecoveredState
	SpawnerState  *SpawnerRecoveredState
	ReviewerState *ReviewerRecoveredState

	// ConsumedTokens and ConsumedCostUSD hold the last-known resource
	// consumption from a heartbeat or checkpoint thought. Used to seed
	// the Budget on restart so agents don't get a full fresh budget.
	ConsumedTokens  int
	ConsumedCostUSD float64
}

// RecoverAll orchestrates the two-tier recovery for the given set of agent roles.
//
// Tier 2 (Open Brain / warm path): for each agent, search ThoughtStore for a
// recent checkpoint thought. If found and parseable, the agent starts warm.
//
// Tier 1 (chain replay / cold path): for agents with no warm checkpoint, replay
// the event chain to reconstruct budget, CTO, Spawner, and Reviewer state.
//
// A hive summary thought is attached to ALL agents regardless of mode.
//
// The function is maximally tolerant: any error degrades to a cold start for
// the affected agent without aborting the overall recovery.
func RecoverAll(agents []string, thoughts ThoughtStore, s store.Store, staleness time.Duration) (map[string]*RecoveryState, error) {
	result := make(map[string]*RecoveryState, len(agents))

	// Initialize all agents as cold-start.
	for _, role := range agents {
		result[role] = &RecoveryState{Role: role, Mode: ModeCold}
	}

	// ── Tier 2: Open Brain warm-start ────────────────────────────────────────

	if thoughts != nil {
		for _, role := range agents {
			// Query uses the exact header prefix from FormatCheckpoint so the
		// StubThoughtStore substring match works in tests, and Open Brain
		// semantic search matches it in production.
		query := "[CHECKPOINT] " + role
			found, err := thoughts.SearchRecent(query, staleness)
			if err != nil {
				log.Printf("checkpoint: recover: ThoughtStore search error for %q: %v — cold start", role, err)
				continue
			}
			if len(found) == 0 {
				continue
			}

			// Take the most recent matching thought (SearchRecent returns newest first
			// or any order — we take index 0 which is what we got).
			thought := found[0]

			parsed, err := ParseCheckpoint(thought.Content)
			if err != nil {
				log.Printf("checkpoint: recover: parse error for %q: %v — cold start", role, err)
				continue
			}

			rs := result[role]
			rs.Mode = ModeWarm
			rs.Iteration = parsed.ApproxIteration
			rs.Intent = parsed.Intent
			rs.CurrentTaskID = extractTaskID(parsed.Task)

			// Seed consumed budget from the thought's snapshot.
			rs.ConsumedTokens = parsed.TokensUsed
			rs.ConsumedCostUSD = parsed.CostUSD

			// Tier 2b: query chain for a heartbeat newer than the thought to refine
			// iteration count and budget consumed values.
			hb, err := QueryLatestHeartbeat(s, role, thought.CapturedAt)
			if err != nil {
				log.Printf("checkpoint: recover: heartbeat query error for %q: %v — using thought iteration", role, err)
			} else if hb != nil {
				if hb.Iteration > rs.Iteration {
					rs.Iteration = hb.Iteration
				}
				if hb.TokensUsed > rs.ConsumedTokens {
					rs.ConsumedTokens = hb.TokensUsed
				}
				if hb.CostUSD > rs.ConsumedCostUSD {
					rs.ConsumedCostUSD = hb.CostUSD
				}
			}
		}
	}

	// ── Tier 1: Chain replay for cold-start agents ────────────────────────────

	// Only run chain replay if at least one agent is cold-starting.
	hasCold := false
	for _, rs := range result {
		if rs.Mode == ModeCold {
			hasCold = true
			break
		}
	}

	if hasCold {
		budgets, err := ReplayBudgetFromStore(s)
		if err != nil {
			log.Printf("checkpoint: recover: budget replay error: %v — budgets unavailable", err)
		}

		ctoState, err := ReplayCTOFromStore(s)
		if err != nil {
			log.Printf("checkpoint: recover: CTO replay error: %v — CTO state unavailable", err)
		}

		spawnerState, err := ReplaySpawnerFromStore(s)
		if err != nil {
			log.Printf("checkpoint: recover: Spawner replay error: %v — Spawner state unavailable", err)
		}

		reviewerState, err := ReplayReviewerFromStore(s)
		if err != nil {
			log.Printf("checkpoint: recover: Reviewer replay error: %v — Reviewer state unavailable", err)
		}

		// Replay iteration counters from heartbeat + agent.stopped events.
		// Without this, cold-started agents reset to iteration 0, making
		// replayed cooldown maps incoherent with the loop counter.
		iterations, err := ReplayIterationFromStore(s)
		if err != nil {
			log.Printf("checkpoint: recover: iteration replay error: %v — iterations unavailable", err)
		}

		for _, role := range agents {
			rs := result[role]
			if rs.Mode != ModeCold {
				continue
			}

			// Seed iteration from chain replay.
			if iter, ok := iterations[role]; ok && iter > rs.Iteration {
				rs.Iteration = iter
			}

			// Attach budget state (keyed by role name).
			if budgets != nil {
				if bs, ok := budgets[role]; ok {
					bsCopy := bs
					rs.BudgetState = &bsCopy
				}
			}

			// Attach role-specific chain state.
			switch role {
			case RoleCTO:
				rs.CTOState = ctoState
			case RoleSpawner:
				rs.SpawnerState = spawnerState
			case RoleReviewer:
				rs.ReviewerState = reviewerState
			}
		}
	}

	// ── Hive summary: attach to ALL agents ───────────────────────────────────

	if thoughts != nil {
		summaries, err := thoughts.SearchRecent("hive summary", staleness)
		if err != nil {
			log.Printf("checkpoint: recover: hive summary search error: %v — summary unavailable", err)
		} else if len(summaries) > 0 {
			for _, role := range agents {
				result[role].HiveSummary = summaries[0].Content
			}
		}
	}

	return result, nil
}

// extractTaskID extracts the task ID from a TASK field value formatted as:
//
//	"task-77 -- title -- status"
//
// Returns the first segment (before " -- "), or the whole string if there is
// no separator. Returns empty string when s is empty.
func extractTaskID(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.SplitN(s, " -- ", 2)
	return strings.TrimSpace(parts[0])
}
