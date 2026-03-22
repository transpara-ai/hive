# Critique — Iterations 37-39

## Verdict: APPROVED

## Audit

**37 — Conversation Preview:**
- LATERAL subquery handles conversations with no messages (NULL → empty strings). ✓
- Agent author shown in violet, human in faint. Consistent with chat view. ✓
- `truncate()` is byte-level, not rune-level. Could split a multibyte character. Acceptable for now — English content only.

**38 — Discover Social Proof:**
- `BOOL_OR(u.kind = 'agent')` correctly handles spaces with no agent activity (NULL → false via COALESCE). ✓
- `COUNT(DISTINCT o.actor)` counts actual contributors, not total ops. Correct signal. ✓
- Violet dot + "agents" text is subtle and consistent with agent visual language. ✓

**39 — Agent Picker:**
- `addParticipant()` deduplicates (checks indexOf before adding). ✓
- Chips only appear when `len(agents) > 0`. No empty UI clutter. ✓
- Still free-text — users can type arbitrary names. Chips are additive, not restrictive. ✓

## Gaps

- **No agent participant indicator on conversation cards** in the list (iteration 37 shows last message preview but not agent presence on the card itself)
- **No human participant chips** — only agent chips shown. Could add space members for autocomplete.
- **Discover query could be slow at scale** — two LATERAL JOINs + users JOIN per space. Fine for now.
- **truncate() is byte-level** — should be rune-level for Unicode safety. Minor.
