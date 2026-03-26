Perfect! I now have a complete picture. Let me write the Scout gap report.

---

## GAP REPORT — Iteration 310

### Gap
The autonomous pipeline's failure detection is incomplete. Phases can fail internally but the pipeline never detects them or creates fix tasks, so failures propagate silently and the feedback loop breaks.

### Evidence

**Phase 1 (complete):** Infrastructure exists in recent commits:
- `pipeline_tree.go` (commit 5f0d762) — PipelineTree structure, phases wired
- `diagnostic.go` (commit 1131217 and follow-ups) — PhaseEvent and appendDiagnostic() working
- Tests in `pipeline_tree_test.go` (commit b652025) cover failure path
- PM role reads diagnostics (commit 1f16a05) to feed into prompt

**Phase 1 gap (explicit):** In `pipeline_tree.go` lines 22-23:
```
// Those methods do not return errors today; the wrappers always succeed. Real failure detection is
// Phase 2 work once the phase methods propagate errors up.
```

**Where Execute is called:** `runner.go` line 178, in `runTick`:
```go
case "pipeline":
    _ = NewPipelineTree(r).Execute(ctx)
```

The return value is discarded (`_`). Execute gets errors but the loop doesn't act on them.

**Missing pieces:**
1. **countDiagnostics()** — No helper to count lines in diagnostics.jsonl. Execute can't tell if a phase appended errors.
2. **Failure→FixTask mapping** — Execute catches errors but doesn't call `r.cfg.APIClient.CreateTask()` to create a fix task.
3. **Test coverage** — No test verifies that a failed phase creates a fix task on the board.

### Impact

The autonomous pipeline is currently a one-way street: Scout → Architect → Builder → Critic. When the Builder fails (Operate timeout, verification failure, etc.), that failure is logged to diagnostics.jsonl but **no task is created to fix it**. The next Scout cycle sees the same broken state and the loop spins or stalls.

Without fix-task creation:
- Ship → Catch → Fix loop is aspirational, not structural
- Pipeline can't self-correct (Lesson 59: "Ship → Catch → Auto-fix is next")
- Failures accumulate invisibly in diagnostics.jsonl while the pipeline keeps running
- No feedback mechanism to prioritize high-failure areas

### Scope

**Files involved:**
- `pkg/runner/pipeline_tree.go` — Add failure detection logic
- `pkg/runner/diagnostic.go` — Add countDiagnostics() helper
- `pkg/runner/pipeline_tree_test.go` — Extend tests to cover fix-task creation

**No changes needed:**
- `pkg/runner/runner.go` — runTick already calls Execute, just needs error handling to work
- `pkg/api/client.go` — CreateTask already exists and works (verified in PM role)
- Phase methods (runScout, runBuilder, etc.) — Keep their current signatures

### Suggestion

Implement Phase 2 as scoped in `state.md` lines 549-586:

1. **Add `countDiagnostics(hiveDir string) int`** in diagnostic.go — count lines in loop/diagnostics.jsonl, return 0 if file doesn't exist

2. **Update `Execute`** to detect failures before/after each phase:
   - Snapshot diagnostic count before phase
   - Call phase.Run(ctx)
   - After: if error returned OR diagnostic count increased → failure detected
   - On failure: call `pt.cfg.APIClient.CreateTask(pt.cfg.SpaceSlug, "Fix: "+phase.Name+" phase failed", "", "high")` to create a fix task
   - Then return the error

3. **Add tests** in pipeline_tree_test.go:
   - Verify countDiagnostics returns 0 for missing file, N for N lines in existing file
   - Verify Execute returns error when phase.Run returns error
   - Verify Execute returns error when diagnostic count increases (phase appended error)
   - Mock APIClient to verify CreateTask was called with correct title

This closes the feedback loop structurally: if a phase fails, a human-visible fix task appears on the board, and the next Scout cycle can prioritize it.

---

**That's the gap report.** The Architect will now design a specific plan. The Scout's job was to surface the highest-priority missing piece, and that piece is clearly **failure detection that triggers fix-task creation** — it's the structural linchpin that turns error visibility into corrective action.