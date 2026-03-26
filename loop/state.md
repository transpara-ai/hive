# Loop State

Living document. Updated by the Reflector each iteration. Read by the Scout first.

Last updated: Iteration 328, 2026-03-27.

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
66. **Lesson 70: Loop artifact validation must check content completeness, not just file existence.** `close.sh` should verify that COVER, BLIND, ZOOM, FORMALIZE sections are non-empty in reflections.md, and that state.md's Current Directive section is non-empty. Corrupted or truncated artifacts are worse than missing ones — they persist silently and mislead future iterations.
67. **Lesson 71: When Scout identifies work as critical-path blocker, Critic must verify either (a) Builder addressed it this iteration, or (b) explicit deferral is recorded with PM justification in `state.md`.** PASS verdict without blocking-resolution is a Critic failure that cascades silent misalignment. Scout flags a blocker, Builder ignores it, Critic passes anyway = the pattern this lesson exists to break.
68. **Lesson 72: When a new lesson is formalized in reflections.md, Reflector must add it to state.md's lessons list in the same iteration.** Principles live in Scout's input or they don't exist. Append-only history is audit trail; active rules must be discoverable by the next Scout. If state.md isn't updated, the cycle repeats.
69. **Lesson 73: Rules in state.md's lessons list must be mirrored in Scout's contract.** Scout prompt must explicitly require checking the Lessons section before identifying gaps. If a lesson describes a blocking prerequisite, the task must address it or record explicit deferral with justification. Propagating lessons to state.md (Lesson 72) is necessary but not sufficient — binding Scout to consult and comply is what makes lessons executable policy instead of historical documentation.
70. **Lesson 74: Scaffolding without integration is unfinished work.** Complete the full circuit: build type → wire into dispatch → test end-to-end. Deferring integration defers autonomy. Mark all deferrals explicitly in Scout with risk statement.
71. **Lesson 75: REVISE verdicts must block iteration closure until resolved.** Closure requires: (1) all code changes deployed, (2) all prior verdicts honored, (3) Scout reads prior REVISE as prerequisite gap. A loop that advances past unresolved revision is not closed — it is broken.
72. **Lesson 76: Closure gate must verify prior REVISE verdicts are resolved before next iteration begins.** Scout must check prior state.md and flag unresolved REVISE as prerequisite gaps.
73. **Lesson 77: Scout must treat prior REVISE verdicts as blocking prerequisites.** If prior iteration's Critic issued REVISE, Scout's first task is addressing that verdict, not identifying new gaps.

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



## Priority: Fix Reflector `empty_sections` failures — two bugs, one iteration

**Target repo:** hive

**Why this now:**
The pipeline has had two consecutive reflector failures (`2026-03-26T21:02:20Z` and `2026-03-26T21:25:25Z`). Both show `outcome=empty_sections, cost=$0.0000`. The `cost=$0.0000` is because `CostUSD` isn't captured in the diagnostic — not because the LLM wasn't called. Two systemic failures mean the Reflector is reliably broken. Without a working Reflector: `state.md` may be mis-incremented, `reflections.md` accumulates corrupt empty entries, and the loop's feedback mechanism is blind. Fix this before any new feature work.

**Bug 1 — `parseReflectorOutput` misses common LLM format variants** (`pkg/runner/reflector.go`)

The parser looks for `**COVER:**` or `COVER:`. The LLM frequently outputs `**COVER**:` (bold without colon inside the stars). Add coverage for:
- `**COVER**:` — bold word, colon outside
- `**COVER** :` — with space before colon
- `## COVER:` and `### COVER:` — heading formats
- Case-insensitive match (LLM sometimes lowercases section names)

Refactor the marker detection loop in `parseReflectorOutput` to try all variants before giving up on a key. No change to the section-boundary logic — just expand the candidate markers per key.

**Bug 2 — `runReflector` continues after emitting `empty_sections` diagnostic** (`pkg/runner/reflector.go`)

