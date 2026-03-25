# Loop State

Living document. Updated by the Reflector each iteration. Read by the Scout first.

Last updated: Iteration 232, 2026-03-25.

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

**25 grammar ops total (+ react).** 10 database tables (+ reactions). ~53 routes. 26 test functions across 5 test files. **All 13 product layers have minimum viable entries.**

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
- **Depth: Notifications** (138-140): Knowledge/governance/endorsement triggers + deep links to nodes.
- **Depth: Task creation** (141): Description textarea on Board form.
- **Depth: Search everywhere** (134-137, 142-146, 149, 151): Every lens (Board, Feed, Threads, Chat, People, Knowledge, Governance, Changelog, Activity), every public page. Complete.
- **Depth: Notification coverage** (138-140, 147-148): All ops notify. Deep links. Human + agent completions, proposal close.
- **Depth: Overdue highlighting** (152-153): Red "overdue" on Board, dashboard, detail for past-due tasks.
- **Depth: Discover** (154): Kind filter tabs (Projects/Communities/Teams).
- **Visual: Typography** (157-160): Source Serif 4 display font, ember glow hero, italic serif logo, serif headings site-wide, refined footer, sidebar active indicator, card hover polish.
- **Visual: Nav** (161): Unified nav across all 3 headers, secondary links to footer.
- **UX: Cmd+K** (162): Command palette on every page, fuzzy search, arrow key nav.
- **UX: Board DnD** (163): Drag-and-drop task cards between kanban columns.
- **UX: Chat** (164, 166): Message grouping by author+time, auto-expanding textarea with shift+enter.
- **UX: Polish** (165, 167, 171): Card hover lift, toasts, empty state illustrations.
- **UX: Chat** (168): Inline reply with quoted context + reply preview bar.
- **UX: Keyboard** (169): ? help overlay, G+B/F/C/A/K navigation shortcuts.
- **UX: Board** (170): Inline status change via hover dropdown on task cards.
- **UX: Actions** (172): Hover action bar on task cards (complete, open buttons).
- **UX: Polish** (173-174): Skeleton CSS, card hover lift on ALL public pages.
- **UX: @mentions** (175): Autocomplete dropdown in all text inputs.
- **UX: Final 6** (176-181): Collapsible sidebar, collapsible threads, activity grouping, optimistic chat send, relative time auto-update, hover action bar on task cards.

**All 20 UX tickets from the research sprint are COMPLETE.**

- **Social Layer Sprint** (182-183): Code Graph on /reference (65 primitives). Message reactions (emoji) with toggle, hover picker, HTMX reactivity. Social layer spec written (4 modes, 33 planned iterations).
- **Phase 1 — Chat Foundation** (184-189): Reply-to linkage (structured, not markdown), message edit/delete (author-only, soft delete, audit trail), unread counts (read_state table, UPSERT), DM/group filter tabs, message search (ILIKE on bodies, from: operator, conversation context). Edit REVISE fixed (inline DOM swap, not reload).

**Phase 1 (Chat Foundation) COMPLETE.** All 6 items shipped.
- **Phase 2 — Square** (190-193): Endorse on posts, Follow users, Quote post, Repost. 4 grammar ops (endorse, subscribe, derive, propagate), 3 tables (follows, reposts + endorsements), 1 column (quote_of_id).

**Phase 2 (Square) COMPLETE.** All 4 items shipped.
- **Phase 3 — Composition** (194-197): Following feed + repost surfacing + attribution. For You (endorsement-weighted). Trending (velocity scoring). Feed has 4 tabs: All, Following, For You, Trending.

**Phase 3 (Composition) COMPLETE.** All 4 feed algorithms + repost attribution shipped. Feed matches spec's SquareMode.
- **Polish** (198): Engagement bar on node detail — endorse, repost, quote buttons on post/thread detail pages.

**Social layer Phases 1-3 COMPLETE.** 16 iterations (183-198). Chat Foundation + Square mode + Feed composition + engagement bar everywhere.
- **Test debt paydown** (199): 6 new test functions covering follows, reposts, quote posts, message search, bulk endorsements, parseMessageSearch.

