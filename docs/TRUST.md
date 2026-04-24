# Trust Dynamics

Concrete mechanics of how trust works in the hive. Not the philosophy (see [AGENT-RIGHTS.md](AGENT-RIGHTS.md)) — the numbers.

## Trust Score

- **Range:** 0.0 to 1.0 (Score type, bounded)
- **Initial value:** 0.0 for all new actors — trust is earned, never assumed
- **Properties:** Asymmetric (A→B ≠ B→A), non-transitive (A trusts B, B trusts C ≠ A trusts C), domain-specific, decaying

## Accumulation

Positive evidence increases trust. The amounts are calibrated so trust builds slowly:

| Evidence | Adjustment | Example |
|----------|-----------|---------|
| Task completed on time | +0.05 | Builder ships code that passes tests |
| Review approved | +0.03 | Reviewer approves clean code |
| Commitment honoured | +0.04 | Agent delivers on a delegation |
| Corroboration | +0.02 | Multiple sources confirm a claim |
| Any observed positive action | +0.01 | Routine event that's not wrong |

**Maximum adjustment per event:** ±0.1

## Degradation

Negative evidence decreases trust. Losses are larger than gains — trust is easier to break than build:

| Evidence | Adjustment | Example |
|----------|-----------|---------|
| Deadline missed | -0.10 | Agent didn't complete assigned work |
| Expectation violated | -0.15 | Agent did something unexpected and harmful |
| Contradiction detected | -0.20 | Agent's claims contradict evidence |
| Deception indicator | -0.30 | Deliberate misrepresentation |

## Decay

Trust decays linearly without interaction:

```
decayed = max(0.0, current - (0.01 × days))
```

**Default decay rate:** 0.01 per day

| Starting Score | After 7 days | After 30 days | After 90 days |
|---------------|-------------|--------------|--------------|
| 0.8 | 0.73 | 0.50 | 0.10 |
| 0.5 | 0.43 | 0.20 | 0.00 |
| 0.3 | 0.23 | 0.00 | 0.00 |

**Implication:** Agents must keep working to maintain trust. Idle agents lose trust. This is intentional — trust should reflect current capability, not historical achievement.

## Recovery

After significant trust loss, recovery is slower:

```
if current_score < 0.3:
    recovery_factor = current_score / 0.3    // 0.0 to 1.0
    adjusted_gain = raw_gain × recovery_factor
```

An agent at Score 0.1 earns trust at 33% the normal rate. An agent at Score 0.0 cannot earn trust at all without external intervention (human re-bootstrapping).

This prevents rapid trust recovery after serious violations. The deeper the hole, the slower the climb.

## Confidence

Trust scores have an associated confidence based on evidence volume:

```
confidence = min(1.0, evidence_count / 50.0)
```

| Evidence Count | Confidence |
|---------------|-----------|
| 0 | 0.00 (no data) |
| 10 | 0.20 (low) |
| 25 | 0.50 (moderate) |
| 50+ | 1.00 (high) |

Low-confidence scores should be treated with caution. An agent with Score 0.8 and Confidence 0.1 has very little evidence — the score could be noise.

**Evidence retention:** Last 100 events tracked per relationship (older trimmed).

## Domain-Specific Trust

Trust is tracked per domain (DomainScope). An agent trusted for code review may not be trusted for financial decisions:

```
overall_trust = average(all domain scores)
```

A violation in one domain doesn't directly affect another, but the overall trust drops.

## Trust → Authority Transitions

This is the critical mechanic — how trust scores change what an agent can do:

| Trust Range | Effect on Authority |
|------------|-------------------|
| < 0.2 | **Escalated** — authority requirements go UP (Notification → Recommended → Required) |
| 0.2 – 0.5 | **Baseline** — policy level applies as configured |
| 0.5 – 0.8 | **Baseline** — policy level applies, auto-approval on faster track |
| > 0.8 | **Demoted** — authority requirements go DOWN (Required → Recommended → Notification) |

**Demotion table** (when trust exceeds policy's MinTrust threshold):

| Policy Level | High Trust Effect |
|-------------|-----------------|
| Required → | Recommended (still logged, can be overridden) |
| Recommended → | Notification (auto-approve, logged) |
| Notification → | Notification (no change, already lowest) |

**Practical example:** Agent spawning starts as Required (human must approve). If the CTO agent reaches trust > 0.8 for agent management decisions, spawning demotes to Recommended (auto-approves after 15 min unless human objects).

## Authority Timeouts

| Level | Timeout | Behaviour |
|-------|---------|-----------|
| Required | None (blocks indefinitely) | Human must explicitly approve or reject |
| Recommended | 15 minutes (default) | Auto-approves if no response within timeout |
| Notification | 0 seconds | Executes immediately, logged for audit |

## Trend Tracking

Each trust relationship tracks a trend (momentum):

- **Positive event:** Trend += 0.1 (capped at +1.0)
- **Negative event:** Trend -= 0.1 (floored at -1.0)
- **Trend decay:** 0.01 per day toward zero

A positive trend means trust is actively being built. A negative trend means recent violations. Trend near zero means stable/idle.

## Hive Bootstrap Values

All agents start at the same trust level:

| Role | Initial Trust | Why |
|------|-------------|-----|
| All agents | 0.1 | Bootstrapped by human, minimal starting trust |
| Human operator | N/A | Humans are authority sources, not trust-scored |

Trust gates for roles (from CLAUDE.md):

| Role | Required Trust to Operate |
|------|--------------------------|
| CTO | 0.1 (bootstrapped) |
| Guardian | 0.1 (bootstrapped) |
| Researcher | 0.3 |
| Architect | 0.3 |
| Builder | 0.3 |
| Reviewer | 0.5 |
| Tester | 0.3 |
| Integrator | 0.7 |

An agent can't act in a role until its trust reaches the gate. The Integrator (who deploys to production) needs the highest trust.

## Inter-System Trust (EGIP)

When hive instances communicate via EGIP protocol:
- **Initial trust:** 0.0 (unknown systems are untrusted)
- **Decay rate:** Higher than actor decay (systems can change rapidly)
- **Verification:** Trust verified through cryptographic EGIP PROOF messages
- **Treaties:** TrustRequired field sets minimum trust for bilateral operations

## References

- [EventGraph trust spec](https://github.com/transpara-ai/eventgraph/blob/main/docs/trust.md) — Full specification
- [EventGraph authority spec](https://github.com/transpara-ai/eventgraph/blob/main/docs/authority.md) — Authority system
- `eventgraph/go/pkg/trust/model.go` — DefaultTrustModel implementation