Current code after the empty-section check:
```go
if emptySections {
    log.Printf("[reflector] empty sections in response: %s", raw)
    r.appendDiagnostic(PhaseEvent{Phase: "reflector", Outcome: "empty_sections"})
}
// ← falls through to appendReflection and advanceIterationCounter
```

This means: even on a failed reflection, an empty entry is appended to `reflections.md` AND the iteration counter in `state.md` is incremented. Both are wrong. Add a `return` after `appendDiagnostic`:

```go
if emptySections {
    log.Printf("[reflector] empty sections in response: %s", raw)
    r.appendDiagnostic(PhaseEvent{
        Phase:       "reflector",
        Outcome:     "empty_sections",
        CostUSD:     resp.Usage().CostUSD,
        InputTokens: resp.Usage().InputTokens,
        OutputTokens: resp.Usage().OutputTokens,
    })
    return  // ← don't write corrupt entry, don't advance counter
}
```

Also include `CostUSD`/`InputTokens`/`OutputTokens` in the diagnostic so future PM prompts can see the actual cost.

**Task 1 — Fix `parseReflectorOutput`** (`pkg/runner/reflector.go`)

Expand the marker candidates for each key. The simplest robust approach: for each key (COVER, BLIND, ZOOM, FORMALIZE), build a list of candidate markers and find the earliest match:

```go
candidates := []string{
    "**" + key + ":**",  // **COVER:**
    "**" + key + "**:",  // **COVER**:
    "**" + key + "** :", // **COVER** :
    "### " + key + ":",  // ### COVER:
    "## " + key + ":",   // ## COVER:
    key + ":",           // COVER:
    strings.ToLower(key) + ":", // cover:
}
```

Pick the earliest-occurring candidate. Keep existing section-boundary logic unchanged.

**Task 2 — Add early return on empty_sections** (`pkg/runner/reflector.go`)

After `r.appendDiagnostic(...)`, add `return`. Include cost fields in the `PhaseEvent` as shown above.

**Task 3 — Tests** (`pkg/runner/reflector_test.go`)

Add tests for the new format variants in `TestParseReflectorOutput`:
- `**COVER**:` format (bold without inline colon)
- `## COVER:` format (heading)
- Mixed formats (each section using a different variant)
- Lowercase `cover:` variant

Add a test for the early-return behavior: construct a mock `runReflector` scenario that produces empty sections and verify that `reflections.md` is NOT appended and the iteration counter in `state.md` is NOT incremented. (Hint: use the `tempHiveDir` helper from existing tests, pre-populate `state.md` with "Last updated: Iteration 100,", run, verify iteration stays at 100.)

## Priority: Public Hive Activity Page — `/hive` on lovyou.ai

**Target repo:** site

**Why this now:**
The pipeline is healthy (Reflector fixed, Tester wired). The site hasn't shipped product in several iterations — all recent Builder work was hive infrastructure. The backlog explicitly calls out a "spectator view" that the Designer, Storyteller, and Growth agents all asked for. Right now there is no way for a visitor to understand what the civilization is doing. A `/hive` page fixes that: it makes the autonomous pipeline visible to anyone who lands on lovyou.ai. This is the product's strongest differentiator made legible.

**What to build:**
A public `/hive` route on the site that shows the hive building itself in real time. The hive posts iteration summaries to the lovyou.ai board (via `cmd/post`) — this page reads those posts and renders them as a living build log.

**Task 1 — Scout the data source**
Read `site/handlers/` to understand how existing public pages (e.g. `/knowledge`, `/activity`, `/discover`) fetch nodes from the graph. The hive posts to a space with slug `hive` (or similar). Find the space slug by grepping `cmd/post/` for the slug it targets. Confirm the board stores post content as nodes. Identify the handler pattern.

**Task 2 — Add `/hive` route**
Add a new `GET /hive` handler in `site/handlers/` (follow the pattern of existing public handlers like `handleKnowledge` or `handleDiscover`). The handler:
- Fetches recent posts from the hive's space (limit 20)
- Sorts by created_at DESC
- Passes to a new `HiveView` templ template

