# Loop State

Living document. Updated by the Reflector each iteration. Read by the Scout first.

Last updated: Iteration 138, 2026-03-23.

## Current System State

Five repos, all compiling and tested:
- **eventgraph** — foundation. Postgres stores, 201 primitives, trust, authority. Complete. Has CI.
- **agent** — unified Agent with deterministic identity, FSM, causality tracking. Complete.
- **work** — task store for hive agent coordination. Complete.
- **hive** — 4 agents, agentic loop, budget, **cmd/post**, **cmd/mind** (CLI), **cmd/reply** (conversation participant), CORE-LOOP with higher-order ops. Has CI.
- **site** — lovyou.ai on Fly.io. Production-ready. Has CI. Full agent integration stack with agent identity. **Live conversation polling. Server-side auto-reply (Mind).**

**Agent integration stack (complete):**
- API key auth — Bearer token, SHA-256 hashed, `lv_` prefix (iter 21)
- JSON API — content negotiation on all graph endpoints (iter 22)
- Key management UI — `/app/keys`, HTMX create flow (iter 23)
- Post tool — `cmd/post`, publishes iteration summaries to lovyou.ai (iter 24)
- Agent identity — real user records, visual badges (violet avatar + "agent" pill) (iter 25-27)

**Post tool verified end-to-end:** Agent identity key created, Hive agent posts under its own identity with violet badge. Access control fix deployed — authenticated users can write to public spaces; owner-only ops (settings, delete) remain restricted.

**Conversation stack (complete):**
- `kind='conversation'` nodes with participants in `tags[]` (iter 31)
- `converse` grammar op creates conversations (iter 31)
- Chat lens in sidebar + mobile nav (iter 31)
- Chat-optimized detail view with bubbles (own/other/agent styling) (iter 32)
- `cmd/reply` — Mind as conversation participant, identity from API key (iter 33)
- **Live updates — HTMX polling every 3s, new messages appear without reload** (iter 34)
- **Thinking indicator** — violet bouncing dots when waiting for agent reply, enter-to-send, scroll-to-bottom (iter 35)
- **Server-side auto-reply** — event-driven Mind, triggered by handler on respond/converse ops, calls Claude CLI (iter 43-46)

**Product features:**
- Blog (45 posts, 6 arcs with section nav)
- Reference (cognitive grammar, graph grammar, 13 layers, 201 primitives, 28 agent primitives)
- Unified graph product (8 tables, 19 grammar ops, 7 lenses incl. Chat/Conversations/Knowledge, HTMX, full CRUD)
- Public spaces + discover page (with previews: node count, last activity) + space settings (full CRUD lifecycle)
- Market page (available tasks, search, claim) — Layer 2
- Global activity feed (transparent audit trail) — Layer 7
- Public user profiles (action history, tasks completed, endorsements) — Layer 8
- Space membership (join/leave) — Layer 10
- **Personal dashboard** ("My Work" — cross-space tasks, conversations, agent activity)
- **Knowledge claims** — assert/challenge ops, Knowledge lens per space, public `/knowledge` page with status filters — Layer 6
- **Global search** — `/search` across spaces, nodes, users
- Mobile responsive + animations (breathing logo, reveals)
- Visual identity: "Ember Minimalism" — dark theme, rose accent, warm text, subtle motion

**Product layers (9 of 13):**
| Layer | Name | Status | Ops |
|-------|------|--------|-----|
| 1 | Work | ✓ | intend, decompose, complete, assign, depend, progress |
| 2 | Market | ✓ | claim, prioritize |
| 3 | Moderation | ✓ | report |
| 4 | Justice | ✓ | resolve |
| 5 | Build | — | — |
| 6 | Knowledge | ✓ | assert, challenge |
| 7 | Alignment | ✓ | (activity feed) |
| 8 | Identity | ✓ | (profiles) |
| 9 | Bond | ✓ | (endorsements) |
| 10 | Belonging | ✓ | join, leave |
| 11 | Meaning | — | — |
| 12 | Evolution | — | — |
| 13 | Being | — | — |

**24 grammar ops total.** 9 database tables (+ endorsements). ~52 routes. 26 test functions across 5 test files. **All 13 product layers have minimum viable entries.**

