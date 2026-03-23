# Build Report — Iteration 122

## Dependency visibility — depends-on and blocking sections on node detail

### Changes

**store.go:**
- `ListDependencies(ctx, nodeID)` — returns all nodes that nodeID depends on (both done and not done), sorted incomplete-first
- `ListDependents(ctx, nodeID)` — returns all nodes that depend on nodeID, sorted incomplete-first

**handlers.go:**
- `handleNodeDetail` now fetches dependencies and dependents, passes them to the template
- JSON API response includes `dependencies` and `dependents` arrays

**views.templ:**
- `NodeDetailView` signature extended with `dependencies []Node, dependents []Node`
- New "Dependencies" section between task metadata and body:
  - "Depends on" — lists tasks this node depends on with status badges
  - "Blocking" — lists tasks that depend on this node
- New `depRow` component: clickable link with status icon (emerald check for done, amber ring for incomplete), title, state badge, assignee

### Files changed
- `graph/store.go` — 2 new methods
- `graph/handlers.go` — handler update
- `graph/views.templ` — template + depRow component
- `graph/views_templ.go` — generated

### Deployed
`ship.sh` — generated, built, tested, deployed (408 retry succeeded), committed, pushed.
