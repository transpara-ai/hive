# Scout Report — Iteration 222

## Gap: Role entity kind missing

**Source:** Director directive + unified-spec.md (Role = "Capability + responsibility", Organize mode) + state.md entity kind pipeline priority + iter 221 failure (permission-blocked, zero code shipped).

## What's missing

The `role` entity kind doesn't exist. The unified spec defines it as one of 18 core entity types:

> Role | `role` | Capability + responsibility | Organize | Organize, Govern, Execute

Examples: "Engineer", "Moderator", "Sprint Lead", "Board Member", "Delivery Driver."

Roles are how groups define capability and responsibility. Without them, every member is undifferentiated — you can't express "Alice is an Engineer" or "Bob is Moderator."

## Why this gap

1. **Director-specified.** Explicitly requested.
2. **Iter 221 failed.** Correctly identified but permission-blocked. Zero code shipped. Retry.
3. **Proven pipeline.** Projects (205) and Goals (206) proved the pattern: ~110 lines, 6 changes, 3 files, 0 schema changes.
4. **Organize mode prerequisite.** Roles + Teams = Organize mode.

## What's needed — 6 changes, 3 files

| # | File | Change |
|---|------|--------|
| 1 | `store.go` | `KindRole = "role"` constant |
| 2 | `handlers.go` | Route: `GET /app/{slug}/roles` → `handleRoles` |
| 3 | `handlers.go` | `handleRoles` function (copy handleProjects, filter `kind=role`) |
| 4 | `handlers.go` | Add `"role"` to intend op kind allowlist |
| 5 | `views/views.templ` | `rolesIcon()` + sidebar/mobile entries |
| 6 | `views/views.templ` | `RolesView` template (list + create form) |

No schema changes. No new ops. No new tables. Role is a Node with `kind=role`.

## Risk

**Low.** Third entity through a proven pipeline. Only risk is the permission issue that blocked iter 221.
