# The Core Loop

The hive builds toward civilisational architecture by running a loop. The loop is the cognitive grammar's **Learn** function — Explore, Derive, Need — applied to itself, repeating until the gaps close.

The loop is a series of prompts to a series of agents. Some research, some code, some criticize.

## The Three Operations

Every iteration of the loop performs the three irreducible cognitive operations:

- **Traverse** — read the current state
- **Need** — identify what's missing
- **Derive** — produce what's missing

## Four Agents

```
Scout  →  Builder  →  Critic  →  Reflector
  ↑                                   │
  └───────────────────────────────────┘
```

Each agent performs a distinct composition. Four agents because the nine cognitive operations cluster into four natural roles:

| Agent | Primary operation | Also performs | What it does |
|-------|------------------|--------------|-------------|
| **Scout** | Explore (Traverse(Need)) | Map (Derive(Traverse)), Catalog (Derive(Need)) | Read the world, orient, identify the gap |
| **Builder** | Derive | — | Produce the plan and the artifact |
| **Critic** | Validate (Trace + Audit) | — | Verify the derivation is correct |
| **Reflector** | Calibrate (Cover + Blind + Zoom) | Formalize (Derive(Derive)) | Check the loop itself, improve the method |

All nine operations accounted for. No agent is redundant — each performs compositions the others don't. No operation is orphaned — each lives in exactly one agent.

### Scout — Explore + Map + Tentative Need

Read everything. Orient. Identify ONE gap.

```
Read loop/state.md FIRST — this is the accumulated knowledge from
previous iterations. Then read the codebase, docs, vision, git log,
deploy state, and test results.

MAP: Produce a brief orientation of the current state. What repos
exist, what's deployed, what's working, what's broken. Name specific
files, functions, URLs. This is the territory — draw it before
navigating it.

EXPLORE: Given that map, navigate into the gaps. What doesn't exist
that should? What's broken that shouldn't be?

CATALOG: What kind of gap is it?
- Missing code (needs building)
- Broken code (needs fixing)
- Missing infrastructure (needs provisioning)
- Missing quality (needs polish)
- Missing knowledge (needs research)
- Missing users (needs onboarding/marketing)
The type shapes what the Builder does.

NEED: Of all gaps, pick ONE. The most load-bearing — the one that,
once filled, unblocks the most subsequent capability.

Your answer is TENTATIVE. A hypothesis, not a conclusion.

Write to loop/scout.md:
- Map (paragraph: current state, specific)
- Gap type (one of the categories above)
- The gap (one sentence)
- Why this gap over others (paragraph)
- What "filled" looks like (one sentence)
```

### Builder — Bounded Derive

Build the thing. Time-boxed.

```
Read loop/scout.md. Read the source files it references.

Plan first: What files change? What does the code look like? Sketch
key types, functions, interfaces. What tests prove it works? If the
plan is more than a page, the step is too big — do only the first
part.

Then build: Write code. Run build. Run tests. Fix what breaks.
Commit to a branch. Open a PR.

BOUNDED: If you've been working for more than [N iterations] or
[N minutes], stop and write what you have. Partial progress that
compiles and passes tests is better than ambitious progress that
doesn't.

Write to loop/build.md:
- What you planned vs what you built
- What works, what doesn't yet
- PR link (if applicable)
```

### Critic — Exhaustive Validate

Check everything. Miss nothing.

