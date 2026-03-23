# Scout Report — Iteration 121

## Gap: Knowledge claims have no evidence

Layer 6 (Knowledge) has `assert`, `challenge`, `verify`, and `retract` ops — the full lifecycle. But claims are bare assertions. There's no evidence field, no reasoning, no supporting links. A claim says "X is true" but never says *why*. A challenge says "I dispute this" but never says *on what grounds*.

The challenge button on KnowledgeCard sends `reason="disputed"` as a hidden field — no user input. Verify and retract store no reason at all. The activity section on node detail shows ops without payloads. Evidence exists in the data model (ops.payload JSONB) but is never collected or displayed.

**Why this gap outranks others:** 34 iterations of UI polish. The platform looks great. But Knowledge is the most differentiating layer (no competitor has built-in claim provenance) and it's currently hollow — you can assert but not justify. This is the first depth improvement that makes an entire layer *real*.

**Scope:** Add reason fields to challenge/verify/retract, expandable forms on KnowledgeCard and node detail, evidence trail display.
