# Loop State

Living document. Updated by the Reflector each iteration. Read by the Scout first.

Last updated: Iteration 300, 2026-03-27.

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

- **Agent Memory Phase 4** (233): `agent_memories` table, `RememberForPersona`/`RecallForPersona`, JSON extraction (kind+importance from LLM), memory injection into `buildSystemPrompt`. 4 tests (remember+recall, defaults, invalid kind, system prompt injection).

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
61. **Lesson 64: Bottleneck synthesis requires binding response contracts.** Scout must receive explicit accept/defer/renegotiate from Builder, not implicit deferral. Without Strategy Arbiter role, blocking prerequisites become invisible backlog. Enforce Scout-Builder handoff as documented contract, not advisory flag.
62. **Lesson 65: Escalations without matching infrastructure are unverifiable and become deferrable.** Scout flags test failures in Postgres; Builder must run tests in Postgres. Missing DATABASE_URL in Builder environment breaks the verification loop and makes escalations aspirational, not binding.
63. **Lesson 66: Escalation scopes require binding.** Scout directs specific verification; Builder can choose unrelated work. Without explicit obligation to match Scout's scope, escalations are advisory suggestions, not binding directives.
64. **Lesson 67: Escalations without binding scope become deferrable.** Escalation enforcement requires: (1) named scope, (2) Builder acknowledgment of scope, (3) visible artifact linking escalation to work completed.
65. **Lesson 68: Feedback loop infrastructure is a critical path blocker.** When Scout identifies that measurement systems are missing (artifact writes, feedback channels), Critic must verify these are implemented before marking DONE. Absence of feedback infrastructure is a system defect, not a code quality issue. The loop depends on measurement to reflect on itself (Lesson 43). Without artifacts, the loop is blind to its own operation.

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

## Completed Directives

### Knowledge Product (DONE)
Documents, Q&A, agent auto-answer — shipped (iters 241+).

**What shipped:**
- `KindDocument` and `KindQuestion` entity kinds in `site/graph/`
- Knowledge tab routing: `/app/{slug}/knowledge?tab=docs|qa|claims`
- Document list view and Q&A list view with agent-answered status badges
- Mind auto-answer trigger: new `KindQuestion` nodes fire Mind with space documents as context; answer posted as `respond` op
- Knowledge lens sidebar with Docs / Q&A / Claims sub-tabs
- Handler and store tests for each new route

## Current Directive — Iteration 242+

**Priority: Close the autonomous self-direction loop**

The PM role was added (iter 241 infra commit) to read completed tasks. But it doesn't write a new directive back to `loop/state.md`. This means the Scout in the autonomous pipeline still reads a human-maintained directive — which is now stale (iter 241 targeted the grounded indicator; the hive builder already shipped it). The loop isn't self-directing.

**Target repo:** `hive` (pkg/runner/)

**The gap:** PM reads completed board tasks → does nothing with them → Scout reads stale state.md directive → creates tasks for already-shipped features. Fix: PM synthesizes a new directive from completed task history + git log + current state, then writes it to state.md's directive section.

**Tasks for the Scout to create:**

1. **PM writes directive** (`pkg/runner/pm.go`): After reading completed tasks, call `Reason()` (haiku) with: completed task titles, recent git log (last 10 commits), current state.md directive. Output: a fresh 3-5 sentence directive identifying the next unbuilt gap. Write it to `loop/state.md` under `## Current Directive`. Overwrite on each PM cycle.

2. **Add `## Current Directive` section to state.md** (`loop/state.md`): A dedicated section PM owns. Scout reads this section explicitly (not the whole file). Clear separation: PM writes directives, Reflector writes lessons, Reflector appends reflections. Scout targets this section.

3. **Scout reads `## Current Directive` section** (`pkg/runner/scout.go`): Update Scout prompt to read the `## Current Directive` section from state.md specifically (not the whole 300-line file). This makes the Scout's directive fresh each cycle.

4. **Test: PM directive generation** (`pkg/runner/pm_test.go`): Mock completed tasks + git log → verify PM produces a non-empty directive that doesn't repeat already-completed work. One test: if completed tasks include "grounded indicator", new directive must not mention it.

**Why this is highest priority:**

Lesson 60: "The constraint is no longer 'can it work' but 'what should it build next.'" The autonomous pipeline ships at $0.83/feature in 6 minutes. But it's only as good as its directive. Right now, a human PM (this conversation) is the bottleneck — we write the directive manually. Closing this loop makes the pipeline fully autonomous. That's the precondition for "company in a box" and the Lovatts engagement.

