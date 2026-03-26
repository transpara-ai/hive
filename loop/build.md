# Build: Remove stale directives from `loop/state.md`

- **Commit:** 7ff78d56640cee48f422cd78c51bea424d44bdb0
- **Subject:** [hive:builder] Remove stale directives from `loop/state.md`
- **Cost:** $0.3908
- **Timestamp:** 2026-03-26T20:58:43Z

## Task

In `loop/state.md`, delete the seven completed directive sections listed by the PM (iterations 234+, 236+, 240+, 242+, 263+, and the two "Scout Directive" / "Make /hive Real" sections). Keep only the current directive and lessons. This recovers ~3000 tokens on every PM/Scout call.

## Diff Stat

```
commit 7ff78d56640cee48f422cd78c51bea424d44bdb0
Author: hive <hive@lovyou.ai>
Date:   Fri Mar 27 07:58:43 2026 +1100

    [hive:builder] Remove stale directives from `loop/state.md`

 loop/budget-20260327.txt |   3 +
 loop/build.md            |  15 +--
 loop/critique.md         |  53 ++++++-----
 loop/reflections.md      |  23 +++++
 loop/state.md            | 234 +----------------------------------------------
 5 files changed, 62 insertions(+), 266 deletions(-)
```
