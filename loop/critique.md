# Critique: [hive:builder] Fix: builder title-compounding - strip existing [hive:X] prefix before prepending

**Summary:** **Derivation chain:**
- Gap: `prTitleFromSubject` used `strings.TrimPrefix` — only strips one exact `[hive:builder]` prefix, fails on compounded prefixes like `[hive:builder] [hive:builder] …` or `[hive:critic] [hive:builder] …`
- Fix: delegate to `stripHivePrefix` (already exists at line 613), which loops until no `[hive:` prefix remains
- Tests: two new cases in `TestPRTitleFromSubject` — same-role double prefix and mixed-role compound prefix

**Implementation check:**

`stripHivePrefix` loops `for strings.HasPrefix(s, "[hive:")`, finds `]`, slices + trims. Both new test cases trace correctly:
- `"[hive:builder] [hive:builder] Add KindQuestion"` → 2 iterations → `"Add KindQuestion"` ✓
- `"[hive:critic] [hive:builder] Fix: compounded prefix"` → 2 iterations → `"Fix: compounded prefix"` ✓

Existing cases unaffected. The function is also used at line 535 for commit message formatting — consistent usage.

**Invariant 12:** New behavior is tested. ✓  
**Invariant 11:** Not applicable — stripping display prefixes from commit subjects for human-readable PR titles, not identity comparison. ✓  
**No regressions, no magic values, no new violations.**

The test comment at line 62 still says "asserts that the [hive:builder] prefix is stripped" — understates the new multi-prefix capability — but that's cosmetic, not a violation.

VERDICT: PASS
