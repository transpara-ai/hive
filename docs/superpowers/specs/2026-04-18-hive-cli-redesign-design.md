# Hive CLI Redesign — Surface Reorganization (Phase A)

## Status

Design — phase A of a two-phase redesign. Phase A is a surface-only reorganization of `cmd/hive` (subcommand verbs, flag scoping, renames). Phase C — daemon process plus a separate `hivectl` control client — is deferred. Phase A's job is to restructure the surface so phase C is a natural next step rather than a re-design.

## Problem

Today's `cmd/hive` invocation is structured as a flat flag soup with implicit mode dispatch:

- `--human Michael --idea ...` → multi-agent runtime (`runLegacy`)
- `--pipeline` → Scout→Builder→Critic state machine
- `--role <name>` → single-agent runner
- `--council` → one-shot deliberation
- `--ingest <path>` → post a spec to the API

Five problems compound:

1. **Magic-flag dispatch.** `--human` doubles as the operator-name parameter *and* the implicit selector for the multi-agent runtime (see `cmd/hive/main.go:115`). A new reader cannot tell what is a mode signal versus a parameter.
2. **`--loop` is misnamed.** The flag does not enable a loop — every agent already loops. It removes the quiescence timeout so idle agents block on the bus instead of exiting (`pkg/loop/loop.go:823`). The internal field is honestly named `Keepalive`; only the user-facing flag is `--loop`.
3. **`--one-shot` is overloaded.** For `--pipeline`, it means one full Scout→Builder→Critic cycle. For `--role`, it means one task with phase-frequency throttling disabled. Same flag, different semantics, no signal in the name.
4. **Cross-mode flags are not scoped.** `--interval` only applies to pipeline. `--approve-requests` and `--approve-roles` only apply to the multi-agent runtime. `--help` lists them flat with parenthetical mode hints.
5. **Lifecycle posture is implicit.** Whether a given invocation is "do this once and exit" or "stay up indefinitely" is encoded across `--loop`, `--one-shot`, `--interval`, and the choice of mode flag. There is no single place that says "this is a daemon."

## Design Principles

- **One verb per mode.** The subcommand the user types *is* the mode signal. No flag does double duty as a dispatch trigger.
- **Lifecycle is syntactic.** Where a mode supports both one-shot and long-running variants, the variant is a sub-verb (`run` vs `daemon`), not a flag.
- **Flags belong to the verb that uses them.** No mode-irrelevant flags appear under a subcommand's `--help`.
- **Names match what the code does.** `--loop` becomes either a sub-verb (`daemon`) or, where it survives as a flag, `--keepalive`.
- **Phase A introduces no new runtime behavior.** Every existing invocation has a clear mapping. Phase A is rewiring, not feature work.

## The Surface

```
hive <verb> [subverb] [flags]

╭─ Multi-agent runtime ──────────────────────────────╮
│  hive civilization run         Seed and exit at quiescence
│  hive civilization daemon      Block on bus, await webhook work
╰────────────────────────────────────────────────────╯

╭─ Fixed Scout→Builder→Critic state machine ─────────╮
│  hive pipeline run             One cycle, then exit
│  hive pipeline daemon          Loop at --interval
╰────────────────────────────────────────────────────╯

╭─ Single agent ─────────────────────────────────────╮
│  hive role <name> run          Pick one task, work it, exit
│  hive role <name> daemon       Continuous, throttled phases
╰────────────────────────────────────────────────────╯

╭─ One-shot utilities ───────────────────────────────╮
│  hive ingest <file>            Post a spec as a task
│  hive council [--topic ...]    Convene one deliberation
╰────────────────────────────────────────────────────╯
```

## Verbs and their flags

### `hive civilization run` — multi-agent, one-shot

Boots the multi-agent runtime, seeds it (via `--idea` or `--spec`), agents work to quiescence, process exits.

Required:

- `--human NAME` — operator name (no longer doubles as mode signal)

Seed (one of):

- `--idea STRING` — freeform seed
- `--spec PATH` — markdown spec file (POSTs through the same channel as `hive ingest`)

Optional:

- `--store DSN` — postgres DSN, or in-memory if omitted (`DATABASE_URL` env fallback)
- `--repo PATH` — working directory for `Operate()` calls
- `--approve-requests` — auto-approve authority requests (file writes, git ops)
- `--approve-roles` — auto-approve role proposals (skip Guardian)
- `--space SLUG` — lovyou.ai space (default: `hive`)
- `--api URL` — lovyou.ai API base

### `hive civilization daemon` — multi-agent, long-running

Same agent set as `run`. No quiescence timeout — agents block on the bus indefinitely. Webhook listener on `:8081` is always opened. Reconciliation poller is on if `LOVYOU_API_KEY` is set.

Required:

- `--human NAME`

Optional:

- `--seed-spec PATH` — optional initial seed; daemon survives past quiescence regardless
- All flags from `civilization run` except `--idea` (long-running daemons should not have a single freeform seed; use `--seed-spec` or webhook ingest)

### `hive pipeline run` — Scout→Builder→Critic, one cycle

Required:

- `--space SLUG`

Optional:

- `--api URL`
- `--store DSN`
- `--repo PATH` (or `--repos NAME=PATH,...`)
- `--budget USD` (default: 10.0)
- `--agent-id ID`
- `--pr` — branch + PR instead of pushing to main
- `--worktrees` — per-task worktrees
- `--auto-clone` — clone missing repos from registry

### `hive pipeline daemon` — Scout→Builder→Critic, looping

Same flags as `pipeline run` plus:

