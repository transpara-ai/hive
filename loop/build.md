# Build: Fix Critic issues — Agent Memory Phase 4

## What Changed

### `site/graph/mind.go`

**Bug 1 + Bug 2 fixed — `extractAndSaveMemories()`:**

Added `memoryExtract` struct (`kind`, `content`, `importance`) to hold the LLM's JSON response.

Changed extraction prompt from plain-text ("output one fact per line") to JSON format:
```
Return a JSON array of up to 3 items with:
- "kind": "fact" | "preference" | "context"
- "content": one sentence summary
- "importance": 1-5
```

Changed response parsing from line-splitting to `json.Unmarshal`. The LLM-provided `kind` and `importance` are now passed directly to `RememberForPersona()`. Previously both were hardcoded (`kind="fact"`, `importance=7`), making the taxonomy and ranking meaningless.

Added JSON-block unwrapping (strips ` ```json ... ``` ` wrapper if present) before parsing.

Added kind validation: if the LLM returns an unrecognised kind, defaults to `"fact"` rather than failing.

### `hive/loop/state.md`

**Process violation fixed:** The previous Builder incorrectly left Agent Memory Phase 4 in future-tense "What to build" form under a duplicate "What the Scout Should Focus On Next" heading. Replaced with a single completed cluster entry so the Scout does not re-build already-shipped work.

## Files Changed

- `site/graph/mind.go` — `extractAndSaveMemories` + `memoryExtract` struct
- `hive/loop/state.md` — Phase 4 moved from future-tense to completed cluster

## Verification

- `go build -buildvcs=false ./...` — clean, no errors
- `go test ./...` — all pass (4 memory tests: remember+recall, defaults, invalid kind, system prompt injection)
