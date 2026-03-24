# Scout Report — Iteration 220

## Gap: Agent runtime is cold-start, no persistent sessions, no event triggering

**Current:** cmd/loop cold-starts every agent with 15K+ tokens of context per phase. No --resume. No event system for @mention triggering. Agents don't know where they run.

**What's needed:**
1. Persistent sessions per agent (--resume with named sessions)
2. First run: full context injection (CONTEXT + METHOD + agent prompt). Subsequent runs: just the new task.
3. Event-driven triggering for background agents
4. Clear runtime model (local dev vs Fly.io production)
