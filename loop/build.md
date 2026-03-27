# Build: Fix: add tests for buildPart2Instruction and buildOutputInstruction (apiKey empty/set branches)

- **Commit:** 616775438d27eda5207a9bf6c47feb377a3bb85b
- **Subject:** [hive:builder] Fix: add tests for buildPart2Instruction and buildOutputInstruction (apiKey empty/set branches)
- **Cost:** $0.2900
- **Timestamp:** 2026-03-27T11:55:00Z

## Task

observer.go was refactored into buildPart2Instruction + buildOutputInstruction with new apiKey=="" skip paths, but no observer_test.go exists. Invariant 12: add table-driven tests covering (a) apiKey empty ? skip text returned, no curl; (b) apiKey set ? curl command with key+slug embedded. Also cove...

## What Was Built

All done. The `observer_test.go` now has:

- **`TestBuildPart2Instruction`** — table-driven, 2 cases: empty apiKey → skip text/no curl; set apiKey → curl with key+slug
- **`TestBuildOutputInstruction`** — table-driven, 2 cases: empty apiKey → `TASK_TITLE:` text format/no curl; set apiKey → curl with key+slug/no text format  
- **`TestBuildObserverInstruction`** — new, covers top-level format: empty apiKey gets skip+text in both sections; set apiKey gets 2× curl auth headers (part2 + output section), key+slug present

Build and tests both pass clean.

ACTION: DONE

## Diff Stat

```
commit 616775438d27eda5207a9bf6c47feb377a3bb85b
Author: hive <hive@lovyou.ai>
Date:   Fri Mar 27 22:54:59 2026 +1100

    [hive:builder] Fix: add tests for buildPart2Instruction and buildOutputInstruction (apiKey empty/set branches)

 loop/budget-20260327.txt    |   4 +
 loop/build.md               |  25 +++--
 loop/critique.md            |  25 ++---
 loop/reflections.md         |  10 ++
 loop/state.md               |   3 +-
 pkg/runner/observer_test.go | 222 ++++++++++++++++++++++++++++++++++++--------
 6 files changed, 224 insertions(+), 65 deletions(-)
```
