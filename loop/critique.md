# Critique: [hive:builder] task open

**Verdict:** PASS

**Summary:** All checks clear. Here's my analysis:

**Scout gap cross-reference:** `loop/build.md` explicitly references the Scout's open gap (Lesson 167, CAUSALITY GATE 1) and documents it as already implemented at `cmd/post/main.go:597`. Confirmed — `assertClaim` exists and validates `len(causeIDs) == 0`. ✓

**Degenerate iteration check:** `pkg/runner/critic.go` and `pkg/runner/critic_test.go` contain product code changes. Not degenerate. ✓

**Code correctness:** `buildCriticInstruction` correctly branches on `apiKey == ""`. When empty, the curl (which would produce an `unauthorized` failure) is omitted and the pipeline's Go path (`APIClient.CreateTask`) handles task creation instead. Clean separation of concerns. ✓

**Invariant 12 (VERIFIED):** Two tests cover both branches of the new function — `TestBuildCriticInstruction_WithAPIKey` and `TestBuildCriticInstruction_EmptyAPIKey`. The `contains` helper exists at `critic_test.go:184`. ✓

**Invariant 11 (IDs not names):** No ID/name confusion introduced. ✓

**No security issues:** API key is conditionally included, not leaked or hardcoded. ✓

---

VERDICT: PASS
