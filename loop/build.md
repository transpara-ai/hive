# Build: Daemon resets to main before each PRMode cycle

- **Files changed:**
  - `cmd/hive/main.go` — added `os/exec` import; added `daemonResetToMain` call at top of `runDaemon` loop body (guarded by `prMode`); added `daemonResetToMain` helper
  - `pkg/runner/runner_test.go` — appended `TestBranchResetOnDaemonCycle` test
- **What changed:** When `prMode` is true, each daemon cycle runs `git fetch origin && git checkout main && git pull origin main` in `repoPath` before `runPipeline`. Branch is logged before and after. The guard condition (`PRMode=false → buildBranchName returns ""`) is verified by the new test.
- **Build:** `go.exe build -buildvcs=false ./...` — OK
- **Tests:** `go.exe test ./...` — all pass
- **Timestamp:** 2026-03-27T00:00:00Z
