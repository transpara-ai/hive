# Build: Add `writeCritiqueArtifact` to critic.go

## What changed

**`pkg/runner/critic.go`**
- Extracted inline critique-writing in `reviewCommit` into a new function `writeCritiqueArtifact(hiveDir, subject, verdict, summary string) error`
- New format: `# Critique: <commit subject>\n\n**Verdict:** PASS | REVISE\n\n**Summary:** <findings>`
- Replaced the old inline `os.WriteFile` block (flat format) with a call to the new function
- Error handling unchanged: log on failure, don't halt

## Verification

- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test ./...` — all pass (pkg/runner: 1.237s)

## Why

Without a properly structured `loop/critique.md`, the Reflector reads empty or malformed critique data. This gives the Reflector a consistent artifact: commit subject in the heading, bold verdict, and full LLM analysis in the summary section.
