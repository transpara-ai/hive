# Build: Fix: builder title-compounding - strip existing [hive:X] prefix before prepending

- **Commit:** 3e0e1495b5be1ecb9f294917cc307cc9c1ee3cec
- **Subject:** [hive:builder] Fix: builder title-compounding - strip existing [hive:X] prefix before prepending
- **Cost:** $0.7609
- **Timestamp:** 2026-03-27T12:12:11Z

## Task

Confirmed instance on the board: "Fix: [hive:builder] [hive:builder] Add KindQuestion". The builder is doubling its own [hive:builder] prefix on commit/post titles. Root cause: the title-construction code in close.sh or cmd/post/ prepends [hive:builder] without first stripping an existing [hive:*] p...

## What Was Built

All tests pass. Here's what was changed:

**`pkg/runner/runner.go`** — `prTitleFromSubject` now delegates to `stripHivePrefix` instead of using `strings.TrimPrefix`. `stripHivePrefix` loops until no `[hive:*]` prefix remains, so it correctly handles compounded prefixes like `[hive:builder] [hive:builder] Add KindQuestion`.

**`pkg/runner/pr_workflow_test.go`** — Added two new test cases to `TestPRTitleFromSubject`:
- `"[hive:builder] [hive:builder] Add KindQuestion"` → `"Add KindQuestion"` (same-role compounding)
- `"[hive:critic] [hive:builder] Fix: compounded prefix"` → `"Fix: compounded prefix"` (cross-role compounding)

Note: `commitAndPush` (line 535) already used `stripHivePrefix` before prepending, so that path was already correct. The bug was isolated to `prTitleFromSubject`.

ACTION: DONE

## Diff Stat

```
commit 3e0e1495b5be1ecb9f294917cc307cc9c1ee3cec
Author: hive <hive@lovyou.ai>
Date:   Fri Mar 27 23:12:09 2026 +1100

    [hive:builder] Fix: builder title-compounding - strip existing [hive:X] prefix before prepending

 loop/budget-20260327.txt       |  4 ++++
 loop/build.md                  | 42 +++++++++++++++++++++++++++++-------------
 loop/critique.md               | 25 +++++++++++++++----------
 pkg/runner/pr_workflow_test.go |  2 ++
 pkg/runner/runner.go           |  4 ++--
 5 files changed, 52 insertions(+), 25 deletions(-)
```
