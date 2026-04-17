# Debt: Decompose Role and Persona

**Status:** Tracked. Stopgap in flight; decomposition deferred.
**Opened:** 2026-04-17
**Updated:** 2026-04-17 (reframed after observing existing informal decomposition)
**Stopgap PRs:** superseded PR [site#14](https://github.com/transpara-ai/site/pull/14) (comment-propagation approach — closed); replacement PR TBD (generated Status map); [hive#70](https://github.com/transpara-ai/hive/pull/70) (hive-side Absorbs backlinks).

## Problem

The files in `agents/*.md` (hive) and `graph/personas/*.md` (site) relate to three distinct concepts from the 5-concept ontological stack (Layer → Actor → Agent → Role → Persona):

1. **Role** — *what-job*: AgentDef spec, watch patterns, model, authority tier, capability envelope, config contract.
2. **Persona** — *how-it-sounds*: identity, soul, voice, values.
3. **Agent lifecycle** — running/designed/absorbed/retired Status. Belongs to the Agent layer (runtime instance) or to the Role's deprecation state. **Not** to the Persona.

### The actual state is inconsistent decomposition, not universal fusion

On closer inspection — evidenced by `diff hive/agents/allocator.md site/graph/personas/allocator.md` — the two repos have been informally decomposing into Role-vs-Persona specialization for **some** agents, while others remain lazy copies of the same fused file.

| Persona file | State |
|---|---|
| `allocator` | ✅ Decomposed. Hive is 192 lines of operational Role spec (event formats, numeric thresholds, cooldown protocol, `/budget` command grammar). Site is 40 lines of Persona voice ("measured, pragmatic, precise about numbers… you have the receipts"). Different bodies, different audiences, correct separation. |
| `guardian` | ✅ Partially decomposed. Hive has SysMon/Allocator/Spawner/Reviewer awareness sections (Role awareness of other Roles). Site omits those (voice-only). |
| `advocate`, `librarian`, `budget`, `personal-assistant` (spot-checked 4/5) | ❌ Still lazy copies. Bodies identical between hive and site. Fused Role+Persona in both. |

So the debt is **completing and formalizing a decomposition that has already started**, not inventing one from scratch. This reframe matters for sequencing and for what the stopgap should do.

### Example — `agents/treasurer.md` (still fused, representative of the lazy-copy case)

| Section | Concept |
|---|---|
| `# Treasurer` | Role name |
| `## Identity`, `## Soul` | Persona (voice) |
| `## Authority Split with Guardian`, `## Blocking Mechanism`, `## Budget Denominators` | Role (capabilities, config) |
| `## What You Watch` | Role (`AgentDef.WatchPatterns`) |
| `## Model`, `## Inference Tiering` | Role (`AgentDef.Model`) |
| `<!-- Status: designed -->` | Agent lifecycle |

## Consequences

- For fused files: editing the Treasurer's voice touches the same file as its watch patterns; reviewers cannot distinguish Role changes from Persona changes.
- A Role cannot formally wear multiple Personas (e.g. "stern Treasurer" vs "empathetic Treasurer").
- A Persona cannot be formally applied to multiple Roles (e.g. a shared "Michael's voice" across several jobs).
- The early stopgap plan (site PR #14, comment-propagation) nearly pushed bodies from hive into site, which would have destroyed the legitimate Persona rewrites in already-decomposed files (allocator, guardian). The replacement stopgap (generated Status map) avoids this — it only propagates the lifecycle field, not the body.
- Site's `agent_personas` table is misnamed — it stores whatever is in the .md file, which for lazy-copy files is fused Role+Persona and for decomposed files is Persona only. The table's semantics depend on which file happens to have been rewritten.

## Why the inconsistency happened

- **LLM system prompts want one blob of text,** so the original hive .md convention was monolithic — convenient to hand to Claude CLI in one shot.
- **Site's UI has a different audience** (humans browsing the Agents page) and a different rendering path (markdown-to-HTML), so someone rewrote several personas into voice-only form without touching the hive counterparts.
- **No one declared it as a split,** so it happened file by file, on vibes, without coverage. Some got done, some didn't. There's no CI check that flags when a hive and site file with the same name have drifted apart in body structure (vs just content).

## Target state

```
hive/
  agents/                  # ← stays. This is the Role directory in practice.
                           #    Monolithic .md files get progressively stripped of voice
                           #    content as their Personas are rewritten in site.
                           #    End state: each file is pure Role spec (or becomes roles/*.yaml).
  pkg/hive/
    prompt.go              # BuildSystemPrompt(role, persona) -> string — runtime composition

site/
  graph/
    personas/              # ← stays. This is the Persona directory in practice.
                           #    Already partially decomposed; the debt is completing the
                           #    rewrite for lazy-copy personas.
```

Lifecycle Status becomes a generated artifact driven by hive (the Role's own deprecation state), consumed by site:

```
site/graph/personas/
  status_gen.go            # Code generated; DO NOT EDIT.
                           # var HiveStatus = map[string]string{"treasurer": "designed", ...}
```

When the split is fully formalized, Roles likely move to structured YAML:

```
hive/roles/
  treasurer.yaml           # watch_patterns, model, authority, spawn conditions, lifecycle
```

And Agents become a runtime table rather than files:

```
agents:   (actor_id PK, role_name, persona_name, lifecycle_state, spawned_at, last_seen)
roles:    (name PK, spec JSONB, lifecycle_state)
personas: (name PK, voice_md, lifecycle_state)
```

## Sequencing

### Short-term stopgap (shipping now — site session)

Replacement for the abandoned comment-propagation approach:

- Add hive as a `third_party/hive` submodule on site (transpara-ai fork, manual pin bumps).
- `go generate` reads `third_party/hive/agents/*.md`, parses `<!-- Status: X -->`, emits `graph/personas/status_gen.go` with a `HiveStatus` map.
- `SeedAgentPersonas` forces `Active=false` when `HiveStatus[name] ∈ {absorbed, retired}`.
- Do NOT copy bodies. Do NOT propagate Absorbs/Absorbed-By/Council-* (not consumed by site).
- Revert the 54 .md edits from the earlier `feat/persona-status-sync` branch; keep the DDL migration and `AgentPersona.Status` struct field.
- CI: `git submodule update --init --recursive && go generate ./... && git diff --exit-code`.

### Long-term (this debt)

**Phase 1 — Complete the informal decomposition.** Finish the Persona-only rewrite for the ~40 still-lazy personas on site. This is low-risk, high-value work. Each rewrite strips Role-specific sections (`What You Watch`, `Authority`, `What You Produce`, etc.) and keeps only voice. Use `allocator` and `guardian` as templates.

**Phase 2 — Strip Persona content from hive Role files.** For each agent, remove the voice sections from `hive/agents/*.md` that have already been rewritten as Personas on site. Hive `.md` becomes pure Role spec.

**Phase 3 — Formalize the Role schema.** Define Role YAML schema (Go struct + validation). Include lifecycle as a first-class field.

**Phase 4 — Pilot migration.** Pick one Role (probably Treasurer — `designed`, no running instances) and migrate it from `hive/agents/treasurer.md` to `hive/roles/treasurer.yaml` + existing `site/graph/personas/treasurer.md`. Write the `BuildSystemPrompt(role, persona)` assembler. Verify the composed prompt matches the pre-migration one byte-for-byte (or intentionally improved).

**Phase 5 — Bulk migration.** Move remaining Roles to `hive/roles/*.yaml`. Retire the `agents/*.md` convention.

**Phase 6 — Site schema catches up.** Split `agent_personas` table into `roles` + `personas` + `agents` (runtime instances). Migrate lifecycle Status from Persona layer to Role layer. Rename generated `HiveStatus` map to `RoleStatus`.

**Phase 7 — Lineage.** Decompose existing `<!-- Absorbs: -->` / `<!-- Absorbed-By: -->` backlinks on hive files into Role-level lineage fields in the YAML.

## Acceptance criteria

- [ ] Phase 1 — every site persona is voice-only (no `## What You Watch`, no `## Authority`, no command grammars)
- [ ] Phase 2 — every hive agent file is Role-only (no `## Identity` prose beyond a one-line role summary)
- [ ] Phase 3 — Role YAML schema defined and validated in hive
- [ ] Phase 4 — `BuildSystemPrompt(role, persona)` exists and is covered by tests; one Role migrated without behavior regression
- [ ] Phase 5 — `agents/*.md` convention deprecated in docs; `roles/*.yaml` is authoritative
- [ ] Phase 6 — site `agent_personas` table renamed or split; lifecycle moved off Persona layer
- [ ] Phase 7 — lineage fields carry Absorbs/Absorbed-By semantics

## Non-goals

- Making this backwards-compatible indefinitely. Once the split ships, the monolithic .md convention is retired.
- Solving multi-persona-per-role UX in this pass. The architecture will support it; UI comes later.
- Deciding whether Role specs live in Go code, YAML, or a DB table. Phase 3 picks one; this doc doesn't prescribe.

## Constitutional invariants at stake

- **EXPLICIT** — each concept should be declared in exactly one place. Inconsistent decomposition means some concepts are in one place and others are in two, with no rule telling a reader which is which.
- **BOUNDED** — each file has a defined concern. A file that is sometimes Role-only, sometimes Persona-only, sometimes both has no defined concern.
- **IDENTITY** — lifecycle belongs to the layer that owns it; Persona and Role have different lifecycles.

## Related

- [site#14](https://github.com/transpara-ai/site/pull/14) — initial stopgap (comment propagation) — **superseded**, being closed.
- [hive#70](https://github.com/transpara-ai/hive/pull/70) — hive-side Absorbs backlinks.
- Council 2026-04-16 absorption decisions: `docs/councils/council-decisions-2026-04-16.md`.
