## SCOUT GAP REPORT — Iteration 352

**Gap:** The autonomous loop is stuck in a pathological REVISE cycle. Three infrastructure bugs prevent the Reflector from advancing the iteration counter, causing the loop to accumulate recursive "Fix: Fix: Fix:" commit subjects and lost diagnostic information. The loop cannot close cleanly until these are fixed.

**Evidence:**

1. **REVISE gate missing** (`pkg/runner/reflector.go`):
   - Reflector runs after Critic issues `VERDICT: REVISE` in `loop/critique.md`
   - Should: read critique.md before running; if REVISE, skip Reflector and emit diagnostic
   - Currently: ignores REVISE status, writes corrupt reflection entry, increments iteration counter
   - Result: loop advances on broken code, repeats cycle

2. **Recursive commit subjects** (`pkg/runner/builder.go`, commit 647471e):
   - Most recent build.md shows: `Fix: [hive:builder] Fix: [hive:builder] Fix: [hive:builder] Fix: [hive:builder] Fix: ...`
   - Builder reads `git log --oneline`, embeds previous subject in new template
   - Pattern repeats per iteration: Fix: prepended to prior Fix:
   - Should: derive subject from task title + diff summary only. Pattern: `[hive:builder] <task-title>`

3. **Architect parser silent failures** (`pkg/runner/architect.go`):
   - When `parseArchitectSubtasks` returns 0 tasks, full LLM response is lost (stderr only)
   - Future iterations have no diagnostic data about why parsing failed
   - Should: capture LLM preview (first 2000 chars) in PhaseEvent when parse fails

4. **State.md explicitly identifies this as the blocker** (line 642-658):
   - "What the Scout Should Focus On Next" section declares: "Priority: Fix pipeline gate ordering, recursive commit subjects, and Architect parser"
   - Notes REVISE gate is "most critical — without it, the loop can never close cleanly"
   - Explicitly marks these as blocking all new feature work

**Impact:**

- **Loop corruption** — iteration counter advances despite REVISE, polluting `reflections.md` and `state.md`
- **Unreadable history** — commit subjects become meaningless noise, audit trail loses traceability
- **Lost diagnostics** — Architect failures leave no evidence for debugging
- **Momentum collapse** — loop cannot ship anything until these gates are fixed; featurework is blocked indefinitely

**Scope:**

| File | Changes | Why |
|------|---------|-----|
| `pkg/runner/reflector.go` | Read `loop/critique.md` before run; early-return on VERDICT: REVISE; expand section-detection markers (**, ##, lowercase variants); early-return on empty sections without incrementing counter | Gate prevents advancing on broken code; marker expansion fixes parsing; empty-check prevents corrupt entries |
| `pkg/runner/builder.go` | Derive commit subject from task title + diff summary only; pattern `[hive:builder] <title>`; never embed prior subject | Eliminates recursive nesting; restores readability |
| `pkg/runner/architect.go` | Capture LLM response preview (first 2000 chars) in PhaseEvent when `parseArchitectSubtasks` returns 0 | Preserves diagnostic data for future debugging |
| `pkg/runner/*_test.go` | Tests for REVISE gate blocking, commit subject stability, Architect diagnostic capture | Verify gates work and prevent regression |

**Suggestion:**

**Priority: Close the REVISE gate, fix commit subjects, capture Architect diagnostics. One iteration.**

This is pure infrastructure work — no product changes, no UX. Three substeps:

1. **REVISE gate** (most critical) — Read `loop/critique.md` at Reflector start. If it contains `VERDICT: REVISE`, emit `PhaseEvent{Phase: "reflector", Outcome: "revise_blocked"}` and return. Do not write to `reflections.md`. Do not increment `state.md` iteration counter.

2. **Commit subject** — In `builder.go`, replace the template logic that embeds git log. Use only `task.Title` and a short diff summary (e.g., "5 files changed, 120 added"). Pattern: `[hive:builder] <title>`.

3. **Architect diagnostic** — In `architect.go`, when parse returns 0 tasks, set `Preview: resp.Content()[:2000]` in the PhaseEvent. This shows future iterations what the LLM actually returned.

**Tests:**
- Verify `reflections.md` is NOT appended when Critic issues REVISE
- Verify iteration counter doesn't advance
- Verify commit subject does NOT contain "Fix: Fix:"
- Verify Architect PhaseEvent captures LLM preview on parse failure

This is the blocking prerequisite for all future work. Until this closes, the loop cannot ship anything.
