# Scout Report — Iteration 210

## Gap: Three unresolved spec questions blocking fixpoint

1. **Organization ↔ Space** — Spaces contain Nodes. Organizations should contain Spaces. Current model can't express this.
2. **Thin kinds** — ~half of the 54 proposed entity kinds fail the lifecycle test. Need to filter.
3. **Market exchange flow** — entity kinds listed but the multi-step exchange mechanism isn't designed.

**Method:** Address all three in one pass. Apply the thin-kinds test (distinct lifecycle, create form, list view). Resolve Organization ↔ Space architecturally. Design the Market exchange flow at grammar-op level.

**Output:** Updates to unified-spec.md and layers-general-spec.md. Fixpoint or honest admission of remaining gaps.
