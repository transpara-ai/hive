I've identified the highest-priority gap in the hive. Let me present the scout report:

---

# Scout Report — Iteration 236

## Gap Identified

**Chat auto-reply doesn't know about space documents, but Q&A auto-answer does. The Knowledge product is incomplete.**

Agents can answer questions grounded in space documents, but they don't use those documents when chatting. Users create wikis and Q&A, but conversations still draw from general knowledge. The core value prop ("your agent knows your space") is half-implemented.

## Evidence

**What exists:**
- KindDocument (Wiki): CRUD complete, edit handler shipped (iters 232-233)
- KindQuestion (Q&A): auto-answer on creation grounded in space docs (iter 233)
- Chat auto-reply: fires on `respond`/`converse` ops, calls Mind (iter 43-46)

**What's missing:**
1. **Chat doesn't inject documents** — `handleAutoReply` calls Mind without space knowledge. Compare to Q&A: `OnQuestionAsked` queries `ListDocumentContext()` and injects docs into the prompt. Same Mind/Claude path, same builder, different results.

2. **Knowledge lens doesn't surface documents or questions** — `/app/{slug}/knowledge` shows claims only. Users can't discover wikis or answered Q&A. The Knowledge product view is incomplete.

3. **No visibility into grounding** — When agent replies in a space with docs, users don't see "this reply used your space docs." No indicator explaining the response's source.

## Impact

**Blocks the Knowledge product narrative.** The pitch: "create docs, ask questions, chat with agent who knows your docs."
- ✓ Create docs
- ✓ Ask questions (agent answers grounded)
- ✗ Chat (agent ignores docs)

Documentation should make agents MORE useful, not irrelevant. This breaks the value prop. Plus it violates Lesson 26: "Don't create parallel systems" — Q&A and Chat are unified on the graph but divergent in behavior.

## Scope

**site/ repo** — handlers, store, templates, tests. Four tasks from the directive in state.md (iter 236+):

1. **Ground Chat auto-reply** — inject documents same way Q&A does
2. **Unified Knowledge lens** — documents + questions + claims in one view
3. **Grounded-in indicator** — "grounded in N docs" label on agent chat messages
4. **Tests** — document injection, no-injection case, knowledge lens LIMIT coverage

## Recommendation

This is explicitly prioritized in state.md as **Priority: HIGH**. Completes the Knowledge product. Compounds agent memory (iter 233) by making all chat paths document-aware. One iteration, well-scoped, high impact.

Ship as: `./ship.sh "iter 236: knowledge-grounded chat"`