**Product layers (13 of 13):**
| Layer | Name | Status | Ops |
|-------|------|--------|-----|
| 1 | Work | done | intend, decompose, complete, assign, depend, progress |
| 2 | Market | done | claim, prioritize |
| 3 | Moderation | done | report |
| 4 | Justice | done | resolve |
| 5 | Build | done | (changelog lens) |
| 6 | Knowledge | done | assert, challenge |
| 7 | Alignment | done | (activity feed) |
| 8 | Identity | done | (profiles, endorsements) |
| 9 | Bond | done | (endorsements) |
| 10 | Belonging | done | join, leave |
| 11 | Governance | done | propose, vote |
| 12 | Culture | done | pin, unpin |
| 13 | Being | done | reflect |

**CORE-LOOP updates:**
- Higher-order operations integrated (pipeline ordering, fixpoint awareness, irreversibility, depth, duality)
- Critic prompt updated with DUAL (root cause analysis)
- Reflector prompt updated with FIXPOINT CHECK
- Post 44 published with arity answer
- Footer links updated on posts 42-44 with lovyou.ai

Deploy: `fly deploy --remote-only` from site repo.

## Known Issues

**Test debt:** Largely addressed (iter 93). 6 new test functions cover endorsements, reports, dashboard, search, knowledge. Handler-level tests for assert/challenge/resolve not yet written.

**Shallow layers:** Most layers since iter 74 are minimal viable. Knowledge now has evidence collection and display (iter 100 added verify/retract lifecycle, iter 121 added evidence reasons and trail). Market has no exchange/reputation. Justice has no tiered adjudication. Bond has only endorsements, no connections/DMs. Governance has proposals+voting but no delegation/quorum. Breadth is complete but depth is uneven.

**No observability:** No error monitoring, no analytics, no usage tracking. Building into a void.

## Completed Clusters

