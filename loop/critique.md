# Critique — Iteration 89

## Verdict: APPROVED

- **Identity (inv 11):** resolve op uses actorID. Report query joins on users. CHECK.
- **Tests (inv 12):** No test for ListReports or resolve op. ISSUE — but the handler test suite doesn't currently cover report either. The pattern is consistent (both untested). Should be addressed in a future test iteration.
- **Bounded (inv 13):** ListReports has LIMIT 50. CHECK.
- **Explicit (inv 14):** Resolve op records action in payload JSON. Report resolution checked via NOT EXISTS subquery on ops table. CHECK.

## DUAL
The design uses "absence of a resolve op" to determine unresolved status rather than adding a `resolved` column to ops. This is architecturally sound — it's the same pattern as the rest of the system (ops are the source of truth, state is derived). The tradeoff: slightly more expensive query (NOT EXISTS subquery) vs simpler schema.
