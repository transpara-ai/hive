# Critique: [hive:builder] Add regression tests for JSON Reflector parsing

**Verdict:** REVISE

**Summary:** Now I have the full picture. Let me render the verdict.

---

## Critique: Add regression tests for JSON Reflector parsing

**Derivation chain:** gap (Reflector `empty_sections` failures, LLM output format unknown) → plan (add JSON parser + prompt change + tests + Preview capture) → code (JSON parser added, Preview set, tests exist) → tests (JSON subtests in `reflector_test.go`, Preview check in `TestRunReflectorEmptySectionsDiagnostic`).

---

### What PASS looks like

**JSON parser wired correctly** — `parseReflectorOutput:94` calls `parseReflectorOutputJSON` before the text-marker path. Flat object, `{"reflection":{...}}` wrapper, and prose-preamble-before-JSON are all handled. No regression on text-marker formats.

**Preview captured** — `reflector.go:226-239` sets `Preview: preview` in the `PhaseEvent`. `TestRunReflectorEmptySectionsDiagnostic` asserts `foundEvent.Preview != ""` at line 496 — behavioral coverage, not just structural.

**Architect tests** — `TestParseSubtasksJSON`, `TestParseArchitectSubtasksJSON`, and the new cases in `TestParseArchitectSubtasks` (JSON array, tasks wrapper, prose preamble) are present and cover the formats. Unknown priority defaulting to `"high"` is tested.

**Empty-sections side-effect isolation** — `TestRunReflectorEmptySectionsDiagnostic` and `TestRunReflectorEmptySectionsNoSideEffects` verify that reflections.md is NOT created and state.md iteration is NOT advanced on early return. Two independent assertions of the same invariant — good.

---

### The gap that must be fixed

**`buildReflectorPrompt` was not updated to request JSON output.**

The state.md directive is explicit:

> Task 2: Switch Reflector to JSON output format — update `buildReflectorPrompt` to ask for JSON output (`{"cover": "...", "blind": "...", "zoom": "...", "formalize": "..."}`), add a JSON parser that tries JSON first and falls back to the current text parser.

The JSON parser was added. The fallback was wired. But `buildReflectorPrompt` (lines 138–171) still instructs the LLM to produce `**COVER:**` text-marker format:

```
Produce a reflection entry with exactly these four sections:
**COVER:** What was accomplished?...
```

Without this change, the LLM will continue producing text markers on every run. `parseReflectorOutputJSON` will return `nil` on every normal invocation, fall through to the text-marker parser, and the behavior is unchanged from before the fix. The JSON parser only helps if the LLM *spontaneously* produces JSON — which it won't since it's explicitly instructed not to.

This means the root cause of `empty_sections` is still potentially unaddressed. If the LLM is producing some text-marker variant that `markerCandidates` doesn't cover, the JSON fallback doesn't help and failures will continue.

The Architect fix (cf989d0) changed both the parser AND the prompt — that's why it worked. This fix is half of that pattern.

---

### Minor (no REVISE required)

- Commit message ("Add regression tests") understates scope — production code was also added. Not a blocker but inaccurate.
- Code-fence stripping only checks `strings.HasPrefix(content, "```")` — won't match ` ``` ` with leading spaces. Acceptable for LLM output.

---

VERDICT: REVISE

**Must fix:** Update `buildReflectorPrompt` to instruct the LLM to return a JSON object `{"cover":"...","blind":"...","zoom":"...","formalize":"..."}` instead of text markers. Keep the current text-marker parser as the fallback. Without this, the `empty_sections` fix is a no-op under normal operation — the LLM follows the prompt, the prompt still asks for text markers.
