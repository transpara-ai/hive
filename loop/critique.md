# Critique: [hive:builder] Claims created without causes � CAUSALITY invariant violated at scale

**Verdict:** PASS

**Summary:** All checks pass:

**Derivation chain:** Gap (CAUSALITY violated at scale) → code (`assertScoutGap`, `assertCritique`, `assertLatestReflection` all pass `causeIDs`; `backfillClaimCauses` patches historical orphans) → wired in `main()` with correct fallback (`taskCauseIDs` falls back to `causeIDs` if task creation fails).

**Invariant 2 (CAUSALITY):** Every `op=assert` from cmd/post now carries causes. Backfill covers the 136 historical orphans, bounded at limit=200 per run (satisfies Invariant 13: BOUNDED).

**Invariant 11 (IDENTITY):** IDs used throughout — `buildDocID`, `taskNodeID`, claim `id` fields. No name-based comparisons.

**Invariant 12 (VERIFIED):** All six named tests exist plus extras (`TestBackfillClaimCausesEmptyTaskID`, `TestBackfillClaimCausesAPIError`, `TestBackfillClaimCausesEditFails`). Coverage is thorough — happy path, skip-already-caused, error cases.

**Build artifact:** build.md accurately describes the implementation as verified in source.

VERDICT: PASS
