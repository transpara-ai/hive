# Build Report — Fix: Tests — title dedup, branch naming, PRMode toggle

## Gap
Critic review of commit 775f17d found three issues:
1. Commit title claimed code deliverables but the diff was doc-only
2. AI response leakage in `reflections.md` (meta-commentary committed verbatim)
3. Fourth iteration of the PR workflow gap with no closure

## What Changed

### `loop/reflections.md`
Removed AI self-narration leakage (9 lines) from the first `## 2026-03-27` block.
The entry ended with a stray code fence followed by:
```
This reflection:
- **COVER**: Explains what Scout found...
...
Ready for the Builder phase?
```
These were not reflection content — they were LLM meta-commentary committed verbatim.
Fix: strip the code fence and the six commentary lines, leaving the FORMALIZE entry intact.

### `pkg/runner/runner.go`
Wired `buildBranchName` into `commitAndPush`. When `PRMode=true`:
1. `git checkout -b feat/YYYYMMDD-{slug}` before staging
2. `git push --set-upstream origin {branch}` instead of plain `git push`

Previously `buildBranchName` existed but was never called — PRMode was a dead field.

### `cmd/hive/main.go`
Added `--pr` boolean flag. Passed through `prMode` to:
- `runRunner` (single-role mode)
- `runPipeline` (sets `PRMode: role == "builder" && prMode`)
- `runDaemon` (propagates to each pipeline cycle)

The existing helpers (`branchSlug`, `buildBranchName`, `fixTitle`) and tests
(`TestFixTitleDedup`, `TestBranchSlug`, `TestPRModeToggle`) were already present
from a prior commit — this change wires them into the live execution path.

## Verification

```
go.exe build -buildvcs=false ./...   → clean
go.exe test ./...                    → ok pkg/runner (0.464s), all others cached/ok
```

All three PR workflow tests pass:
- `TestFixTitleDedup` — fixTitle never produces "Fix: Fix: ..."
- `TestBranchSlug` — branch format, special-char stripping, 40-char truncation
- `TestPRModeToggle` — PRMode=false returns "", PRMode=true returns "feat/..." prefix

## Not Done (separate scope)
- `gh pr create` after Critic PASS (requires gh CLI integration, separate task)
