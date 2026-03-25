# Build Report — Fix: KindQuestion Route + state.md Duplicate Header

## What Changed

### 1. state.md — Removed duplicate `## Current Directive` block
**File:** `hive/loop/state.md`

The builder had inserted the new Q&A directive above the existing document-edit directive instead of replacing it. Result: two identical `## Current Directive — Iteration 235+` H2 headers. Removed the old document-edit directive block (the second one), keeping only the current Q&A directive.

### 2. handlers.go — Fixed singular route to plural
**File:** `site/graph/handlers.go`

Changed `GET /app/{slug}/question/{id}` to `GET /app/{slug}/questions/{id}` — consistent with the list route and the KindDocument pattern (`/documents/{id}`).

### 3. views.templ — Fixed detail link to plural
**File:** `site/graph/views.templ`

Changed `/app/%s/question/%s` to `/app/%s/questions/%s` in the QuestionsView list item link so it points to the corrected route.

### 4. handlers_test.go — Updated test URL to plural
**File:** `site/graph/handlers_test.go`

Changed test URL from `/app/handler-qa-test/question/` to `/app/handler-qa-test/questions/` in the `question_detail` subtest.

### 5. views_templ.go — Regenerated
Ran `templ generate` after the views.templ change. 15 updates processed.

## Critic Issue 3 (answer count) — Not a bug
`QuestionsView` uses `q.ChildCount` (populated by the store from `child_count` DB column). Answers are stored as child nodes (parent_id = question ID), so `ChildCount` correctly reflects the answer count. No fix needed.

## Verification

```
templ generate           → 15 updates, no errors
go.exe build ./...       → clean
go.exe test ./...        → ok
```
