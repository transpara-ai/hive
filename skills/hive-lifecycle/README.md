# hive-lifecycle — Operator Skill (Claude + Codex dialects)

The operator runbook for the transpara-ai hive stack on nucbuntu: status,
help, safe startup/shutdown/restart of the Dockerized Postgres and the
systemd `--user` services (`work-server`, `hive-ops-api`), read-only telemetry
probes, on-demand `cmd/hive` runtime commands, logs, and clean-slate
diagnostics.

Canonical home per `skills/README.md`: this repo, this folder. Dialects:

| Dialect | Contents | Installed at |
|---|---|---|
| `claude/` | symlink to `.claude/skills/hive-lifecycle/` — the physical `SKILL.md` lives there because Claude Code auto-discovers project skills only at that path (committed via #259) | project-level: auto-discovered in this repo; user-level: `~/.claude/skills/hive-lifecycle/` |
| `codex/` | `SKILL.md` + `agents/openai.yaml` (Codex frontmatter + UI discovery metadata) | `~/.codex/skills/hive-lifecycle/` |

There is exactly one physical copy of each dialect in this repo; the symlink
keeps both dialects visible together here without duplicating content.

## Provenance (hive#265)

- Source of the runbook content: the Claude skill at
  `~/.claude/skills/hive-lifecycle/SKILL.md` (rewritten from current code via
  hive PR #259).
- The Codex dialect was ported locally by Codex to
  `~/.codex/skills/hive-lifecycle/` and validated with the Codex skill
  validator; this folder preserves that port in a governed, versioned home.
- Both dialects then received the reviewed safety repairs enumerated in
  `FO-HIVE-265-LIFECYCLE-SKILL-HOME` v0.5.0 (CFAR findings, hive#267): the
  repo copies are truth; re-sync local installs from the repo after merge
  using the install commands in `skills/README.md`.

## Validation

```bash
python3 ~/.codex/skills/.system/skill-creator/scripts/quick_validate.py skills/hive-lifecycle/codex
```

Plus: no literal private-network addresses (hostnames and `localhost` only),
no secrets, read-only default posture in both dialects.

## Safety boundaries

Both dialects default to read-only status/help. Mutating lifecycle operations
(start/stop/restart, runtime launch, clean slate, destructive database
actions) run only on explicit user request in the current turn. Neither
dialect sets `ANTHROPIC_API_KEY`/`HIVE_ANTHROPIC_API_KEY` (they break the
Claude CLI subscription auth). Updates to commands or boundaries are reviewed
via governed PRs on this repo.
