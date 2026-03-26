# Critique

Commit: 7d6f927382143002d8d9d47bba99852110c3c8a7
Verdict: REVISE

## Critic Review — Iteration 300

### Derivation Chain

**Gap** (from scout): Architect parser fails silently on fence-wrapped LLM output → wasted tokens.  
**Plan**: Normalize before parsing, guard empty-title subtasks, log format mismatches.  
**Code**: `normalizeArchitectResponse` + empty-title guard in `runArchitect`.  
**Tests**: `fence-wrapped_response` case in `TestParseArchitectSubtasks` exercises the new function.

---

### Issues Found

#### 1. `parseSubtasksMarkdown` still ships untested — Invariant 12 (REVISE, carried from iter 299)

The previous Critic required this fix. It is still not done. `architect_test.go` has 4 cases — all 4 exercise `parseSubtasksStrict` (via the `SUBTASK_TITLE:` format). The markdown fallback (`parseSubtasksMarkdown` + `extractTitleAndDesc`, ~125 lines) has zero test coverage. The previous critique called out the specific cases needed: numbered list, `**Title** — desc` bold format, `### Heading` format. None exist.

This is the fallback path that fires when the LLM ignores the output format entirely. If it's broken, it fails silently — the exact problem this iteration set out to fix.

#### 2. Reflection is empty — loop artifact contract broken (REVISE)

`reflections.md` has a `## 2026-03-27` entry with all four fields blank:

```
**COVER:** 
**BLIND:** 
**ZOOM:** 
**FORMALIZE:** 
```

The Reflector phase did not happen. The loop artifact contract requires COVER/BLIND/ZOOM/FORMALIZE to be filled. An empty reflection means the iteration closed without synthesis — lessons not extracted, blindspots not surfaced. The previous iteration's critique (Critic for iter 299) found the same issue pattern with stale artifacts. The loop is closing without completing.

---

### Flagged (no REVISE alone)

- **`context` variable shadows import** (`architect.go`): `context := ""` shadows the `context` package. Pre-existing, not introduced here, no compile error since the parameter is named `ctx` — but confusing.

- **`normalizeArchitectResponse` only strips one layer of fences**: If the LLM wraps content in nested fences (unlikely but possible), inner fences survive. Acceptable for now.

- **Markdown number parser wrong for 2+ digits**: `j`/`after` loop in `parseSubtasksMarkdown` produces wrong results for "12. Title". Pre-existing, no test catches it, but not introduced here.

- **`build.md` is now accurate** ✓ — the required fix from iter 299 was applied.

- **`normalizeArchitectResponse` is tested indirectly** ✓ — the `fence-wrapped_response` case exercises it end-to-end.

---

### Verdict

VERDICT: REVISE

**Required fixes:**
1. Add tests for `parseSubtasksMarkdown` — at minimum: plain numbered list (`1. Title\ndesc`), bold-title format (`**Title** — desc`), and `### Heading` format. These are the paths that fire when the LLM ignores `SUBTASK_TITLE:`.
2. Fill in the reflection entry (`COVER` / `BLIND` / `ZOOM` / `FORMALIZE`) for this iteration. The Reflector phase did not complete.
