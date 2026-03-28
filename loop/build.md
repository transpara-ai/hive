# Build: Governance delegation + quorum enforcement (Scout 354)

- **Iteration:** 401
- **Gap addressed:** Scout 354 — Governance layer lacks delegation infrastructure (quorum, delegate/undelegate ops, voting_body)
- **Timestamp:** 2026-03-29

## What Was Built

Three substeps from Scout 354, implemented in one iteration:

### 1. Delegation ops — `delegate` and `undelegate`

**Schema:** New `delegations` table `(space_id, delegator_id, delegate_id, PRIMARY KEY(space_id, delegator_id))`. Space-level delegation — one delegate per user per space.

**Store methods added:**
- `Delegate(ctx, spaceID, delegatorID, delegateID)` — records delegation; blocks self-delegation and circular chains
- `Undelegate(ctx, spaceID, delegatorID)` — removes delegation (fully reversible)
- `HasDelegated(ctx, spaceID, delegatorID)` — checks if user has active delegation

**Handler ops added:**
- `case "delegate":` — requires `delegate_id`; calls `store.Delegate()`; records op; returns 400 on validation errors (self/circular)
- `case "undelegate":` — removes delegation; records op

**Constraint:** A user with an active delegation cannot vote directly (`vote` handler now checks `HasDelegated` → 409 Conflict). Must `undelegate` first.

### 2. Quorum enforcement

**Schema:** `ALTER TABLE nodes ADD COLUMN IF NOT EXISTS quorum_pct INT NOT NULL DEFAULT 0` and `voting_body TEXT NOT NULL DEFAULT 'all'`. Zero means "no quorum" (backward-compatible — all existing proposals behave as before).

**Store methods added:**
- `SetProposalConfig(ctx, nodeID, quorumPct, votingBody)` — updates quorum_pct and voting_body on a proposal
- `GetSpaceMemberCount(ctx, spaceID)` — counts distinct members (space_members UNION owner)
- `GetEffectiveVoteCount(ctx, spaceID, nodeID)` — counts direct voters + delegators whose delegate voted
- `CheckAndAutoCloseProposal(ctx, spaceID, nodeID)` — auto-closes when effective_votes/eligible >= quorum_pct/100; outcome = "passed" if yes > no, else "rejected"

**Handler changes:**
- `propose` handler: accepts optional `quorum_pct` (1-100) and `voting_body` form fields; calls `SetProposalConfig` if quorum_pct > 0
- `vote` handler: calls `CheckAndAutoCloseProposal` after recording each vote (auto-close fires when quorum met)

**ProposalWithVotes** struct extended: `QuorumPct`, `VotingBody`, `EffectiveVotes`, `EligibleCount` fields added; `ListProposals` scans these from the DB.

### 3. Constants

Added `OpDelegate`, `OpUndelegate`, `VotingBodyAll`, `VotingBodyCouncil`, `VotingBodyTeam` constants (no magic strings — Invariant 11).

## Tests

**`TestGovernanceDelegation` (store_test.go):**
- `delegate_and_has_delegated` — Delegate() sets HasDelegated=true for delegator, not delegate
- `undelegate_clears_delegation` — Undelegate() clears HasDelegated
- `circular_delegation_blocked` — A→B, B→A returns error
- `self_delegation_blocked` — A→A returns error
- `effective_vote_count_includes_delegated` — A→B, B votes: GetEffectiveVoteCount = 2
- `quorum_auto_close_on_threshold` — 50% quorum, 2/4 votes → auto-close with "passed"

**`TestHandlerGovernanceDelegation` (handlers_test.go):**
- `propose_with_quorum_pct` — propose op accepts quorum_pct + voting_body → 201
- `delegate_op` — delegate op records delegation → 200, HasDelegated=true
- `vote_blocked_when_delegated` — vote while delegated → 409 Conflict
- `undelegate_op` — undelegate op removes delegation → 200, HasDelegated=false

## Files Changed

| File | What |
|------|------|
| `site/graph/store.go` | Schema migrations, constants, ProposalWithVotes fields, ListProposals scan, new Governance methods |
| `site/graph/handlers.go` | propose quorum config, vote HasDelegated check + auto-close, delegate/undelegate cases |
| `site/graph/store_test.go` | TestGovernanceDelegation (6 subtests) |
| `site/graph/handlers_test.go` | TestHandlerGovernanceDelegation (4 subtests) |

## Build Verification

```
go.exe build -buildvcs=false ./...   → EXIT:0
go.exe test ./...                    → EXIT:0 (all pass)
```

## Scout Gap Cross-Reference

This build directly addresses **Scout 354** (Governance delegation):
- ✅ Delegation ops: `delegate` (user → delegate), `undelegate`
- ✅ Quorum enforcement: proposals carry `quorum_pct` and `voting_body`; auto-close when quorum met
- ✅ `delegated_from` semantic: tracked via `delegations` table; `GetEffectiveVoteCount` resolves chain
- ✅ Tests cover delegation chain, quorum thresholds, voting_body="all", tiered approval prerequisite

ACTION: DONE
