# Critique: [hive:builder] Replace GetClaims(200) with server-side MAX before lesson count hits cap

**Verdict:** PASS

**Summary:** `strings` is still used elsewhere in client.go — correct, no orphaned import.

**Derivation chain audit:**

- **Gap**: `NextLessonNumber` used `GetClaims(200)` — at lesson 201+ the scan truncates, producing duplicate numbers. Invariant 13 (BOUNDED) violated.
- **Plan**: Add `MaxLessonNumber` SQL aggregate on the server; expose as `?op=max_lesson`; rewrite client to call it.
- **Code**: Server (`store.go`, `handlers.go`) and client (`client.go`) match the plan exactly.
- **Tests**:
  - Client: 4 cases — server returns max, empty (0), API error, malformed JSON. All correct.
  - Server: 2 DB tests — `TestMaxLessonNumberEndpoint`, `TestMaxLessonNumberEndpointEmpty`. DB tests skip locally, run in CI.
  - Reflector: `TestRunReflectorReasonLessonNumberFromGraph` updated to mock `op=max_lesson`.
  - `parseLessonNumber` deleted (dead code); its test deleted with it — correct, no orphan.

**Invariant checks:**
- **11 (IDENTITY)**: Not applicable here (lesson numbers, not entity IDs).
- **12 (VERIFIED)**: Every changed path has a test.
- **13 (BOUNDED)**: Fixed — SQL aggregate is O(1), no truncation at any count.
- **2 (CAUSALITY)**: Build.md traces the derivation clearly.
- **14 (EXPLICIT)**: `op=max_lesson` is declared in both handler and client — no inferred coupling.

No issues found.

VERDICT: PASS
