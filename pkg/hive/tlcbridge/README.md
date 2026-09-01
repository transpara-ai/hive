# Exercise the TLC bridge boundary

`tlcbridge.Bind` validates a complete TLC change brief, combines it with
Hive-private source identity, and returns a deterministic idempotency key plus
repository-confined effects. It performs no persistence or external action.

Run the executable example:

```bash
go test ./pkg/hive/tlcbridge -run '^(TestExerciseRecord|TestBriefCannotInjectHiveExecutionOrRepositoryState|ExampleBind_forExerciseRecord)$' -v
```

The example captures an Issue in `transpara-ai/repo-x`, calls
`tlcbridge.Bind`, and records all three returned boundary values:

```text
IdempotencyKey
Effects.WorktreeRepository = transpara-ai/repo-x
Effects.PullRequestRepository = transpara-ai/repo-x
```

Both repository effects come only from the independently supplied
`Source.Repository`; the public TLC brief cannot override either target.
Changing the source identity changes the idempotency key. Repeating the same
normalized source and brief produces the same key.

Application code must capture `Source.Kind`, `Source.Identity`, and
`Source.Repository` from its trusted ingress, pass the original JSON bytes to
`Bind`, and treat the returned `BoundRequest` as a library result. This example
does not construct another intake type, register or invoke a dispatcher,
persist state, create a worktree, open a pull request, or call a provider.
