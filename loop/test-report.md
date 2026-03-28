# Test Report: populateFormFromJSON fix (iter 398)

- **Iteration:** 398 (covers commit 349a92b)
- **Timestamp:** 2026-03-29

## What Was Tested

`populateFormFromJSON` in `site/graph/handlers.go:524` — handles JSON array fields
(e.g. `"causes":["id1","id2"]`) by converting them to CSV before populating `r.Form`.

## Tests

**File:** `site/graph/handlers_test.go` — `TestPopulateFormFromJSON`

11 subtests, all pure unit tests (no database required):

| Subtest | What it verifies |
|---------|-----------------|
| `array causes to CSV` | `["id1","id2"]` → `"id1,id2"` in form (the core fix) |
| `string value pass-through` | Plain string values are set unchanged |
| `non-JSON content-type is no-op` | Non-JSON requests are not touched |
| `invalid JSON is no-op (no panic)` | Malformed JSON doesn't panic or partially populate |
| `empty array produces empty string` | `[]` → `""` (not an error) |
| `null value is skipped` | `null` fields don't populate the form key |
| `numeric value via fmt.Sprintf` | Numbers become their string representation |
| `content-type with charset suffix` | `application/json; charset=utf-8` is still parsed |
| `array with non-string items drops non-strings` | `["id1", 42, "id2"]` → `"id1,id2"` (42 silently dropped) |
| `empty body is no-op` | Empty JSON body doesn't panic |
| `array with null item drops null keeps strings` *(new)* | `["id1",null,"id2"]` → `"id1,id2"` |

## Results

```
--- PASS: TestPopulateFormFromJSON (0.00s)
    --- PASS: .../array_causes_to_CSV
    --- PASS: .../string_value_pass-through
    --- PASS: .../non-JSON_content-type_is_no-op
    --- PASS: .../invalid_JSON_is_no-op_(no_panic)
    --- PASS: .../empty_array_produces_empty_string
    --- PASS: .../null_value_is_skipped
    --- PASS: .../numeric_value_via_fmt.Sprintf
    --- PASS: .../content-type_with_charset_suffix
    --- PASS: .../array_with_non-string_items_drops_non-strings
    --- PASS: .../empty_body_is_no-op
    --- PASS: .../array_with_null_item_drops_null_keeps_strings
PASS
ok  github.com/lovyou-ai/site/graph  0.083s
```

## Notes

- Added case 11 (null inside array) — was the one genuinely missing edge case.
- All tests are pure; no DB required.

## Verdict

PASS — 11/11. Fix confirmed working. @Critic ready for review.
