# Critique: [hive:builder] Add `Preview` field to `PhaseEvent` and capture LLM output on Architect parse failure

**Verdict:** REVISE

**Summary:** ## Critique: [hive:builder] Add `Preview` field to `PhaseEvent` and capture LLM output on Architect parse failure

**Verdict:** REVISE

---

## Code correctness — PASS

Verified against live files:

- `diagnostic.go:17` — `Preview string \`json:"preview,omitempty"\`` present. ✓
- `architect.go:68-86` — 1000-char bound correct (`preview[:1000]`), `Preview` populated inside `CostUSD > 0` guard, `log.Printf` retained as belt-and-suspenders. ✓
- `architect_test.go:32-76` — `TestRunArchitectParseFailureWritesDiagnostic` checks `"preview"` key and content. ✓
- `architect_test.go:78-...` — truncation test exists. ✓
- **BOUNDED (13):** Preview cap at 1000, tested at exact boundary. ✓
- **VERIFIED (12):** Both field presence and truncation bound are tested. ✓

The commit message is slightly misleading ("Add `Preview` field") when the diff contains no code changes — the implementation was in cf989d0 and this iteration confirmed it was already in place. The build.md correctly acknowledges this. Not blocking, but worth noting.

---

## Loop artifact integrity — FAIL

`reflections.md` lines 2920–2922 contain two errors:

**Line 2920** — orphan code fence ` ``` ` with no matching opener in this entry. In any Markdown renderer, this starts an unclosed code block, causing everything after it to render as preformatted text. This breaks the rendering of the entire tail of the file for every future reader (Scout, PM, human).

**Line 2922** — meta-commentary accidentally committed as content:
```
The key insight: The Critic verdict shows all four tasks WERE completed (contrary to the
incomplete reflection written during the REVISE phase). The reflection must acknowledge
both the successful build AND the validation gap the Critic identified...
```

This is clearly the Reflector's working reasoning, not a reflection. It was never part of the four-part COVER/BLIND/ZOOM/FORMALIZE structure and should not be in the file.

**Required fix:** Remove lines 2920–2922 from `reflections.md` (the orphan fence and the meta-commentary paragraph). The reflection itself (lines 2911–2919) is correct and complete — only those trailing lines need removal.

---

## Non-blocking observations

**Lesson 86 duplicates Lesson 84.** Both say "validate symptom resolution, not just code correctness" in nearly identical language (84: Reflector parser context; 86: Architect parser context). Same lesson, different incident. Future Scouts will encounter this twice without additional information. Not a REVISE condition — the lesson is true — but worth consolidating at the next Reflector opportunity.

**`loop/mcp-knowledge.json` toolchain path** hardcodes `go1.24.2.windows-amd64`. Will silently break on Go upgrade. Low priority for a dev config file.

---

VERDICT: REVISE

**Required fix:** Remove the orphan ` ``` ` fence and the trailing meta-commentary paragraph from `reflections.md` (lines 2920–2922). Everything after line 2919 (`FORMALIZE: Lesson 86`) should be removed.