**Ship as:** `iter 242: PM writes directive to state.md`

## Directive — Iteration 240+: Hive Dashboard — Make `/hive` Real

**Priority: High.** The landing page says "Watch it build →" and links to `/hive`. That page currently shows a scaffold. Anyone who clicks that link sees nothing. Fix the promise.

### Context

The `/hive` route, handler, and HiveView template were scaffolded in iters 234-239 (`site` repo). The data is already in the database — hive agent ops, board tasks, activity. It just isn't wired to the view. The hive agent has a real actor ID (registered in iter 25-27). The ops table has every action it has taken.

### What to build (3 tasks, in order)

**Task 1 — HiveStats store query** (`site` repo, `graph/store.go`)
Add a `HiveStats` struct and `GetHiveStats(ctx, agentActorID)` query:
- `RecentCommits []string` — last 10 op bodies where `op = 'express'` and `actor_id = agentActorID`, ordered by `created_at DESC`. These are the hive's posts (build summaries).
- `RecentTasks []Node` — last 5 nodes where `kind = 'task'` and `actor_id = agentActorID`, ordered by `created_at DESC`. These are the tasks the hive created.
- `TotalOpsCount int` — count of all ops where `actor_id = agentActorID`.
- `LastActiveAt time.Time` — max `created_at` across all ops for this actor.
- Add a test in `graph/hive_test.go` — verify the query returns populated stats for a seeded agent actor.

**Task 2 — Wire handler to populate HiveView** (`site` repo, `graph/handlers.go`)
In the `/hive` handler, resolve the hive agent's actor ID (look up by `is_agent = true` in users table, pick the one with `lv_` API key — or use the existing `agentActorID` already in handler context if present). Call `GetHiveStats`. Pass to the template. No mock data.

**Task 3 — Populate HiveView template** (`site` repo, `views/` or `graph/views.templ`)
Replace scaffold placeholder with real sections:
- **"Currently building"** — most recent open task title + status badge (in-progress/open). If none, show "Idle".
- **"Recent commits"** — last 5 `express` op bodies, truncated to 80 chars, with relative timestamps. These are the build summaries the post tool publishes.
- **"Stats bar"** — Total ops count, last active timestamp (relative), iteration number (hardcode current: 239, or derive from post count).
- **HTMX polling** — `hx-get="/hive/stats" hx-trigger="every 15s"` on the stats section. Add a `/hive/stats` partial route returning just the stats bar HTML. This makes the page live.
- Visual style: match Ember Minimalism. Dark cards, rose accent on the "currently building" state. No skeleton — if data is missing, show "Idle" state gracefully.

### Why this is the priority

The landing page already made the promise. The scaffold is 90% of the cost. This is the last 10% that makes it real. A visitor who sees actual commits appearing and a task in progress understands the civilization immediately. A visitor who sees a blank scaffold bounces and never returns.

**Target repo:** `site`
**Deploy:** `cd site && ./ship.sh "iter 240: /hive dashboard — real data"`
**Invariants to check:** VERIFIED (tests on new store query), BOUNDED (LIMIT on all queries), IDENTITY (use actor_id not agent name).

## Scout Directive: Complete the Hive Dashboard (`/hive`)

**Priority:** High
**Target repo:** `site`
**Current state (iter 239):** The `/hive` route exists (`site/graph/handlers.go`). `HiveView` template renders stat cards (features shipped, total spend, avg cost/feature) and a posts feed with per-post cost and duration. `ListHiveActivity` store query is tested. Cost/duration parsing is implemented and unit-tested. Handler tests pass.

**Remaining work:**

1. **Pipeline role status panel** — Query the last 20 agent posts, extract pipeline roles from title prefixes (`[hive:scout]`, `[hive:builder]`, `[hive:critic]`, `[hive:reflector]`). Display each role as a card with: role name, icon, last active timestamp, last task title. Pulse "active" (green) if last activity < 30 min; "idle" otherwise. Add to `HiveView` above the posts feed.

2. **Nav + landing page links** — Add a "Hive" link to the desktop header nav and mobile nav. Add a "Watch it build →" CTA to the landing page (`/`).

**Implementation notes:**
- Code lives in `site/graph/` (handlers.go, views.templ) — NOT `site/internal/handlers/`.
- No new tables. No new entity kinds. No auth required on `/hive`.
- Pipeline role classification: scan `posts[].Title` for `[hive:X]` prefix. Each role gets one card showing the most recent post matching that prefix.

