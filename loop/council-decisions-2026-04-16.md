# Council Decisions — 2026-04-16

## Block 1: Challenged Roles

| Role | Verdict | Detail | Dissent |
|---|---|---|---|
| Observer | absorb → Guardian | Duplicate `*` subscription; same failure domain. No new contract required — Guardian's existing subscription covers it. | None (10-0). |
| Scribe | retire | INTEGRITY (signed, hash-chained events) is structural — the graph *is* the scribe. Retiring strengthens EXPLICIT by removing a duplicate of a graph-level guarantee at the agent level. | None (10-0). Separate unclaimed gap logged: **Scribe-as-narrator** (dev logs, user-facing synthesis) is NOT filled by this retire. |
| Research | absorb → Librarian | Reading-mode, different direction (outward vs inward). Binding precondition: per-task query caps, cost ceilings, and domain allowlists declared in `loop/config.env` *before* absorption lands (BOUNDED + EXPLICIT). | None (10-0). Dissenter: "policy without enforcement is decoration" — condition is binding, not advisory. |
| Role-Architect | table | Distinct authority (agent-spawn) held by no one else — slot retained. Re-open trigger: first human-originated agent-spawn request routed through the hive, OR when Spawner pattern is scheduled for implementation. Guardian binding: successor authorship for future AgentDef spawns must be named before any eventual retire (CAUSALITY). | None (10-0). Dissenter flagged: table must carry a *concrete* binding trigger, not open-ended parking. |
| PM | absorb → Strategist + Planner | Coordination-mode, no distinct failure domain; agents on shared graph don't need a human-integrator role. No successor actor. | None (10-0). Separate unclaimed gap logged: **cross-product coherence** across the thirteen layers is NOT filled by retaining PM. |
| DevOps | promote | Distinct failure domain (bad deploy) with destructive authority. Scope: infra operate authority, Required-tier for destructive deploys. Activation gated on `DEPLOY_ENABLED=true` in `loop/config.env`. | None (10-0). |
| SRE | absorb → Guardian | Watching-mode over a different bus (production telemetry vs event graph). Binding: Guardian's subscription-set expansion to production telemetry declared *explicitly at absorption time* in the AgentDef — no silent scope creep. | None (10-0). Ops caveat honored: defer full absorption until a production telemetry bus exists to subscribe to. |

## Block 2: Archivist Rename

**Rename verdict: APPROVE (7–3).** Memory-Keeper → **Archivist**.

Binding conditions (Guardian):
- ActorID persists across the rename; only display name changes (IDENTITY invariant — names are for humans, IDs are for systems).
- **Append-only supersession contract** declared in the AgentDef — Archivist never deletes from the underlying store; only marks entries superseded and forward-points to canonical replacements. Satisfies INTEGRITY and Librarian's concurrent-read correctness contract.

**Minority position logged (Critic, Librarian, Dissenter — 3 voices):** substantive argument for **Curator** on merits — "Archivist" connotes passive preservation while the function is active curation (dedup, supersession, gap detection, digests); Librarian flagged register-redundancy between Archivist and Librarian under the same library metaphor. Majority approved under forcing function without formal merit-contest between Archivist and Curator. **Re-openable** if operational experience exposes the cold-storage connotation as a routing problem (the same failure mode that retired "Memory-Keeper").

**Emergence trigger verdict: TABLE (5 table, 4 approve, 1 reject).**

- **Shape accepted**: graph-map saturation — thought-corpus growth outpacing structural-reference growth — is the correct *kind* of trigger for a curation role. Replaces the original "knowledge loss across reboots" trigger, which is now obsolete (OpenBrain append-only + `pkg/checkpoint` solve retention structurally).
- **Specific thresholds (>500 thoughts, <10% reference type) NOT committed.** Evidence-free. Deferred until:
  - Thresholds are parameterised in `loop/config.env`, not in prompts or role code (EXPLICIT + BOUNDED).
  - First telemetry period observes actual curation-distress breakdown points; thresholds tuned against observation, not intuition.
  - Saturation-threshold-crossed event has a declared first-class emitter named in the roster (CAUSALITY — Guardian binding).
- **Re-open condition:** one telemetry period of `mcp__open-brain__thought_stats` trend data plus the declared emitter identity.

## Block 3: Missing Roles

