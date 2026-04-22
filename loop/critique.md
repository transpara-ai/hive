# Critique: [hive:builder] test task

**Verdict:** PASS

**Summary:** `★ Insight ─────────────────────────────────────`
1. The diff moves `apiKey := os.Getenv("LOVYOU_API_KEY")` before the `canOperate` type assertion so both the LLM instruction path and the Go `CreateTask` path share a single key-presence check — a good example of eliminating dual state.
2. The new test `TestReviewCommit_NoAPIKey_SkipsCreateTask` uses a mock server that sets `createTaskCalled = true` on any `intend` op, then asserts it was never set — clean black-box verification of the guard.
3. `TestParseVerdictCaseSensitivity` uses `t.Logf` instead of `t.Errorf` for mismatches, making it non-fatal — but the existing `TestParseVerdict` already covers those cases with `t.Errorf`, so there is no coverage gap on that path.
`─────────────────────────────────────────────────`

---

**Required Check 1 — Scout gap cross-reference:**

`loop/scout.md` open gap: `hive/cmd/post/main.go` — missing `assertClaim` wrapper, CAUSALITY GATE 1.

The new `loop/build.md` says: *"Per `loop/state.md` (DONE item 8), this was already implemented in iter 408. The current fix addresses the Critic's task-creation loop — a separate infrastructure gap identified by this iteration."*

Build.md explicitly references the Scout's gap. ✓

**Required Check 2 — Degenerate iteration:**

Changed files: `loop/budget-20260422.txt`, `loop/build.md`, `loop/critique.md`, `loop/test-report.md`, **`pkg/runner/critic.go`**, **`pkg/runner/critic_test.go`**. Product code is present. Not degenerate. ✓

---

**Code review:**

`critic.go:112` — `apiKey` is now at function scope, before the `canOperate` block. Both the instruction-building path and the `CreateTask` guard share the same value. ✓

`critic.go:163-169` — Guard skips `CreateTask` when `apiKey == ""`, with a log message. The `return` exits the REVISE case entirely — the critique artifact has already been written at that point, so the Builder can still pick up the REVISE signal from the file. ✓

`critic.go:263-264` — Explicit prohibition: `"Do NOT attempt to create a task via curl, Bash, or any other tool — there is no API key in this environment and any such call will return 401 Unauthorized."` Closes the LLM-side failure path. ✓

**Test coverage (Invariant 12):**

| Test | New code covered | Assertion style |
|------|-----------------|-----------------|
| `TestBuildCriticInstruction_EmptyAPIKey` (updated) | prohibition text present | `t.Error` ✓ |
| `TestReviewCommitFixTaskHasCauses` (updated) | with-key path calls CreateTask | `t.Fatal` ✓ |
| `TestReviewCommit_NoAPIKey_SkipsCreateTask` (new) | no-key guard skips CreateTask | `t.Error` ✓ |

All three critical tests use proper failure assertions. The `TestParseVerdictCaseSensitivity` test uses `t.Logf` making it non-fatal, but it covers existing (not new) code and the same cases are hard-asserted in `TestParseVerdict`.

**Invariant 11 (IDENTITY):** No ID/name confusion. ✓  
**Invariant 2 (CAUSALITY):** Fix tasks still receive `causes` from the critique claim ID (lines 177-179). ✓  
**Invariant 13 (BOUNDED):** No unbounded operations introduced. ✓

---

VERDICT: PASS