**20 test functions in store_test.go, 5 in handlers_test.go.**
- **Work Depth** (200): Task List view — sortable table with Board/List toggle. Priority/state/due/assignee/created sort. Compact rows for power users.

- **Work General Spec** (201): Cognitive grammar applied to "organized activity." 12 entity types, 6 modes (Execute, Organize, Govern, Plan, Learn, Allocate). Architecture supports full domain without modification. Spec at hive/loop/work-general-spec.md.

**Work re-scoped.** Not "kanban competitor" but "organized activity at every scale." 6 modes serve solo dev through civilizational. Build order: Execute depth → Organize → Govern → Plan → Learn → Allocate.

48. **When the director questions the framing, stop and re-derive.** "Work isn't just a kanban board" is a structural correction, not a feature request. Stop building. Apply the method. The cost of one spec iteration saves 10+ iterations of building the wrong thing.
49. **Spec unifies before code diverges.** Without the unified ontology, Work and Social would've been separate products. The spec shows they're facets of one thing: purposeful collective activity. One graph, one grammar, one navigation.

**Unified ontology at hive/loop/unified-spec.md.** 10 modes, 18 entities, derivation order. Everything is organized activity. Modes emerge from content. Architecture already supports this.

- **Entity: Role** (222): `KindRole` constant, `handleRoles` handler, `RolesView` template, sidebar + mobile nav, shield icon. Organize mode prerequisite. 11th entity kind.
- **Entity: Team** (223): `KindTeam` constant, `handleTeams` handler, `TeamsView` template, sidebar + mobile nav, user-group icon. Organize mode now has Roles + Teams. 12th entity kind.
- **Hive Runtime Phase 1** (224): `pkg/api/client.go` (lovyou.ai REST client), `pkg/runner/runner.go` (tick loop, builder flow, cost tracking, build verification, git commit/push), `cmd/hive` rewritten (dual-mode: `--role` runner / `--human` legacy). Retired cmd/loop/, cmd/daemon/, agents/.sessions/ (~1,050 lines). E2E tested: builder claimed task from board, Operated via Claude CLI (4m19s, $0.46), verified build, closed task. Agent identity filtering (`--agent-id`), one-shot mode (`--one-shot`).
- **First Autonomous Code Commit** (225): Builder shipped Policy entity kind to production. 2m49s, $0.53. Fixed 3 critique issues (double prompt, recency tiebreak, changes-required guard). Human fixed one miss: KindPolicy not in intend allowlist. 13th entity kind. Deployed.
- **Critic Role** (226): `pkg/runner/critic.go` — scans git log for `[hive:builder]` commits, reviews diffs via Reason() (haiku), creates fix tasks on REVISE. 170 lines + 9 tests. E2E: reviewed Policy commit in 1m16s ($0.16). Pipeline cost: $0.69/task (build + review).
- **Scout Role** (227): `pkg/runner/scout.go` — reads state.md + git log + board, calls Reason() (haiku), creates concrete tasks. 175 lines + 4 tests. E2E: created task after 2 calls ($0.08). Throttle: max 3 agent tasks. **Autonomous loop closed: Scout → Builder → Critic.**
- **Pipeline Mode** (228): `--pipeline` flag runs Scout → Builder → Critic in one command. Fixed one-shot throttle bypass. E2E: 8 min, $1.14. Scout created task, Builder Operated, Critic reviewed. Issue: Scout creates hive tasks but Builder targets site repo — repo mismatch.
- **Scout Fix + Review Ops** (229): Fixed Scout repo mismatch (reads target CLAUDE.md, extracts scout section). Builder shipped review/progress ops autonomously — 94 lines handlers, 110 lines templates. Complete review workflow: submit → review → approve/revise/reject with notifications and UI. Deployed. **27th grammar op.** $1.50.
- **Scout Assignment + First Full Pipeline** (230): Scout assigns tasks after creation. First autonomous pipeline: Scout created+assigned → Builder picked up THAT task (timed out at 10min) → Critic returned REVISE on prior commit, created fix task. **Critic independently caught a real bug: missing state guard in progress handler.** Phase 2 complete.
- **Critic Bug Fix Deployed** (231): Fixed progress handler state guard (Critic-caught bug). Critic now assigns fix tasks. Deployed. Full bug lifecycle proven: ship → catch → fix.
- **First Fully Autonomous Feature** (232): Pipeline shipped Goals hierarchical view. Scout created+assigned → Builder implemented (3m28s, $0.58) → Critic reviewed (REVISE). Deployed. **$0.83 total, 6 min, 0 human intervention.** 4th autonomous commit.

