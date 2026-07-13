# CFADA Round Log — DF-DESIGN-HIVE-TRANSPARA-ENV-FAMILY-RENAME

gate: CFADA (interim round log; the final gate artifact is written at
convergence). Author family: Claude/Anthropic. Reviewer family: Codex/OpenAI
via `codex-cli 0.144.1` `codex exec` (materially independent lineage — meets
the independence floor). Read-only audits in this workspace; full transcripts
in `codex-round1.out` / `codex-round2.out`, prompts in `round1-prompt.md` /
`round2-prompt.md`.

## Round 1 — 2026-07-12

- Audited: packet v0.2.0 blob `42ff97c0b139fbc4f77eb5f8d607d9f2119a3b56`;
  FO v0.3.0 blob `9080f0e0f0cb2882e1be8c173248bcf0ad3453a5`.
- Verdict: **FAIL (5 design blockers)**; binding + independence confirmed by
  the auditor; 121-vs-124 counting and two wording defects as observations.
- Dispositions (all accepted-repaired in FO v0.4.0 + packet v0.3.0):
  - F2 table-cell regex false-pass + unstated exit semantics → fenced R1
    Scan Protocol, PASS = empty output + exit 1, exit ≥ 2 deny,
    base-liveness precondition (94 lines at `bf3f126`).
  - F3 AC-7 protected only part of R7's domain → protected trees extended
    to `docs/designs/` (18 files), `docs/.adversarial-design/`,
    `docs/runbooks/`.
  - F4 wrong truth table + phantom tests → per-call-site table re-verified;
    §5 names only tests that exist at base.
  - F5 false "no test seam" for mcp-graph → `newServer()` seam +
    `TestNewServerDefaults`; inspection reserved for verified-seamless sites.
  - F6 unpinned confirmation referent → restatement archived + hash-pinned
    (`1e6a52b0…`) with the FO-version chain; hive-only scope grounded in the
    repo-set sweep.

## Round 2 — 2026-07-12

- Audited: packet v0.3.0 blob `81b2b32515645af768da6b572bc4945dce36af87`;
  FO v0.4.0 blob `9de5cc74be25b56e91cae43df8e7f397332a9ed3`; restatement
  sha256 `1e6a52b0…`.
- Verdict: **FAIL (4 design blockers)**. The auditor confirmed F2 repaired
  (ran the fenced protocol: 94 lines/exit 0 at base; escaped variant 0
  lines/exit 1; malformed regex exit 128 ⇒ deny path real), F6's archival
  half repaired, all §5 "existing" names real, counts reproduced, and the
  authority boundary intact.
- Findings and dispositions:
  - R2-F3 (blocker): `M`/`D`-only check bypassable via rename/type statuses
    (`R082` historical example) → **accepted-repaired** in FO v0.5.0 +
    packet v0.4.0: `--no-renames` + status allowlist (only `A` on
    allowlisted paths).
  - R2-F4 (blocker): webhook site's blank-rejecting partition (TrimSpace)
    absent from the truth table and untested → **accepted-repaired**:
    partition recorded; `TestResolveWebhookBearerTokenWholeDomain` extended
    with the whitespace-only case (current behavior asserted only).
  - R2-F5 (blocker): runner row false (observer gates; critic tested;
    seams misclassified) → **accepted-repaired**: per-file rows; observer's
    five and critic's two named existing tests; seven non-branching runners
    to inspection under the FO's refined three-way rule.
  - R2-F6 (blocker): confirmed restatement's empty-key posture vs council's
    verbatim empty-bearer client — reviewer cannot resolve →
    **escalated-to-named-human-authority**: FO v0.5.0 D-1 (Michael;
    preserve vs align). Blocks convergence until answered.
- Current bytes after repairs: packet v0.4.0 blob
  `bda05f21dccbfa967e105258fce46b65266ac8a3`; FO v0.5.0 blob
  `740a94ce286481a8dabc11753a9dab3cf624b799`. IADA round 3 passed at these
  SHAs (author-side).

## State

**OPEN — awaiting operator decision D-1.** On answer: fold in, bump, rerun
IADA, run CFADA round 3 at the final bytes. A clean CFADA will still
authorize nothing; Human Design Review (stage 6) follows it.