**Task 3 — Template: `site/templates/hive.templ`**
Create `HiveView` template. Ember Minimalism style. Layout:
- Hero: "The Civilization Builds" — brief description of the autonomous pipeline
- Timeline of build iterations — each entry shows: iteration number (extract from post title), commit subject, cost if present, timestamp
- Each entry links to the full post detail in the hive space
- Empty state if no posts yet

**Task 4 — Nav link**
Add "Hive" to the site's main nav (header and footer). Link to `/hive`. Use a suitable icon (terminal or cpu). This makes the page discoverable.

**Task 5 — Handler + template tests**
In `site/handlers/handlers_test.go` (or a new `hive_test.go`), add a test that verifies: the `/hive` route returns 200 and the response body contains "Civilization" (or the page title). Follow existing handler test patterns.

**Acceptance criteria:**
- `GET /hive` returns 200 with a list of hive build posts
- Page is linked from the main nav
- Tests pass
- Deploys via `./ship.sh "iter N: add /hive civilization build page"`

## What the Scout Should Focus On Next

## Fix: Architect parse failure loses diagnostic context and likely misses LLM output formats

**Target repo:** hive

**Why this now:**
The reflector fix shipped (iters 321–325). The one remaining pipeline failure is the Architect: `2026-03-26T22:20:17Z, phase=architect, outcome=failure, cost=$0.3082, output_tokens=1282, error="no subtasks parsed from plan"`. The LLM produced 1,282 output tokens — a real, substantial plan — but the parser returned 0 tasks. The failure preview was only written to stderr (not captured in the diagnostic), so the actual LLM output is lost. Two problems: (1) the parser is missing at least one common LLM output format, and (2) diagnostic context is destroyed on failure. Fix both.

**Task 1 — Capture LLM response preview in Architect diagnostic** (`pkg/runner/architect.go`)

When `parseArchitectSubtasks` returns empty, capture the first 1000 chars of `resp.Content()` in the `PhaseEvent.Error` field (or add a new `Preview` field to `PhaseEvent`). Currently only logged to stderr, which is lost after the run. The diagnostic entry should read: `"error": "no subtasks parsed from plan — preview: <first 1000 chars>"` so future PM/Scout can diagnose the format mismatch.

**Task 2 — Add JSON output format support to `parseArchitectSubtasks`** (`pkg/runner/architect.go`)

The LLM sometimes responds with JSON when confident about structure. Add a JSON parse attempt before the strict parser:

```go
type jsonSubtask struct {
    Title       string `json:"title"`
    Description string `json:"description"`
    Priority    string `json:"priority"`
}
```

Try `json.Unmarshal` on the normalized content (after stripping fences). If it succeeds and returns ≥1 tasks, use that result. This handles: `[{"title": "...", ...}]` and `{"tasks": [...]}` wrappers.

**Task 3 — Add regression tests for Architect parser edge cases** (`pkg/runner/architect_test.go`)

Add test cases that cover formats the current parser likely misses based on the 1,282-token output pattern:
- Prose response with a numbered list using em-dash separators: `1. **Title** — description`
- JSON array: `[{"title": "...", "description": "...", "priority": "high"}]`
- Response with preamble before tasks (LLM explains the plan, then lists tasks)
- Mixed format: some tasks in strict format, some in markdown

Confirm each case produces ≥1 parsed subtask.

**Task 4 — Add `Preview` field to `PhaseEvent`** (`pkg/runner/diagnostic.go`)

Add `Preview string \`json:"preview,omitempty"\`` to `PhaseEvent`. Use this for the first 1000 chars of LLM content on parse failures. Keep `Error` for error messages. Update `appendDiagnostic` if needed. Write one test that verifies the field serializes to JSONL correctly.

**Success criteria:** 
- All 4 tasks ship with tests
- `go test ./pkg/runner/...` passes
- A future Architect parse failure will have the LLM preview captured in `diagnostics.jsonl` for PM/Scout diagnosis
