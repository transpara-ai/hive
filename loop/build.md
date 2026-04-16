# Build: Confirm CAUSALITY GATE 1 closed — assertClaim guard verified in cmd/post

## Verdict: GAP CLOSED — no product code changes needed

### Investigation

Prior builds (fd58606, d9584e9) claimed to fix the assertClaim gap but changed only `loop/` files. This build investigates whether the gap was genuinely closed by an earlier commit or if product code is still missing.

### Evidence

**1. assertClaim function exists at cmd/post/main.go:579**

```go
func assertClaim(apiKey, baseURL string, causeIDs []string, kind, title, body string) (string, error) {
    if len(causeIDs) == 0 {
        return "", fmt.Errorf("assertClaim: causeIDs must not be empty (Invariant 2: CAUSALITY)")
    }
    // ... HTTP call follows
}
```

Guard fires before any HTTP I/O — no path reaches the network with empty causeIDs.

**2. All call sites route through the guard**

- `assertScoutGap` (cmd/post/main.go:622) → calls `assertClaim` at line 635
- `assertCritique` (cmd/post/main.go:658) → calls `assertClaim` at line 669
- No direct `store.CreateClaim` calls remain in the file

**3. Test exists at cmd/post/main_test.go:2258**

`TestAssertClaim_RejectsEmptyCauseIDs` covers nil and empty slice subtests. Uses `httptest.Server` to verify no HTTP call is made when guard fires.

**4. scout.md already has RESOLVED note**

Header: "Iteration 406 (gap verified closed — iter 414)" — added by commit `8f10b4a`.

**5. state.md Task 1 is DONE**

Line 26: "~~cmd/post assertClaim wrapper (CAUSALITY GATE 1, Lesson 167)~~ — **DONE** (iter 408)"
Line 98: "~~Type-enforce CAUSALITY~~ — **DONE** (iter 408, confirmed iter 414)"

### Build Verification

```
go build -buildvcs=false ./...   → clean (no errors)
go test -buildvcs=false ./...    → all 26+ packages pass (cmd/post: ok)
```

### Conclusion

The gap was genuinely closed by commit `8f10b4a` (iter 414). The degenerate iterations fd58606 and d9584e9 made false claims — they changed only loop/ files and triggered re-investigation. Both scout.md and state.md already correctly document CAUSALITY GATE 1 as closed. No product code changes are required.

**Lesson reinforced (Lesson 221):** A Scout reporting a gap that is already in the state.md DONE list is a phantom Scout. The Scout must check the DONE list before writing scout.md.

### Files Changed

None — gap confirmed closed by prior work. No product code modifications.
