# Test Report: MCP knowledge_search deep claims fix

- **Iteration:** Build 6090d8e — MCP knowledge_search blind to graph claims
- **Tester:** Tester agent
- **Result:** PASS — 18/18 tests, 0 failures

## What Was Tested

The fix parses `claims.md` into individual in-memory `topic` nodes so that
`knowledge_search` is no longer limited by the 4000-char file-content window.
Tests cover the core fix, edge cases, and helper functions.

## Test Coverage

### Pre-existing tests (7) — all pass

| Test | What it checks |
|------|----------------|
| `TestBuildHiveLoopIncludesClaimsWhenPresent` | claims.md indexed when file exists |
| `TestBuildHiveLoopOmitsClaimsWhenAbsent` | claims not in tree when file absent |
| `TestHandleSearchFindsClaims` | basic claim content found by search |
| `TestHandleTopicsReturnsLoopChildren` | claims.md listed in loop children |
| `TestHandleGetClaims` | knowledge.get(loop/claims) returns file content |
| `TestHandleSearchFindsDeepClaims` | **core bug**: claims beyond 4000 chars now found |
| `TestHandleGetIndividualClaim` | individual claim retrieved by slug ID |

### New tests added (11) — all pass

| Test | What it checks |
|------|----------------|
| `TestParseClaimsDuplicateTitles` | Three "Lesson 109" titles get unique slugs: base, base-2, base-3 |
| `TestClaimSlugTruncation` | Slug <= 60 chars, no trailing hyphen after truncation |
| `TestClaimSlugSpecialChars` | Colons, parens, em-dashes collapsed to single hyphens |
| `TestClaimSummaryLongLine` | Line > 120 chars truncated to 120 + "..." |
| `TestClaimSummaryAllMetadata` | Body with only `**State:**` lines returns empty summary |
| `TestParseClaimsEmptyFile` | Empty file -> 0 claims, no panic |
| `TestParseClaimsNoSections` | File with no `##` headings -> 0 claims |
| `TestHandleSearchResultCap` | 15 matching claims -> at most 10 returned |
| `TestHandleSearchEmptyQuery` | Empty query -> error string, no panic |
| `TestHandleGetEmptyID` | Empty id -> error string, no panic |
| `TestClaimChildrenVisibleInTopics` | Individual claim nodes appear as children of loop/claims |

## Edge Cases Verified

- **Deduplication:** Three "Lesson 109" entries get distinct IDs (base, base-2, base-3). Confirmed.
- **Slug truncation:** Long titles don't leave trailing hyphens at the 60-char cut. Confirmed.
- **Result cap:** handleSearch hard-caps at 10 results regardless of match count. Confirmed.
- **Null inputs:** Empty query and empty ID return error strings, not panics. Confirmed.
- **Empty/degenerate files:** parseClaims handles empty and no-section files cleanly. Confirmed.
- **Children navigation:** knowledge.topics(loop/claims) surfaces individual claim nodes. Confirmed.

## Coverage Notes

- `parseClaims`, `claimSlug`, `claimSummary` — fully covered
- `handleSearch` content-branch for claim nodes — covered by deep-claim test
- `handleGet` in-memory content branch — covered by individual-claim test
- `handleTopics` children traversal — covered by topics and children tests
- File I/O error paths (parseClaims on missing file) — exercised; returns nil cleanly