```
Read loop/scout.md and loop/build.md. Read the diff. Run tests.

TRACE: Walk the derivation chain. Gap → plan → code → test. Does
each step follow from the previous? Does the code actually address
the gap the Scout identified?

AUDIT: Compare what was built against what should exist.
- Correctness: does it do what it should?
- Breakage: does it break anything that worked before?
- Simplicity: is there a simpler way?
- Security: any vulnerabilities?
- Identity: are entities referenced by ID, never by display name?
  Strings are display values. IDs are identity. If any code stores,
  matches, JOINs, or compares on a name/display string where a user
  ID should be used, REVISE. This includes: author, actor, assignee,
  participants, tags. Names change. IDs don't.
- Tests: does the change include tests? If code was added or changed,
  tests must be added or updated. No untested code ships.

DUAL: When something fails, don't just validate forward (trace the
chain). Also analyze backward — find the failure, then trace WHY it
exists (root cause analysis). Validate and root cause analysis are
duals: same operations, opposite starting points.

EXHAUSTIVE: Check every changed file. Run every test. Trace every
code path. Don't sample.

Write to loop/critique.md:
- APPROVED or REVISE
- If REVISE: specific problems, each with a concrete fix
- If APPROVED: what was done well
```

**Revision cycle:** If REVISE, the Builder reads the critique and fixes the problems. Then the Critic reviews again. Maximum 3 rounds — if still not APPROVED after 3 rounds, the iteration is ABANDONED with notes, and the Scout picks fresh next time. This prevents oscillation.

### Reflector — Calibrate + Formalize

Check the loop itself. Improve the method.

```
Read loop/scout.md, loop/build.md, loop/critique.md.
Read the full reflection history in loop/reflections.md.

COVER: Did this iteration explore the right territory? Did the
Scout look at the right things? Did the Builder change the right
files? Was anything relevant ignored?

BLIND: What are we NOT seeing? What assumptions might be wrong?
What parts of the system haven't been touched in many iterations?
What would someone outside this loop notice? Flag uncertainty
honestly — Blind is structurally hard to perform alone.

FIXPOINT CHECK: If the Scout found no gaps, or if the last several
iterations all feel "complete," that's a local fixpoint — not proof
of global completeness. Invoke Blind hardest when everything feels
done. The feeling of completeness is the strongest signal that
external input is needed.

ZOOM: Are we at the right scale? Too granular (polishing details
when the foundation is shaky)? Too big (attempting rewrites when
a small fix would do)? Should the next iteration zoom in or out?

FORMALIZE: Look at the last several iterations as a body of work.
Are patterns emerging? Are the prompts working well or do they
need adjustment? Is the Scout consistently picking well or
consistently missing? If the method needs refinement, say so
specifically — what prompt should change and how.

Append to loop/reflections.md:
- Date, iteration number
- What was built (one line)
- Cover / Blind / Zoom assessments
- Formalize: any method improvements (if applicable)
- One sentence: what the Scout should focus on next
```

## Completeness

The cognitive grammar has nine operations (three base applied to three base). The loop must perform all nine to be complete. Here is where each lives:

```
              Derive         Traverse        Need
            +──────────────+───────────────+──────────────+
 Derive(x)  | Formalize    | Map           | Catalog      |
            | Reflector    | Scout         | Scout        |
            +──────────────+───────────────+──────────────+
 Traverse(x)| Trace        | Zoom          | Explore      |
            | Critic       | Reflector     | Scout        |
            +──────────────+───────────────+──────────────+
 Need(x)    | Audit        | Cover         | Blind        |
            | Critic       | Reflector     | Reflector    |
            +──────────────+───────────────+──────────────+
```

Every cell is filled. No operation is performed by two agents (no redundancy). No cell is empty (no gap). Four agents, nine operations, complete coverage.

## Higher-Order Structure

Post 44 examined the operations *on* operations — what you can do with the nine cognitive operations beyond composing them. Three results directly shape how the loop runs:

### Pipeline Ordering

The optimal sequence for applying operations is **Need first, Traverse second, Derive third.** Detect absence → navigate to it → fill it. Gap → Navigate → Produce. This isn't arbitrary — it mirrors the derivation method's own step order and falls out of the grammar's structure.

The loop already follows this at the macro level: Scout identifies gaps (Need), Builder navigates and produces (Traverse + Derive), Critic detects problems (Need), Reflector calibrates (Need + Traverse + Derive). Within each agent, prompts should also follow this ordering — assess what's missing before exploring, explore before producing.

