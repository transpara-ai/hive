# Critique — Iteration 92

## Derivation Chain

**Gap:** Layer 6 (Knowledge) — no epistemic infrastructure; all content is flat text without verifiable truth status.
**Plan:** `claim` node kind, `assert` + `challenge` ops, Knowledge lens per space, public `/knowledge` page.
**Code:** Exactly what was planned. Constants, store queries, two ops in handleOp, lens in sidebar/mobile, public page with status filters, sitemap entry.

The chain holds. Every code change traces to the plan, and the plan addresses the gap.

## Audit

### Correctness — ISSUE

**`challenge` op accepts any node, not just claims.** The handler at `handlers.go:1255` takes a `node_id`, records a challenge op, and sets the node's state to `ClaimChallenged` ("challenged") — without verifying that the target node is actually a `claim`. A crafted POST (`op=challenge&node_id=<any_task_id>`) would set a task or thread to state "challenged", corrupting its state machine.

Other ops (`complete`, `assign`, `claim`) also skip kind checks, but "done" and "assigned" are universal states. "challenged" is claim-specific. This is uniquely dangerous for `challenge`.

**Fix:** Fetch the node, check `node.Kind == KindClaim`, return 400 otherwise:
```go
node, err := h.store.GetNode(ctx, nodeID)
if err != nil || node == nil {
    http.Error(w, "node not found", http.StatusNotFound)
    return
}
if node.Kind != KindClaim {
    http.Error(w, "can only challenge claims", http.StatusBadRequest)
    return
}
```

### Correctness — ISSUE

**`UpdateNodeState` error silently dropped in challenge.** Line 1269 ignores the return value. Compare with the `complete` op at line 986, which properly checks the error and returns 500. Inconsistent and could silently fail.

**Fix:** Check the error:
```go
if err := h.store.UpdateNodeState(ctx, nodeID, ClaimChallenged); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
```

### Breakage — CLEAN

No existing functionality broken. The two new ops are additive. The sidebar/nav additions are correctly positioned. The `assert` op name correctly avoids colliding with the existing `claim` (Market layer). Existing tests pass.

### Simplicity — CLEAN

Good reuse. Claims as `nodes` with `kind='claim'` and `state` mapping to epistemic status is the simplest representation. No new tables. `CountChallenges` uses a subquery in `ListKnowledgeClaims` and a standalone function for the lens — both appropriate. The `KnowledgeClaim` view type duplicates fields across `graph` and `views` packages, but that follows the existing pattern of separating store types from view types.

Minor: `pluralS` in `graph/views.templ` and `knowledgePluralS` in `views/knowledge.templ` are identical functions. Could share, but trivial.

### Security — CLEAN

SQL queries properly parameterized (`$N` placeholders). State filter goes through parameterized query, no injection risk. Auth: `handleOp` is behind `writeWrap` (requires authentication). Knowledge lens is behind `readWrap` (public for public spaces, which is correct). No new attack surface beyond the kind-check issue above.

### Tests — VIOLATION (Invariant 12)

Zero test coverage for: `assert` op, `challenge` op, `ListKnowledgeClaims`, `CountChallenges`, kind validation (once added). Invariant 12: "No code ships without tests." This is the sixth consecutive layer shipped without tests (endorsements, reports, dashboard, search, and now knowledge). The test debt is compounding.

### Build Report Accuracy — MINOR

Build report says challenge default reason is "disputed" (line 49). Code at `handlers.go:1263` uses "challenged". Trivial but the report should match the code.

## DUAL (Root Cause)

Why does the kind-check gap exist? Because the codebase never had kind-specific state transitions until now. `complete`, `assign`, `claim` all set states that are broadly applicable. `challenge` is the first op whose target state is meaningful only for one kind. The existing pattern — trust the UI to send correct node IDs — breaks when the state is kind-specific, because the API is a second interface that doesn't enforce kind constraints.

## Verdict: REVISE

Two required fixes before approval:

1. **Add kind check to `challenge` op** — verify target node is `kind='claim'` before setting claim-specific state. Without this, any authenticated user can corrupt task/thread state via the API.

2. **Check `UpdateNodeState` error in `challenge`** — match the pattern used by `complete` and other state-change ops.

## Observation

Test debt is now the largest systemic risk. Six features shipped without tests. The Critic flags it each time, but it keeps shipping. This needs to become a Scout-level priority — not just a Critic footnote.