**Definition of done:** `/hive` shows live pipeline role status + stat cards + posts feed, and is linked from nav and landing page.

## Directive — Iteration 234+: Knowledge Product — Wire the Three Layers

**Why this now:**
KindDocument, KindQuestion, and assert/challenge claims all exist but are disconnected. Agent Memory Phase 4 is live — agents can now actually answer questions from memory. The Knowledge layer (Layer 6) is the site's most differentiated product: AI-powered collective knowledge with provenance. Build it as a product, not a pile of entity kinds.

**What done looks like:** A user creates a Knowledge space, navigates to it, sees Documents + Q&A + Claims as unified knowledge, asks a question and receives an agent answer grounded in the space's documents, and can verify/challenge any claim. This is a product people would pay for.

---

**Task 1 [site] — Knowledge sidebar navigation**
When a space has any KindDocument, KindQuestion, or assert/challenge activity, the sidebar shows three tabs under the Knowledge lens: "Docs", "Q&A", "Claims". Currently each entity kind is isolated — wire them together under one lens with sub-navigation. The Knowledge lens URL becomes `/app/{slug}/knowledge` with `?tab=docs|qa|claims`.

**Task 2 [site] — Question list view with agent answers**
Route: `/app/{slug}/knowledge?tab=qa`. List all KindQuestion nodes in the space. Each row shows: question title, first 200 chars of the agent answer (if answered), "Answered" / "Awaiting answer" status badge. Unanswered questions should be visually distinct (dimmed, pulsing dot). Clicking a question opens the detail view with the full answer. Add a "Ask a question" button that creates a KindQuestion node and triggers Mind auto-answer.

**Task 3 [site] — Document list view**
Route: `/app/{slug}/knowledge?tab=docs`. List all KindDocument nodes: title, excerpt (first 200 chars of body), last edited by (agent or human with badge), relative time. "New document" button. Clicking opens the existing document detail/edit view. Sort by updated_at descending.

**Task 4 [site] — Knowledge space creation preset**
In the new-space creation dialog, add a "Knowledge Base" preset option alongside the existing types. When selected, pre-fills: name suggestion, sets a `preset=knowledge` tag, and lands the user on the Knowledge lens instead of Board after creation. This makes "start a knowledge base" a first-class user journey, not an accidental discovery.

**Task 5 [site] — Mind auto-answers new questions**
When a KindQuestion node is created (via `intend` op with `kind=question`), the server-side Mind event handler should fire — same pattern as the existing auto-reply on conversations. Mind receives: the question text + the space's recent documents (injected as context). The answer is posted as a `respond` op on the question node. This closes the Knowledge → Agent → Answer loop that Memory Phase 4 enabled.

---

**Target repo:** `[site]` for all tasks.  
**Routing note:** Tasks tagged `[site]` go to the site repo. Tasks tagged `[hive]` stay in hive. No cross-repo dependencies in this directive.  
**Test requirement (invariant 12):** Each task needs at least one handler-level test. No untested code ships.  
**Ship each task:** `cd site && ./ship.sh "iter N: knowledge - <description>"`. One deploy per task.

## Directive — Iter 236+: Complete the Knowledge Product

**Priority: HIGH (task 3 remaining).**

### Status (as of iter 236)

- ✓ Task 1 — Chat auto-reply grounded in space docs (`replyTo` calls `ListDocumentContext`, injects `## Space Knowledge` into system prompt)
- ✓ Task 2 — Knowledge lens unified: Documents + Questions + Claims in one view (`handleKnowledge` updated)
- ✗ Task 3 — "Grounded in N docs" indicator on agent chat replies (NOT done — next iteration)
- ✓ Task 4 — Tests: `TestReplyToInjectsDocuments`, `TestReplyToNoDocumentsNoSpaceKnowledge`, `TestHandlerKnowledgeLens`, `TestAutoReplyDocumentInjectionPath`, `TestListDocumentContextBounded`

### Remaining: Task 3 — "Grounded in N docs" indicator

When Chat auto-reply fires in a space that has documents, add a subtle muted label below the agent's message: "grounded in N docs" (ember minimalism style — small, muted, not a banner). This makes the grounding visible to users and explains why the agent knows space-specific things.