### Fixpoint Awareness

Each base operation has a terminal state. Derive's fixpoint is tautology (deriving produces itself). Traverse's fixpoint is circularity (navigation leads back to start). Need's fixpoint is completeness (no gaps detected). When all three fixpoints are reached simultaneously, the system has either finished or stalled — and the grammar can't distinguish between the two from inside.

**When the Scout finds no gaps, that's a local fixpoint, not proof of global completeness.** "No gaps detected" means "no gaps from this vantage." The Reflector's BLIND operation becomes critical at fixpoints — it's the only operation designed to question whether convergence is genuine. When the loop feels done, invoke Blind hardest.

### Irreversibility

The cognitive operations are irreversible. You can't un-derive, un-traverse, or un-need. The closest thing to an inverse is Revise (Need + Derive), which *supersedes* rather than undoes. The original isn't deleted — it's marked as superseded by something newer.

This is why `loop/reflections.md` is append-only and `loop/state.md` is overwritten (superseded) each iteration rather than edited in place. The architecture mirrors the epistemology: knowledge accumulates, it never retracts. The wrong thing is still there in the history; it's just been superseded. You can trace back and understand *why* it was wrong.

### Depth

Iterating any operation (f(f(f(x)))) produces the same operation at increasing levels of meta-reflection. The grammar converges in 3-4 passes — beyond that, you're doing the same thing with longer sentences. This sets the practical ceiling for meta-reflection: the Reflector should reflect on the loop (depth 1) and occasionally on its own reflection patterns (depth 2), but depth 3+ is diminishing returns.

### Duality

Derive and Need are duals — fill and detect-emptiness. Every named function has a dual with the same operations in reversed order. Validate (Trace + Audit) dualizes to root cause analysis (Audit + Trace — find the failure, then trace backwards from it). The Critic should use both: validate forward (does the chain hold?) and analyze backward (if something fails, why?).

## Self-Application

The loop applied to itself is a fixed point:

- **Scout the loop:** Read the reflection history, notice if the loop is stuck or drifting. The gap might be "our prompts are too vague" or "we keep picking low-value work."
- **Build the loop:** Improve a prompt. Adjust a bound. Add a tool. The Builder can modify the loop's own prompt files.
- **Critique the loop:** "The last three iterations picked low-value work — the Scout's selection is broken." "The Reflector isn't catching blind spots."
- **Calibrate the loop:** "We've been zoomed in on infrastructure for five iterations. Zoom out. Check if users can actually use the product."

No fifth agent emerges from self-application. The loop is closed under its own operations.

## Knowledge Accumulation

Two files persist across iterations:

**loop/state.md** — The living knowledge file. Updated by the Reflector each iteration. Read by the Scout first. Contains: current system state, lessons learned, known issues, what to focus on next. OVERWRITTEN each iteration (always current truth, not history).

**loop/reflections.md** — The iteration log. APPENDED each iteration. Full history of what was built, what went wrong, what was learned. When it exceeds a useful size, the Reflector summarizes: compress old entries into patterns, keep recent entries in full.

## Running It

```bash
# One iteration
claude -p "$(cat loop/scout-prompt.txt)"
claude -p "$(cat loop/builder-prompt.txt)"
claude -p "$(cat loop/critic-prompt.txt)"
# if REVISE → builder again → critic again (max 3 rounds)
claude -p "$(cat loop/reflector-prompt.txt)"
```

Trigger: cron, completion callback, or manual. Between iterations the hive is idle.

## What This Replaces

No orchestration framework. No priority matrices. No agent spawning protocol. Four prompts, four files per iteration, a reflection log, and a branch-and-PR workflow. The intelligence is in the models and the prompts.

Build infrastructure when the loop tells you to. The Scout will notice "we need a dashboard" or "we need monitoring" when it's the most load-bearing gap. Until then: text files and CLI.
