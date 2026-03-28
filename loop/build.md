# Build: Fix populateFormFromJSON array handling — CAUSALITY unblocked

## Gap
`populateFormFromJSON` decoded JSON bodies into `map[string]string`. Any JSON field with an array value (e.g. `"causes":["id1","id2"]`) caused the entire decode to fail silently, returning early without populating `r.Form`. This left `op` as empty string, falling through to the "unknown op" handler. Invariant 2 (CAUSALITY) was structurally unenforceable for all LLM-driven ops that pass causes as a JSON array.

## Fix

**`site/graph/handlers.go` — `populateFormFromJSON`**

Changed decode target from `map[string]string` to `map[string]any`. Added a type switch:
- `string` → set as-is
- `[]interface{}` → join as CSV (e.g. `["id1","id2"]` → `"id1,id2"`)
- `nil` → skip
- other → `fmt.Sprintf("%v", val)`

Preserves all existing behavior for string fields. The server-side causes parser (`strings.Split(causesStr, ",")`) continues to work unchanged.

**`site/graph/knowledge_test.go` — `TestAssertOpMultipleCauses`**

Updated test to send `"causes":["id1","id2"]` JSON array format (previously used CSV workaround). Updated comment to reflect the fix.

## Files Changed
- `site/graph/handlers.go` — `populateFormFromJSON` function
- `site/graph/knowledge_test.go` — `TestAssertOpMultipleCauses` comment + payload

## Verification
- `go.exe build -buildvcs=false ./...` — PASS
- `TestAssertOpReturnsCauses` — PASS
- `TestKnowledgeClaimsCausesFieldPresent` — PASS
- `TestAssertOpMultipleCauses` — PASS (now uses JSON array)
- All 9 knowledge tests — PASS
