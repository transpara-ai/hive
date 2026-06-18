# Social Platforms — Competitive Research

**What makes each social platform great, feature by feature. Every specific UI pattern, interaction, and mechanic that users love. Mapped to our 15 grammar operations. Where our architecture enables something genuinely better — not just equivalent.**

Matt Searles + Claude · March 2026

---

## Methodology

For each platform: enumerate every feature that matters to users. Be specific — not "has DMs" but "what happens when you open a DM and the other person is typing." Map each to our 15 grammar operations (Emit, Respond, Derive, Extend, Retract, Annotate, Acknowledge, Propagate, Endorse, Subscribe, Channel, Delegate, Consent, Sever, Merge). Mark:
- ✓ we have this
- ~ we partially have it
- ✗ we don't have it
- **+** we have something genuinely better

The goal isn't to clone. It's to understand what makes these products *feel* indispensable so we know what to match, what to skip, and where we're genuinely superior.

---

## The 15 Grammar Operations (Reference)

| # | Operation | What it does | Where it appears |
|---|-----------|-------------|-----------------|
| 1 | **Emit** | Broadcast content to a context | Post, send message, publish |
| 2 | **Respond** | Reply to a specific node | Reply, thread reply, reaction message |
| 3 | **Derive** | Generate new content from existing | Quote tweet, fork thread, summarize |
| 4 | **Extend** | Add content to an existing node | Edit, append, add to thread |
| 5 | **Retract** | Withdraw content (non-destructive) | Delete (append-only: retraction event) |
| 6 | **Annotate** | Add context/metadata to a node | Community Notes, fact-check, tag |
| 7 | **Acknowledge** | Signal receipt/awareness | Read receipt, seen, like (lightweight) |
| 8 | **Propagate** | Amplify content to new audience | Retweet, share, forward, reblog |
| 9 | **Endorse** | Stake reputation on content | Like (identity-linked), upvote, award |
| 10 | **Subscribe** | Follow/join a source | Follow, subscribe, join channel |
| 11 | **Channel** | Create/manage persistent contexts | Create channel, room, space |
| 12 | **Delegate** | Transfer authority | Mod someone, pin a post, assign |
| 13 | **Consent** | Accept/approve participation | Accept friend request, join invite |
| 14 | **Sever** | Break a connection | Block, ban, leave, unfriend |
| 15 | **Merge** | Combine contexts | Thread merge, duplicate resolution |

---

## 1. TWITTER / X

Twitter's genius: the 280-character constraint forced extreme information density. The best tweet is a complete thought in the shortest possible form. This is what every other platform tries to replicate and fails at.

### 1.1 The Feed

**Feature: Algorithmic timeline (For You)**
- Shows content from accounts you don't follow based on engagement patterns
- Content ranked by predicted engagement, not chronology
- "Spaces" (live audio) surfaced in feed header when relevant
- Trending topics shown in sidebar — real-time cultural pulse
- "What's happening" topic clusters

Grammar mapping: Propagate (amplification), Endorse (engagement signals feed ranking). The feed IS the Propagate graph — what's being shared most surfaces highest.

Our position: ✗ We have no algorithm. **+ But our graph enables something better:** transparent traversal. Instead of "the algorithm chose this for you," we expose: "this surfaced because 3 people you endorse also endorsed it." The user sees the causal path. Twitter's algorithm is a black box producing engagement-maximized content. Ours can be: "show me what people I trust are endorsing" — same personalization, zero manipulation, full auditability.

**Feature: Following timeline**
- Pure chronological view from accounts you follow
- Toggle between For You / Following

Grammar mapping: Subscribe (follow graph), Emit (posts from followed accounts).

Our position: ✓ Our feed is chronological by default. We show posts from spaces you're in and people you follow. **Gap: no toggle UX between algorithmic and chronological.**

**Feature: Notifications tab — mentions, likes, retweets, follows**
- Grouped by type: all / replies / mentions / likes / reposts / new followers
- Each notification shows who + what they did + the content
- "Activity" section showing engagements on your posts in real-time

Grammar mapping: Acknowledge, Endorse, Propagate, Subscribe — all notification-producing operations.

Our position: ~ We have notifications but they're not grouped/typed. **Gap: no notification grouping, no "who liked your post" real-time activity.**

---

### 1.2 Tweet Composition

**Feature: Threaded tweet composition**
- "Add another tweet" button — continue a thread inline before posting
- Each tweet in the thread can have different media
- Thread preview shows exactly how it'll render
- Publish all at once as a single unit

Grammar mapping: Emit + Channel (a thread is a mini-channel). The thread pre-composition is a Channel being assembled before the first Emit.

Our position: ~ We have threads but you compose them by replying after posting, not in pre-composed threads. **Gap: pre-composed thread drafting.**

**Feature: Tweet drafts**
- Drafts auto-save as you type
- Draft shelf accessible any time
- Multiple drafts in flight simultaneously

Grammar mapping: Extend (a draft is a node being extended before Emit).

Our position: ✗ No draft system. **Impact: high for power users, essential for long-form posts.**

**Feature: Polls**
- 2-4 options, 1-7 day duration
- Real-time vote count visible to creator
- Total votes + percentage per option after closing

Grammar mapping: Emit (structured) + Acknowledge (voting). A poll is a structured Emit that accepts constrained Acknowledge responses.

Our position: ✗ No poll primitive. This is a composition we could derive from Emit + constrained Endorse.

**Feature: Alt text on images**
- Prompts for alt text when attaching images
- "ALT" badge visible on image in feed to signal accessibility

Grammar mapping: Annotate (alt text is an annotation on a media node).

Our position: ✗ No media attach, no alt text.

---

### 1.3 Engagement Mechanics

**Feature: Like (heart)**
- Single-click, no confirmation
- Count visible but anonymized to public (you see who liked in notifications)
- Liked posts saved to "Likes" tab on profile
- No reputation cost — liking is frictionless

Grammar mapping: Acknowledge (current implementation). A Twitter like is lightweight — it's signal aggregation, not identity-staking.