- `--interval DURATION` — cycle interval (default: 30m)

### `hive role <name> run` — single agent, one task

Positional: `<name>` ∈ `builder | scout | critic | monitor`.

Same optional flags as `pipeline run` (this runner already shares the lovyou.ai API contract).

### `hive role <name> daemon` — single agent, continuous

Same as `role <name> run` but with phase-frequency throttling enabled (today's default behavior when `--one-shot` is not set).

### `hive ingest <file>` — utility

Positional: `<file>` is the markdown spec.

Optional:

- `--space SLUG`
- `--api URL`
- `--priority high|normal|low` (default: `normal`)

Behavior identical to today's `--ingest`. Writes a `[SPEC]` task and (unless `HIVE_INGEST_SKIP_REPO` is set) ensures the target repo exists.

### `hive council` — utility

Optional:

- `--topic STRING`
- `--space SLUG`
- `--api URL`
- `--budget USD`
- `--repo PATH`

## Migration Mapping

Every current invocation has a one-line replacement:

| Today | Phase A |
|---|---|
| `hive --human M --idea "..."` | `hive civilization run --human M --idea "..."` |
| `hive --human M --idea "..." --loop` | `hive civilization daemon --human M --seed-spec ...` (or no seed) |
| `hive --human M --approve-requests --approve-roles --loop` | `hive civilization daemon --human M --approve-requests --approve-roles` |
| `hive --pipeline --space hive` | `hive pipeline daemon --space hive` |
| `hive --pipeline --one-shot --space hive` | `hive pipeline run --space hive` |
| `hive --pipeline --interval 15m --space hive` | `hive pipeline daemon --interval 15m --space hive` |
| `hive --role builder --one-shot` | `hive role builder run` |
| `hive --role builder` | `hive role builder daemon` |
| `hive --council --topic "..."` | `hive council --topic "..."` |
| `hive --ingest spec.md` | `hive ingest spec.md` |

Flags removed entirely from the surface:

- `--loop` — encoded by the `daemon` sub-verb
- `--one-shot` — encoded by the `run` sub-verb
- `--pipeline`, `--council`, `--role`, `--ingest` — promoted to subcommands

## Backward Compatibility

The old flag form is **not** preserved. Two callers must be updated in lockstep with the cmd change:

1. `.claude/skills/hive-lifecycle/SKILL.md` — every `go run ./cmd/hive ...` example.
2. Any shell scripts under `loop/` or `scripts/` that invoke the binary.

Discovery step (part of the implementation plan, not this spec): `grep -r "go run \./cmd/hive\|\bhive --\(human\|pipeline\|role\|council\|ingest\|loop\|one-shot\)\b"` across the repo to find every caller.

A one-cycle deprecation alias layer was considered and rejected. Reasons: (1) the alias layer would re-introduce magic-flag dispatch internally, undermining design principle 1; (2) every caller is in-tree and updatable atomically with the rename; (3) external users of `cmd/hive` are not a documented target.

## Out of Scope for Phase A

- **No autonomy collapse.** `--approve-requests` and `--approve-roles` stay as two flags. The `--autonomy=supervised|partial|full` shorthand is a behavior question for a separate spec.
- **No config-file or env-only migration.** `--store`, `--api`, `--space` stay as flags even though they are cross-cutting. A `~/.hiverc` or env-only design is for phase C.
- **No daemon/client split.** `civilization daemon` is still an in-process foreground binary. It is not a long-lived background process you talk to. Phase C is where `hive daemon` becomes an actual daemon and `hivectl` becomes the client.
- **No removal of `cmd/approve-role` or other utilities.** They keep their current invocation; only `cmd/hive` is restructured.

## Implementation Sketch (informational; the plan will detail)

`cmd/hive/main.go` currently uses the standard library `flag` package with a single global flag set and an if/else dispatch chain at lines 105–117. Phase A replaces this with one of:

- **Hand-rolled subcommand router.** A `switch os.Args[1]` at the top of `main()`, each case parses its own `flag.FlagSet`, then calls the corresponding `run*` function. No new dependency. ~150 lines of routing code, contained to `main.go`. Recommended.
- **`cobra`.** Adds a dependency. Gives nested help formatting and shell completion for free. Heavier diff. Worth it only if subcommand depth grows past two; phase A is exactly two deep.

Recommend hand-rolled. Each `run*` function already exists; phase A only changes how they are dispatched and which flags they read.

The internal config struct in `pkg/hive/runtime.go` (`hive.Config.Loop`) keeps its honest name. Only the user-facing flag is removed.

## Verification

A successful phase A:

1. `hive --help` shows the four sections above; each subcommand's `--help` shows only its own flags.
2. Every row in the migration mapping table executes equivalently to its "today" form.
3. `--loop` and `--one-shot` no longer appear anywhere in `cmd/hive/main.go`.
4. `.claude/skills/hive-lifecycle/SKILL.md` and any caller scripts are updated and pass `hive status` post-rename.
5. Existing tests in `cmd/hive/*_test.go` and `pkg/loop/loop_test.go` pass without semantic changes (only invocation-string changes in tests that exercise the CLI).

## Open Questions

None blocking. Possible follow-ups for phase B (a hypothetical interim phase):

- Should `--store` move to `DATABASE_URL`-only? It is already an env fallback; the flag may be redundant.
- Should `civilization daemon` require `--store` (refuse in-memory)? In-memory + daemon is a footgun (state evaporates on restart).
- Should `hive ingest` accept stdin (`hive ingest -`) for piped specs?

These are notes, not gates.
