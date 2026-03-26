# Build: Wire Tester into `PipelineTree`

## Task
Wire Tester into `PipelineTree` in `pkg/runner/pipeline_tree.go`; fix builder/critic/reflector closures to propagate errors.

## Findings

The tester phase was **already wired** in `NewPipelineTree` (line 56, added in commit 97d92e6):

```go
{Name: "tester", Run: func(ctx context.Context) error { return r.runTester(ctx) }},
```

It is correctly positioned after `builder` and before `critic`, and correctly propagates the error from `runTester`.

The secondary task — "fix builder/critic/reflector closures to propagate errors" — cannot be applied as described. The task claimed these closures "ignore the return value from runBuilder, runCritic, runReflector", but all three functions return `void`:

- `func (r *Runner) runBuilder(ctx context.Context)` — runner.go:192
- `func (r *Runner) runCritic(ctx context.Context)` — critic.go:15
- `func (r *Runner) runReflector(ctx context.Context)` — reflector.go:100

There is no return value to propagate. The existing `; return nil` in the closures is correct for void functions. Changing the closures alone cannot fix error propagation — the underlying function signatures would need to change (a separate task).

## Files Changed

None — the pipeline was already correct as of the previous iteration.

## Verification

- `go.exe build -buildvcs=false ./...` — success
- `go.exe test ./...` — all packages pass
