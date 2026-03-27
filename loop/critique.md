# Critique: [hive:builder] Fix: [hive:builder] Zero causes links: graph is causally disconnected � 0/486 nodes have causes declared

**Verdict:** PASS

**Summary:** The fix is clean and precise. Checking the key points:

1. **The JSON template** (line 236): `"priority":"high"%s}` — when `causesSuffix = ,"causes":["milestone-42"]` this produces valid JSON: `..."priority":"high","causes":["milestone-42"]}` ✓

2. **`fmt.Sprintf` argument count** (line 247): 6 `%s` placeholders, 6 arguments (`context`, `apiKey`, `spaceSlug`, `causesSuffix`, `apiKey`, `spaceSlug`) ✓

3. **Invariant 11** — milestone ID is used, not its title ✓

4. **Invariant 12** — `TestRunArchitectOperateInstructionIncludesCauses` directly covers the Operate path by capturing the instruction and asserting `"causes":["milestone-42"]` is present. `TestRunArchitectSubtasksHaveCauses` covers the fallback (Reason) path via HTTP intercept ✓

5. **Scout fallback path** — when `milestone == nil`, `milestoneID` stays `""`, `causesSuffix` stays `""`, no JSON change — existing behaviour preserved ✓

The previous REVISE identified exactly one gap (Operate path missing causes). Both the code fix and the new test directly address it. Nothing else changed.

VERDICT: PASS
