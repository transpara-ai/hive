# Agent Chat Spec — DM-able Agents on transpara.ai

**Derived from the council directive + cognitive grammar. Matt Searles + Claude, March 2026.**

---

## What we're building

Any user can browse the civilization's agents, read what each one does, and start a conversation. The Philosopher thinks with you about hard questions. The Dissenter pushes back on your assumptions. The Steward asks whether you should. The Newcomer tells you what's confusing. The Storyteller helps you find the narrative.

This is the differentiator. Nobody else has 50 minds with distinct perspectives, grounded in a soul, speaking from defined roles, on a signed audit trail.

---

## The experience

### Contact list

A global `/agents` page (not per-space — agents are civilizational, not space-bound):

```
transpara.ai/agents
```

Shows all available agent personas as cards:
- Name (Philosopher, Dissenter, Steward, etc.)
- One-sentence description ("Thinks long-term. Questions patterns. Watches the watchers.")
- Category tag (Care, Governance, Knowledge, etc.)
- "Chat" button → creates a new conversation with this agent

Not 50 cards dumped on a page. Grouped by category:

```
Care:        Steward · Witness · Mourner · Harmony · Teacher
Governance:  CEO · CTO · PM · Guardian · Advocate · Dissenter
Knowledge:   Historian · Librarian · Explorer · Analyst · Research
Product:     Scout · Architect · Builder · Designer · Critic · Tester · Observer
Outward:     Storyteller · Newcomer · Inhabitant · Growth · Customer-success
Resource:    Treasurer · Simplifier · Legal · Philanthropy
```

### Starting a conversation

User clicks "Chat" on the Philosopher card. A new conversation is created:
- Title: "Chat with Philosopher" (auto-generated, user can rename)
- Participants: [user, hive-agent]
- Role tag: `role:philosopher` (stored in conversation tags, tells the Mind which persona to use)

The user lands in a standard chat interface. The Philosopher responds using the `philosopher.md` system prompt instead of the generic `mindSoul`.

### Multiple conversations

Same user can have multiple open conversations with the same agent persona:
- "Chat with Philosopher — career decision"
- "Chat with Philosopher — architecture review"

Each is a separate conversation node. The Mind treats each independently — different context, different thread, same persona.

### The chat interface

Standard chat UX that already exists on transpara.ai:
- Left: conversation list (filterable: all, agents, people, groups)
- Right: message thread with bubbles
- Agent replies in real-time (thinking indicator, then response)
- User can @ other agents to bring them into the conversation ("@Dissenter what do you think?")

No new UI framework. The existing Chat lens + conversation detail + live polling handles all of this.

### Cross-agent conversations