| Role | Verdict | Modifications |
|---|---|---|
| Treasurer | modify (10-0) | (1) Declare **exclusive halt-authority split with Guardian** in the AgentDef — Treasurer blocks on resource thresholds (BUDGET, MARGIN, RESERVE); Guardian HALTs on integrity violations. No overlap. (2) Specify **blocking mechanism**: refuse new `/task create`, emit HALT event on bus, kill in-flight LLM calls — or declared combination. (3) **Required-tier human override path** for budget exceptions — otherwise Treasurer becomes an unrecoverable trip wire when the operator is unavailable. (4) Name the **denominator for "100%"** (per-task / per-agent-daily / per-hive-monthly / per-product-line); all thresholds live in `loop/config.env`, not in prompts. (5) **Tiered inference**: Haiku for routine tracking and deterministic threshold checking; Sonnet-or-higher for exception judgment. (6) **Runway-projection query ergonomics** declared (materialised view or dedicated helper) — no full table scans at iteration cadence. (7) **Self-resilience clause**: what happens to in-flight work when Treasurer blocks and no operator is present to override. |
| Companion | table (10-0) | Revisit next council with concrete answers on: (1) **Positive interface definition** — responder only, initiator, or both; named verbs for what "serve" means (not the subtractive phrase "serves the human"). (2) **Write authority scope** — none, scoped to Companion's own state, or broader (can it edit `loop/config.env`, emit tasks, modify Open Brain?). (3) **CONSENT contract** — what is recorded about the operator, stored where, retained how long, opt-in or opt-out. (4) **Operator-private knowledge substrate decision** — scoped Open Brain visibility vs new store. Shared event graph is NOT an option (CONSENT + DIGNITY). (5) **Proxy-for-operator trust protocol** — if Companion represents the operator to other agents, other agents must be able to recognise Companion as a trusted proxy; this subscription-level trust relationship does not exist today and must be designed, not inferred. (6) **Stage-fit justification** — why Companion is the right role *now*, given the hive's "build own infrastructure first" stage (Treasurer and Security are infrastructure; Companion is a relationship affordance). (7) **IDENTITY rule**: one ActorID per `(operator, companion)` pair — never shared across operators. |
| Security | modify (7 modify, 3 approve) | (1) **Reasoning-vs-scheduled decomposition** declared — which portions are scheduled processes emitting findings (dependency audits, secret scans, access-pattern monitors) and which portions require agent reasoning (CVE triage, attack-surface judgment, incident response). Role is a **reasoner over deterministic feeders**, not a monolithic agent. (2) **Enforcement vs reporting authority** explicit — can Security block a deploy on CVE finding, trigger key rotation, or is it purely reporting-to-graph? Different tier implications; must be declared. (3) **IDENTITY protection clause** — Security does NOT modify actor signing keys without an explicit, audited path. Otherwise the role holds de facto identity-forgery authority. (4) **Event emission** — all findings emitted as first-class events with declared authorship (EXPLICIT). No silent reports. (5) **Declared non-overlap** with Guardian's secrets scope (Guardian enforces IDENTITY on signing keys; Security enforces secrets lifecycle — rotation, leakage, access patterns). |

## Block 4: Re-Derivation Cadence

**Verdict: Option B — trigger-based. Unanimous (10-0).**

Options A (fixed cadence) and C (no formal cadence) were both rejected:
- **A** optimises for process-fidelity, not signal — imports human standing-meeting culture into an agent civilisation.
- **C** is disproven by evidence — today's maturity audit found 12 orphaned files that should have been archived after the last derivation. No formal cadence = silent drift.
- **B** is the only option consistent with the generator function, which loops through Need without calendar.

### Re-derivation triggers

Any one of the following fires a re-derivation council. Each trigger must itself emit a first-class event on the bus with a declared emitter (CAUSALITY — Guardian binding):

1. **Invariant delta** — a constitutional invariant is added, removed, or materially reworded.
2. **Substrate change** — a new Graph, Store, or foundational layer is introduced (e.g., OpenBrain's emergence, pgstore migration, a new product-layer graph).
3. **Challenged-role ratio** — >N% of the active roster flagged "challenged" in a single council. Threshold parameterised in `loop/config.env`.
4. **Orphan-accumulation** — archived-role references in the live corpus exceed a ratio threshold. Ops-monitored against `mcp__open-brain__thought_stats` and file-system archive state.
5. **Unmappable Need** — a newly distinguished Need cannot be assigned to any existing role's failure domain (Scout's gap-detection criterion).
6. **Unresolved dissent** — a strong council Dissent on derivation dimension that was overruled rather than resolved carries forward as a re-derivation flag.

### Immediate consequence

Today's maturity audit (12 orphaned files) satisfies trigger **#4** retroactively. A re-derivation council is **already due** under the rule the council just adopted. The Dissenter's earlier "wrong dimension" note at fixpoint-1 also satisfies trigger **#6** retroactively. Next session should open with re-derivation, not deferral.
