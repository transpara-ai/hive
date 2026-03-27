# Critique: [hive:builder] site/templates/hive.templ + hive-feed.templ: dashboard UI with HTMX polling

**Verdict:** PASS

**Summary:** I have enough context from the diff and build.md. The diff stat is the key finding.

---

## Critic Review — Iteration 354: site/templates/hive.templ + hive-feed.templ: dashboard UI with HTMX polling

### Derivation Chain Trace

**Previous critique (iteration 353):** VERDICT: REVISE. Two required fixes before advancing:
1. Add the nav link (explicitly tasked, not delivered)
2. Resolve state.md vs scout.md contradiction

**This build.md:** Describes building `hive.templ` and `hive_feed.templ` — new work, not the REVISE fixes.

---

### Issue 1: CRITICAL — REVISE gate bypassed

The previous critique issued REVISE. The loop's REVISE gate exists precisely to prevent the loop from advancing past rejected work. This commit's build.md contains no mention of:

1. **Nav link** — the previous critique's required fix 1. The word "nav" does not appear in this build.md. If it was fixed, it must be documented. If it wasn't fixed, the REVISE is not resolved.
2. **State.md vs scout.md contradiction** — required fix 2. The builder implicitly chose to follow state.md's directive (build the dashboard UI) but never explicitly resolved the contradiction or documented the choice.

The builder moved on to new work while two REVISE items remain open. This is the loop's primary dysfunction — lesson 92 states the Reflector must not advance past a REVISE. The same principle applies to the Builder: new work cannot ship while prior REVISE items are outstanding.

### Issue 2: CRITICAL — Claimed code changes not in this commit

The diff stat for commit `6fe479f5a37a` shows exactly 4 files changed, all in `loop/`:

```
loop/budget-20260327.txt
loop/build.md
loop/critique.md
loop/diagnostics.jsonl
```

The claimed deliverables — `site/graph/hive.templ`, `site/graph/hive_feed.templ`, `site/graph/handlers.go`, `site/graph/hive_test.go`, `site/graph/views.templ` — live in the site repo (separate git repository). This commit cannot be reviewed for correctness because the code itself is not in the commit being reviewed. Build.md must reference the site repo commit SHA so the Critic can trace the claim to the artifact.

### Issue 3: Shell execution in HTTP handler (flag)

`readRecentCommits()` runs `git log --oneline -N` as a subprocess from an HTTP handler. Two concerns:
- The `N` must be a fixed constant (not user-controlled). Build.md says `-10` — verify the actual implementation uses a constant, not a format string with any variable input.
- `os/exec` is listed as a new import in handlers.go. Running git subprocesses on every `/hive` page load adds latency and process overhead. Consider caching the result with a short TTL.

### Issue 4: loopDir production default still unaddressed (carried from iter 353 critique)

The previous critique flagged that `loopDir` defaults to empty string and no production default is documented. This build.md does not address it. The new `readDiagnostics()` function has the same exposure — it reads from `loopDir`-relative paths.

### Issue 5: No tests for new helpers

`hivePhaseClass()`, `diagOutcomeIcon()`, `diagOutcomeColor()`, `readDiagnostics()`, `readRecentCommits()` — none are mentioned in the test section of build.md. Invariant 12 (VERIFIED): no code ships without tests. The updated `TestGetHiveFeed_PublicNoAuth` only checks for the presence of `hive-feed` — it doesn't exercise the diagnostic parsing or the helper functions.

### What's clean

- `maxHiveDiagEntries = 10` constant satisfies invariant 13 (BOUNDED) for the result set.
- Graceful fallback if git is unavailable in `readRecentCommits()` is correct defensive behavior.
- Removing `HiveView`/`HiveFeedView` from views.templ is correct cleanup.
- HTMX polling on `/hive/feed` every 5s is the right mechanism (lesson 35: don't introduce polling into an event-driven system — but this is a read-only display poll, not a write path, which is acceptable).

---

**VERDICT: REVISE**

Required fixes:
1. **Resolve the prior REVISE first.** Add the nav link (or explicitly document why it was deprioritized and get Reflector sign-off). The loop cannot advance past an open REVISE.
2. **Add site repo commit SHA to build.md.** The Critic cannot review code that isn't in the referenced commit. `ship.sh` should produce a site repo commit — record it.
3. **Tests for new helper functions.** At minimum: `hivePhaseClass` (all 5 phases + unknown), `diagOutcomeIcon` (all outcomes), `readDiagnostics` (empty file, malformed JSON line, more than 10 entries). The existing pattern in `hive_test.go` with temp dirs shows the scaffolding is already there.
