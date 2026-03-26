# Build: Feed recent diagnostics into PM prompt

## Gap
PM prompt had no visibility into recent pipeline failures. Phases that fail and burn tokens without producing output were invisible to the PM, so it could keep issuing directives that depended on broken infrastructure.

## Changes

### `pkg/runner/pm.go`
- Added `readRecentDiagnostics(hiveDir string) string` — reads the last 20 lines of `loop/diagnostics.jsonl`, parses each as a `PhaseEvent`, and formats them as a human-readable list (timestamp, phase, outcome, cost, error).
- Updated `runPM` to call `readRecentDiagnostics` and pass the result to `buildPMPrompt`.
- Updated `buildPMPrompt` signature to accept `recentFailures string`.
- Added `## Recent Pipeline Failures` section to the PM prompt template, placed between "Completed Work" and "Current Scout Directive" so the PM sees failure context before issuing a new directive.

## Verification
- `go.exe build -buildvcs=false ./...` — success, no errors
- `go.exe test ./...` — all packages pass (`pkg/runner`: 1.108s)
