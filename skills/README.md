# Operator Skills — Canonical Homes and Model Dialects

Operator verdict (Michael Saucier, 2026-07-11, in-session, recorded in
`transpara-ai/hive#265`): agent skills live **with the feature they operate**,
and model-specific variants of one feature live **together** in that feature's
home.

## The convention

1. **Per-feature canonical home.** Each skill's canonical, versioned home is
   the repository that owns the substrate the skill manages. The
   `hive-lifecycle` skill manages the hive stack, so it lives here, in
   `transpara-ai/hive`. A skill for another repo's substrate belongs in that
   repo, decided feature by feature — there is no central skill repo.
2. **Dialect subfolders.** When the skill's behavior differs by model family,
   each family gets a subfolder under the feature:

   ```text
   skills/<feature>/
     claude/   # Claude dialect (SKILL.md, Claude-native frontmatter)
     codex/    # Codex dialect (SKILL.md + agents/openai.yaml)
   ```

   Today the dialects are `claude/` and `codex/`. The set may grow with
   additional dialects for other model optimizations; each new dialect is a
   new subfolder under the same feature, never a separate home.

   **One physical copy per dialect.** Claude Code auto-discovers a repo's
   project-level skills only at `.claude/skills/<feature>/`, so that path
   holds the Claude dialect's physical files and `skills/<feature>/claude`
   is a relative symlink to it — the dialects stay together here without
   duplicating content. A dialect whose tooling has no such discovery
   requirement (Codex today) keeps its physical files directly under
   `skills/<feature>/<dialect>/`.
3. **Installed copies are caches; the repo is truth.** Local installs
   (`~/.claude/skills/<feature>/`, `~/.codex/skills/<feature>/`) are copies of
   the committed dialect folders. Drift is resolved toward the repo.
4. **Updates are reviewed.** Changes to a skill's commands or safety
   boundaries land only through a governed PR on this repo (TLC arc with
   cross-family review), never by editing installed copies in place.

## Install

Copy the dialect folder's contents into the model's local skill directory:

```bash
cp -r skills/hive-lifecycle/claude/. ~/.claude/skills/hive-lifecycle/
cp -r skills/hive-lifecycle/codex/.  ~/.codex/skills/hive-lifecycle/
```

## Safety baseline (all skills, all dialects)

Skills default to read-only help/status behavior. Start, stop, restart,
runtime launch, clean-slate, or any mutating operation requires explicit user
intent in the current turn. Skills must not embed secrets or literal private
network addresses (prefer hostnames and `localhost`), and must not assume any
public URL for the on-prem stack.