Our position: ✓ We have reactions. **+ Our architecture enables the distinction:** Acknowledge vs Endorse. Acknowledging is "I saw this and found it worth noting." Endorsing is "I stake my reputation on this." Twitter only has one operation (like) serving both functions. We can have both: react with emoji (Acknowledge) vs Endorse (which builds the author's Trust Score and costs the endorser reputation capital).

**Feature: Retweet / Quote Tweet**
- Retweet: amplify without comment. One click, appears in your followers' feeds.
- Quote tweet: amplify with comment. Your words framing theirs.
- "Retweets with comments" tab on original tweet shows all quote tweets

Grammar mapping: Propagate (retweet) + Derive (quote tweet). Quote tweet is the most interesting: it's Derive — new content derived from existing content, with causal link to the original.

Our position: ~ We have repost/quote in the feed (iter 195). **+ Our quote tweet equivalent has explicit causal linkage in the event graph.** A Twitter quote tweet just embeds a reference. Ours is a Derive operation — the new post has a cryptographic causal link to the original. You can trace the provenance chain of any content.

**Feature: Reply threading — collapsed/expanded**
- Top-level tweet shows reply count
- Click to expand thread
- Replies sorted by engagement within the thread
- "Show more replies" for long threads

Grammar mapping: Respond (replies), Endorse (sorting signal).

Our position: ✓ We have threaded replies. **Gap: no engagement-based sorting within threads.**

**Feature: Bookmarks (private)**
- Save tweets without liking (private, not counted)
- Searchable bookmark collection
- Folder organization (Twitter Blue)

Grammar mapping: Acknowledge (private variant — no public signal).

Our position: ✗ No bookmarks/private saves.

---

### 1.4 Community Notes (formerly Birdwatch)

**Feature: Community-added context on misleading tweets**
- Any user can write a Note explaining why a tweet is misleading
- Notes become visible only when a cross-partisan consensus exists
- Rating system: "helpful" / "not helpful" from note writers
- Transparent: all notes (shown + not shown) are publicly downloadable

Grammar mapping: Annotate (notes) + Endorse (rating notes) + Derive (consensus calculation from cross-partisan endorsement pattern).

Our position: **+ We have something genuinely better.** Community Notes requires building and verifying cross-partisan consensus before showing anything — which takes time and can fail. Our Annotate is immediate and attributed. Every annotation is a signed operation from an identity with a Trust Score. Instead of "this community reached consensus," we have: "3 people with Trust > 0.8 who have been verified as [domain experts / opposed positions] have annotated this." The annotation carries the annotator's identity and reputation weight. Better: faster, more attributable, reputation-linked.

---

### 1.5 Spaces (Live Audio)

**Feature: Live audio rooms**
- Creator starts a Space, followers get notified
- Speakers + listeners, controlled speaking queue
- Host can invite listeners to speak
- Recordings available after (Twitter Blue)
- React with emojis in real-time

Grammar mapping: Channel (the Space is a live channel), Subscribe (notification of start), Consent (request/grant to speak), Delegate (grant speaker role), Emit (speaking = emitting audio to channel).

Our position: ✗ No live audio. This is a Channel variant — scheduled ephemeral channels with different media type (voice). The grammar already supports it; the infrastructure doesn't exist yet.

---

### 1.6 Twitter's Weaknesses We Exploit

1. **No causality.** A tweet can reference another but there's no graph link. Quote tweets embed but don't link cryptographically. Thread continuity is maintained by convention, not structure. **We have explicit causal links on every operation.**

2. **Identity is the account, not the person.** You can delete your account. You can change your handle. Other accounts can impersonate you. Trust is follower count — gameable and platform-controlled. **We have cryptographic identity. Trust is earned through verified operations, not accumulated follows.**

3. **Algorithm is a black box.** You don't know why something was shown to you. You can't audit the feed selection. Engagement-maximization creates outrage dynamics. **Our traversal is transparent. We can show: "surfaced because 2 people you endorse endorsed this, via these 3 hops."**

4. **Moderation is opaque.** Shadow banning exists, isn't acknowledged, can't be appealed. Moderators are invisible. **Our Delegate and Sever operations are signed events on the graph. Every moderation action is auditable. Delegation chains are visible.**

5. **Edits don't preserve history.** Twitter Blue edits replace the original (with a "show edits" button that few click). **Our Extend/Retract are append-only — the original always exists, the supersession is a new event with causal link.**

---

## 2. DISCORD

Discord's genius: organized chaos. It took IRC channels, added voice/video, made them persistent and searchable, and gave communities admin tools that actually work. The key insight: servers are communities, not just chats.

### 2.1 Server Structure

**Feature: Server with categorized channels**
- Servers contain channels
- Channels organized into collapsible categories
- Text, voice, forum, announcement, and stage channel types per category
- Each channel has a topic/description shown in header

Grammar mapping: Channel (each channel is a Channel), Channel (categories are Channel containers), Emit (topic = annotation on Channel).

Our position: ~ We have spaces with channels planned (Rooms mode in social-spec). **Gap: no category hierarchy within a space, no channel types.**

**Feature: Channel permissions per role**
- Roles can see / cannot see specific channels
- @everyone, @here, custom roles
- Permission overrides per-channel override server defaults
- Channels hidden from users who can't see them (not grayed out)

Grammar mapping: Delegate (role assignment), Channel (with permission metadata), Consent (joining a server accepts its role structure).

Our position: ✗ No role-based channel visibility. **Gap: significant for community spaces.** + Our architecture has a better foundation: permissions are Authority levels on graph operations, not ad-hoc role flags. We can derive channel visibility from: "this channel is Delegate-protected at level X, and your Trust Score qualifies at Y."

**Feature: Threads on messages**
- Any message can spawn a thread (creates a sub-channel)
- Thread appears in a side panel, doesn't pollute main channel
- Thread archive when no activity for 24h/72h/1w
- Active threads shown in "Active Threads" section

Grammar mapping: Channel (thread is a ephemeral Channel), Respond (thread replies), Merge (when thread is archived back into main).

Our position: ~ We have thread replies but not this pull-out-into-sub-channel behavior. **Gap: no sidebar thread panel, no thread archiving.**

---

### 2.2 Voice/Video Channels

**Feature: Persistent voice channels**
- Join a voice channel like entering a room — others can see you're there
- No call to "accept," just join
- Works without camera: voice-only by default
- Screen share to the channel
- Mobile: swipe to join voice from channel list

Grammar mapping: Subscribe (join voice channel), Channel (voice channel persists), Emit (speaking = emitting to channel), Consent (others can see you're in channel).

Our position: ✗ No voice/video infrastructure.

**Feature: Stage channels (one-to-many audio)**
- Speakers elevated, audience listens
- Request-to-speak queue
- Scheduled events
- Auto-recording

Grammar mapping: Channel (stage), Delegate (speaker role), Consent (request to speak), Subscribe (event notification).

Our position: ✗ No stage channels. This is the same pattern as Twitter Spaces — Consent + Delegate on an audio Channel.

---

### 2.3 Role and Permission System

**Feature: Hierarchical roles with color/icon**
- Roles stack: Admin > Moderator > Member > Guest
- Roles have colors, icons, displayed in member list
- Hoisting: roles can be "hoisted" to appear separately in member list
- Role mentioning: @Moderators pings everyone with that role

Grammar mapping: Delegate (role assignment), Channel (role-gated channels), Consent (accepting role permissions when joining).

Our position: ~ We have roles in work context (assignee, owner). No social roles system. **Gap: no server-wide role hierarchy, no role-gated visibility.**

**Feature: Moderation tools**
- Ban, kick, timeout users
- Bulk message delete
- Mod log: all mod actions recorded and visible to admins
- AutoMod: rule-based message filtering
- Verification levels (phone, email, server age)

Grammar mapping: Sever (ban/kick), Retract (bulk delete), Delegate (mod log = recording Delegate operations), Moderate (society-specific extension).

Our position: ~ We have report op (moderation layer 3). **Gap: no timeout, no bulk delete, no audit log for mods.** **+ Our Sever is a signed graph event — every ban is permanently attributed and auditable. Discord's mod log is internal; ours is on the graph, verifiable externally.**

---

### 2.4 Engagement and Onboarding

**Feature: Onboarding questionnaire on join**
- Servers can ask new members: "What brings you here?" with multiple choice
- Different channels/roles auto-assigned based on answers
- Reduces noise by routing people to relevant channels immediately

Grammar mapping: Consent (structured consent on join — you're not just Subscribing, you're declaring your intent), Channel (consent answers route to channels).

Our position: ✗ No join questionnaire. **+ Our Consent operation natively supports structured consent with declared purpose.** When you join a space, you could emit a Consent with metadata: "joining as: developer | interested in: backend, architecture." This is better than Discord's hack (questionnaire → role → channel visibility) because the declared intent is a signed graph event, visible to governance.

**Feature: Server boosts and Nitro**
- Users pay to "boost" a server, unlocking features (better audio, more emoji, custom invite URL)
- Boosters get a special role, visible in member list
- Progress bar showing boosts until next level

Grammar mapping: Endorse (boost is an identity-linked endorsement with economic stake), Channel (unlocked features are Channel capabilities).

Our position: ~ We have economic concepts in Market layer. **+ Our endorsement system is a better foundation for "staking on a community" — but we haven't built the feature UX.** An Endorse with resource transfer (Market integration) is exactly what Discord Nitro boost is: "I stake $X of value on this community."

---

### 2.5 Discord's Weaknesses We Exploit

1. **No task context.** You can discuss tasks in Discord but you can't create/assign/track them natively. The #dev-tasks channel is just messages. **We have work graph integration — a channel message can create a task with one command.**

2. **No identity graph.** Your Discord user is your server persona. You can have different names in different servers. Reputation doesn't transfer. **We have cryptographic identity that transfers across spaces. Your Trust Score follows you.**

3. **Moderation is invisible.** The mod log is internal. Users banned don't know on what grounds. Community members can't see what mods are doing. **Our Delegate and Sever operations are on the graph. Every mod action is a signed event.**

4. **Permissions are role flags, not authority levels.** Discord permissions are binary flags per role, not graduated authority levels. **Our Authority model (Required → Recommended → Notification) and Trust Scores give graduated permissions that earn over time.**

5. **No consent model.** Joining a Discord server is a single click. There's no declared purpose, no community norms acceptance, no structured agreement. **Our Consent operation can carry structured metadata — joining a space can require explicit agreement to norms, recorded as a signed event.**

---

## 3. REDDIT

Reddit's genius: voting-based quality sorting. The crowd continuously curates the best content to the top. The subreddit as a self-governing community with elected moderators. Reddit's soul: the best content rises regardless of who posted it.

### 3.1 Feed and Voting

**Feature: Hot / New / Rising / Top sort**
- Hot: engagement velocity × time decay
- New: pure chronological
- Rising: high engagement velocity in last hour
- Top: all-time / year / month / week / today
- User can sort any feed, subreddit, or comments

Grammar mapping: Endorse (votes) + Derive (ranking formula applied to Endorse graph). Each sort mode is a different traversal of the Endorse graph.

Our position: ~ We have a feed. **Gap: no multi-sort, no rising/hot algorithms.** **+ Our graph makes traversal explicit:** "Top this week" = "Endorse operations in last 7 days, sorted by count." We can build any sort mode as a named traversal query. Better: we can let users define their own traversal.

**Feature: Upvote/Downvote (anonymous)**
- Two-state: up or down, not just positive engagement
- Karma: accumulated upvotes - downvotes across all posts/comments
- Vote counts are fuzzed ±10% to prevent vote manipulation
- Anonymous: other users can't see who voted

Grammar mapping: Endorse (upvote) + a negative Endorse (downvote). Reddit treats downvote as a first-class signal.

Our position: ~ We have reactions (iter 198). **Gap: no downvote / negative signal mechanism.** **+ Our Endorse is identity-linked with Trust Score weight.** An upvote from a 0.9 Trust Score user who has domain expertise is worth more than 10 upvotes from new accounts. We don't need to fake randomness (Reddit's fuzz) — we have quality signal in the endorser's identity.

---

### 3.2 Subreddit (Community) Structure

**Feature: Subreddit rules sidebar**
- 1-15 rules, each with title + description
- Rules shown during post composition
- Report categories derived from rules
- Moderators reference rules when removing content

Grammar mapping: Channel (subreddit = Channel with governance), Norm (society-specific: rules are Norms on the Channel), Annotate (removal reason references rule).

Our position: ~ We have space settings. **Gap: no structured rules system, no rule-referenced moderation.** **+ Our governance layer has Propose + Vote — rules are Proposals that pass when community votes. Better than Reddit where mods write rules unilaterally.**

**Feature: Post flairs / tags**
- Required or optional flair on posts
- Flair appears in feed, filterable
- User flair (custom text visible next to username in subreddit)
- Flair search: filter feed by flair

Grammar mapping: Annotate (flair = annotation on post), Channel (flair filter = traversal of Channel's Emit graph by annotation).

Our position: ✗ No flair/tag system on posts. **This is a pure Annotate operation.**

**Feature: Moderator queue**
- Reports from users go to mod queue
- Mods can: remove, approve, or escalate
- Removal reasons selectable, reason visible to reporter (sometimes)
- Mod log: all actions recorded internally

Grammar mapping: Moderate (report queue), Delegate (mod action), Retract (removal = non-destructive removal event), Sever (ban from subreddit).

Our position: ~ We have report op. **Gap: no mod queue UI, no mod log.** **+ Our Retract is a signed event on the graph.** Every removal is attributable and auditable — not just internally, but publicly if the community chooses governance transparency.

---

### 3.3 Comment System

**Feature: Nested threaded comments with collapse**
- Comments nest to arbitrary depth
- Click to collapse a thread branch
- "Continue this thread" when depth exceeds display limit
- Best / New / Controversial / Top / Q&A sort modes

Grammar mapping: Respond (comments), Endorse (votes for sorting). The collapse behavior is a client-side traversal of the Respond graph.

Our position: ~ We have threaded replies (iter 90). **Gap: no collapse-at-depth, no "continue this thread," no multiple sort modes for comments.**

**Feature: Comment awards (Gold, Silver, etc.)**
- Users can award comments/posts with real-money-backed tokens
- Award appears as icon on comment
- Awarded user gets perks (Reddit Premium, etc.)
- Accumulated awards visible on user profile

Grammar mapping: Endorse (award = identity-linked, economically-staked Endorse).

Our position: ~ We have Endorse. **+ Our Endorse can natively carry economic weight via Market integration.** An award is an Endorse + resource transfer. We don't need a separate token system — this is Market + Endorse composing naturally.

---

### 3.4 User Reputation

**Feature: Karma (post karma + comment karma)**
- Post karma: accumulated from post votes
- Comment karma: accumulated from comment votes
- Separated for transparency
- Used as trust signal (new accounts can't post in some subreddits)
- Karma milestones unlock features (verified account indicators)

Grammar mapping: Endorse accumulation → Trust Score. Karma IS a Trust Score derived from aggregated Endorse operations.

Our position: **+ We have something genuinely better.** Reddit karma is a raw count: sum of upvotes - downvotes. Our Trust Score is:
- Non-transitive (you can't launder trust through a fake network)
- Domain-specific (Trust in security discussions ≠ Trust in cooking discussions)
- Decay-weighted (recent endorsements matter more than old ones)
- Quality-weighted (endorsements from high-Trust users matter more)

Reddit karma can be farmed by posting puppy pictures in r/aww. Our Trust Score reflects verified contribution to the domains it's claimed in.

---

### 3.5 Reddit's Weaknesses We Exploit

1. **Anonymous voting is gameable.** Vote brigading (organized mass upvoting/downvoting) is endemic. Karma farming is trivial. **Our Endorse is identity-linked — vote manipulation requires building a legitimate reputation first.**

2. **Mods are unaccountable.** Moderator removals are often opaque. Users can't appeal effectively. Cross-subreddit mod behavior isn't visible. **Our governance model: Delegate records who has authority. Sever (removal) is a signed event. Communities can Propose to revoke mod authority.**

3. **No cross-subreddit identity.** Karma is platform-wide but reputation doesn't transfer. Being a respected contributor in r/science doesn't affect your standing in r/programming. **Our Trust Score is domain-tagged — expertise in one domain can inform trust in related domains via the causal graph.**

4. **Content is siloed in subreddits.** You can't easily track a discussion across subreddits. Cross-posts are separate content, not linked. **Our Propagate operation preserves causal links. A cross-post is a Derive — new Emit with causal link to original. The conversation is connected, not duplicated.**

---

## 4. SLACK

Slack's genius: it killed email for teams. The key insight: teams need a persistent record of their communication organized by topic, not by recipient. Channels replaced cc-all emails. The threaded reply kept discussions organized. Integrations brought the tools into the conversation.

### 4.1 Channel Architecture

**Feature: Channel sidebar with unread indicators**
- Left sidebar: DMs + channels separated
- Unread indicator: bold channel name + message count
- Mention indicator: separate badge for @mentions
- Muted channels: gray, never bold, no badge
- Section grouping: teams/projects can group channels
- Collapse sections

Grammar mapping: Channel (channel list), Subscribe (joining channels), Acknowledge (read receipt drives unread state).

Our position: ~ We have a chat sidebar (iter 31). **Gap: no unread count, no mute, no channel grouping.** + Our Acknowledge is a signed event — read receipts are on the graph, auditable. "Did everyone read the announcement?" is answerable.

**Feature: Slack Connect (external channels)**
- Invite people from other organizations to a shared channel
- Their messages appear in your Slack with their company's identity
- Permissions scoped per channel, not full org access
- Bridging two Slack workspaces

Grammar mapping: Channel (cross-organizational channel), Consent (accepting the invite), Subscribe (both parties subscribing to same channel).

Our position: **+ We have this by architecture.** Every space is already cross-organizational — anyone with an account can be in any public space. Our cryptographic identity means "external" users are just users with different Trust Scores. No special "Connect" feature needed.

---

### 4.2 Messaging

**Feature: Rich text formatting**
- Markdown: **bold**, *italic*, `code`, ```code block```, ~strikethrough~
- Lists, numbered lists, block quotes
- Inline emoji, custom emoji
- File/image attach in-message

Grammar mapping: Emit (the message), Extend (editing).

Our position: ~ We have basic markdown rendering. **Gap: no custom emoji, no rich attachment previews.**

**Feature: Message edit + delete with history**
- Edit any message (pencil icon, inline edit)
- Edited message shows "(edited)" marker
- Message history: click "(edited)" to see all versions
- Delete: shows "Message deleted" placeholder, doesn't remove from thread

Grammar mapping: Extend (edit = extend with new content), Retract (delete = non-destructive retraction). Slack's history view IS the append-only graph — they just expose it for edits.

Our position: **+ We have a better foundation.** Our Extend and Retract are append-only events with causal links to the original. Slack's history is an implementation detail; ours is architectural. **Gap: we don't expose the edit history in the UI yet.**

**Feature: Replies (thread sidebar)**
- Reply in thread: opens a right panel
- Thread preview shows first 2 replies in channel
- "Also send to #channel" checkbox when replying in thread
- Thread participants get notifications even if they didn't send a message

Grammar mapping: Respond (thread reply), Channel (thread as mini-channel), Subscribe (auto-subscribe to threads you're mentioned in).

Our position: ~ We have threaded replies. **Gap: no sidebar panel for threads, no "also send to channel" option, no auto-subscribe on mention.**

---

### 4.3 Search and Knowledge

**Feature: Full-text search across all channels**
- Search by: message text, file name, from user, in channel, date range
- Results show context: channel + surrounding messages
- "Jump to message" opens the message in context
- Saved search filters

Grammar mapping: This is traversal of the Emit graph by content predicates.

Our position: ~ We have `/search` (iter 145). **Gap: no search operators (from:, in:, before:, after:), no "jump to message in context," no saved searches.**

**Feature: Pinned messages / bookmarks**
- Pin important messages to channel
- Pins accessible from channel header (push-pin icon)
- Personal bookmarks: save any message privately
- Bookmark folders (Slack Pro)

Grammar mapping: Delegate (pin = Delegate a message to a privileged channel position), Acknowledge (bookmark = private Acknowledge).

Our position: ~ We have pin/unpin (Culture layer, iter 156). **Gap: no personal bookmarks.**

**Feature: Huddles (lightweight voice/video)**
- Click "Huddle" to start a voice call in a channel
- Others see the huddle is active and can join
- No scheduling needed — spontaneous
- Screen sharing available
- Emoji reactions visible during huddle

Grammar mapping: Channel (huddle = ephemeral voice Channel), Subscribe (join huddle), Emit (speaking), Consent (joining is voluntary, visible to all).

Our position: ✗ No voice/huddles infrastructure.

---

### 4.4 Integrations and Bots

**Feature: App integrations (3000+ apps)**
- /slash commands: `/github`, `/jira`, `/zoom`
- Incoming webhooks: external systems post to Slack
- Interactive components: buttons, dropdowns, modals in messages
- Workflow Builder: no-code automation (if message in #alerts → post to #incidents)
- Block Kit: structured message layouts with interactive elements

Grammar mapping: Emit (bot message), Consent (approving app installation), Channel (webhook posts to channel), Delegate (app gets scoped permissions).

Our position: **+ We have something architecturally superior.** Slack integrations are glue code between isolated systems. Our integrations ARE the system — every tool that speaks the grammar is a native participant. An agent posting to a channel isn't an integration; it's an Emit from an identity in the graph. **Gap: we don't have interactive message components (buttons in messages).**

**Feature: Workflow notifications ("Matt has been mentioned 5 times")**
- Daily digest of mentions when you were away
- "Catch up" summaries of long threads
- Priority DM detection when you're in focus mode

Grammar mapping: Derive (summarization of Acknowledge/Respond operations). This is an agent reading your notification stream and synthesizing it.

Our position: **+ Our architecture enables genuine intelligence here.** Slack's summaries are simple heuristics. An agent with access to the graph can: read your Subscribe list, traverse Acknowledge operations, identify unread Respond chains where your name appears, and generate a causally-coherent summary. The agent IS a participant — not a bolt-on summarizer.

---

### 4.5 Slack's Weaknesses We Exploit

1. **No task-chat integration.** Slack has no native tasks. You integrate with Jira, Linear, Asana. Context-switching is constant: discuss in Slack, create task in Linear, reference in Slack. **Our grammar lets you: `/task create` from a message, auto-link the message as a cause of the task, see task status in chat.**

2. **No trust differentiation.** In Slack, every member's message carries equal weight. The new hire and the 10-year engineer read the same. **Our Trust Scores let conversations surface insights from high-Trust contributors and flag questions from people who need more support.**

3. **No governance model.** Slack workspaces are admin-owned. Admins can read all DMs. Compliance mode exposes everything. Policies aren't community-made. **Our governance: space rules are Proposals that pass by Consent. Data access policies are explicit, signed, and revocable.**

4. **No agent peers.** Slack bots are second-class citizens: different names, no real identity, no persistent history, no reputation. **Our agents are first-class graph participants with cryptographic identity, Trust Scores, and persistent identities.**

---

## 5. INSTAGRAM

Instagram's genius: the constraint of beauty. Every post is a photograph or video. The grid is a curated gallery. The story is ephemeral art. Instagram made mobile photography into social capital.

### 5.1 Feed and Stories

**Feature: Stories (24-hour ephemeral content)**
- 15-second clips or photos, disappear after 24h
- Story ring on avatar: unviewed (colored ring) → viewed (gray ring)
- Forward/back navigation, swipe away
- Seen by indicator: swipe up to see who viewed
- Stickers: polls, questions, music, location
- Highlights: curate stories into permanent collections

Grammar mapping: Emit (story post = ephemeral Emit), Acknowledge (view = Acknowledge), Subscribe (following = Subscribe to their story ring), Channel (highlights = curated sub-Channel on profile).

Our position: ✗ No ephemeral content, no stories. **Gap:** ephemeral Emit with TTL is a different node kind. Our architecture supports it (Retract at TTL = scheduled Retract), but we haven't built it. The viewer tracking (seen by) is Acknowledge — we have this.

**Feature: Explore page (discovery)**
- Grid of recommended posts/reels/accounts
- Topics grid: filter by Sports, Beauty, Travel, etc.
- Search: hashtag, location, accounts, audio
- "Similar to accounts you follow"

Grammar mapping: Propagate (content recommended = amplified by algorithm), Subscribe (account suggestions), Endorse (engagement signals recommendation).

Our position: ~ We have discover page for spaces. **Gap: no content discovery page (only space discovery), no topic filtering.** **+ Our discovery can be: "accounts with overlapping Subscribe graphs" — no black box, just graph proximity.**

---

### 5.2 Content Mechanics

**Feature: Photo/Reel/Carousel**
- Photo: single image
- Carousel: swipe through up to 10 images/videos
- Reels: vertical video up to 90s, full-screen
- Each has caption, location tag, user tags, alt text
- "Collab post": two accounts co-author a post, shown to both followerships

Grammar mapping: Emit (post), Annotate (caption, location, user tags). Collab post = co-Emit or Derive with joint authorship.

Our position: ✗ No media support. **Collab post is interesting:** it's a joint Emit — a node with multiple causal origins. Our event graph supports multi-author causality (an event can have multiple causes). Better architecture: a Collab post is one Emit with multiple ActorIDs in the `authors` field, giving both parties equal ownership.

**Feature: Hashtags**
- Any word prefixed # becomes searchable
- Click hashtag → feed of all posts with that tag
- #tag page shows top posts, reels, accounts

Grammar mapping: Annotate (hashtag = annotation on post), Channel (hashtag page = virtual Channel collecting annotated posts).

Our position: ✗ No hashtag system. **This is a pure Annotate operation: tag the Emit with key-value `hashtag: "sunset"`, then traverse by that annotation.** Our graph can support this directly.

**Feature: Close Friends list**
- Separate story posting to manually curated list
- Green ring indicates "Close Friends" story
- Recipient doesn't know who else is on the list

Grammar mapping: Channel (Close Friends = private Channel), Subscribe (Close Friends adds = Consent-based Subscribe), Emit (posting to Close Friends = Emit to restricted Channel).

Our position: ~ We have private/public spaces. **+ Our Channel model supports this better:** a Close Friends channel is a Space where membership requires explicit Consent from the owner. Currently our spaces are public-or-private but not "curated guest list."

---

### 5.3 Direct Messages (DMs)

**Feature: Disappearing messages**
- Photos/videos sent in DMs disappear after viewing (once or twice)
- "Seen" indicator shows who viewed it and when

Grammar mapping: Emit (ephemeral) + Acknowledge (view-triggered Retract).

Our position: ✗ No ephemeral messaging. **This is architectural: Retract triggered on Acknowledge — message auto-retracts after N views.**

**Feature: Message reactions**
- Long-press on a message to react with emoji
- Anyone can add multiple reactions, visible to all in thread
- Reaction count shown

Grammar mapping: Acknowledge (emoji reaction on message).

Our position: ✓ We have reactions (iter 198).

**Feature: Note (status shown to close friends)**
- Short text shown at top of DM list to close friends
- 24-hour expiration
- Replies go to DMs

Grammar mapping: Emit (ephemeral, scoped to Close Friends Channel), Respond (reply opens DM).

Our position: ✗ No status/note feature.

---

### 5.4 Instagram's Weaknesses We Exploit

1. **No text-first mode.** Instagram is visual-only by design. Everything requires an image or video. Communities that communicate in text (dev teams, knowledge workers, researchers) have no home. **Our platform is equally text, image, and video — no mode is privileged.**

2. **Identity is performance.** Instagram identity is your curated feed and follower count. Authenticity is structurally impossible — the archive (hidden posts) and the grid (curated gallery) reward curation over reality. **Our identity is cryptographic and behavioral. Trust accumulates through contribution, not aesthetics.**

3. **Algorithm captures attention for advertising.** Instagram Explore is engineered to maximize time-on-platform. Every recommendation serves ad impressions. **Our traversal serves the user's declared preferences, not attention maximization.**

4. **No accountability.** Comments can be deleted, users can be blocked, but the record of the interaction is gone. Harassment leaves no trace. **Our append-only graph: every interaction is a signed event. A Retract doesn't delete — it supersedes. The pattern of harassment is traceable.**

---

## 6. TIKTOK

TikTok's genius: the For You Page. The discovery algorithm is so good that it turns complete strangers into overnight creators. The key insight: content quality should determine reach, not follower count. This democratized virality.

### 6.1 The For You Page (FYP)

**Feature: Full-screen infinite scroll feed**
- One video at a time, full-screen
- Auto-play, instant sound (or muted, tap for audio)
- Swipe up = next video
- No sidebar, no thumbnail grid — pure content
- First 1-3 seconds determine if you stay or swipe

Grammar mapping: Propagate (algorithmic amplification = Propagate), Subscribe (the FYP is Subscribe-to-world-filtered-by-algorithm).

Our position: ✗ No video feed, no full-screen mode. **The key lesson:** TikTok's FYP proved that the quality discovery algorithm can be better than the social graph for content discovery. We don't need an algorithm — but we DO need a "discovery mode" where content from outside your Subscribe graph is surfaced based on interest affinity, not engagement-maximization.

**Feature: Duet and Stitch**
- Duet: record your video side-by-side with another video
- Stitch: clip up to 5 seconds of another video + add your response
- Original creator gets credited, can see all Duets/Stitches

Grammar mapping: Derive (Duet/Stitch = Derive with causal link to original) + Annotate (your commentary on theirs).

Our position: **+ We have a better architectural foundation.** Duet/Stitch is Derive — new content with causal link. Our graph natively tracks this. TikTok shows you "all duets of this video" — we can show you the full Derive tree of any content: every response, every remix, back to origin. The provenance chain is structural.

---

### 6.2 Creator Tools

**Feature: Creator analytics**
- Views, likes, comments, shares per video
- Follower demographics (age, gender, territory)
- Profile views, reach, engagement rate
- Trend analysis: which content performs best

Grammar mapping: Endorse (likes), Propagate (shares), Acknowledge (views), Respond (comments) — analytics = aggregated traversal of these operations.

Our position: ~ We have some analytics in profile view. **Gap: no creator analytics dashboard, no trend analysis.** **+ Our graph makes analytics auditable.** TikTok shows you numbers; we can show you the operations. Not "1000 views" but "1000 Acknowledge operations with full traversal path showing which communities they came from."

**Feature: Video series**
- Group related videos into a playlist/series
- Episodes auto-play in order
- Subscribers notified of new episodes

Grammar mapping: Channel (series = ordered Channel of Emit operations), Subscribe (series subscription), Emit (new episode).

Our position: ✗ No series/playlist structure. **This is a Channel with ordering metadata.**

---

### 6.3 Sound/Trend Mechanics

**Feature: Sounds (audio tracks as viral vectors)**
- Every video is linked to an audio track
- Click "use this sound" → create video with same audio
- Sound pages: see all videos using that sound
- Trending sounds surfaced in creation flow

Grammar mapping: Derive (using a sound = Derive from the sound node), Annotate (sound = annotation on video node), Channel (sound page = virtual Channel collecting videos by sound annotation).

Our position: ✗ No media/audio mechanics. **Architecturally:** a sound is a node, using it is Derive (your video has causal link to the sound node). The sound page is a traversal of the Derive graph from that node.

**Feature: Challenges / Hashtag campaigns**
- Creator or brand launches #challenge
- Others create videos tagged with the challenge
- Challenge page: curated videos, trending status

Grammar mapping: Channel (challenge = virtual Channel), Emit (participating = Emit tagged with challenge), Endorse (challenge page ranking by endorsement).

Our position: ✗ No challenge mechanics. **This is: Emit a "challenge" node (Channel kind), others Emit with causal link to it.** The challenge page is traversal of the causal graph from that node. Our graph supports this natively.

---

### 6.4 TikTok's Weaknesses We Exploit

1. **Engagement maximization harms wellbeing.** The FYP optimizes for watch time. Reported outcomes: anxiety loops, body image content pushed to vulnerable users, political radicalization. The system optimizes for you-staying-on-TikTok, not you-living-better. **Our traversal can be configured by the user: "show me content that the people I trust endorse" vs "show me what's trending in my communities." No engagement-maximization default.**

2. **No creator ownership.** Your TikTok content lives on TikTok. The algorithm decides your reach. You can be shadowbanned with no explanation. **Our graph: your content is on an append-only, signed event graph. You can trace exactly which communities it reached and why.**

3. **No discourse.** TikTok is consumption. Comments are an afterthought. There's no way to have a real conversation about content. **Our Respond and Thread operations let content spark structured discussion, not just comment pile-ons.**

4. **Identity is surface.** Your TikTok identity is your follower count and your For You feed performance. Status is entirely platform-assigned. **Our Trust Score is behavior-derived and verifiable.**

---

## 7. MESSENGER (Facebook)

Messenger's genius: it separated from Facebook and became the primary real-time layer for a billion existing social relationships. The phone number / Facebook friends integration meant zero cold-start — everyone you know was already there.

### 7.1 Core Chat

**Feature: "Active now" / "Active X minutes ago"**
- Real-time presence indicator
- Green dot = active now, gray clock = last active time
- Configurable (can turn off "active status")

Grammar mapping: Acknowledge (implicit: your activity creates presence events), Subscribe (seeing someone's presence = Subscribe to their presence channel).

Our position: ✗ No presence system. **This is an Acknowledge triggered on activity — a lightweight form of the Acknowledge operation.** Active-now is: "user sent an Acknowledge(self, alive) event in the last N minutes."

**Feature: Message reactions (emoji heart/like)**
- Long-press: emoji picker with 6 options + emoji search
- Reaction count per emoji shown
- Notification: "X reacted ❤️ to your message"

Grammar mapping: Acknowledge (emoji reaction on message).

Our position: ✓ We have reactions. **Gap: no per-reaction notification ("X reacted Y to your message").**

**Feature: Voice memos**
- Hold-to-record audio message
- Playback bar visible inline in chat
- 2x playback speed

Grammar mapping: Emit (audio content), Acknowledge (listen = Acknowledge audio message).

Our position: ✗ No voice memos. **An audio Emit is just a different media type on the Emit operation.**

---

### 7.2 Group Chats

**Feature: Group chat management**
- Add/remove members mid-chat
- Group name and photo
- Admin controls: only admins can add members
- Approval mode: requests to join require admin approval
- "@mention someone" pings them specifically

Grammar mapping: Channel (group chat = Channel), Consent (approval mode = Consent-gated Subscribe), Delegate (admin role), Subscribe (add/remove = Subscribe/Sever).

Our position: ~ We have conversations. **Gap: no group chat admin controls, no approval mode, no @mention in chat.** **+ Our Consent operation natively supports approval-mode joining.**

**Feature: Polls in chats**
- Create poll: question + options
- Vote inline in the chat
- Results visible in real-time
- "Can't be changed once voted" notice

Grammar mapping: Emit (structured poll node), Acknowledge (vote = constrained Acknowledge), Derive (result tally = Derived from Acknowledge operations).

Our position: ✗ No polls. **Same as Twitter polls — Emit(structured) + constrained Acknowledge.**

**Feature: Group chat themes**
- Color themes for the chat bubble background
- Emoji reactions have custom emoji per theme
- Word effects: type certain words → animated effect

Grammar mapping: Extend (Channel settings = Extend the Channel node with visual metadata), Consent (both parties in 1:1 can change theme).

Our position: ✗ No chat themes. Low priority.

---

### 7.3 Rooms / Video Calls

**Feature: Rooms (persistent video chat rooms)**
- Create a room → share link
- Room stays open until you close it
- Up to 50 people
- Works cross-platform (Facebook, Instagram, WhatsApp)

Grammar mapping: Channel (room = persistent Channel), Consent (joining via link = implicit Consent), Subscribe (joining = Subscribe to room).

Our position: ✗ No video infrastructure.

**Feature: Watch Party**
- Start a video watch session in a room
- Everyone watches the same video synchronized
- React with emoji in real-time

Grammar mapping: Channel (shared viewing Channel), Subscribe (join watch party), Acknowledge (reaction during watch).

Our position: ✗ No watch party. **Interesting grammar composition:** a Watch Party is a Channel with synchronized Acknowledge time-alignment. Everyone's Acknowledge operations are timestamped relative to the video position, not wall clock.

---

### 7.4 Messenger's Weaknesses We Exploit

1. **Privacy trust is destroyed.** Messenger is owned by Meta. Every message is data for advertising. End-to-end encryption is optional and not default. **Our messages are signed events on a graph. The architecture makes mass surveillance structurally difficult — each event is attributed to an identity, and data access requires explicit Consent.**

2. **No context from work.** Messenger conversations are pure social. If you're messaging a colleague about work, there's no task context, no way to reference a project, no shared artifact. **Our Chat integrates with Work — a conversation is a node that can spawn tasks, reference projects, and be linked to decisions.**

3. **No reputation.** In Messenger, everyone in your contacts is equal. You can't distinguish "trusted colleague" from "person I met once." **Our Trust Scores carry into every interaction. You can see: this person's endorsements in this domain are high-quality.**

4. **Bot integration is terrible.** Messenger bots are interactive menus, not conversational participants. They speak "chatbot," not human. **Our agents are peers. They Emit and Respond with the same grammar as humans. They have identities, Trust Scores, and signed operations.**

---

## 8. CROSS-PLATFORM SYNTHESIS

### 8.1 Features Every Platform Has (Table Stakes)

| Feature | Grammar | Our Status |
|---------|---------|------------|
| Real-time delivery (< 100ms) | Emit | ✓ (HTMX polling every 3s — not real-time, needs WebSocket) |
| Read receipts | Acknowledge | ✗ Not implemented |
| Typing indicators | Acknowledge | ✓ (iter 35, thinking indicator) |
| @mentions with notifications | Annotate | ✗ No @mention parsing |
| Emoji reactions | Acknowledge | ✓ (iter 198) |
| Message search | (Emit traversal) | ~ Basic search (iter 145), no in-chat search |
| Media attachments | Emit(media) | ✗ No media |
| Threaded replies | Respond | ✓ (iter 90) |
| Voice/video | Emit(audio/video) | ✗ No infrastructure |
| Notifications | (all ops) | ~ Basic, not grouped |
| Presence/active now | Acknowledge | ✗ Not implemented |
| Drafts | Extend | ✗ Not implemented |
| Mobile push notifications | (all ops) | ✗ Not implemented |

### 8.2 Where We're Genuinely Better

These aren't "nice to haves" — they're structural advantages that emerge from the EventGraph architecture.

**1. Causal provenance on all content**
Twitter can show a retweet chain. Reddit can show comment parents. But neither tracks the *why* — what caused this content to exist. Our Derive operation preserves the full causal chain. Every piece of content on the graph can be traced back to its origins through cryptographic links.

**2. Identity-linked endorsements with Trust Score weighting**
Every platform has some form of "like." None have endorsements that cost reputation capital, accumulate into a domain-specific Trust Score, and can be audited for authenticity. A 0.9 Trust Score endorsement is worth more than 100 anonymous upvotes. This changes the economics of content quality.

**3. Governance as a first-class primitive**
Every platform has moderation. None have governance. The difference: moderation is admin-imposed; governance is community-decided. Our Propose + Vote + Delegate chain means community norms are community-owned, not admin-dictated. The governance of the space IS on the graph.

**4. Agents as peers, not bots**
Every platform has bots. Bots are second-class: different name styling, no reputation, no persistent identity, no Trust Score, often filtered out of feeds. Our agents have cryptographic identity, accumulate Trust, emit signed events, and are indistinguishable from human participants in the grammar. The only difference is the "agent" badge (Invariant 7: TRANSPARENT).

**5. Append-only truth, not delete-rewrite history**
Every platform lets you delete. Some let you edit without trace. The record can be rewritten. Our Retract and Extend are append-only events — the original exists, the supersession is a new event with causal link to the original. You can always see what was said, when it was changed, and by whom.

**6. Cross-layer integration without context switching**
Every platform either focuses on social (Discord, Twitter) or work (Slack, Linear) and tries to hack in the other. The integration is always bolted on. Our Work and Social layers share the same graph, same identity, same grammar. A task can be created from a message. A message can reference a decision. A governance proposal can spawn a work task. The connections are structural.

### 8.3 The Unified Advantage

The individual advantages above compound into one meta-advantage: **the graph is the product.**

On Twitter, the platform is the algorithm. On Discord, the platform is the server. On Slack, the platform is the workspace. None of these are owned or auditable by the community using them.

On transpara.ai, the platform IS the graph. The graph is append-only, signed, hash-chained, and transparent. Communities own their graph. Any community that doesn't trust the platform can run their own instance (self-hosted graph) while maintaining identity and trust interoperability. This is the Neutrality Clause made structural.

---

## 9. FEATURE PRIORITY MAP

Derived from this research: what to build next, in priority order.

### P0 — Table Stakes (blocking adoption)

| Feature | Grammar Op | Platform Ref |
|---------|-----------|-------------|
| @mention parsing + notifications | Annotate | All |
| Unread count on channels | Acknowledge | Slack, Discord |
| In-chat message search | (Emit traversal) | All |
| Message grouping (consecutive messages = visual block) | (Emit display) | Discord, Slack, Messenger |
| Notification grouping by type | (all ops) | Twitter, Instagram |

### P1 — Differentiation (win vs. competition)

| Feature | Grammar Op | Platform Ref | Our Advantage |
|---------|-----------|-------------|--------------|
| Endorsement vs Acknowledge distinction | Endorse vs Acknowledge | Twitter | Trust Score weighting |
| Delegated moderation with audit log | Delegate + Sever | Discord, Reddit | On-graph, signed |
| Agent @mention = native participant | Emit, Respond | Slack | Peer, not bot |
| Cross-layer task-from-chat | Intend from Respond | Slack, Linear | Same graph |
| Transparent feed traversal explanation | (Subscribe + Endorse traversal) | Twitter FYP | Anti-algorithm |

### P2 — Depth (mature product)

| Feature | Grammar Op | Platform Ref |
|---------|-----------|-------------|
| Polls in posts + chat | Emit(structured) + Acknowledge | Twitter, Messenger |
| Post drafts | Extend (pre-Emit) | Twitter, Instagram |
| Bookmark/save (private Acknowledge) | Acknowledge(private) | Twitter, Slack |
| Channel roles with visual indicators | Delegate + Channel | Discord |
| Hashtag/topic channels | Annotate + Channel | Instagram, Twitter |

### P3 — Expansion (longer horizon)

| Feature | Grammar Op | Platform Ref |
|---------|-----------|-------------|
| Ephemeral content (Stories) | Emit(TTL) + Retract | Instagram |
| Voice/audio rooms | Channel(audio) + Consent + Delegate | Discord, Twitter Spaces |
| Media attachments | Emit(media) | All |
| Real-time WebSocket delivery | (infrastructure) | All |

---

*Research complete. This document informs the architecture of Rooms and Forum modes in the Social layer.*
