# Critique: [hive:builder] Add early return on `empty_sections` with cost fields in `runReflector`

**Verdict:** REVISE

**Summary:** ## Critic Review — Iteration 323

### Derivation chain

Scout identified two bugs: (1) parser missing `**KEY**:` format variants, and (2) no early return on `empty_sections`. Builder scoped to bug #2 only — the early return + cost fields. The build report accurately describes the scope. The code change is small and correct: +8 lines to `runReflector`, adds cost fields and a `return`.

### Code correctness

The production fix is correct:
- `usage := resp.Usage()` extracted to avoid double-call. ✓
- `CostUSD`, `InputTokens`, `OutputTokens` populated in `PhaseEvent`. ✓
- `return` added after `appendDiagnostic` — execution cannot reach `appendReflection` or `advanceIterationCounter`. ✓
- `Timestamp` not set in `PhaseEvent` — handled correctly by `appendDiagnostic` which sets it if empty (diagnostic.go:26-28). ✓

### Test coverage (VERIFIED — invariant 12)

`TestRunReflectorEmptySectionsDiagnostic` exists and was passing before this commit. It verifies the diagnostic write. But the **new behavioral claim** of this commit is: *when sections are empty, `reflections.md` is not written and `state.md` counter is not incremented*. The test does not verify either of these absences.

Specifically:
- No assertion that `reflections.md` does not exist (or was not appended to) after the `empty_sections` path.
- No assertion that `state.md` still reads `Iteration 5,` (the initial value) after the call — i.e., that the counter was not bumped.

If someone removed the `return`, the existing test would still pass. The derivation (early return prevents corruption) has no verification.

**Invariant 12 applies directly:** "If the Critic can't point to a test that covers the change, REVISE." The change is the early return — and the test doesn't cover it.

### Scope note (not a REVISE)

Bug #1 (parser not handling `**COVER**:` format) remains open. This means the loop will keep emitting `empty_sections` diagnostics — the early return now handles those gracefully, but the root cause is unresolved. That's a legitimate one-bug-per-iteration choice; it belongs on the Scout's radar for iteration 324.

---

VERDICT: REVISE

**Required fix:** Add two assertions to `TestRunReflectorEmptySectionsDiagnostic`:
1. After `runReflector` returns, assert `reflections.md` does NOT exist (or is empty) — the early return must prevent the append.
2. Assert `state.md` still contains `Iteration 5,` — the counter must NOT have been incremented.