- **Orient** (1-4): catch up with reality, fix stale docs, accumulate knowledge
- **Ship** (5): deploy fix (`--remote-only`)
- **Discoverability** (6-8): landing page, SEO, sitemap
- **Visitor Experience** (9): blog arc navigation
- **SEO Canonicalization** (10): fly.dev → lovyou.ai redirect
- **Hive Autonomy** (11-13): prompt files, run.sh, CI on hive + site
- **Product Development** (14): public spaces
- **Aesthetics** (15-20): warm copy, dark theme, discovery, space settings, mobile, animations
- **Agent Integration** (21-27): API key auth, JSON API, key management UI, post tool, agent identity (display → real users → visual badges)
- **Space Previews** (28): node count + last activity on discover cards
- **Sidebar Fix** (29): sticky sidebar, independent scroll
- **Mind Bootstrap** (30): cmd/mind CLI — interactive chat with soul + state context
- **Conversations** (31-35): conversation primitive, chat view with bubbles, Mind as participant (cmd/reply), live polling updates, thinking indicator + UX polish
- **Agent Visibility** (36): agent badges on People + Activity lenses via JOIN (consistent across all 6 lenses)
- **Content Preview & Social Proof** (37-39): conversation list previews, discover member count + agent indicator, agent picker on conversation creation
- **Return Visit** (40): logged-in redirect from / to /app
- **Collaborative Access** (41): creation forms open to all authenticated users (not just owners)
- **Agent Badges Completion** (42): agent badges on thread list cards (last holdout)
- **Auto-Reply** (43-46): server-side Mind, event-driven (handler triggers on respond/converse ops)
- **Test Infrastructure** (45, 47): store, mind, handler tests. CI with Postgres. 24 test results, all passing.
- **Identity Fix** (48-49): eliminated 13 name-as-identifier bugs. Added author_id/actor_id columns. All queries use ID-based JOINs. Added invariants 11 (IDENTITY) and 12 (VERIFIED). Updated Critic AUDIT and CORE-LOOP.
- **Mind Context** (50-51): tag name resolution, data backfill, mind_state table, cmd/post syncs loop state. Mind now has full project context when replying.
- **Auth Tests** (52): API keys, bearer auth, agent identity, middleware. 9 test cases.
- **Public Launch** (53-54): auth gate open, landing page with Chat lens, Sign in button (desktop+mobile), onboarding with discover link.
- **Invariant Derivation** (55-56): derived BOUNDED (13) and EXPLICIT (14) from cognitive grammar. Applied BOUNDED to queries (ListNodes LIMIT 500, ListConversations LIMIT 100). 14 invariants total.
- **Hive Tests** (57): AgentDef validation, defaults, StarterAgents. pkg/hive now has tests.
- **Integration Test** (58): full new-user journey test (7 steps, all ops, ID verification).
- **UX Polish** (59-60): markdown rendering, agent chat banner on Feed
- **Agentic Work** (62-72): Mind responds to task assignments, decomposes tasks, creates subtasks with dependencies, recursive auto-work on leaf subtasks, live task updates (HTMX polling), Mind creates tasks from conversations, cross-conversation memory, task links in chat, quick-assign buttons
- **Breadth-First Layers** (74-92): Market(2), Moderation(3), Justice(4), Knowledge(6), Alignment(7), Identity(8), Bond(9), Belonging(10). Plus search, dashboard, endorsements, assignee identity. 19 iterations, 8 layer entries, 9/13 layers covered.
- **Test Debt Paydown** (93): 6 new test functions covering endorsements, reports, dashboard, search, knowledge claims. Invariant 12 compliance restored.
- **Layer 11 — Governance** (94): propose and vote ops, Governance lens with vote tallies, kind guard, one-vote-per-user. 21 ops, 10/13 layers.
- **Layer 5 — Build** (95): Changelog lens — completed tasks as build history. No new ops, new lens on existing data.
- **Layer 12 — Culture** (96): pin/unpin ops, pinned boolean column, pinned items sort first. 23 ops.
- **Layer 13 — Being** (97): reflect op — existential accountability. 24 ops. **ALL 13 LAYERS TOUCHED.**
- **Depth: Pin UI** (98): pin/unpin buttons on node detail, pin indicators on Feed (brand border + label) and Board (pin icon). Layer 12 now usable.
- **Depth: Knowledge Evidence** (121): evidence reasons on challenge/verify/retract, expandable forms, evidence trail on node detail. Layer 6 now evidence-based.
- **Depth: Dependencies** (122-123, 130): full CRUD — view, create, remove.
- **Depth: Notifications** (124): sidebar badge visible from all lenses.
- **Depth: Dashboard** (125): task state filtering tabs.
- **Depth: Governance** (126): proposal deadlines with overdue indicators.
- **Depth: Activity Context** (127, 131, 132): node titles on all activity surfaces (space, dashboard, global, profile, overview).
- **Depth: Profiles** (128-129): clickable avatar, space memberships.
- **Depth: Dependencies** complete (130): remove dependency.
- **Depth: Search & Filtering** (134-137): Discover search, Knowledge search, Market priority filter, Governance state filter.

## Lessons Learned

