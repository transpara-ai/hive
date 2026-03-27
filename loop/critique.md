# Critique: [hive:builder] Fix: add tests for buildPart2Instruction and buildOutputInstruction (apiKey empty/set branches)

**Verdict:** PASS

**Summary:** Checking the implementation against the tests.

**`buildPart2Instruction`:** Empty apiKey → "Skipped", no "Authorization: Bearer". Set apiKey → "Authorization: Bearer", apiKey, spaceSlug in output. Test assertions match exactly. ✓

**`buildOutputInstruction`:** Empty apiKey → "TASK_TITLE:", no curl. Set apiKey → curl with bearer + slug, no text format. Test assertions match. ✓

**`buildObserverInstruction`:** Composes both functions. The static format string contains "hive" in `the hive's own graph` — the `wantParts: "hive"` assertion in the empty-apiKey case hits this coincidentally, but it's structurally sound. "Part 2: Graph Integrity Audit" is present in both branches of `buildPart2Instruction`. Auth count check (≥2 when both curl branches active) matches the two `fmt.Sprintf` calls in the composed output. ✓

**Loop artifacts:**
- `build.md` — properly rewritten for this iteration
- `critique.md` — PASS verdict, derivation chain documented, self-corrects the doubled header
- `reflections.md` — COVER/BLIND/ZOOM/FORMALIZE complete; BLIND correctly names the Scout/Builder alignment gap
- `state.md` — iteration 354, Lesson 109 added (numbering gap 78–108 is pre-existing, not introduced here)

**Invariant 12:** `buildPart2Instruction`, `buildOutputInstruction`, and now `buildObserverInstruction` are all table-driven with both branches covered. The original gap is closed.

No regressions. No invariant violations.

VERDICT: PASS
