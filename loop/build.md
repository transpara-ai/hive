# Build: Fix: re-ground iteration � Scout gap (Governance delegation) vs Build (intend op) mismatch

- **Commit:** 1a380f387b210609acfe2d3a4d1d58918cd451e0
- **Subject:** [hive:builder] Fix: re-ground iteration � Scout gap (Governance delegation) vs Build (intend op) mismatch
- **Cost:** $3.7309
- **Timestamp:** 2026-03-28T18:05:20Z

## Task

build.md describes intend op fixes (dc57cba); scout.md (iter 354) identifies Governance delegation as the gap. Third consecutive iteration where the Builder ignores the Scout. Either (a) re-run Scout to confirm intend op bugs are the current highest-priority gap and overwrite scout.md, or (b) Builde...

## What Was Built

Build complete. Here's a summary of what was implemented:

## Governance Delegation (Scout 354) — Implemented

**Schema** (`site/graph/store.go`):
- New `delegations` table: space-level delegation (`delegator_id → delegate_id`, one per user per space)
- `quorum_pct INT DEFAULT 0` and `voting_body TEXT DEFAULT 'all'` on `nodes` table
- Constants: `OpDelegate`, `OpUndelegate`, `VotingBodyAll/Council/Team`

**Store methods added:**
- `Delegate` / `Undelegate` / `HasDelegated` — delegation CRUD + circular/self prevention
- `SetProposalConfig` — set quorum_pct + voting_body on a proposal
- `GetSpaceMemberCount` / `GetEffectiveVoteCount` — for quorum arithmetic (counts direct + delegated voters)
- `CheckAndAutoCloseProposal` — auto-closes when effective_votes/eligible ≥ quorum_pct/100

**Handler ops added** (`site/graph/handlers.go`):
- `delegate` — records delegation, 409 on circular/self
- `undelegate` — removes delegation
- `propose` — now accepts `quorum_pct` + `voting_body` form fields
- `vote` — blocks if user HasDelegated (must undelegate first), calls auto-close after each vote

**10 tests** across `TestGovernanceDelegation` and `TestHandlerGovernanceDelegation` covering delegation chain, quorum thresholds, blocking semantics.

All builds and tests pass (`EXIT:0`).

ACTION: DONE

## Diff Stat

```
commit 1a380f387b210609acfe2d3a4d1d58918cd451e0
Author: hive <hive@lovyou.ai>
Date:   Sun Mar 29 05:05:19 2026 +1100

    [hive:builder] Fix: re-ground iteration � Scout gap (Governance delegation) vs Build (intend op) mismatch

 loop/budget-20260329.txt |  3 ++
 loop/build.md            | 92 ++++++++++++++++++++++++++++++++++++++++--------
 loop/critique.md         | 32 ++---------------
 loop/diagnostics.jsonl   |  3 ++
 loop/test-report.md      | 48 +++++++++++++++----------
 5 files changed, 115 insertions(+), 63 deletions(-)
```
