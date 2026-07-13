# Operator Migration Runbook — `LOVYOU_*` → `TRANSPARA_*` environment rename

**Executed by the operator only, after this PR merges. Never run by CI or the PR itself.**

After merge, a freshly built binary reads the `TRANSPARA_*` names while the live
nucbuntu deployment still exports `LOVYOU_API_KEY`. In that window the
key-*gated* paths (ingest, pipeline, role, civilization, webhook) are
fail-closed or default-safe (required-error, skip-with-notice, Site client
disabled, built-in defaults). **Exception — `cmd/mcp-graph`:** the changed
`loop/mcp-graph.json` supplies neither the new key nor a local base URL, so
`mcp-graph` defaults to `https://transpara.ai` and its `apiGet`/`apiPost`
transmit even with an empty key — MCP tool input can reach production during
this window. Before starting, set a local `TRANSPARA_BASE_URL` for `mcp-graph`
or avoid MCP tool operations until the migration below completes. This runbook
makes the window zero for the managed services.

## Affected units and files (resolved live 2026-07-13 — re-verify before running)

- **Env file:** `~/.config/hive/hive.env`
- **`systemd --user` units with `EnvironmentFile=…/hive.env`:** `hive.service`,
  `work-server.service` (resolve again with
  `grep -rl 'hive.env' ~/.config/systemd/user/`).
- **Local skill installs:** `~/.claude/skills/hive-lifecycle`,
  `~/.codex/skills/hive-lifecycle`.
- **MCP config:** the repo's `loop/mcp-graph.json` (now carries no credential and
  uses `TRANSPARA_SPACE`); update any operator-local copy you maintain.

> `hive.service` carries `--approve-requests --approve-roles` in its ExecStart, so
> restarting it resumes full autonomy — restart it only with explicit current-turn
> approval, per the hive-lifecycle skill's start/restart preflight. The steps below
> restart only `work-server.service` (and `hive-ops-api.service` if it consumes the
> key) unless you deliberately choose to bounce `hive.service`.

## Steps (each with its check)

1. **Rename the key(s) in `~/.config/hive/hive.env`.**
   `LOVYOU_API_KEY` → `TRANSPARA_API_KEY` (and `LOVYOU_BASE_URL` →
   `TRANSPARA_BASE_URL`, `LOVYOU_SPACE` → `TRANSPARA_SPACE` if present).
   *Check:* `grep -c '^\s*LOVYOU_' ~/.config/hive/hive.env` → `0`.

2. **Reload systemd and restart the affected stack services.**
   `systemctl --user daemon-reload`
   `systemctl --user restart work-server.service` (add `hive-ops-api.service`
   if it references the key; add `hive.service` only with current-turn approval).
   *Check:* `systemctl --user is-active work-server.service` → `active`.

3. **Confirm the credential posture under the new name (per restarted unit).**
   Run the hive-unit preflight (`hive factory preflight-hive-unit` with the
   deployed binary) against a unit that was restarted in step 2 — a unit still
   running on the pre-rename environment reports the new key absent.
   *Check:* credential posture reports **PRESENT** under `TRANSPARA_API_KEY`
   (field `transpara_api_key=present`).

4. **Confirm no legacy name remains in the effective unit environment (names only).**
   For each running unit, read `/proc/<MainPID>/environ` and list variable names
   only (never print values):
   `tr '\0' '\n' < /proc/$(systemctl --user show -p MainPID --value work-server.service)/environ | cut -d= -f1 | grep '^LOVYOU_'`
   *Check:* no `LOVYOU_` name in the effective environment (empty output).

5. **Re-sync the local skill installs from the repo** (FO-265 `rsync -a --delete`
   convention; adjust the source path to your checkout):
   `rsync -a --delete <repo>/.claude/skills/hive-lifecycle/ ~/.claude/skills/hive-lifecycle/`
   `rsync -a --delete <repo>/skills/hive-lifecycle/codex/ ~/.codex/skills/hive-lifecycle/`
   The Codex dialect's files under `skills/hive-lifecycle/codex/` install to the
   skill **root** `~/.codex/skills/hive-lifecycle/` (per `skills/README.md`) — a
   nested `…/codex/` target would leave the active root `SKILL.md` stale and able
   to blank `LOVYOU_API_KEY` while `TRANSPARA_API_KEY` is live.
   *Check:* `grep -RE 'LOVYOU_' ~/.claude/skills/hive-lifecycle ~/.codex/skills/hive-lifecycle` → no matches.

6. **Update operator-local MCP config copies.**
   Ensure any local copy of `loop/mcp-graph.json` carries no credential value and
   no `LOVYOU_` name, and that `TRANSPARA_API_KEY` is supplied via the environment
   (not the JSON — the child inherits it).
   *Check:* the local MCP config has no `LOVYOU_` name and no committed credential.

## Post-migration acceptance

Per service, **after it has been restarted onto the renamed `hive.env`** (step 2):
the preflight posture reads **PRESENT** under `TRANSPARA_API_KEY`, and **no
`LOVYOU_` name** appears in that unit's effective `/proc/<MainPID>/environ`.

A still-running service that was **not** restarted (e.g. `hive.service` when its
approval-gated restart is deferred) keeps the old environment in `/proc` and is
explicitly **not yet migrated** — its steps 3–4 checks are expected to still show
the legacy name and must not be read as failure; restart it (with current-turn
approval) before asserting its acceptance.

## Not covered here (separate operator actions)

- **Rotation** of the exposed `lv_b7fb22…` credential on the transpara.ai service —
  urgent, independent of this rename, and the only thing that kills the leaked value
  (removing it from the tree does not). See the rotation runbook.
- The pre-existing empty-key confidentiality behavior and the stale operator-doc
  claims found during design are deferred to a separate governed order (see the FO's
  Named Residual Risks).
