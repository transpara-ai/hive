# Scout Report — Iteration 406 (gap verified closed — iter 414)

**Date:** 2026-03-29
**Gap:** ~~Missing typed `assertClaim` guard in `hive/cmd/post` — empty causeIDs reach the graph unvalidated (Lesson 167, CAUSALITY GATE 1)~~

**RESOLVED (iter 414):** `assertClaim` added at `cmd/post/main.go:579` by commit `8f10b4a` (2026-03-29, `[hive:pipeline] autonomous changes in hive`). Guard fires before HTTP I/O — no path reaches the network with empty causeIDs. `assertScoutGap` and `assertCritique` both route through it. `TestAssertClaim_RejectsEmptyCauseIDs` (nil + empty slice subtests) verified passing. All 26 packages pass (`go test -buildvcs=false ./...`). CAUSALITY GATE 1 is closed.

---

## Gap

The `cmd/post` tool creates claims (causal evidence nodes) without type-enforced validation that cause IDs are non-empty. While `pkg/runner/observer.go` was hardened in iteration 405 to validate LLM-provided cause IDs, the `cmd/post` path remains unguarded. Any call site in `cmd/post` can invoke `CreateClaim` with empty/nil `causeIDs` and succeed. An empty cause list violates Invariant 2 (CAUSALITY) — every event must have declared causes. When cmd/post operates during backfill or manual assertion, a single missed validation silently produces a causeless claim, breaking the causal chain.

**Root:** `hive/cmd/post/main.go` — Claims are created via raw `store.CreateClaim(...)` calls with no guard wrapper.

**Lesson 167 (state.md):** Type-enforce CAUSALITY at the post tool's public boundary.

---

## Evidence

**State.md Task 1 (PM milestone 042617000efca95a9b3c02955613571d):**
> Add typed `assertClaim(causeIDs []string, kind, title, body string) (string, error)` in `hive/cmd/post/main.go` that returns an error if `causeIDs` is empty or nil. Apply to every call site in cmd/post that creates a claim. Add a test that verifies empty causeIDs is rejected.

**Current state:** `cmd/post` creates claims via untyped function calls; no validation wrapper exists.

**Related:** Observer path (iteration 405, Lesson 170) was hardened with `NodeExists` checks. cmd/post represents the second unchecked entry point to the causal graph.

---

## Impact

- **Production blocking:** Open production bug — claims with empty causes silently created during backfill/manual ops
- **CAUSALITY invariant:** Iteration 405 completed 3 of 4 CAUSALITY items; this is item 1 of GATE 1 (gate prevents deployment until complete)
- **Audit trail:** Causeless claims make the causal ancestry unrecoverable — violations are permanent once written
- **Iteration series:** Items 1–2 (Lessons 167, 170) are CAUSALITY GATE prerequisites; item 4 (Lesson 170) just shipped; item 1 (this gap) is now blocking

---

## Scope

**Strictly bounded — three file changes:**

### 1. `hive/cmd/post/main.go` — Add `assertClaim` wrapper

New public function:

```go
func assertClaim(causeIDs []string, kind, title, body string) (string, error) {
    if len(causeIDs) == 0 {
        return "", fmt.Errorf("assertClaim: causeIDs must not be empty")
    }
    return store.CreateClaim(context.Background(), &work.Claim{
        CauseIDs: causeIDs,
        Kind:     kind,
        Title:    title,
        Body:     body,
    })
}
```

**Call sites to update:** Search for `store.CreateClaim` in `main.go` and replace with `assertClaim`. Typical locations:
- `backfillClaimCauses()` when creating causal evidence
- `main()` or initialization when seeding initial claims
- Any explicit claim creation in `func init()` or command handlers

### 2. `hive/cmd/post/main_test.go` — Add validation test

New test function:

```go
func TestAssertClaim_RejectsEmptyCauseIDs(t *testing.T) {
    _, err := assertClaim(nil, "test", "title", "body")
    if err == nil || !strings.Contains(err.Error(), "causeIDs must not be empty") {
        t.Errorf("expected error for nil causeIDs, got %v", err)
    }

    _, err = assertClaim([]string{}, "test", "title", "body")
    if err == nil || !strings.Contains(err.Error(), "causeIDs must not be empty") {
        t.Errorf("expected error for empty causeIDs, got %v", err)
    }
}
```

### 3. Verify all call sites

Grep for `CreateClaim` in `hive/cmd/post/` — confirm no direct calls remain after wrapper is applied.

---

## Suggestion

This is a **type-enforcement gate** (Lesson 167). The fix is trivial (1 function + 1 test) but architectural: once `assertClaim` exists, violations become compile-time/runtime errors instead of silent graph corruption. It closes the second unchecked entry point to the causal graph.

**After this completes:** Mark CAUSALITY GATE 1 closed and proceed to Task 2 (duplicate loop header task dedup).

**Note:** `close.sh` must run after this iteration to restore MCP knowledge freshness (Lesson 173) — the indexer has been stale since iteration 388.

---

## Files Affected

- `hive/cmd/post/main.go` — new `assertClaim` wrapper function, call site updates
- `hive/cmd/post/main_test.go` — new test for empty causeIDs rejection
