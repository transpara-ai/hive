# TLC boundary

`tlcbridge.Bind` is Hive's intentionally thin boundary to the external TLC
workflow. It validates the stable transport fields needed for execution and
display, preserves route-specific and future TLC fields as canonical JSON, and
binds the result to independently captured source provenance.

TLC owns route selection, brief content, Critical observations, and optional
collaboration rules. Hive owns persistence, replay, retries, providers,
worktrees, pull requests, and effect guards. Improving TLC does not require a
Hive state-machine refactor.

Binding is pure: it performs no persistence, provider invocation, repository
mutation, pull-request action, merge, settings change, or deployment.

Run the boundary tests with:

```bash
go test ./pkg/hive/tlcbridge
```
