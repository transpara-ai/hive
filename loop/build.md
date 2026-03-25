# Build Report — Iteration 236: Tests — document injection paths and knowledge lens coverage

## Gap

Tests for document injection paths and knowledge lens coverage (Scout iter 236 — task 4 of 4).

## Files Changed

### `site/graph/handlers.go`
- **`handleKnowledge`**: Updated to query KindDocument (Limit: 50) and KindQuestion (Limit: 50) alongside claims. JSON response now includes `documents` and `questions` fields. Template render path unchanged.

### `site/graph/mind_test.go`
- Added `fmt` import.
- **`TestAutoReplyDocumentInjectionPath`** — two subtests:
  - `docs_present_injected_in_prompt`: creates 2 KindDocument nodes, calls `store.ListDocumentContext`, passes result to `mind.buildSystemPrompt`, asserts "## Space Knowledge" section and doc content present in prompt.
  - `no_docs_no_space_knowledge_section`: calls `ListDocumentContext` for a nonexistent space, asserts no "Space Knowledge" section in prompt.
- **`TestListDocumentContextBounded`** — creates 15 documents, asserts `ListDocumentContext` returns ≤10 (Invariant 13: BOUNDED).

### `site/graph/handlers_test.go`
- **`TestHandlerKnowledgeLens`** — creates a space with 1 document, 1 question, 1 claim; hits `GET /app/{slug}/knowledge` with `Accept: application/json`; asserts `documents` and `questions` fields present in response; asserts both counts within BOUNDED limit of 50.

## Verification

```
go.exe build -buildvcs=false ./...  → EXIT:0
go.exe test -buildvcs=false ./...   → ok (DB tests skip without DATABASE_URL — consistent with all other tests in this package)
```

## Notes

- `TestBuildSystemPromptDocumentInjection` (pre-existing in mind_test.go) already covers the buildSystemPrompt-level injection. New tests add coverage for the store→prompt pipeline path and the BOUNDED invariant on `ListDocumentContext`.
- All three new tests follow the `testDB(t)` skip pattern — they run with live Postgres and skip cleanly without one.
