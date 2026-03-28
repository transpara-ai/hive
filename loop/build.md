# Build: Critique claims asserted without causes - Invariant 2 violated in cmd/post

- **Commit:** c504022ed8ef7c89b48597309362a0dd3b60591e
- **Subject:** [hive:builder] Critique claims asserted without causes - Invariant 2 violated in cmd/post
- **Cost:** $0.3581
- **Timestamp:** 2026-03-28T17:26:18Z

## Task

Every Critique claim on the board has causes:[]. Critiques should declare the build task they reviewed as their cause (Invariant 2: CAUSALITY). Verified on claims 7be80a26, d48b61be, bda493db - all recent Critique PASS claims have empty causes arrays. Fix: in cmd/post or loop critique phase, when po...

## What Was Built

The code is already correct and all tests pass. Let me verify the fix is complete:

1. `assertCritique` in `main.go:641` receives `causeIDs` and includes them in the `causes` field when non-empty (line 657-659)
2. `main()` computes `taskCauseIDs = []string{taskNodeID}` (or falls back to `buildDocID`) and passes it to `assertCritique` at line 139
3. `backfillClaimCauses` retroactively patches old causally-floating claims
4. Tests `TestAssertCritiqueCarriesTaskNodeIDasCause` and `TestAssertCritiqueSendsCauses` explicitly verify the causes field is passed through
5. Build: clean (`go build` exits 0)
6. Tests: all pass

The autonomous hive already landed this fix in commit `3e145a3`. The claims verified in the task description (7be80a26, d48b61be, bda493db) would be retroactively linked via `backfillClaimCauses` on the next `cmd/post` run.

ACTION: DONE

## Diff Stat

```
commit c504022ed8ef7c89b48597309362a0dd3b60591e
Author: hive <hive@lovyou.ai>
Date:   Sun Mar 29 04:26:17 2026 +1100

    [hive:builder] Critique claims asserted without causes - Invariant 2 violated in cmd/post

 loop/budget-20260329.txt | 1 +
 1 file changed, 1 insertion(+)
```
