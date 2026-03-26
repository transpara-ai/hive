# Build: Tests for critique artifact write and 5-phase tree

## Gap
VERIFIED invariant: `writeCritiqueArtifact` and 5-phase pipeline tree had no tests.

## Changes

### `pkg/runner/critic_test.go`
- Added `os`, `path/filepath`, `strings` imports
- Added `TestWriteCritiqueArtifact`: tests PASS and REVISE cases by calling `writeCritiqueArtifact` with a temp dir, then reading back `loop/critique.md` and asserting the verdict and summary strings are present

### `pkg/runner/pipeline_tree_test.go`
- Already contained `TestNewPipelineTreeHasFivePhases` with the correct 5-phase assertion and "reflector" name check — no change needed

## Verification
- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test ./pkg/runner/...` — ok (1.169s)
