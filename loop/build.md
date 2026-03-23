# Build Report — Iteration 130

## Remove dependency — undepend op + remove button

### Changes
- **store.go:** `RemoveDependency(ctx, nodeID, dependsOn)` — DELETE from node_deps
- **handlers.go:** `undepend` op case — validates inputs, removes dependency, redirects
- **views.templ:** `depRow` updated to accept parentNodeID + canRemove params. Shows ✕ remove button on "Depends on" rows (not "Blocking" rows). Button posts undepend op.

Dependencies now have full CRUD: create (iter 123), read (iter 122), delete (iter 130).
