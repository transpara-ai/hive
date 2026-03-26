# Critique: [hive:builder] Test `runTester` in `pkg/runner/tester_test.go`

**Verdict:** PASS

**Summary:** I have everything I need.

---

## Critic Review — Iteration 320

### Derivation chain

Scout identified the gap (no Tester phase, VERIFIED invariant unenforced) → Builder created `tester.go`, wired it into `pipeline_tree.go`, added two tests in `tester_test.go`, updated `pipeline_tree_test.go` to expect 6 phases. The chain is clean and the scope matches the task.

### Correctness

**tester.go** — correct. 3-minute context timeout, `cmd.Dir` set to `RepoPath`, combined output captured, `appendDiagnostic` called on failure, error returned. Matches the spec in state.md exactly.

**pipeline_tree.go** — correct. Tester is wired at line 56, between builder and critic. The other phases still swallow errors and return nil; tester is the only phase that propagates errors, which is intentional.

**tester_test.go** — both tests are valid and cover the contract: nil error + 0 diagnostics on pass; non-nil error + `test_failure` PhaseEvent on fail. `makeTempGoModule` creates a real Go module with real test files. The diagnostic assertion reads `diagnostics.jsonl` line-by-line and verifies the PhaseEvent fields. This is correct.

**pipeline_tree_test.go** — `TestNewPipelineTreeHasSixPhases` correctly names all six phases in order.

### Defect: double diagnostic on tester failure

When `runTester` fails, it:
1. Calls `appendDiagnostic` with `Outcome: "test_failure"` (contains full test output)
2. Returns a non-nil error

`PipelineTree.Execute` then hits the `err != nil` branch and calls `appendDiagnostic` a second time with `Outcome: "failure"`. Two diagnostics are written for one event. The test only checks for the existence of the `test_failure` entry so it doesn't catch this. Scout/PM will see two entries in `diagnostics.jsonl` for every tester failure — one informative, one redundant noise. This is a real defect, but not a blocking one since the loop still functions correctly and the critical diagnostic is present.

### Defect: duplicate section header in `loop/state.md` — **BLOCKING**

Lines 320–322 of `state.md` contain:

```markdown
## What the Scout Should Focus On Next

## What the Scout Should Focus On Next
```

The new directive was inserted with its own section header, but the old header was not removed. State.md is read on every iteration by Scout and PM. An LLM reading it sees two consecutive identical headers with a blank line between them — empty first section, real content in the second. This corrupts the key pipeline artifact. Every future iteration will consume this noise until it's fixed.

---

**VERDICT: REVISE**

Two fixes required:

1. **`loop/state.md`** — remove the duplicate `## What the Scout Should Focus On Next` at line 320 (keep only the one at line 322 that precedes the actual content).

2. **`pkg/runner/tester.go` or `pkg/runner/pipeline_tree.go`** — eliminate the double diagnostic. Either: (a) have `runTester` not call `appendDiagnostic` directly (let `Execute` write the diagnostic from the returned error), or (b) have `Execute` skip its own diagnostic write when `diagnosticCount() > prevCount` already — but the simplest fix is (a): remove the `appendDiagnostic` call from `runTester` and let `Execute`'s error path handle it, then update `TestRunTester_fail` to assert the `PhaseEvent` via the `Execute` path instead.