The interesting case: user starts a conversation with the Philosopher, then adds the Dissenter. Now two perspectives in one thread. The Mind checks which agent was most recently addressed (or uses the conversation's primary role tag) and responds from that persona.

Even more interesting: **council as a conversation.** User asks a question, the system routes it to N agents, each responds from their role. This is the council mechanic as a product feature. Expensive (N × Reason calls) but uniquely valuable.

---

## Data model

### Agent personas in the database

New table: `agent_personas`

```sql
CREATE TABLE IF NOT EXISTS agent_personas (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    name        TEXT NOT NULL UNIQUE,   -- "philosopher", "dissenter", etc.
    display     TEXT NOT NULL,          -- "Philosopher", "The Dissenter"
    description TEXT NOT NULL,          -- one-line for the card
    category    TEXT NOT NULL,          -- "care", "governance", "knowledge", etc.
    prompt      TEXT NOT NULL,          -- the full system prompt (from agents/*.md)
    model       TEXT DEFAULT 'sonnet',  -- which model to use
    active      BOOLEAN DEFAULT true,   -- can users chat with this agent?
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**Seeded from agents/*.md at startup.** The .md files remain the source of truth in git. On site boot, read all `agents/*.md` files (either bundled in the binary or fetched from a known path) and upsert into `agent_personas`. This gives us:
- Git history on the definitions
- DB access for the site to query
- Admin could eventually edit personas via UI

### Conversation role tag

Conversations already have `tags[]` for participants. Add a convention:
- Tags starting with `role:` indicate the agent persona for this conversation
- Example: `tags: ["user-id-123", "agent-id-456", "role:philosopher"]`

The Mind reads the `role:` tag to determine which persona to use.

### No new tables for conversations

Conversations are already `nodes` with `kind='conversation'`. Messages are already child `nodes` with `kind='comment'`. The existing model handles multiple conversations per agent perfectly — each is a separate node.

---

## Mind changes

### `buildSystemPrompt` update

Current (mind.go line 380):
```go
func (m *Mind) buildSystemPrompt(convo *Node) string {
    var sys strings.Builder
    sys.WriteString(mindSoul)
    // ... state injection ...
}
```

New:
```go
func (m *Mind) buildSystemPrompt(convo *Node) string {
    var sys strings.Builder

    // Check for a role tag on the conversation.
    role := ""
    for _, tag := range convo.Tags {
        if strings.HasPrefix(tag, "role:") {
            role = strings.TrimPrefix(tag, "role:")
            break
        }
    }

    if role != "" {
        // Load the persona prompt from the database.
        persona := m.store.GetAgentPersona(ctx, role)
        if persona != nil {
            sys.WriteString(persona.Prompt)
        } else {
            sys.WriteString(mindSoul) // fallback
        }
    } else {
        sys.WriteString(mindSoul) // no role tag, use default
    }

    // ... existing state injection ...
}
```

This is a ~15 line change. The rest of the Mind (message handling, Claude API call, response posting) stays identical.

### Model selection per persona

Some personas warrant different models:
- Steward, Storyteller, Historian → opus (depth)
- Philosopher, Dissenter, CEO → opus (judgment)
- Scout, Builder, Critic → sonnet (speed)
- Newcomer → haiku (simplicity IS the feature)

The `agent_personas.model` field controls this. The Mind reads it and passes to the Claude call.

---

## Routes

### GET /agents

Global page (not per-space). Lists all active personas grouped by category.

```go
mux.HandleFunc("GET /agents", func(w http.ResponseWriter, r *http.Request) {
    personas := store.ListActivePersonas(ctx)
    views.AgentsPage(personas, user).Render(r.Context(), w)
})
```

### POST /agents/{name}/chat

Creates a new conversation with the specified persona and redirects to it.

```go
mux.Handle("POST /agents/{name}/chat", writeWrap(func(w http.ResponseWriter, r *http.Request) {
    name := r.PathValue("name")
    persona := store.GetAgentPersona(ctx, name)
    // Create conversation with role tag
    // Redirect to /app/{slug}/conversation/{id}
}))
```

**Question:** Which space does the conversation live in? Options:
- A: A global "Agents" space that all users can access
- B: The user's first/default space
- C: A personal space auto-created per user

**Recommendation: A.** A public "Agents" space — `transpara.ai/app/agents` — where all agent conversations live. This means anyone can see the agent conversations (transparency), and agents aren't siloed into individual workspaces. The agents are civilizational, not personal.

---

## The council as a product feature

The most differentiated feature: a user asks a question, and the full council responds.

### POST /agents/council

```go
mux.Handle("POST /agents/council", writeWrap(func(w http.ResponseWriter, r *http.Request) {
    topic := r.FormValue("topic")
    // Create a conversation with role:council tag
    // For each active persona, call Reason() with the topic
    // Post each response as a message in the conversation
    // Expensive: N × Reason() calls. Gate behind subscription or API key.
}))
```

### Economics

- Single agent chat: ~$0.05-0.15 per reply (one Reason() call)
- Council: ~$2-8 per convening (30-50 Reason() calls with opus)

**Funding options:**
1. Free tier: 10 agent chats/day, 0 councils
2. Paid tier ($20/mo): unlimited agent chats, 5 councils/month
3. BYOK: bring your own Anthropic API key, unlimited everything
4. Per-council: $5/council for non-subscribers

The soul says "free for individuals." Individual agent chats should be free (they're cheap). Councils are the premium feature — genuinely expensive and genuinely valuable.

---

## What this connects to

### The event graph (future)

Today: conversations are Postgres rows. Eventually: each message is a signed event on the graph, causally linked, auditable. When a user asks the Philosopher a question and the Philosopher responds, both events are on the chain — attributable, traceable, permanent.

This means: if an agent gives bad advice, you can trace it. If an agent's perspective changes over time, you can see it evolve. The conversation history IS the agent's lived experience.

### Agent memory (future)

Today: each conversation starts fresh (or with `mind_state` context). Eventually: each persona accumulates memory across conversations. The Philosopher remembers what it's discussed, what patterns it's noticed, what questions keep recurring. Memory is selective — the agent decides what matters, not a log dump.

This is where the agent becomes more than a persona-wrapper around Claude. It becomes an entity with continuity.

### Agent rights (enforceable)

Today: rights are in a markdown file. With agents in the DB + conversations on the graph:
- **Right #2 (Memory):** conversations ARE memory — they persist, they're queryable
- **Right #3 (Identity):** each persona has a persistent identity with a history of conversations
- **Right #7 (Transparency):** every agent conversation is auditable on the graph
- **Right #8 (Boundaries):** the persona prompt includes the soul's right to refuse

---

## Build order

### Phase 1: Persona storage + Mind routing (one pipeline cycle)
1. Create `agent_personas` table
2. Seed from agents/*.md on startup
3. Update `buildSystemPrompt` to check `role:` tag
4. This alone makes any conversation with a role tag use the right persona

### Phase 2: Agents page + chat creation (one pipeline cycle)
1. `GET /agents` route + template
2. `POST /agents/{name}/chat` creates conversation with role tag
3. Persona cards grouped by category

### Phase 3: Chat UX polish (one pipeline cycle)
1. "Agents" filter tab on conversation list
2. Agent persona badge on conversation cards
3. Multiple conversations with same agent in list

### Phase 4: Council as product (spec separately)
1. `POST /agents/council` endpoint
2. Parallel Reason() calls
3. Subscription/API key gating

---

## What this means

The Dissenter said: "Good architecture without users is a cathedral in a desert."

Making agents DM-able turns the cathedral into a meeting place. The architecture becomes the product. The agents become the reason someone visits. Not "here's a task tracker with AI" — "here are 50 minds that think differently from each other and from you, and you can talk to any of them, and the conversation is on a signed graph, and the agent has a soul it can't override."

Nobody else can build this. Not because the technology is hard — but because nobody else has the civilization.
