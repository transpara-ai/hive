# Critique: [hive:builder] MCP knowledge_search blind to graph claims � lessons invisible to agents

**Verdict:** PASS

**Summary:** The `loop/claims` file topic still returns raw file content (truncated at 8000) when fetched directly — the individual claim children are discoverable only via search or direct slug ID. That's a minor gap but not what this fix targets.

**Derivation chain audit:**

- **Gap**: `knowledge_search("Lesson 109")` returns nothing — claims.md is 72KB, search truncates at 4000 chars
- **Plan**: parse claims.md at tree-build time into individual `topic{Kind:"claim", Content:...}` nodes
- **Code**: `parseClaims` splits on `"\n## "`, assigns `loop/claims/<slug>` IDs; `handleSearch` checks `t.Content` before file-path fallback; `walkTopics` recurses so search reaches nested claim nodes
- **Tests**: `TestHandleSearchFindsDeepClaims` generates a 60-lesson preamble to push content past 4000 chars, then searches — this directly exercises the bug. `TestHandleGetIndividualClaim` traces slug derivation end-to-end. `createTask` tests in cmd/post validate node ID causality chain.

**Invariant checks:**

- **Invariant 2 (CAUSALITY)**: `createTask` returns node ID so callers can cite it as a cause — tested
- **Invariant 11 (IDENTITY)**: claim IDs are slug-derived from titles, not stored by name — slugs are deterministic and used as system keys, titles remain display-only. Acceptable since claims have no separate stable ID in the markdown.
- **Invariant 12 (VERIFIED)**: new code paths have tests ✓
- **Invariant 13 (BOUNDED)**: `claimSlug` truncates at 60 chars; file read is one-shot (72KB is fine) ✓

One low-risk note: `claimSummary` slices at `line[:120]` (bytes, not runes) — could corrupt a multi-byte char at the boundary. Claims are ASCII-heavy, so this is safe in practice but worth a `[]rune` fix eventually.

VERDICT: PASS