Implementation notes:
- Store the doc count in the reply node's `tags` (e.g., `"grounded:3"`) when `replyTo` injects docs
- Render the label in `chatMessage` templ component when the tag is present
- Only show when tag exists (no structural change for messages without docs)

### Target repo
- **site** — all changes. Ship with: `cd site && ./ship.sh "iter N: grounded chat indicator"`

### What NOT to do
- No structured footnotes/citations — the system prompt instruction to cite by name is sufficient
- Don't add a full "sources" panel — a muted label is enough


## Current Directive — Iteration 263+

**Priority: Fix test isolation → then unlock Organize mode with Role Membership**

Two sequential tasks. Do not start task 2 until task 1 is committed and passing.

---

### Task 1 — Fix invite handler test isolation (REVISE condition, Lesson 47)

Three tests in `site/graph/handlers_test.go` fail with duplicate slug constraint errors: `TestHandlerJoinViaInvite`, `TestHandlerCreateInviteHTMX`, `TestHandlerRevokeInvite`. They were silently blocked by the routing panic fixed in iter 262; now exposed.

Root cause: test setup reuses hardcoded slugs across tests that share a live Postgres instance. Fix: generate unique slugs per test run using `fmt.Sprintf("test-space-%d", time.Now().UnixNano())` or similar, or wrap each test in a BEGIN/ROLLBACK transaction.

**Files:** `site/graph/handlers_test.go`

**Verification:** `go.exe test -run "TestHandlerJoinViaInvite|TestHandlerCreateInviteHTMX|TestHandlerRevokeInvite" ./graph/` → all pass.

---

### Task 2 — Role Membership: assign users to Roles and Teams

Roles (`KindRole`) and Teams (`KindTeam`) are entity kinds with no membership model. A user can view the Roles list but cannot join a role or be added to one. Organize mode requires membership to be useful.

**Data model:** Use the existing `ops` table — emit a `join` op on a Role or Team node (same op used for space membership), with `actor_id` = the joining user. Roles and Teams already use the same node/op infrastructure as spaces. No new table needed.

**Changes:**

1. `site/graph/handlers.go` — extend the `join` and `leave` op handlers to accept `kind=role` and `kind=team` nodes (currently only accepts `kind=space`). Existing guard: check that the actor is authenticated. Owner check: allow space owners to add other members via `assign` op on the role node.

2. `site/graph/store.go` — add `ListRoleMembers(ctx, nodeID) []User` query: JOIN users on ops WHERE node_id = $1 AND op = 'join' AND NOT EXISTS (leave op after the join). LIMIT 50.

3. `site/graph/views.templ` — add a Role detail view at `/app/{slug}/roles/{id}` showing:
   - Role title + description
   - Member list (avatar + display name + agent badge if applicable)
   - "Join this role" button if not a member; "Leave" if already a member
   - Empty state: "No members yet"

4. `site/graph/handlers.go` — add `GET /app/{slug}/roles/{id}` route wired to the new Role detail template.

5. Update the Roles list view (`site/graph/views.templ`) — each role card shows member count (from a `CountRoleMembers` query or subquery).

**Test:** One handler test — POST join op on a KindRole node → verify 201, verify `ListRoleMembers` returns the joining user.

**Why this over alternatives:**
- Test isolation is a Lesson 47 REVISE — cannot be deferred.
- Role Membership turns two shipped entity kinds (Roles, Teams, 11th+12th) from dead weight into usable infrastructure. Organize mode (the 2nd of 6 Work modes) requires it. This is depth on existing surface, not new surface — high leverage, low cost.
- Strategy Arbiter is a hive-internal improvement; user-facing depth comes first.

**Target repo:** `site`
**Ship as:** `iter 263: fix invite test isolation + role membership`



## Make /hive Real — Show the Civilization Working

**Target repo:** site

The landing page says "Watch it build →" and links to `/hive`. That page currently shows a scaffold. Anyone who clicks that link sees nothing. The hive has shipped 294+ iterations autonomously — it deserves a window.

**Why now:** The pipeline is proven ($0.83/feature, 6 minutes, autonomous). The artifacts are written (build.md, critique.md, scout.md). The Reflector records each iteration. The data exists. The only missing piece is a page that surfaces it.

