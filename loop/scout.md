# Scout Report — Iteration 229

## Gap Identified

**Repo mismatch: Scout creates hive tasks, Builder targets site repo.** Lesson 56: "The Scout must know the Builder's target." The pipeline ran but produced no useful output — the Scout created a hive infrastructure task, the Builder couldn't implement it on the site repo.

## Plan

1. Read the target repo's CLAUDE.md (if it exists) — this has the product context
2. Include target repo path and a `ls` of top-level structure in the Scout prompt
3. Add explicit instruction: "Create tasks for the TARGET REPO, not the hive."
4. Extract just the "What the Scout Should Focus On Next" section from state.md (instead of truncating the whole file)
5. Re-run the pipeline and verify: Scout creates a site product task → Builder implements it → Critic reviews

## Priority

**P0** — Without this fix, the pipeline can't ship product features autonomously.
