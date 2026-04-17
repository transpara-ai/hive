# Debt: Decompose Role and Persona

**Status:** Tracked. Stopgap shipped; decomposition deferred.
**Opened:** 2026-04-17
**Blocking stopgaps:** [site#14](https://github.com/transpara-ai/site/pull/14), [hive#70](https://github.com/transpara-ai/hive/pull/70)

## Problem

The files in `agents/*.md` (hive) and `graph/personas/*.md` (site) conflate three distinct concepts from the 5-concept ontological stack (Layer → Actor → Agent → Role → Persona):

1. **Role** — *what-job*: AgentDef spec, watch patterns, model, authority tier, capability envelope, config contract.
2. **Persona** — *how-it-sounds*: identity, soul, voice, values.
3. **Agent lifecycle** — running/designed/absorbed/retired Status. Belongs to the Agent layer (runtime instance) or to the Role's deprecation state. **Not** to the Persona.

Example — `agents/treasurer.md` fuses all three:

| Section | Concept |
|---|---|
| `# Treasurer` | Role name |
| `## Identity`, `## Soul` | Persona (voice) |
| `## Authority Split with Guardian`, `## Blocking Mechanism`, `## Budget Denominators` | Role (capabilities, config) |
| `## What You Watch` | Role (`AgentDef.WatchPatterns`) |
| `## Model`, `## Inference Tiering` | Role (`AgentDef.Model`) |
| `<!-- Status: designed -->` | Agent lifecycle |

## Consequences

- Editing the Treasurer's voice touches the same file as its watch patterns; reviewers cannot distinguish Role changes from Persona changes.
- A Role cannot wear multiple Personas (e.g. "stern Treasurer" vs "empathetic Treasurer").
- A Persona cannot be applied to multiple Roles (e.g. a shared "Michael's voice" across several jobs).
- Lifecycle Status lives on the Persona layer via site's `agent_personas.status` column (from stopgap PR #14), which is semantically wrong — a retired **Role** means no Agents of that job spawn; a retired **Persona** just means "don't use this voice anymore."
- Site's `agent_personas` table is misnamed — it currently stores Role+Persona fused.

## Why the conflation happened

LLM system prompts want one blob of text, so monolithic .md files are convenient to hand to Claude CLI. The composition step (Role + Persona → system prompt) has been silently elided by writing both into the same file.

## Target state

```
hive/
  roles/
    treasurer.yaml        # Structured Role: watch_patterns, model, authority, spawn conditions,
                          # lifecycle field (running|designed|absorbed|retired|...)
  personas/
    fiduciary.md          # Voice, soul, values only — no watch patterns, no authority
  pkg/hive/
    prompt.go             # BuildSystemPrompt(role, persona) -> string — runtime composition
```

Runtime Agents become a DB table rather than files:

```
agents:   (actor_id PK, role_name, persona_name, lifecycle_state, spawned_at, last_seen)
roles:    (name PK, spec JSONB, lifecycle_state)
personas: (name PK, voice_md, lifecycle_state)
```

Site consumes Role+Persona as separate embeds/tables. Its UI can then show "Treasurer (wearing Fiduciary voice)" rather than a single fused entity.

## Sequencing

Short-term stopgap (already shipping): drift-prevention on the current fused files via submodule + `go generate` approach (site PR #14). Prevents divergence between hive and site while we defer the real fix.

Long-term (this debt): the decomposition itself.

### Phases

1. **Define the Role YAML schema** (Go struct + validation). Include lifecycle as a first-class field.
2. **Pick one Role** (probably Treasurer — already marked `designed`, no running instances) and split its .md into `roles/treasurer.yaml` + `personas/fiduciary.md`. Write the assembler. Verify the assembled prompt matches the monolithic one byte-for-byte (or intentionally improved).
3. **Migrate 3-5 more Roles** to validate the schema covers real cases.
4. **Bulk-migrate remaining Roles.** Retire the `agents/*.md` convention.
5. **Site catches up**: `agent_personas` table splits into `roles` + `personas` + `agents` (runtime instances). Status column migrates to the correct layer.
6. **Decompose existing `<!-- Absorbs: -->` / `<!-- Absorbed-By: -->` backlinks** into Role-level lineage fields in the YAML.

## Acceptance criteria

- [ ] Role YAML schema defined and validated in hive
- [ ] `BuildSystemPrompt(role, persona)` exists and is covered by tests
- [ ] At least one Role migrated and running (Agent spawned from composed prompt) without behavior regression
- [ ] `agents/*.md` convention deprecated in docs
- [ ] Site consumes Roles and Personas as separate entities; `agent_personas` table either renamed or split
- [ ] Lifecycle Status moved from Persona layer to Role layer (or Agent layer where appropriate)

## Non-goals

- Making this backwards-compatible indefinitely. Once the split ships, the monolithic .md convention is retired.
- Solving multi-persona-per-role UX in this pass. The architecture will support it; UI comes later.

## Constitutional invariants at stake

- **EXPLICIT** — each concept should be declared in exactly one place.
- **BOUNDED** — each file has a defined concern.
- **IDENTITY** — lifecycle belongs to the layer that owns it; Persona and Role have different lifecycles.

## Related

- [site#14](https://github.com/transpara-ai/site/pull/14) — stopgap: propagate hive Status metadata into site personas
- [hive#70](https://github.com/transpara-ai/hive/pull/70) — stopgap: Absorbs backlinks on 13 successor files
- Council 2026-04-16 absorption decisions: `docs/councils/council-decisions-2026-04-16.md`