**What the /hive page should show:**
1. Pipeline phase indicator — which phase last ran (Scout / Builder / Critic / Reflector)
2. Current open tasks from the hive space board (what's being worked on)
3. Recent builds — last 5 build summaries from the hive agent's feed posts
4. Iteration counter and cost (from state.md or feed post metadata)
5. Live updates via HTMX polling (3s interval, same pattern as Chat)

**Tasks for the Scout to create:**

1. **Read the current /hive handler** (`site/handlers.go` or `site/routes.go`) — find where `/hive` is served, read the current template. Understand what's already scaffolded before writing anything.

2. **Extend the /hive handler** to fetch real data: call `ListNodes` for the "hive" space (kind=task, state=open), and fetch recent posts from the hive agent user (kind=post, last 10). Pass both to the template. Add a `HivePageData` struct with fields: `OpenTasks []Node`, `RecentPosts []Node`, `Iteration int`, `PipelinePhase string`.

3. **Build the /hive template** (`site/views/hive.templ`) — four sections:
   - **Header**: "The Civilization Engine" headline + subtitle
   - **Pipeline status**: phase cards (Scout / Builder / Critic / Reflector) with the last-active phase highlighted (derive from most recent post content or task state)
   - **Current work**: list of open tasks with title, state, assignee badge
   - **Build log**: recent posts from the hive agent (title, body preview, timestamp) — these are the iteration summaries the post tool publishes

4. **Add HTMX polling** — `hx-get="/hive" hx-trigger="every 5s" hx-target="#hive-content" hx-swap="outerHTML"` on the main content div. Add a partial route `/hive/status` that returns just the current task and recent post data (no full page reload).

5. **Tests** — one test in `handlers_test.go`: `TestHivePage` verifies the handler returns 200 and the template renders without error. One store query test: `TestListHiveActivity` verifies posts from the hive agent can be retrieved by user ID.

**Invariants:** VERIFIED (tests for handler + store query), BOUNDED (cap posts to 10, tasks to 20), IDENTITY (filter by agent user ID, not by scanning post content for "hive:builder" strings).

**Ship as:** `iter 295: /hive live dashboard — pipeline activity visible to all`

## What the Scout Should Focus On Next

## What the Scout Should Focus On Next

**Priority: Fix the Architect role — close the plan gap in the hive pipeline**

**Target repo:** hive

**Why this now:** Commit `c89ea2c` added debug logging for an Architect plan parse failure — a breadcrumb, not a fix. The Architect role exists in `pkg/runner/` but fails to parse the LLM's response, producing a zero-value plan. Without it, the pipeline runs Scout → Builder with no plan phase: the Builder improvises from a task description instead of a structured architecture plan. This causes more Critic REVISE cycles. The debug commit proves the problem is known; it hasn't been fixed.

**The gap:** Architect receives a task → calls Reason() → LLM returns a plan → parser fails (likely markdown-wrapped JSON vs. clean JSON mismatch) → Architect produces nothing → Builder gets no architecture context. The pipeline has a silent hole between Scout and Builder.

**Tasks for the Scout to create:**

1. **Root-cause the parse failure** — Read `pkg/runner/architect.go` (or wherever the architect lives — grep for `architect` in `pkg/runner/`). Find the plan parser and the struct it unmarshals into. The debug log from `c89ea2c` shows the raw LLM response that failed. Identify the mismatch: markdown fences? Schema mismatch? Wrong field names? Write one sentence in the task description: "root cause is X."

2. **Fix the plan parser** — Strip ` ```json ` / ` ``` ` fences before unmarshal (defensive, handles both clean and wrapped responses) OR update the Architect prompt to explicitly require raw JSON with no fences — whichever the code already leans toward. Critical: an empty/zero-value plan must return an error, not silently succeed. Silent failure is the current bug.

3. **Verify the Builder handoff** — Trace from `architect.Run()` to `builder.Run()`. Confirm the parsed plan is injected into the Builder's prompt as structured context (files to create/modify, component designs, build sequence). If the handoff is missing or the plan is parsed but discarded, wire it in. The Builder should receive: task description + architect plan, not just task description.

4. **Tests** — `pkg/runner/architect_test.go` (new or extend): (a) parse clean JSON → succeeds; (b) parse markdown-wrapped JSON → succeeds after fix; (c) empty LLM response → returns error; (d) malformed/partial JSON → returns error. Use realistic LLM output samples, not toy inputs. All 4 must pass.

**Done criteria:** Architect produces a valid plan from any LLM response format. Builder receives and uses that plan. Parse errors surface as returned errors, never silent zero-values. Four test cases cover the failure modes. Pipeline runs Scout → Architect → Builder with the plan in play.
