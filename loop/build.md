# Build Report — Iteration 89

Layer 4 (Justice) entry point: report resolution.

**New grammar op:** `resolve` — space owner dismisses or removes reported content. Records decision in ops with `{"action": "dismiss|remove"}` payload. Owner-only (403 for non-owners).

**New store query:** `ListReports(ctx, spaceID)` — returns report ops that have no corresponding resolve op for the same node_id. Includes node title/kind and extracted reason from payload.

**New type:** `Report` — extends Op with NodeTitle, NodeKind, Reason.

**Settings update:** Reports section (amber border) appears between save button and danger zone when unresolved reports exist. Shows reported node with link, reporter name, reason, and dismiss/remove buttons.

**Op count:** 17 grammar ops total.

Deployed. All tests pass.
