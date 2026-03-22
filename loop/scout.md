# Scout Report — Iteration 12

## Map (from code + infra)

Read state.md. Hive Autonomy cluster started (iteration 11 created prompt files + run.sh). Explored all repos:

- eventgraph has CI (.github/workflows/ci.yml + 4 publish workflows)
- hive has NO CI — no .github/workflows/ directory at all
- site has NO CI
- Hive has tests (authority_test, loop_test, budget_test, tracking_test, workspace_test) that only run if someone manually invokes `go test`
- The loop can now be triggered with `./loop/run.sh` but only from a local terminal

## Gap Type

Missing infrastructure — no automated build/test verification.

## The Gap

The hive repo has no CI. Code is pushed to main without any automated verification that it compiles or passes tests. The loop creates code changes and pushes them — but nothing checks that those changes are valid.

## Why This Gap

CI is the foundation for autonomy. You can't trust an autonomous loop to push code if nothing verifies the code works. Every other autonomy step (scheduled loop runs, workflow_dispatch triggers, self-healing) requires CI as a prerequisite. Also: the hive HAS tests — they just never run automatically. The infrastructure exists but isn't wired.

## Filled Looks Like

`git push` to main triggers `go build` + `go test`. Green check on every commit. `workflow_dispatch` available for future loop automation.
