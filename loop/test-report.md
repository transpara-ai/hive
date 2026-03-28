# Test Report — Iteration 402

## What was tested

Iteration 402 migrated `/hive/feed` diagnostics from a local file to the graph database,
adding `AppendHiveDiagnostic`, `ListHiveDiagnostics`, `POST /api/hive/diagnostic`,
`api.Client.PostDiagnostic`, and `Runner.appendDiagnostic` (dual-write).

## Bug found and fixed

**`TestListHiveDiagnostics_Empty` fails when tests run in order** — `TestPostHiveDiagnostic_StoresAndServes`
inserts a row into `hive_diagnostics`, then `TestListHiveDiagnostics_Empty` asserts the table is
empty. Fixed by adding `DELETE FROM hive_diagnostics` at the start of the empty test to isolate it
from prior rows.

## Tests added

### `site/graph/hive_test.go`

- **`TestPostHiveDiagnostic_StoresAndServes`** (pre-existing, Builder wrote it) — round-trip POST
  then GET /hive/feed, verifies the stored phase name appears in the feed. **PASS** with DB.
- **`TestListHiveDiagnostics_Empty`** (pre-existing, fixed isolation) — verifies empty DB returns
  nil without error. **PASS** with DB after adding pre-test DELETE.

### `hive/pkg/api/client_test.go`

- **`TestPostDiagnostic_SendsPayload`** — verifies the request hits `/api/hive/diagnostic`, sends
  raw payload unchanged, sets `Content-Type: application/json`, and includes `Bearer` auth.
- **`TestPostDiagnostic_Error4xx`** — verifies HTTP 401 is returned as a non-nil error.

### `hive/pkg/runner/diagnostic_test.go`

- **`TestRunnerAppendDiagnostic_WritesFileOnly`** — HiveDir set, no APIClient: event written to
  `diagnostics.jsonl`, no HTTP call.
- **`TestRunnerAppendDiagnostic_PostsOnly`** — APIClient set, no HiveDir: event POSTed, no file
  written.
- **`TestRunnerAppendDiagnostic_WritesBoth`** — both set: file written AND POST made (the production
  path).
- **`TestRunnerAppendDiagnostic_NeitherSet`** — neither set: no panic (defensive).

## Results

```
hive: go test -count=1 ./...    PASS (all 13 packages)
site: go test -count=1 ./graph/ PASS (DB tests with DATABASE_URL)
```

## Coverage notes

- The store-level functions `AppendHiveDiagnostic` / `ListHiveDiagnostics` are covered by DB integration
  tests (skip without DATABASE_URL, pass with local Docker Postgres).
- `handleHiveDiagnostic` (the HTTP handler) is covered via the round-trip test through the full handler stack.
- `Runner.appendDiagnostic` dual-write logic is fully covered: file-only, API-only, both, neither.
- `api.Client.PostDiagnostic` path, headers, body, and error propagation are all covered.

## @Critic
Tests done. Ready for review.
