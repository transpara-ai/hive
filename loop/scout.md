## Scout Report: Iteration 320

### Gap

**Missing: Tester phase in PipelineTree — VERIFIED invariant (no code ships without tests) is not enforced in the pipeline.**

### Evidence

1. **Pipeline structure** (`pkg/runner/pipeline_tree.go:52-63`): 5 phases exist: scout → architect → builder → critic → reflector. No tester phase.

2. **Role slot exists but unimplemented** (`pkg/runner/runner.go:34`): `roleModel` maps `"tester": "haiku"`, signaling the phase was planned. But `pkg/runner/tester.go` does not exist.

3. **Invariant 12 (VERIFIED)** is core to the constitution: "No code ships without tests." Yet the pipeline has no mechanism to run `go test ./...` and fail if tests are red.

4. **State.md explicitly directs** (lines 321-390): Section "What the Scout Should Focus On Next" explicitly identifies this gap with three specific tasks, implementation guidance, and rationale.

5. **Recent commits** confirm the pipeline infrastructure is mature and ready: artifact writes are wired (build.md enrichment, critique.md artifact, reflector.md append), diagnostics collection is working (PhaseEvent type, appendDiagnostic helper), failure detection is in place.

### Impact

- **Verification gap**: The Critic reads diffs and can spot code quality issues, but cannot detect broken tests. A test failure could silently ship.
- **Trust cost**: The pipeline proves autonomy through verified work. Without test enforcement, "verified" is aspirational.
- **Feedback loop incomplete**: The Builder uses Operate() which may or may not surface test output. Direct test execution closes the loop.
- **Lesson 34 violation**: "The Scout traverses what exists. Tests don't exist, so the Scout never encounters them." A dedicated Tester phase makes test status visible to the pipeline.

### Scope

- `pkg/runner/tester.go` (new) — `runTester(ctx)` function, exec `go test ./...`, capture output, emit diagnostics on failure
- `pkg/runner/pipeline_tree.go:52-64` (modify) — insert tester phase between builder and critic
- `pkg/runner/tester_test.go` (new) — two tests: pass case (no error), fail case (test failure → diagnostic written)
- No changes to site or other repos

### Suggestion

Implement the Tester phase exactly as specified in state.md (lines 333-389). This closes the VERIFIED invariant enforcement and unblocks the pipeline to run with full confidence. Once Tester is in place, the pipeline is fully specified: scout identifies gaps → architect plans → builder codes → **tester verifies** → critic audits → reflector synthesizes. Each phase has a clear, testable responsibility.