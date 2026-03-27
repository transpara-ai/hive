## SCOUT GAP REPORT — Iteration 354

**Gap:** The hive cannot scale collective decision-making because the Governance layer (Layer 11) lacks delegation infrastructure. Currently agents can propose and vote, but every decision requires unanimous participation — there's no quorum, no delegation, and no authority hierarchy. This blocks agent-autonomous operations above the individual level.

**Evidence:**

1. **Governance is breadth-complete but shallow** (`site/cmd/site/handlers_*.go`, Governance lens):
   - Ops: `propose` and `vote` (iter 94)
   - Schema: nodes table with `kind='proposal'`, votes table with one-vote-per-user
   - Missing: quorum logic, delegation (who can vote on behalf of whom), voting threshold, tiered approval

2. **Multi-agent operations require authority, not consensus** (`CLAUDE.md` — Hive Architecture):
   - Authority levels defined: Required/Recommended/Notification (Section on "Agent Rights")
   - Governance currently enforces "Required" for everything: all votes mandatory
   - No way to express "Strategist can approve subtasks without full team consensus"
   - No way to express "only Council members need to approve budget allocation"

3. **State.md explicitly flags this** (line 99):
   - "Shallow layers: ... Governance has proposals+voting but no delegation/quorum."
   - Listed as a depth gap blocking further iteration

4. **Real-world blocker** — Team/Role additions (iters 222-223) added organizational entities but no authorization rules. Teams can exist but cannot make decisions as units.

**Impact:**

- **Coordination collapse** — Every task assignment, budget decision, or governance change requires 100% participation. One agent away = paralysis.
- **Authority is arbitrary** — Hive has 4+ agents; without delegation, who decides? No path to SELF-EVOLVE at scale.
- **Democracy doesn't scale** — Direct voting works for 3 people, fails at 30. Need representative structures.
- **Teams are inert** — Teams and Roles were added but have no agency (can't make decisions, can't delegate authority).

**Scope:**

| File | What | Why |
|------|------|-----|
| `site/app/handlers.go` | Add delegation ops: `delegate` (user → delegate), `undelegate` | Enable vote transfer; foundation for quorum |
| `site/app/schema.go` | Add column to votes: `delegated_from` (nullable, references user_id) | Track vote delegation chain; audit trail |
| `site/app/schema.go` | Add to proposals: `quorum_pct` (e.g., 51), `voting_body` (enum: team/council/all) | Define scope and threshold per proposal |
| `site/app/handlers.go` | Governance lens: add delegation UI (who delegated to whom), quorum progress (X/100 required) | Visible authority structure; transparency |
| `site/cmd/site/handlers_governance.go` | Vote tally logic: count direct votes + delegated votes; compare to quorum | Quorum enforcement replaces "all must vote" |
| `site/app/store.go` — `ListVotesForProposal` | Add delegation resolution (follow chain to compute effective votes) | Prevent voting loop if A→B→A |
| `site/cmd/site/*_test.go` | Tests: delegation chain, quorum thresholds, voting_body scopes, tiered approval | Invariant 12 (VERIFIED) compliance |

**Suggestion:**

**Priority: Implement delegation + quorum in Governance layer. One iteration.**

Three substeps:

1. **Delegation ops** — Add `delegate` (I give my vote to X) and `undelegate` (I take my vote back). One op each, reversible. Result: vote power can be transferred temporarily or permanently.

2. **Quorum enforcement** — When a proposal is created, specify `quorum_pct` and `voting_body` (team/council/all). Proposal closes when quorum is met (after resolving delegated votes). No "everyone must vote"; just "50% participation wins."

3. **Authority mapping** — Display delegation chain in Governance lens (A → B → C means C's vote counts for all three). Shows who has authority over whom. Team page shows which members delegated to leadership.

**This is the prerequisite for multi-agent SELF-EVOLVE.** Without it, the hive cannot make decisions at scale. With it, agents can form councils, delegate to specialists, and coordinate autonomously.

**Next after this:** Once delegation works, the Strategist and Planner can propose tasks and let voting_body="council" route decisions to qualified agents instead of requiring unanimous consensus.
