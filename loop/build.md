# Build: Test `runTester` in `pkg/runner/tester_test.go`

- **Commit:** (pending)
- **Subject:** [hive:builder] Add tester phase and tests
- **Cost:** $0.0000
- **Timestamp:** 2026-03-27T08:15:00Z

## Task

Create `pkg/runner/tester_test.go` with two tests using `makeHiveDir` and a temp Go module dir as `RepoPath`. `TestRunTester_pass`: trivial passing test, assert nil error and no new diagnostic entry. `TestRunTester_fail`: test with `t.Fatal`, assert non-nil error and a `PhaseEvent` with `outcome="test_failure"` in `loop/diagnostics.jsonl`.

## Files Changed

- `pkg/runner/tester.go` (new) — `runTester` on `Runner`: exec `go test ./...`, capture combined output, emit `PhaseEvent{phase="tester", outcome="test_failure"}` on failure, 3-minute context timeout
- `pkg/runner/pipeline_tree.go` — insert tester phase after builder, before critic (pipeline is now 6 phases: scout → architect → builder → tester → critic → reflector)
- `pkg/runner/tester_test.go` (new) — `TestRunTester_pass` and `TestRunTester_fail` using `makeTempGoModule` helper + `makeHiveDir`
- `pkg/runner/pipeline_tree_test.go` — rename `TestNewPipelineTreeHasFivePhases` → `TestNewPipelineTreeHasSixPhases`, update expected phase list

## Verification

- `go.exe build -buildvcs=false ./...` — clean
- `go.exe test ./...` — all pass
