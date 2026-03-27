# Build: Fix commit subject: strip [hive:*] prefix from task title in commitAndPush

## Gap
`commitAndPush` in `runner.go` formatted commit messages as `[hive:builder] <t.Title>` without stripping prior `[hive:builder]` prefixes already embedded in `t.Title` from the board, producing recursive nesting like `[hive:builder] Fix: [hive:builder] Fix: ...`.

## Changes

### `pkg/runner/runner.go`
- Added `stripHivePrefix(s string) string` helper: loops while `s` starts with `[hive:`, finds the closing `]`, trims the prefix and leading whitespace. Handles arbitrary nesting depth without regexp.
- Updated `commitAndPush`: `msg := fmt.Sprintf("[hive:%s] %s", r.cfg.Role, stripHivePrefix(t.Title))` — the role prefix is applied exactly once regardless of what's in the task title.

### `pkg/runner/runner_test.go`
- Added `TestStripHivePrefix` with three cases:
  - no prefix → unchanged
  - single `[hive:builder]` prefix → stripped to bare title
  - double-nested `[hive:builder] [hive:builder]` prefix → stripped to bare title

## Verification
- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test ./...` — all pass (`pkg/runner` 3.812s)

ACTION: DONE
