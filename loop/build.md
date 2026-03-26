# Build: Add early return on `empty_sections` with cost fields in `runReflector`

- **Commit:** 1f92fce15757ba46abbb264673320c471e7d99ce
- **Subject:** [hive:builder] Add early return on `empty_sections` with cost fields in `runReflector`
- **Cost:** $0.3484
- **Timestamp:** 2026-03-26T21:43:09Z

## Task

In `pkg/runner/reflector.go`, after `r.appendDiagnostic(...)` on the empty-sections path, add `return` so execution never reaches `appendReflection` or `advanceIterationCounter`. Also populate `CostUSD`, `InputTokens`, and `OutputTokens` in the `PhaseEvent` from `resp.Usage()` so future PM prompts c...

## Diff Stat

```
commit 1f92fce15757ba46abbb264673320c471e7d99ce
Author: hive <hive@lovyou.ai>
Date:   Fri Mar 27 08:43:08 2026 +1100

    [hive:builder] Add early return on `empty_sections` with cost fields in `runReflector`

 loop/budget-20260327.txt |  4 ++
 loop/build.md            | 40 ++++++-------------
 loop/scout.md            | 59 ++++++++++++++++++++---------
 loop/state.md            | 99 +++++++++++++++++++++++++++---------------------
 pkg/runner/reflector.go  | 10 ++++-
 5 files changed, 120 insertions(+), 92 deletions(-)
```