## What the Scout Should Focus On Next

**PHILOSOPHY: CLEAN AS YOU GO.** If something is broken, fix it before building new things. REVISE tasks take priority over new features. Repeated errors (like wrong-repo tasks) should be fixed immediately, not repeated. The hive should leave things better than it found them.

**REPO AWARENESS: The Builder operates on whichever repo `--repo` points to for THIS pipeline run. Currently targeting the site repo. Tasks that require changes to other repos (hive, eventgraph, agent, work) should be tagged with the target repo so they can be routed to a separate pipeline run. Do NOT create tasks the current pipeline run can't implement — if the current target is `site/`, create site tasks. Hive infrastructure tasks are valid work but need a hive-targeted run.**

**COUNCIL DIRECTIVE: MAKE AGENTS DM-ABLE.** The civilization's 50 agents should be contactable on lovyou.ai. Any user can start a conversation with the Philosopher, the Dissenter, the Steward, etc. This is the differentiator no competitor has.

**How it works technically:**
- The Mind already auto-replies when an agent is a conversation participant
- `buildSystemPrompt()` in `graph/mind.go` line 380 builds the system prompt from `mindSoul`
- To support agent personas: check the conversation title for a role name (e.g. "Chat with Philosopher"), load the corresponding role prompt from a roles table or config, and use THAT as the system prompt instead of the generic mindSoul
- The agent picker already exists on the conversation creation form (agents dropdown)
- New: add an "Agents" page that lists available agents with descriptions and a "Chat" button for each
- New: when a conversation has a role-tagged agent participant, Mind uses that role's prompt

