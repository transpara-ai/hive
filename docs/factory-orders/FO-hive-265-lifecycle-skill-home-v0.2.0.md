---
doc_id: FO-HIVE-265-LIFECYCLE-SKILL-HOME
title: Factory Order — Canonical Versioned Home for the hive-lifecycle Skill (Claude + Codex Dialects)
doc_type: factory-order
status: proposal
version: 0.1.0
created: 2026-07-11
updated: 2026-07-11
owner: Michael Saucier
steward: claude
primary_repo: transpara-ai/hive
source_issue: transpara-ai/hive#265
authority: repository documentation/skill-source preservation only; no Hive start/stop/restart, daemon launch, runtime execution, service restart, deploy, public exposure, private fetch, authentication change, protected settings change, production EventGraph read/query/write, Work runtime write, Test 001 GREEN, production go-live, value allocation, autonomy increase, or wiki work
---

# Factory Order — Canonical Versioned Home for the hive-lifecycle Skill

## Immutable Source Citations

| Source | Pin | Role |
|---|---|---|
| [transpara-ai/hive#265](https://github.com/transpara-ai/hive/issues/265) | issue body as of 2026-07-11 (labels `cc:intake`, `cc:pr-deferred`, `cc:protected-action`, `cc:civilization-presence`, `cc:needs-human-scope`) | Raw intake — the governed tracker for the Codex skill port |
| Michael Saucier, in-session operator scope verdict, 2026-07-11 | "Claude and Codex versions of a particular feature together in the same repo … choose the correct home for the feature … differences subdivided (as of today) into Claude and Codex subfolders … may grow to various dialects" | Channel A human scope decision this FO implements; supplies the `needs-human-scope` answer |
| `~/.claude/skills/hive-lifecycle/SKILL.md` | current local content (runbook rewritten from code via hive PR #259) | Claude dialect source |
| `~/.codex/skills/hive-lifecycle/` (`SKILL.md`, `agents/openai.yaml`) | current local content, validated by the Codex skill validator per #265 kickoff evidence | Codex dialect source (the port to preserve) |

## Requirements

- **R1 — Canonical home selected and recorded.** The hive-lifecycle skill's
  versioned home is `transpara-ai/hive:skills/hive-lifecycle/` (per-feature
  home: the skill manages this repo's stack). The convention itself — feature
  home + dialect subfolders (`claude/`, `codex/`, future dialects) — is
  recorded in `skills/README.md` with the operator verdict cited.
- **R2 — Both dialects preserved verbatim.** `skills/hive-lifecycle/claude/SKILL.md`
  and `skills/hive-lifecycle/codex/{SKILL.md, agents/openai.yaml}` are
  byte-copies of the local sources cited above.
- **R3 — Codex structure valid.** The in-repo Codex dialect passes the Codex
  skill validator (`quick_validate.py`), including frontmatter and UI
  discovery metadata (`agents/openai.yaml`).
- **R4 — Safety posture preserved.** Both dialects default to read-only
  help/status; mutating lifecycle actions require explicit user intent in the
  current turn; no `ANTHROPIC_API_KEY`/`HIVE_ANTHROPIC_API_KEY` use.
- **R5 — No private addresses or secrets.** No literal private-network
  addresses (hostnames/`localhost` only) and no credentials anywhere under
  `skills/`.
- **R6 — Update path defined.** Future changes to lifecycle commands or
  safety boundaries are reviewed via governed PRs on this repo (TLC arc with
  cross-family review); installed copies are caches, repo is truth
  (`skills/README.md`, `skills/hive-lifecycle/README.md`).

## Non-Goals

- No Hive start/stop/restart, daemon launch, or runtime execution.
- No changes to the skill's commands or semantics in this arc — preservation
  only; content diffs from the cited sources are defects.
- No installer tooling or symlink automation (a later slice if wanted).
- No relocation of other skills; this arc moves exactly one feature.

## Verification Plan

- `python3 ~/.codex/skills/.system/skill-creator/scripts/quick_validate.py skills/hive-lifecycle/codex` → valid.
- `diff` of committed dialect files against the cited local sources → identical.
- Private-address/secret scan over `skills/` → no matches.
- `git diff --check` clean; repo build/tests unaffected (no Go changes).
- IAR then CFAR (Codex reviewer; Claude author) at the exact PR head; merge
  consideration remains Michael's.

## Non-Authorizations

This Factory Order states intent and grants nothing beyond the governed PR
flow. It does not authorize any lifecycle mutation the skill documents, nor
any label (`cc:*`) movement on the source issue — label state remains
human-owned.
