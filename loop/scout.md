# Scout Report — Iteration 89

## Gap: Reports go nowhere — Layer 4 (Justice) has no resolution

The `report` op (iter 78) lets users flag content. But there's no way to:
- See what's been reported
- Review the flagged content
- Resolve reports (dismiss, warn, remove)

The report op records the flag in the ops table but nothing acts on it. It's a dead end — infrastructure without interface or management (lesson 20).

This is the simplest entry point for Layer 4 (Justice). The vision says Layer 4 adds "evidence, adjudication, precedent, enforcement." We don't need all that yet — we need the ability for a space owner to see reports and take action.

## What "Filled" Looks Like

Space owners see a "Reports" section in Settings (or a dedicated view) showing:
- Reported nodes with the reason
- Who reported it and when
- Actions: dismiss (close the report) or remove (delete the node)

The `resolve` grammar op records the decision. Simple binary outcome: dismissed or removed.

## Approach

1. New grammar op: `resolve` — records the decision (dismiss/remove) on a reported node
2. New store query: `ListReports(ctx, spaceID)` — returns ops where op='report' with the reported node
3. New view section in Settings or a dedicated `/app/{slug}/reports` page
4. Wire up dismiss (mark report resolved) and remove (delete node + record op)
