# Critique: [hive:builder] Remove stale directives from `loop/state.md`

**Verdict:** PASS

**Summary:** ## Analysis

### What this commit does

Removes ~250 lines of stale iteration 240+/242+ directives from `state.md`, updates `build.md` with iter 317's real metadata (replacing pending placeholders), updates `critique.md` to iter 317's, and appends a reflection for 2026-03-27. The directive removal itself is clean and correct — those were genuinely stale.

### Issues

**1. `reflections.md` is contaminated with raw planning output (blocking)**

The FORMALIZE section ends properly, then the reflection continues with raw instructions that belong to the Reflector's internal reasoning — not the permanent record:

```
**And three lessons to add to state.md's lessons list (after line 284):**
...
74. Lesson 78: ...
75. Lesson 79: ...
76. Lesson 80: ...

Also update line 5 of state.md: `Last updated: Iteration 318, 2026-03-27.`

Shall I write these changes?
```

`reflections.md` is append-only. "Shall I write these changes?" is a Reflector planning artifact — it was written verbatim into the permanent record instead of being acted on. This cannot be undone by a future append; it's corruption of the audit trail.

**2. Lessons 78-80 were not added to `state.md` (blocking)**

The reflection explicitly calls for three lessons to be added to the lessons list. The `state.md` diff only shows the iteration counter changing (317 → 318) and the directive block removal. No lessons were added. The Reflector identified the lesson content, stated it should be written, then didn't write it.

**3. `build.md` has no iter 318 artifact**

The CLAUDE.md requirement: *"Builder writes `loop/build.md`… Overwritten each iteration."* The current `build.md` documents iter 317's metadata (commit 4d76cb4, `$0.2119`, pending → actual). Iter 318's work (the directive removal) has no build artifact. The Reflector is reflecting on an iteration with no build record.

### Checklist

| Check | Result |
|---|---|
| Completeness | N/A (no new constants/kinds) |
| Identity (inv 11) | N/A |
| Bounded (inv 13) | N/A |
| Correctness | PASS — directive removal is clean |
| reflections.md integrity | **FAIL — planning instructions written verbatim into permanent record** |
| state.md lessons | **FAIL — Lessons 78-80 called for but not written** |
| build.md artifact | **FAIL — iter 318 has no build artifact** |

---

**VERDICT: REVISE**

Required fixes:
1. **Truncate `reflections.md`** — strip everything after the FORMALIZE section's closing sentence, from `"**And three lessons to add to state.md's lessons list**"` through `"Shall I write these changes?"` inclusive
2. **Add Lessons 78-80 to `state.md`** — extract the three lessons from the reflection and append them to the lessons list
3. **Write `build.md` for iter 318** — document the stale directive removal: what was removed, why, the commit hash, and cost
