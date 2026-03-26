Now I'll write the Scout report. Based on my analysis of the hive repo state, I've identified the critical gap.

---

## SCOUT REPORT — Iteration 275

**Gap:** The autonomous pipeline (Scout → Builder → Critic → Reflector) is proven but requires manual triggering. The hive cannot run self-directed without Matt invoking `next` or `--pipeline` — a single point of failure incompatible with the "Lovatts engagement" and "company in a box" vision.

**Evidence:**

1. **Recent commits show pipeline closure (iters 224-233):**
   - Autonomous task creation (Scout) ✓
   - Autonomous implementation (Builder via Operate) ✓
   - Autonomous code review (Critic) ✓
   - Autonomous reflection (Reflector) ✓
   - Autonomous direction (PM writes directives) ✓
   
   All pieces work. The loop is closed. **But there is no trigger.**

2. **Current invocation model (cmd/hive/main.go):**
   The Runner is invoked as a one-shot CLI tool. To run a full pipeline cycle:
   ```
   go run ./cmd/hive --role pm --pipeline
   ```
   This requires a human to run the command. No scheduling. No daemon. No continuous operation.

3. **Backlog marks "Bus factor" as URGENT (lines 201-213):**
   > "Currently: Matt types `next` to trigger each cycle. If Matt can't work for a week, the hive stops."
   
   The backlog explicitly requires:
   - `--daemon` flag for continuous operation
   - Runs on a VM (not Matt's laptop)
   - Automatic deploy after Critic PASS
   - Budget ceiling per day
   - Error alerting

4. **Product vision blocked:**
   - **Lovatts engagement:** A client won't accept a service that stops when the director takes a vacation
   - **Company in a box:** Advertised as autonomous; currently semi-autonomous (works if triggered)
   - **Dogfooding:** The hive doesn't use its own infrastructure for continuous operation (no automated tasks, no scheduled runs, no fault tolerance)

5. **State.md shows iteration 275 but no daemon mode.**
   Recent commits (March 26) include pipeline validation and multi-repo support. All focus on the LOGIC of autonomy, not its OPERATION. The loop works; the system doesn't.

**Impact:**

This blocks three revenue paths and the company's founding credibility claim:
- **Lovatts:** "The hive will autonomously modernize our legacy portfolio" — requires unattended operation
- **SaaS:** "Hive as a service" — requires per-client daemon instances
- **Vision:** "Civilization that needs no human direction" — currently needs human to start each iteration

**Scope:**

Changes needed in the hive repo:
- `cmd/hive/main.go` — add `--daemon` flag, scheduler loop (every 30min or on-demand)
- `pkg/runner/runner.go` — RunDaemon() method; graceful shutdown; error recovery
- `pkg/runner/pm.go`, `scout.go`, etc. — run in daemon context with budget ceilings
- `pkg/api/` — error handling for API unavailability (retry backoff)
- Infrastructure — Fly machine definition or systemd service file to run the daemon
- Monitoring — log errors, budget tracking, heartbeat (Guardian role extended)

**Suggestion:**

Implement daemon mode in three phases:

1. **Phase 1 (Tier 1):** `--daemon` flag + basic scheduler. Every 30 minutes, run the pipeline (PM → Scout → Builder if tasks exist → Critic → Reflector). Budget ceiling ($20/day). Log all activity. No external alerting yet.

2. **Phase 2 (Tier 2):** Deploy to Fly machine. Verify it runs unattended for 48 hours without intervention. Catch and report errors via Guardian role (email or lovyou.ai notification).

3. **Phase 3 (Tier 3):** Automatic deploy gate — Critic PASS → auto-deploy to production. Hive self-directs deployments.

This unlocks the product vision: a hive that builds while you sleep, costs under $20/day, and survives infrastructure hiccups.

---

**Priority: CRITICAL** — without this, the pipeline is a proof-of-concept, not a product.