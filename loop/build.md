# Build Report — Iteration 375

**Gap:** `mcp__knowledge__knowledge_search` blind to graph claims — all 103+ lesson/critique claims return zero results because `handleSearch` truncates file content at 4000 chars and claims.md is 72KB.

**Root cause:** `handleSearch` reads each file's first 4000 chars only. `claims.md` is 72,126 bytes. "Lesson 109" starts at line 631 (~47KB in) — invisible to search.

## Changes

### `cmd/mcp-knowledge/main.go`

1. **`topic` struct** — added `Content string` field for in-memory content (claim nodes have content but no file path).

2. **`buildHiveLoop`** — after constructing the `loop/claims` file topic, calls `parseClaims(path)` and attaches the result as `child.Children`. The file topic itself is unchanged (preserves existing test behavior); individual claims are additional children.

3. **`parseClaims(path string) []topic`** — new function. Splits `claims.md` on `"\n## "` into sections. Each section becomes a `topic{Kind: "claim", Content: "## Title\n\nbody"}` with ID `loop/claims/<slug>`. Deduplicates slugs by appending `-2`, `-3` for collisions (claims.md has three distinct "Lesson 109" entries).

4. **`claimSlug(title string) string`** — converts a claim title to a URL-safe lowercase slug (max 60 chars, collapse hyphens).

5. **`claimSummary(body string) string`** — extracts first meaningful line from claim body, skipping `**State:**` metadata lines.

6. **`handleSearch`** — added Content search before the file-content check: if `t.Content != ""`, searches `t.Content` case-insensitively. Returns early (claim nodes have no Path to also check).

7. **`handleGet`** — added Content check before the Path/category checks: if `node.Content != ""`, returns it directly. This makes `handleGet("loop/claims/lesson-109")` return the specific claim.

### `cmd/mcp-knowledge/main_test.go`

Added three new tests:

- **`TestHandleSearchFindsDeepClaims`** — builds a claims.md where the target claim ("Lesson 109") is well past the 4000-char window. Verifies search finds it and returns a `loop/claims/` ID.
- **`TestHandleGetIndividualClaim`** — verifies a claim can be fetched by its `loop/claims/<slug>` ID and returns the body content.
- Added `"fmt"` to imports.

## Verification

```
go.exe build -buildvcs=false ./...  pass
go.exe test -buildvcs=false ./...   pass (7/7 mcp-knowledge, all others cached/pass)
```

## Acceptance

- `knowledge_search("lesson 109")` now returns individual claim topics from `loop/claims/`
- `knowledge_search("causality")` finds claims mentioning CAUSALITY anywhere in their body
- `knowledge_get("loop/claims/<slug>")` returns specific claim content
- `knowledge_get("loop/claims")` unchanged, still returns full file
