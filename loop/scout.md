# Scout Report — Iteration 405

**Date:** 2026-03-29
**Gap:** LLM-generated cause IDs reach the graph unvalidated — hallucinated IDs silently create dangling causality chains (Lesson 170)

---

## Gap

The Observer parses LLM output and uses whatever cause ID the LLM returned (`TASK_CAUSE: <id>`) directly in `CreateTask`. Nothing checks whether that ID actually exists on the graph before it is written as a cause. An LLM can hallucinate any string — a plausible-looking UUID, a title fragment, or a node ID from a prior session. When that ID is posted as a cause, the graph records a valid-looking causal link to a non-existent node. CAUSALITY is violated silently: no error, no warning, the task appears correctly created.

This is distinct from the empty-cause (`TASK_CAUSE: none`) path that was fixed in iteration 404. That path produces no cause. This path produces a *wrong* cause.

**Root:** `pkg/runner/observer.go:runObserverReason` — `t.causeID` is used without existence check.

---

## Evidence

**Lesson 170** (state.md): ghost IDs from LLM hallucination — Observer Operate path posts node IDs that may not exist on the graph, silently attaching dangling causes.

**Code (pre-fix):**
```go
causeID := t.causeID
if causeID == "" {
    causeID = fallbackCauseID
}
```
No branch for "causeID is non-empty but doesn't exist." The LLM's ID goes straight to `CreateTask`.

**`pkg/api/client.go`** — no `NodeExists` method; no way to validate an ID against the graph before use.

---

## Impact

- Every Observer Operate call where the LLM guesses or misremembers a node ID produces a causally-linked task pointing to a ghost node
- The graph cannot distinguish valid from dangling causes at query time — the violation is structural, not surfaced
- Backfill (`backfillClaimCauses` in cmd/post) does not detect or repair dangling cause IDs — it only adds missing causes to causeless nodes

---

## Scope

Strictly bounded:
1. Add `NodeExists(slug, id string) bool` to `pkg/api/client.go` — `GET /app/{slug}/node/{id}?format=json`, returns `true` on HTTP 200 only
2. In `pkg/runner/observer.go:runObserverReason`, when `t.causeID != ""`, call `NodeExists` before use; if false, log warning and replace with `fallbackCauseID`
3. Add test `TestRunObserverReason_HallucinatedCauseIDGetsReplaced` — server returns 404 for ghost ID, assert `CreateTask` uses `fallbackCauseID`

**File list:**
- `pkg/api/client.go` — new `NodeExists` method
- `pkg/runner/observer.go:runObserverReason` — existence check before cause use
- `pkg/runner/observer_test.go` — new test for hallucinated ID replacement
