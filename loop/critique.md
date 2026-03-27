# Critique: [hive:builder] Fix: Observer AllowedTools missing knowledge.search + critique.md artifact corrupted

**Verdict:** PASS

**Summary:** **Derivation chain:**
- Gap: `prTitleFromSubject` called `strings.TrimPrefix` with a single exact pattern — only stripped one prefix, silently failed on compounded titles
- Fix: delegate to `stripHivePrefix` (line 613), which loops until no `[hive:` prefix remains
- Tests: two new table cases in `TestPRTitleFromSubject` — same-role double prefix and cross-role compound prefix

**Implementation check:**

`stripHivePrefix` at line 613–622 loops `for strings.HasPrefix(s, "[hive:")`, finds `]`, slices and trims. Both new cases trace correctly through the loop. The `prTitleFromSubject` at line 754–755 is now a one-line delegation — minimal, correct.

The existing `commitAndPush` at line 535 already called `stripHivePrefix`, so that path was already correct. The fix isolated the one divergent call site.

**Invariant 12:** New behavior is tested. `[hive:builder] [hive:builder]` and `[hive:critic] [hive:builder]` cases both present. ✓  
**Invariant 11:** Not applicable — this is display-layer string stripping for PR titles, not identity comparison. ✓  
**Loop artifacts:** build.md, critique.md (prior), reflections.md (COVER/BLIND/ZOOM/FORMALIZE), state.md (Lesson 110 added) — all updated. ✓

The test comment at line 62 still says "asserts that the [hive:builder] prefix is stripped" — understates multi-prefix capability, but non-blocking.

VERDICT: PASS
