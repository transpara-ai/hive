# Critique — Iteration 49

## Verdict: APPROVED

All 13 name-as-identifier bugs fixed. Tests pass. Loop updated to catch this class of error.

## DUAL (root cause analysis)

Why did 49 iterations of the loop fail to catch name-based identity?

1. **The Critic's AUDIT checklist didn't include identity checks.** It had correctness, breakage, simplicity, security — but not "are entities referenced by ID?" The Critic can only check what it's told to check.

2. **The Scout never traversed the data model.** It looked at features (what's missing?), not foundations (is what exists correct?). Need(Traverse) — "what's wrong with what we built?" — was not performed.

3. **The Reflector's BLIND failed.** "What would someone outside notice?" — someone outside would immediately see `WHERE name = ANY($1)` in a multi-user system and flag it. The loop was too close to the code to see the structural defect.

4. **No invariant existed.** The ten invariants are HALT conditions. None covered identity. Without a rule, there's no violation. Invariant 11 (IDENTITY) and 12 (VERIFIED) now exist.

## What changes prevent recurrence

- Critic AUDIT now includes: "Identity: are entities referenced by ID, never by display name?"
- Critic AUDIT now includes: "Tests: does the change include tests?"
- Invariant 11 (IDENTITY): entities referenced by immutable IDs
- Invariant 12 (VERIFIED): no code ships without tests
- CLAUDE.md coding standards: "IDs are identity, names are display"