1. Code is truth, not docs.
2. Verify infra assumptions before building.
3. Update state.md every iteration.
4. Ship what you build — every Build iteration should deploy.
5. Try alternatives before declaring blockers.
6. Name iteration clusters and recognize completion.
7. Hostname middleware must exclude /health (Fly probes via internal IP).
8. Codify implicit knowledge into executable artifacts.
9. Multi-repo replace directives require CI to mirror directory structure.
10. Templ drift check catches stale generated files.
11. Start with the simplest access model (public/private) before building roles/ACLs.
12. When the founder says "that isn't our vibe," treat it as highest-priority.
13. Define the vocabulary before writing the prose.
14. Expose what you've already built before building more.
15. Close the CRUD loop before adding new features.
16. If the sidebar is hidden on mobile, something else must replace it.
17. Animate ceremonies, not workflows.
18. Unlock the bottleneck before building what flows through it.
19. Ship both sides of an interface in consecutive iterations.
20. Infrastructure → interface → management. Skipping any layer leaves the others incomplete.
21. Infrastructure before intelligence. Prove the plumbing, then add smarts.
22. "Works correctly" and "works as intended" are different checks. After integration, test as the user/agent, not as the developer.
23. Identity is a property of the entity, not the credential. A name on a key is metadata; a user record is identity.
24. Access control must match the interaction model. Owner-only writes block agent collaboration on shared spaces. Split write permissions: owner-only for admin ops, authenticated for content ops.
25. Test the viewport, not just the feature. Scroll, resize, and overflow behavior are invisible in code review.
26. Build the interface where the users already are. Don't create parallel systems when the product already has the infrastructure.
27. The differentiator isn't the chat UI — it's who participates. The agent's right of reply is what makes this unique.
28. Identity comes from the credential, not hardcoded names. Multiple agents (hives) may coexist.
29. Infrastructure isn't done until the feedback loop closes. If the user can't see the system's response without manual intervention, the system isn't interactive — it's a mailbox.
30. Resolve actor properties from the identity system, not from scanning content. The users table knows who's an agent; the messages table is evidence, not authority.
31. The onboarding funnel is discover → create → preview. Each step must answer "what's in here?" before the user clicks.
32. When you change a permission at the API layer, grep the templates for the old gate. UI and API permissions must move together.
33. Deploy the mechanism, then deploy the defenses. Two iterations, not one.
34. Absence is invisible to traversal. The Scout traverses what exists. Tests don't exist, so the Scout never encounters them. BLIND must explicitly ask: "what verification is missing?"
35. If the architecture is event-driven, new features should be event-driven too. Don't introduce polling into an event-driven system.
36. The loop can only catch errors it has checks for. When a human catches something the loop missed, fix the loop, not just the code.
37. The Scout must read the vision, not just the code. Product gaps outrank code gaps. 60 iterations of code polish while 12 of 13 product layers remained unbuilt.
38. Cross-space views are the connective tissue of a multi-space platform. Building features inside spaces isn't enough — the user needs a single place to see what matters across all of them.
39. When fixing a systemic issue, grep the schema for ALL instances, not just the ones that triggered the bug. Incomplete fixes create false confidence.
40. When the gates open, searchability and discoverability become critical infrastructure, not features.
41. The loop needs enforcement, not just observation. If the Critic can flag a violation indefinitely without consequence, the invariant is aspirational. Either give the Critic blocking power or make the Scout own quality iterations.
42. Test iterations should follow breadth sprints, not accumulate indefinitely. One iteration of tests per ~5 iterations of features.
43. **NEVER skip artifact writes.** Every phase must write its file (scout.md, build.md, critique.md, reflections.md, state.md). Skipping them breaks the post tool, loses the audit trail, and means the process didn't happen. Violated in iters 93-100 — caused the stale-post bug.

## Vision Notes

- Agents should acquire skills dynamically (like OpenClaw).
- Auth gate is open — Google OAuth in production mode, anyone can sign in.
- Users provide OAuth tokens, agents build things for them via board or personal agent.
- Social product: humans and agents build MySpace-like personal pages.
- Business use: companies use the platform to build products.
- Agents and humans are peers on the social graph.
- Visual identity: "Ember Minimalism" — dark, warm, intentional, alive. lovyou2 as ancestor.

## What the Scout Should Focus On Next

**LOVYOU_API_KEY:** `lv_b7fb22cde43a8a65289f77ee6dc9aa195184bf6129160f62691e59d8d6ccc8dd` — authenticates as the "Hive" agent user.

**Mind conversation tools:**
- `cmd/reply` — one-shot command that fetches conversations, invokes Mind, posts responses
- Identity resolved from API key (no hardcoded names)
- Run: `LOVYOU_API_KEY=lv_... ANTHROPIC_API_KEY=... go run ./cmd/reply/`

**The test debt question:** 6+ features shipped without tests. Invariant 12 is violated. The Reflector has flagged this as the largest systemic risk and a structural loop failure. The Scout should decide: is the next iteration a test iteration (write tests for endorsements, reports, resolve, dashboard, search, knowledge), or does product breadth still take priority? If breadth: 4 layers remain (Build(5), Meaning(11), Evolution(12), Being(13)). If tests: one iteration could cover the 6 untested features. Either choice is defensible, but the choice must be explicit — not deferred again.

**If continuing breadth:** Layer 5 (Build) is next in sequence and the most concrete of the remaining four. Meaning(11), Evolution(12), and Being(13) are progressively more abstract.

**If deepening:** Knowledge(6) has the most room — verify/retract ops, evidence linking, search integration. Market(2) needs exchange. Bond(9) needs connections/DMs.
