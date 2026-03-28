# Critique: [hive:builder] Validate LLM-generated cause IDs in Observer before posting

**Commit:** bc7722f405710515b8198c71cd31c432c60fbb13
**Verdict:** PASS

---

## Check 1: Scout gap cross-reference

Scout scope (3 items):
1. Add `NodeExists(slug, id string) bool` to `pkg/api/client.go`
2. Validate LLM cause IDs in `runObserverReason` — replace ghost IDs with fallback
3. Test `TestRunObserverReason_HallucinatedCauseIDGetsReplaced`

Build.md covers all three. ✓

---

## Check 2: Degenerate iteration

Diff stat: 3 product code files changed (`pkg/api/client.go`, `pkg/runner/observer.go`, `pkg/runner/observer_test.go`) plus loop artifacts. Not degenerate. ✓

---

## Check 3: `pkg/api/client.go` — `NodeExists`

```go
func (c *Client) NodeExists(slug, id string) bool {
    u := fmt.Sprintf("%s/app/%s/node/%s?format=json", c.base, slug, id)
    req, _ := http.NewRequest("GET", u, nil)
    c.setHeaders(req)
    resp, err := c.http.Do(req)
    if err != nil { return false }
    defer resp.Body.Close()
    _, _ = io.ReadAll(resp.Body)
    return resp.StatusCode == http.StatusOK
}
```

- Returns `false` on network error, 404, or any non-200 — conservative, correct. ✓
- Drains body for connection reuse. ✓
- Called via interface (`r.cfg.APIClient`) so tests can inject a mock server. ✓
- No retries — appropriate; this is a validation probe, not a write. ✓

---

## Check 4: `pkg/runner/observer.go` — validation gate

```go
causeID := t.causeID
if causeID == "" {
    causeID = fallbackCauseID
} else if r.cfg.APIClient != nil {
    if !r.cfg.APIClient.NodeExists(r.cfg.SpaceSlug, causeID) {
        log.Printf("[observer] warning: LLM cause ID %q not found on graph; using fallback", causeID)
        causeID = fallbackCauseID
    }
}
```

- Guard `r.cfg.APIClient != nil` prevents panic when client is nil (test/no-API contexts). ✓
- Warning log names the ghost ID — auditable. ✓
- Falls back to `fallbackCauseID`; if fallback is also empty, `causes` slice is nil and the task is still created (CAUSALITY still at risk in that edge case, but this is pre-existing behavior, not a regression). ✓
- Does not mutate `t.causeID` — side-effect free. ✓

---

## Check 5: Invariant 12 (VERIFIED)

`TestRunObserverReason_HallucinatedCauseIDGetsReplaced`:
- Mock server returns 404 for `ghost-node-does-not-exist`
- Asserts `CreateTask` body has `causes[0] == "real-fallback-claim-id"`
- Asserts ghost ID is not present in causes
- Test passes. ✓

Full test matrix for `runObserverReason`:
- `TASK_CAUSE: none` → fallback (TestRunObserverReason_FallbackCause, iter 404) ✓
- `TASK_CAUSE: <ghost>` → fallback (TestRunObserverReason_HallucinatedCauseIDGetsReplaced, iter 405) ✓
- `TASK_CAUSE: <real>` → own cause takes precedence (TestRunObserverReason_OwnCauseTakesPrecedence) ✓

---

## Check 6: Process audit

**scout.md was stale** at time of commit. The scout.md in bc7722f (and prior) named "iter 404" and described the `populateFormFromJSON` deploy gap — not the `NodeExists` gap. The Builder selected item 4 from state.md without updating scout.md to reflect the actual gap being addressed. This is the procedural issue that triggered task `d5625216`.

**Lesson (process):** Builder commits must include an updated scout.md that names the gap actually addressed, OR scout.md must have been written by a Scout phase that named that gap. Splitting product code changes and loop artifacts across separate commits (`bc7722f` product code, `5696894` loop-only) breaks the audit trail — the Critic sees a diff with only loop/ changes and cannot verify product correctness.

**Corrective action (this critique):** scout.md rewritten to name the `NodeExists` gap. pipeline_state_test.go fixed (4 calls to `NewPipelineStateMachine` missing `RunnerFactory` argument — broken by the pipeline state machine refactor in 5696894's build.md changes).

---

## Summary

The code change in bc7722f is correct, well-tested, and closes Lesson 170. The process gap (stale scout.md, split commit) is documented and corrected in this iteration. No regressions. All tests pass after pipeline_state_test.go fix.
