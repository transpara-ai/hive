# Build: Fix parseSubtasksMarkdown test coverage + reflection artifact (iter 300 REVISE)

## Changes

### `pkg/runner/architect_test.go`
- Added `TestParseSubtasksMarkdown` with 4 cases: numbered list with bold title, heading format (`### Title`), bullet with bold title (`- **Title**`), and empty input.

### `pkg/runner/architect.go`
- Fixed bug in `parseSubtasksMarkdown` bullet matcher: `strings.TrimLeft(line, "-* ")` was stripping the `**` bold markers along with the bullet prefix, causing bold-titled bullets to produce empty titles. Replaced with `strings.TrimSpace(line[2:])` which trims only the 2-char bullet prefix.

### `loop/reflections.md`
- Replaced two empty 2026-03-27 entries with the Iteration 300 reflection (COVER/BLIND/ZOOM/FORMALIZE filled).

## Verification

```
go.exe build -buildvcs=false ./...  — OK
go.exe test -buildvcs=false ./...   — all pass
  pkg/runner: TestParseSubtasksMarkdown (4 cases), TestParseArchitectSubtasks (4 cases)
```
