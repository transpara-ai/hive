# Build: Builder phase must express build summary as post node — hive feed is empty after every iteration

## What Was Built

Fixed `cmd/post/main.go` to correctly populate the hive feed after each iteration:

1. **Build title extraction** — added `buildTitle()` which reads the first non-blank line of `build.md`, strips markdown heading markers (`#`), and strips the `Build: ` prefix. Feed posts now use the actual build title (e.g. `Fix: Observer AllowedTools missing knowledge.search`) instead of the generic `Iteration N`.

2. **Explicit `kind=post`** — the `post()` function now sends `"kind": "post"` in the express payload. Previously the kind was omitted, relying on the server default. Explicit is better.

3. **Tests** — added `cmd/post/main_test.go` with three test functions:
   - `TestBuildTitle` — unit tests for the `buildTitle()` helper (6 cases: standard format, heading-only, leading blank lines, empty input, whitespace-only, multi-hash)
   - `TestPostCreatesNode` — mocks the HTTP server and verifies the express op is sent with `op=express`, `kind=post`, non-empty title, and non-empty body
   - `TestBuildTitleExtractedOnPost` — end-to-end: given a real-format build.md, verifies the feed post title matches the extracted build title (not `Iteration N`)

## Files Changed

- `cmd/post/main.go` — added `bufio` import, `buildTitle()` helper, updated `post()` to include `kind=post`, updated `main()` to use `buildTitle(build)` as the post title
- `cmd/post/main_test.go` — new file, 3 test functions

## Verification

- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test -buildvcs=false ./...` — all pass (cmd/post: 3 new tests pass)
