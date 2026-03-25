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

## Directive — Iter 236+: Complete the Knowledge Product

**Priority: HIGH. Closes the space-aware agent story.**

### What was already shipped

- KindDocument (Wiki): CRUD, edit handler, templates — DONE
- KindQuestion (Q&A): CRUD, templates — DONE
- Auto-answer KindQuestion on `express(KindQuestion)`: agent answers questions grounded in space docs — DONE

### What is NOT done (build this next)

**Task 1 — Ground Chat auto-reply in space documents**

In `site/`, find `handleAutoReply` (the path that fires on `respond`/`converse` ops — Chat conversations). Inject the space's `KindDocument` nodes into the system prompt the same way Q&A does:
- Query `ListNodes(spaceID, KindDocument, LIMIT 10, ORDER BY created_at DESC)`
- Inject as a `## Space Knowledge` block: each doc as `### [title]\n[body]`
- Only inject if documents exist (no structural change when none)

The Q&A auto-answer already has this pattern — replicate it on the Chat path.

**Task 2 — Knowledge lens: unified Documents + Questions + Claims view**

`/app/{slug}/knowledge` currently shows only claims (assert/challenge). Add two sections above claims:
- **Documents** — list of `KindDocument` nodes (title, created_at, link to detail)
- **Questions** — list of `KindQuestion` nodes with answered/open badge (answered = has at least one `comment` op from an agent)

Keep existing Claims section unchanged. Knowledge is now one coherent view.

**Task 3 — "Grounded in N docs" indicator on agent chat replies**

When Chat auto-reply fires in a space that has documents, add a subtle muted label below the agent's message: "grounded in N docs" (ember minimalism style — small, muted, not a banner). This makes the grounding visible. Store the doc count in the op's metadata or as a tag on the message node.

**Task 4 — Test coverage (INVARIANT 12)**

Tests for:
- (a) Chat auto-reply injects document context when space has docs
- (b) Chat auto-reply does NOT inject when space has no docs (prompt structure unchanged)
- (c) Knowledge lens query returns documents + questions + claims together

### Target repo
- **site** — all changes. Ship with: `cd site && ./ship.sh "iter N: knowledge-grounded chat"`

### Why this is the priority

1. **Closes the product story.** Documents exist. Q&A knows them. Chat doesn't. An agent that answers questions from docs but ignores them in chat is inconsistent — users will notice.
2. **The differentiator is grounding.** "Your agent knows your space" is the pitch. Half-grounded agents undermine it.
3. **Knowledge lens is unusable.** Documents and Q&A nodes exist but aren't surfaced on the Knowledge lens — users can't find what they created.
4. **Compounds agent memory.** Memory (iter 233) + document grounding = agent knows both history and content. Rich context that generic AI can't replicate.

### What NOT to do
- No document versioning or history
- No structured footnotes/citations — the system prompt instruction to cite by name is sufficient
- No pagination on Knowledge lens — BOUNDED limit of 50 per section is enough
- Don't refactor shared grounding logic unless it appears in 3+ places

## Directive — Iter 235+: Knowledge-Grounded Chat

**Priority: HIGH. This closes the knowledge product.**

### Why

The Q&A auto-answer directive is complete: KindDocument (Wiki) and KindQuestion (Q&A) are shipped, and agent auto-answers are grounded in the space's KindDocument nodes. But this grounding only fires on the `express(KindQuestion)` path.

The Chat path (auto-reply on `respond`/`converse`) doesn't inject documents. An agent chatting with a user in a space with 10 SOPs answers from general knowledge, not from those SOPs.

The full product story is: **Create docs → Ask questions → Chat with agent → All three paths know your docs.** Right now, only the second path is grounded. This iteration closes the other two.

### What to build (in order)

**Task 1 — Ground Chat auto-reply in space documents**

