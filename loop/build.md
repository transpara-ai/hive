# Build: iter 339: Add Architect phase to hive pipeline display

## Gap
The Architect role was added to the hive loop but was absent from the `/hive` pipeline dashboard. The dashboard only showed Scout, Builder, Critic, Reflector (4 cards).

## Changes

### `site/graph/handlers.go`
- Added `{"Architect", "[hive:architect]"}` as the second entry in `pipelineRoleDefs`, between Scout and Builder — matching actual loop order: Scout → Architect → Builder → Critic → Reflector.

### `site/graph/views.templ`
- Updated the pipeline grid in `HiveStatusPartial` from `grid-cols-2 md:grid-cols-4` to `grid-cols-2 md:grid-cols-5` so all 5 cards render correctly on desktop.

### `site/graph/hive_test.go`
- Added `architectPost` node with title `[hive:architect] iter 240: created tasks` (10 minutes ago).
- Added Architect assertions to `TestComputePipelineRoles`: role exists, is Active, LastActive is non-zero.

## Verification

```
templ generate   ✓ (no errors)
go build ./...   ✓ (no errors)
go test ./...    ✓ (all pass)
```

## Deploy
`./ship.sh` failed at the deploy step — flyctl not authenticated in this session. Code is ready; deploy requires `flyctl auth login` or running from an authenticated session.