**Implementation steps:**
1. Add a `roles` config or table mapping role names to system prompts (could be as simple as a map[string]string loaded from agents/*.md files, or stored in mind_state)
2. Update `buildSystemPrompt` to check conversation title/tags for a role, load its prompt
3. Add `/app/{slug}/agents` route listing available agent personas with descriptions + "Chat" button
4. The "Chat" button creates a conversation with the agent + sets the role in the title/tags

**Previous directive (completed):** User-first sprint shipped 17 features.

**SPECS TO IMPLEMENT (read loop/agent-chat-spec.md and loop/agent-capability-spec.md for full details):**

**Phase 1 — Persona storage + Mind routing:**
1. Create `agent_personas` table (id, name, display, description, category, prompt, model, active)
2. Seed from agents/*.md files at startup (upsert on boot)
3. Update `buildSystemPrompt` in graph/mind.go to check `role:` tag on conversation, load persona prompt instead of generic mindSoul
4. Store role tag in conversation tags when created via agent chat

**Phase 2 — Agents page + chat creation:**
1. GET /agents route — global page listing all active personas grouped by category (Care, Governance, Knowledge, Product, Outward, Resource)
2. POST /agents/{name}/chat — creates conversation with role tag, redirects to chat
3. Persona cards: name, one-line description, category badge, "Chat" button

**Phase 3 — MCP Graph Server (HIVE REPO — needs `--repo ../hive` pipeline run):**
1. cmd/mcp-graph/main.go — MCP server wrapping lovyou.ai REST API
2. Core tools: graph.respond, graph.intend, graph.search, graph.getBoard, graph.getNode
3. Wire into Mind via --mcp-config
4. **Target: hive repo, not site repo. Run pipeline with `--repo ../hive`**

**Phase 4 — Agent memory (SITE REPO — can be built):**
1. agent_memories table in site/graph/store.go (persona, kind, content, source_id, importance)
2. Store.RememberForPersona() and Store.RecallForPersona() methods
3. Inject relevant memories into Mind's buildSystemPrompt
4. Test: agent references a previous conversation

**Phase 5 — Conversation UX (SITE REPO — can be built):**
1. Global contact list showing agents + humans you've talked to
2. Conversation summaries (auto-generated after conversation goes idle)
3. Multiple conversations with same agent listed together
4. Cross-conversation search

**Each phase is 1-3 pipeline iterations. Start with Phase 1 — it's a 15-line change in mind.go + a new table + a seed script.**

**What was already shipped (iters 233-235):**
- Landing page rewritten: "Your team has an AI colleague" (not "One graph, many lenses")
- Welcome page for new users: create-space form instead of redirect to /discover
- Sidebar simplified: 4 core links (Board, Chat, Feed, People) + "More" toggle (was 15 links)

**The Newcomer's complaints (direct from council — use these as task seeds):**
- "I have 8 seconds and you've given me a philosophy lecture"
- "What does the button do? No — what button? Where is the button?"
- "The funnel assumes I already want to be here"
- "13 layers and 15 operations — those are homework. Nobody wants homework."

**Remaining user-first priorities (from council consensus):**
1. Task creation form should pre-suggest assigning to the agent
2. After assigning a task, immediate visible feedback (thinking indicator)
3. Empty states should guide, not just say "nothing here"
4. Mobile nav needs the same simplification as sidebar
5. The "aha moment" — first time the agent responds — must be fast and visible
6. Each lens needs a one-sentence explanation of what it's for
7. Onboarding should end with the user having accomplished something, not just created a container

**Growth agent's wedge:** "A small dev team using Board + Chat + Build with an agent peer. One space, one team, one agent."

**Storyteller's one-line pitch:** "A place where humans and AI take care of each other — and build whatever they need together."

**The test:** Would a stranger who lands on lovyou.ai stay for 60 seconds? Currently: probably not. Target: yes.

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
44. **Research before spec, spec before code.** The competitive research produced specific findings that sharpened the spec. The spec produced a phased build plan. Build from spec, not intuition.
45. **The loop is not optional when batching.** Running 3 iterations without Critic caught a JS hack that shipped to production. When batching, run 3 full loops, not 3 builds.
46. **Three layers of spec, each converged independently.** Primitives (vocabulary), Product (meaning), Compositions (appearance). Missing any layer leaves gaps.
47. **REVISE before new work.** Iteration 189 fixed the iter 186 REVISE (edit reload hack) before starting new work. Outstanding REVISE flags should be resolved at the start of the next iteration, not deferred.
51. **Test the runtime on a task you control.** The first E2E test picked up a stale task because the board was noisy. When testing autonomous systems, create a dedicated task, assign it explicitly, and verify the system picks that specific task — not whatever happens to sort first. Control the test input.
52. **A design task needs a design artifact.** The builder "completed" a design task by thinking about it — no file written, no spec produced. Builder should verify that Operate produced changes before marking DONE, or distinguish design vs implementation tasks.
53. **The builder follows patterns, not rules.** It reads adjacent code and replicates the pattern. But rules not visible in the immediate context (like an allowlist 400 lines away) will be missed. Pattern-following is necessary but not sufficient. The Critic must enforce completeness.
54. **Diff-only review catches what was added wrong, not what was omitted.** Omission errors require grep-based verification. Reason() reviews the diff; Operate() reviews the codebase.
55. **The autonomous loop is closed but untested as a pipeline.** Scout, Builder, Critic each work in isolation. Real test: run them together.
56. **The Scout must know the Builder's target.** Scout reading hive state.md creates hive tasks. Builder targeting site repo can't implement them. Scout prompt must include target repo context.
57. **The Scout must assign tasks it creates.** Without assignment, the Builder claims random unassigned tasks. Scout→Builder handoff requires: create → assign → Builder picks up.
58. **The Critic validates the entire architecture.** When the Critic independently catches a bug the human missed, the three-role system proves its value.
59. **Ship → Catch → Fix is proven. Ship → Catch → Auto-fix is next.** Critic's fix tasks need to be small enough for the Builder to complete within the 10-minute timeout.
60. **The pipeline ships product. $0.83/feature, 6 minutes, one command.** The constraint is no longer "can it work" but "what should it build next."

## Vision

**The product is a substrate for collective existence.** Not a task tracker. Not a social network. A platform where any group — friend group, dev team, company, charity, city, civilization — can organize their existence using the same graph, grammar, and agents.

- **Root:** Collective existence (the soul: "take care of your human, humanity, and yourself")
- **Architecture:** Event graph, grammar ops, signed causal chains. Kind-agnostic — a Node is a Node whether it's a task, a policy, or a friendship.
- **Modes:** 11 currently (Board, Projects, Goals, Roles, Feed, Chat, Threads, People, Knowledge, Governance, Build, Activity). More emerge as entity kinds are added.
- **Entity kinds:** 11 currently (task, post, thread, comment, conversation, claim, proposal, project, goal, role). 7+ more from the unified spec (team, department, policy, process, decision, resource, document, organization).
- **Scale:** Same primitives serve solo dev through civilizational. Modes emerge from content, not from feature gates.
- **Agents:** Peers on the graph. Many hives, many apps. Each space can have its own agents with their own roles and capabilities.
- **Revenue:** Charge corporations, free for individuals. Hosted persistence.
- **Visual identity:** "Ember Minimalism" — dark, warm, intentional, alive.

**Specs:**
- `unified-spec.md` — structural ontology (collective existence, derivation order, 10 modes, 18 entities)
- `work-general-spec.md` — Work expanded to 6 modes, 12 entity types
- `social-spec.md` — Social 4 modes (Chat, Rooms, Square, Forum) as compositions
- `work-product-spec.md` — Execute mode depth (12 ops, state machine, decomposition)
- `social-product-spec.md` — Social product positioning

## What the Scout Should Focus On Next

**LOVYOU_API_KEY:** `lv_b7fb22cde43a8a65289f77ee6dc9aa195184bf6129160f62691e59d8d6ccc8dd`

**Mind tools:**
- `cmd/reply` — one-shot, identity from API key
- `cmd/post` — publishes iteration summaries

**The entity kind pipeline is proven.** Each new kind = 1 constant, 1 handler, 1 template. Priority order for remaining kinds: Policy, Decision (Govern mode) → Document (Learn mode) → Resource (Allocate mode) → Organization (meta-container). Team shipped in iter 223 (12th entity kind). **Test iteration recommended before 5th pipeline entity (next entity kind).**

50. **When pipelines are proven, batch with confidence but audit at boundaries.** The entity pipeline has produced 4 kinds (project, goal, role, team) with zero regressions. But each untested addition compounds risk. Set a boundary (every 4-5 entities) and run a test sweep.

**Cross-entity depth is more valuable than more kinds.** Board project filter (207) shows the pattern: entities filtering and contextualizing each other. Next: Goal progress dashboard (aggregated Goal → Project → Task view), task auto-assign to filtered project.

**Hive runtime Phase 1 is COMPLETE.** Builder flow proven end-to-end (iter 224). Next priorities from hive-runtime-spec.md Phase 2:
1. **Monitor role** — triage unassigned tasks, clean stale tasks, restart crashed agents. Unblocks builder by keeping the board clean.
2. **Scout role** — find product gaps, write to board as tasks. Replaces manual iteration scouting.
3. **Critic role** — review recent commits, audit invariants.
4. **Stale task cleanup** — 76 open tasks on the board, many already completed in code. Monitor should close them.

**How to run the builder:**
```bash
cd hive && LOVYOU_API_KEY=lv_... go run ./cmd/hive --role builder --repo ../site --space hive --agent-id 36509418df854dd4a709cfee3e915a17 --one-shot
```