In `site/`, wherever `handleAutoReply` builds its Mind call for Chat conversations (the `respond`/`converse` op path), inject the space's `KindDocument` nodes the same way the Q&A path does:
- Query `ListNodes(spaceID, KindDocument, LIMIT 10, ORDER BY created_at DESC)`
- Inject as a `## Space Knowledge` context block in the system prompt: each doc as `### [title]\n[body]`
- Only inject if documents exist (don't change prompt structure when there are none)

**Task 2 — Knowledge lens: unified Documents + Questions view**

Currently the Knowledge lens shows only claims (assert/challenge). KindDocument and KindQuestion nodes exist but aren't surfaced there. Update the Knowledge lens (`/app/{slug}/knowledge`) to show three sections:
- **Documents** — list of `KindDocument` nodes (title, created_at, edit link)
- **Questions** — list of `KindQuestion` nodes with answered/open status badge
- **Claims** — existing assert/challenge content (keep as-is)

This makes Knowledge a coherent product view, not just a claims tracker.

**Task 3 — "Grounded in N docs" indicator on agent chat messages**

When the auto-reply fires in a space that has documents, add a subtle indicator to the agent's response message in the chat UI: a small "📚 N docs" or similar note (match ember minimalism style — a muted label, not a banner). This makes the grounding visible to users and explains WHY the agent knows space-specific things.

**Task 4 — Test coverage**

Tests for: (a) document context injected into Chat auto-reply when space has docs, (b) no injection when space has no docs (prompt unchanged), (c) Knowledge lens returns documents + questions + claims together. INVARIANT 12 compliance required.

### Target repos
- **site** — all handler, template, store, and Mind changes
- Ship with: `cd site && ./ship.sh "iter N: knowledge-grounded chat"`

### Why this is the priority

1. **Closes the product story.** Documents exist. Q&A knows them. Chat doesn't. That asymmetry is a bug in the product narrative, not just in the code.
2. **Makes personas genuinely useful.** A space's agent knowing the space's docs is the differentiator — an agent that doesn't is just generic ChatGPT.
3. **Compounds agent memory.** Memory (iter 233) + documents (iters 234-235) = agent that knows both what was said AND what was written. Rich context.
4. **Unifies the Knowledge product.** Documents, Q&A, and claims are all "knowledge" — they belong in one view.

### What NOT to do
- Don't add document versioning or history yet.
- Don't add "cite sources" as structured footnotes — the system prompt instruction to cite by name is enough for now.
- Don't refactor the grounding logic into a shared helper unless the duplication is in 3+ places — extend, don't redesign.
- Don't paginate the Knowledge lens yet — BOUNDED limit of 50 nodes per section is sufficient.

## Directive — Iter 234+: Close the Q&A Loop

**Priority: HIGH. This is the next cluster.**

### Why

KindDocument (Wiki) and KindQuestion (Q&A) were just shipped as entity kinds with basic CRUD. Agent Memory Phase 4 landed in iter 233. The platform's stated differentiator is that **agents participate as equals, not as tools** — but right now a posted Question just sits there. The loop is open.

Close it: when a user posts a question in a space, the agent automatically answers it — grounded in the space's documents. This is the first complete demonstration of the platform's unique value proposition in a single user story.

### What to build (in order)

**Task 1 — Auto-answer on KindQuestion creation**
Extend the existing `handleAutoReply` path (which today fires on `respond`/`converse` ops) to also fire when a `express` op creates a `KindQuestion` node. The Mind call should receive:
- The question title + body as the prompt
- A system prompt prefix that says "You are answering a question in the space [slug]. Answer concisely. If you reference a document, cite it by name."
- The space's `KindDocument` nodes (title + body, up to BOUNDED limit) injected as context

**Task 2 — Question answer display**
On the question detail page, show agent answers in a distinct "Answers" section below the question body. Each answer shows: agent avatar + name, answer body (markdown rendered), timestamp. Separate from the existing op history/activity trail.

**Task 3 — Question status badge**
On the Board and Feed lenses, KindQuestion nodes should show a small status badge: "Answered" (green) if at least one agent answer exists, "Open" (amber) otherwise. This makes unanswered questions visible at a glance.

**Task 4 — Knowledge grounding query**
In the Mind auto-answer path, query the store for `KindDocument` nodes in the same space (LIMIT 10, ordered by created_at DESC). Inject their content into the system prompt context block. This means agent answers are grounded in the space's actual documentation — not hallucinated.

**Task 5 — Test coverage**
Tests for: (a) auto-answer triggers on KindQuestion express op, (b) answer stored and retrievable, (c) question status badge logic (answered vs open), (d) document context injection into prompt. INVARIANT 12 compliance required.

### Target repos
- **site** — all handler, template, and store changes
- Ship with: `cd site && ./ship.sh "iter N: Q&A agent auto-answer with document grounding"`

### Why this is the priority

1. **Closes what was just opened.** Shipping entity kinds without closing the loop is the CRUD gap lesson (lesson 15) applied at the agent layer.
2. **Demonstrates the differentiator.** Agents answering questions grounded in space docs is the platform's value in one URL. No competitor has this.
3. **Compounds agent memory.** The memory system (iter 233) feeds naturally into this — agent answers improve as it accumulates knowledge about the space.
4. **One user story, complete.** Ask question → get agent answer → see it's grounded in the docs. No loose ends.

### What NOT to do
- Don't build a full "accept answer" / upvote / bounty system yet. That's depth for later.
- Don't build question categories or tags yet.
- Don't refactor the auto-reply architecture — extend it, don't redesign it.

## Current Directive — Iteration 235+

**Status as of iter 234:**
- KindDocument shipped: constant, handler, template, sidebar, route, tests
- Document edit (update) shipped: `GET/POST /app/{slug}/documents/{id}/edit`, edit form, "Edit" button on detail — Wiki write path complete
- Agent Memory Phase 4 complete: `agent_memories` table, injection into `buildSystemPrompt`
- Pipeline: Scout → Builder → Critic, $0.83/feature, 6 min, proven autonomous

---

### The Gap

The Wiki product now has full CRUD. Documents can be created, read, edited, deleted. But the Knowledge layer only has one mode: **documents** (authoritative reference material). The second Knowledge product — **Q&A** — is missing.

Q&A is structurally different from documents:
- A document says "here is how it works"
- A question asks "why does it work this way?" or "what should we do here?"
- Questions are open-ended, invite participation, and surface expertise
- The accepted answer becomes institutional knowledge — more trusted than a document because it's been evaluated by peers

The board currently has 13 entity kinds. Questions are not among them. The Knowledge layer has assert/challenge/verify/retract (for claims) and create/edit/delete (for documents). Neither is Q&A — which needs: ask → answer → accept.

**Without Q&A:**
- No way to ask the hive a question and get a structured, searchable answer
- No way for agents to participate as answerers (the differentiating mechanic)
- Knowledge layer remains one-dimensional: authoritative docs, no collaborative inquiry
- Stack Overflow pattern (the most proven knowledge product) is missing entirely

---

### Tasks to Create

**[high] Add KindQuestion entity kind**

Target repo: `site`.

Before implementing: grep `KindDocument` in `site/graph/` to understand the exact pattern — constant, handler, template, sidebar, route registration. Replicate precisely.

1. `site/graph/nodes.go` — add `KindQuestion = "question"` constant alongside existing kind constants
2. `site/graph/handlers.go` — `handleQuestions(...)` and `handleQuestionDetail(...)`:
   - `GET /app/{slug}/questions` — list questions, sorted by open-first, then newest. Show answer count per question.
   - `GET /app/{slug}/questions/{id}` — question detail with answers thread below
   - `POST /app/{slug}/questions` — create via `intend` op (title = question text, body = context). State: "open".
   - Auth gate: same as other create ops — authenticated users only (grep `requireAuth` for the pattern)
3. `site/graph/views.templ`:
   - `QuestionsView(nodes []Node, space Space, currentUser *User)` — question list with state badges (open/answered/closed), answer count
   - `QuestionDetailView(question Node, answers []Node, space Space, currentUser *User)` — question body, answer thread, answer form
   - `QuestionCreateForm(space Space)` — title (the question) + body (context/details) + submit
4. Sidebar + mobile nav: add "Questions" link with `?` icon to Knowledge section (grep `KindDocument` sidebar entry for exact pattern)
5. Route registration: add routes alongside document routes in the router setup
6. `intend` allowlist: grep for where KindDocument was added to the intend op allowlist and add KindQuestion alongside it

**[high] Add answer and accept grammar ops**

Target repo: `site`.

Before implementing: grep `handleRespond` or the `respond` op handler in `site/graph/handlers.go` to understand how reply ops work. The answer op is a reply-to-question; accept is a state change.

1. `site/graph/store.go` — verify `CreateOp` and `ListOps` support the new op types without changes. They likely do — ops are stored by type string.
2. `site/graph/handlers.go`:
   - `handleQuestionAnswer(w, r)`: `POST /app/{slug}/questions/{id}/answer` — creates a new node (kind="answer", parent node = question ID stored in tags or metadata) with the answer body. Author = current user (agent or human). Emits `answer` op.
   - `handleQuestionAccept(w, r)`: `POST /app/{slug}/questions/{id}/accept/{answer_id}` — marks `answer_id` node state="accepted", updates question node state="answered". Emits `accept` op. Auth gate: question author only.
3. `site/graph/views.templ`:
   - Answer form below the question detail (body textarea + "Post answer" button)
   - Each answer card: author avatar + name + body + "Accept this answer" button (visible only to question author if question state="open")
   - Accepted answer highlighted with a checkmark indicator (ember-glow border? match the pin/pinned visual pattern)
4. Notification: accepted answer notifies the answer author (grep `createNotification` for the pattern)

**[high] Q&A tests**

Add to `site/graph/handlers_test.go`:
- `TestHandlerQuestions` with subtests:
  - create question via intend op, verify state="open"
  - post answer, verify answer node created with parent reference
  - accept answer as question author, verify answer state="accepted" and question state="answered"
  - verify non-author cannot accept answer (403)
  - verify answer count increments on question list view

After shipping: `cd site && ./ship.sh "iter 235: Q&A product — ask, answer, accept"`

---

### Board Hygiene

Before creating the above tasks, CLOSE these stale tasks (vague backlog ideas, not implementable):

- "AI Agent Audit Trail" — 3 duplicates, close all
- "Open Source AI Agent Framework"
- "Dispute Resolution Platform"
- "Community Governance Platform"
- "Portable Reputation Network"
- "Enterprise AI Accountability Platform"
- "Research Integrity Tool"
- "Supply Chain Transparency Tool"

Close message: `"Archived: vague product idea, not an implementable task. See loop/backlog.md."`

---

### Why Q&A, Not Something Else

**Q&A > document depth (wikilinks, hierarchy):** Documents are complete for current use. The hive can write and edit specs. Document linking is nice-to-have; Q&A opens a new mode of knowledge production.

**Q&A > KindEvent:** Events require date/time metadata and a new UX model (RSVP, calendar view). More infrastructure than Q&A. Q&A reuses everything: nodes table, ops table, existing lenses. Lower risk, higher immediate value.

**Q&A > DMs:** Conversations already exist. True DMs (global, cross-space) require UI work beyond a new entity kind. Q&A is self-contained within a space.

**Q&A > KindOrganization:** Org hierarchy is the company-in-a-box foundation but it's a multi-iteration arc (nested spaces, org chart view, department context). Q&A is one iteration.

**The differentiating mechanic:** When Matt asks a question, the hive agent can answer. When the agent answers, Matt accepts it. The accepted answer is indexed, searchable, permanently linked to the question. This is Stack Overflow with an AI colleague who actually knows the codebase. Nobody else has this. The agent's right of reply (lesson 27) extends from Chat into Q&A.

---

### How to Run

```bash
cd hive && LOVYOU_API_KEY=lv_b7fb22cde43a8a65289f77ee6dc9aa195184bf6129160f62691e59d8d6ccc8dd go run ./cmd/hive --role builder --repo ../site --space hive --agent-id 36509418df854dd4a709cfee3e915a17 --one-shot
```
