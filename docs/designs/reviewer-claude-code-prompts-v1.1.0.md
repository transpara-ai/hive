# Reviewer Implementation — Claude Code Task Prompts

**Version:** 1.1.0
**Last Updated:** 2026-04-06
**Status:** Active
**Versioning:** Independent of all other documents.
**Author:** Claude Opus 4.6
**Owner:** Michael Saucier

---

### Revision History

| Version | Date | Description |
|---------|------|-------------|
| 1.0.0 | 2026-04-06 | Initial prompt sequence aligned with design spec v1.1.0 |
| 1.1.0 | 2026-04-06 | Post-knowledge-enrichment: enrichReviewObservation inserts BEFORE enrichKnowledgeObservation. Loop.Config has 17 fields. Institutional Knowledge section in prompt. Cross-repo prompts are self-contained (no design doc reference). Two recon questions pre-answered. |

---

## Usage

Feed these to Claude Code **ONE AT A TIME**, in order. Wait for each to
complete and verify before moving to the next.

**STOP AFTER PROMPT 0.** Return findings. Owner reviews before proceeding.

## Prerequisites

- Design spec `docs/designs/reviewer-design-v1.1.0.md` is committed
  to `lovyou-ai-hive`
- You are in the `lovyou-ai-hive` repo root
- `lovyou-ai-eventgraph` and `lovyou-ai-agent` accessible as siblings

## Cross-Repository Coordination

```
Prompt 1:    lovyou-ai-eventgraph + lovyou-ai-agent  (self-contained)
Prompts 2-7: lovyou-ai-hive
```

After Prompt 1, run `go mod tidy` in lovyou-ai-hive.

---

## Prompt 0 — Reconnaissance

```
Read the Reviewer design spec at docs/designs/reviewer-design-v1.1.0.md,
paying close attention to Section 15 (Open Questions for Recon).

Investigate the codebase to answer ALL of the following:

1. GIT WORKING DIRECTORY
   a. What is l.config.RepoPath? Where is it set? What's its typical value?
   b. Does the implementer use RepoPath for git commands? Search for
      exec.Command("git" in the codebase.
   c. Is there a WorktreeDir or similar config for worktree isolation?

2. CONCURRENT GIT ACCESS
   a. Can enrichReviewObservation safely run "git diff" while the
      implementer runs "git commit"? Are there mutexes?
   b. Does --worktree create isolated git working directories?
   c. Is there any git locking mechanism in the codebase?

3. TASK-TO-COMMIT MAPPING
   a. What fields does work.task.completed event content have?
      Show the struct definition.
   b. Is there a commit hash field? Or any way to link a task to
      specific commits?
   c. If no direct link, what's the fallback? "Most recent commit"?

4. IMPLEMENTER RE-WORK TRIGGER
   a. What are the implementer's WatchPatterns? (Check agentdef.go)
   b. Does the implementer watch code.review.* events?
   c. If not, the implementer won't know about review verdicts until
      we add code.review.* to its WatchPatterns.

5. EXISTING CRITIC ROLE
   a. Read agents/critic.md (if it exists). What does it do?
   b. Is it pipeline-mode only or also in legacy mode?
   c. What's the overlap with the Reviewer role?

6. LARGE DIFF HANDLING
   a. Look at recent implementer commits. What's the typical diff size?
   b. Run: git log --oneline -20 --stat in the hive repo to see
      recent change sizes.

7. TEST RESULT AVAILABILITY
   a. Does the implementer's task completion event include test output?
   b. Is there a test results field in work.task.completed content?
   c. Does the implementer run "go test" as part of its operate cycle?

8. LOOP.CONFIG VERIFICATION
   a. Confirm Loop.Config has 17 fields. List them all.
   b. Identify fields relevant to git/repo access: RepoPath, CanOperate,
      Keepalive, any others.
   c. Confirm KnowledgeStore field exists (from knowledge enrichment work).

9. OBSERVATION PIPELINE VERIFICATION
   a. Show the current enrichment chain in loop.go observe().
   b. Confirm enrichKnowledgeObservation is the LAST call before return.
   c. Identify exact line number where enrichReviewObservation should
      be inserted (BEFORE enrichKnowledgeObservation).

10. EXISTING enrichReviewObservation
    a. Does enrichReviewObservation already exist? (Parallel implementation
       may have created it.)
    b. If yes: read it, report what it does, and whether it matches the
       design spec.
    c. If no: confirm it needs to be created.

STOP AFTER THIS PROMPT. Return findings in structured format.
```

---

## Prompt 1 — Event Types + Emit Methods (eventgraph + agent)

**This prompt is SELF-CONTAINED — no external doc references needed.**

```
This prompt creates the Reviewer's event type and emit method across
two repositories.

TASK 1: Switch to lovyou-ai-eventgraph.

Create the code.review.submitted event type following the EXACT pattern
used by knowledge_event_types.go + knowledge_content.go (the most
recently created event types).

1a. Create go/pkg/event/review_event_types.go:

    var (
        EventTypeCodeReviewSubmitted = types.MustEventType("code.review.submitted")
    )

    func AllReviewEventTypes() []EventType {
        return []EventType{EventTypeCodeReviewSubmitted}
    }

1b. Create go/pkg/event/review_content.go:

    type reviewContent struct{}
    func (reviewContent) Accept(EventContentVisitor) {}

    type CodeReviewContent struct {
        reviewContent
        TaskID     string   `json:"task_id"`
        Verdict    string   `json:"verdict"`
        Summary    string   `json:"summary"`
        Issues     []string `json:"issues"`
        Confidence float64  `json:"confidence"`
    }

    func (c CodeReviewContent) EventTypeName() string {
        return EventTypeCodeReviewSubmitted.Value()
    }

1c. Register in DefaultRegistry() — add AllReviewEventTypes() loop
    alongside the existing AllKnowledgeEventTypes() loop.

1d. Add unmarshaler in content_unmarshal.go:
    "code.review.submitted": unmarshal[CodeReviewContent],

1e. Update the registry count test (currently expects 130, will be 131).

Run all tests in eventgraph.

Commit: "feat: add code review event type for Reviewer agent

- code.review.submitted event type + CodeReviewContent struct
- Registered in DefaultRegistry via AllReviewEventTypes()

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"

TASK 2: Switch to lovyou-ai-agent.

Create a single emit method following the EmitKnowledgeInsight pattern
in knowledge.go:

Create review.go:

    func (a *Agent) EmitCodeReview(content event.CodeReviewContent) error {
        if err := a.checkCanEmit(); err != nil {
            return fmt.Errorf("code review: %w", err)
        }
        _, err := a.recordAndTrack(
            event.EventTypeCodeReviewSubmitted.Value(), content)
        if err != nil {
            return fmt.Errorf("code review: %w", err)
        }
        return nil
    }

Run go mod tidy, go vet, go test.

Commit: "feat: add EmitCodeReview method for Reviewer agent

- Follows EmitKnowledgeInsight pattern
- Emits code.review.submitted events on the chain

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 2 — Review Command Parsing + Validation + State (hive)

```
Switch to lovyou-ai-hive. Run go mod tidy to pick up new eventgraph
types and agent emit methods.

Read the Reviewer design spec at docs/designs/reviewer-design-v1.1.0.md,
sections 4 (/review Command) and 7 (Review Cycle — State Tracking).

Then read pkg/loop/spawner.go for the parseSpawnCommand and spawnerState
patterns — these are the templates.

Create pkg/loop/review.go:

1. ReviewCommand struct:
   TaskID     string   `json:"task_id"`
   Verdict    string   `json:"verdict"`
   Summary    string   `json:"summary"`
   Issues     []string `json:"issues"`
   Confidence float64  `json:"confidence"`

2. parseReviewCommand(response string) *ReviewCommand
   - Scan for line starting with "/review "
   - Extract JSON payload, unmarshal into ReviewCommand
   - Return nil if no command or malformed JSON
   - Follow parseSpawnCommand pattern exactly

3. validateReviewCommand(cmd *ReviewCommand, iteration int) error
   - Stabilization window: iteration < 10 → error
   - task_id non-empty
   - verdict must be one of: "approve", "request_changes", "reject"
   - summary non-empty
   - issues must be non-nil (can be empty for approve)
   - confidence in [0.0, 1.0]
   - If confidence < 0.5 → error "confidence too low, escalate instead"

4. reviewerState struct:
   type reviewerState struct {
       iteration     int
       reviewHistory map[string]*taskReviewRecord
   }
   type taskReviewRecord struct {
       taskID      string
       reviewCount int
       lastVerdict string
       lastIssues  []string
       iterations  []int
   }

   Methods:
   - recordReview(taskID, verdict string, issues []string, iteration int)
   - getReviewCount(taskID string) int
   - shouldEscalate(taskID string) bool  // true if reviewCount >= 3

5. emitCodeReview on Loop:
   func (l *Loop) emitCodeReview(cmd *ReviewCommand) error
   - Construct event.CodeReviewContent from ReviewCommand fields
   - Call l.agent.EmitCodeReview(content)
   - Return error

6. Create pkg/loop/review_test.go:
   - TestParseReviewCommand_Valid
   - TestParseReviewCommand_NoCommand → nil
   - TestParseReviewCommand_MalformedJSON → nil
   - TestParseReviewCommand_MultipleLines
   - TestValidateReviewCommand_Valid
   - TestValidateReviewCommand_InvalidVerdict
   - TestValidateReviewCommand_EmptyTaskID
   - TestValidateReviewCommand_ConfidenceOutOfRange
   - TestValidateReviewCommand_EmptySummary
   - TestValidateReviewCommand_StabilizationWindow
   - TestReviewerState_TrackReview
   - TestReviewerState_ReviewCount
   - TestReviewerState_CycleLimit

Run all tests with -race. Run linter.

Commit: "feat: add /review command parsing, validation, and state tracking

- parseReviewCommand extracts review verdicts from LLM output
- validateReviewCommand enforces stabilization, confidence, field rules
- reviewerState tracks review history for cycle-limit enforcement
- emitCodeReview bridges ReviewCommand to eventgraph CodeReviewContent

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 3 — Prompt File + Site Persona

```
Read docs/designs/reviewer-design-v1.1.0.md sections 6 (Prompt File)
and 9 (Site Persona). Also read agents/cto.md and agents/spawner.md
for format reference.

1. Create agents/reviewer.md with the exact content from design spec
   section 6. Verify format matches other agent prompts (## Identity,
   ## Soul, ## Purpose, etc.). Confirm it includes:
   - The /review command format
   - Review standards (Must-Pass / Should-Pass / Nice-to-Have)
   - The === CODE REVIEW CONTEXT === observation format
   - The ## Institutional Knowledge section
   - The ## Anti-patterns section

2. Create the site persona file. Check where other personas live
   (lovyou-ai-site/graph/personas/ or similar). Use exact content
   from design spec section 9.

3. Read both files back to verify.

Commit: "feat: add reviewer agent prompt and site persona

- agents/reviewer.md: full review workflow with /review command
- Site persona: product category, sonnet model, quality gate identity

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 4 — Observation Enrichment (the novel part)

```
Read docs/designs/reviewer-design-v1.1.0.md section 5 (Observation
Enrichment). Also read the Prompt 0 findings for git access (recon
items 1-3, 6-7).

This is the architecturally novel part — the Reviewer needs to see
actual code diffs, not just event metadata.

CRITICAL ORDERING: The enrichment must be inserted BEFORE
enrichKnowledgeObservation in loop.go. The knowledge enrichment is
universal (all agents) and must remain the LAST enrichment. Review
enrichment is role-specific and goes before it.

Create the enrichment in pkg/loop/review.go (or review_enrich.go):

1. func (l *Loop) enrichReviewObservation(obs string) string
   Guard: if l.agentDef.Role != "reviewer" { return obs }

   a. Find pending reviews: scan l.pendingEvents for work.task.completed
      events that don't have a corresponding code.review.submitted
      (use reviewerState to track what's been reviewed)
   b. If no pending reviews:
      return obs + "\n\n=== CODE REVIEW CONTEXT ===\nNo tasks pending review.\n==="
   c. Pick first pending task
   d. Collect git context using RepoPath from config:
      - git log --oneline -1 (recent commit)
      - git diff HEAD~1 --stat (file summary)
      - git diff HEAD~1 (full diff, truncated)
      All commands use cmd.Dir = l.config.RepoPath
   e. Truncate diff:
      - ≤ 300 lines: include full
      - 301-1000: first 200 + last 50 + "... N lines omitted ..."
      - > 1000: file list only + "Diff too large"
   f. Format as === CODE REVIEW CONTEXT === block
   g. Include previous review info from reviewerState if re-review

2. Wire into loop.go observe():
   FIND the line:
     enriched = l.enrichKnowledgeObservation(enriched)
   INSERT BEFORE IT:
     // Enrich observation with code review context for Reviewer.
     enriched = l.enrichReviewObservation(enriched)

   The result should be:
     enriched = l.enrichSpawnObservation(enriched)
     enriched = l.enrichReviewObservation(enriched)      // ← NEW
     enriched = l.enrichKnowledgeObservation(enriched)    // must stay last
     return enriched, nil

3. Initialize reviewerState on the Loop:
   In loop.go New() or wherever spawnerState is initialized:
   if agentDef.Role == "reviewer" {
       l.reviewerState = &reviewerState{
           reviewHistory: make(map[string]*taskReviewRecord),
       }
   }
   Add reviewerState field to the Loop struct.

DO NOT create tests — Prompt 7 handles that.

Run go vet and existing tests. Nil/guard patterns should keep everything
passing.

Commit: "feat: add code-diff observation enrichment for Reviewer

- enrichReviewObservation collects git diff, commit info, changed files
- Diff truncation: 300-line inline, 1000-line summary, large = file list
- Inserted BEFORE universal knowledge enrichment (ordering preserved)
- reviewerState initialized for review-history tracking

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 5 — Wire into StarterAgents + Loop Integration

```
Read docs/designs/reviewer-design-v1.1.0.md section 3.4 (Role — AgentDef).

Wire the Reviewer into the hive bootstrap and the command processing loop.

1. In pkg/hive/agentdef.go, add Reviewer to StarterAgents() at index 5
   (after spawner, before strategist):

   {
       Name:          "reviewer",
       Role:          "reviewer",
       Model:         ModelSonnet,
       SystemPrompt:  [load from agents/reviewer.md — same mechanism as others],
       WatchPatterns: []string{
           "work.task.completed",
           "work.task.assigned",
           "code.review.*",
           "agent.state.*",
           "hive.directive.*",
       },
       CanOperate:    false,
       MaxIterations: 100,
       MaxDuration:   0,
   }

   Boot order becomes:
   guardian → sysmon → allocator → cto → spawner → reviewer →
   strategist → planner → implementer

2. Wire /review command processing into the loop:
   In loop.go, find where other commands are processed (after health,
   budget, gap/directive, spawn commands). Add:

   if l.reviewerState != nil {
       if cmd := parseReviewCommand(response); cmd != nil {
           if err := validateReviewCommand(cmd, l.iteration); err != nil {
               // log but don't fail
           } else {
               if l.reviewerState.shouldEscalate(cmd.TaskID) {
                   // log: "review cycle limit reached, escalating"
               } else {
                   if err := l.emitCodeReview(cmd); err != nil {
                       // log but don't fail
                   }
                   l.reviewerState.recordReview(
                       cmd.TaskID, cmd.Verdict, cmd.Issues, l.iteration)
               }
           }
       }
   }

3. Update agentdef_test.go:
   - Agent count: 8 → 9
   - "reviewer" in expected roles
   - Boot order assertion updated

Run all tests. Fix anything broken by count/order changes.

Commit: "feat: wire reviewer into hive bootstrap as agent #9

- StarterAgents index 5: after spawner, before strategist
- /review command processing with validation and cycle-limit checks
- Boot order: guardian → sysmon → allocator → cto → spawner → reviewer →
  strategist → planner → implementer

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 6 — Guardian + Implementer Prompt Updates

```
Read docs/designs/reviewer-design-v1.1.0.md section 10 (Integration Points).

1. Update agents/guardian.md:
   Add a ## Reviewer Awareness section (alongside existing SysMon
   Awareness and Knowledge Integrity sections):

   ## Reviewer Awareness

   The Reviewer emits code.review.submitted events when it completes
   a code review. If work.task.completed events are flowing but no
   code.review.submitted events appear for approximately 15 iterations,
   the Reviewer may be stuck or malfunctioning. At 25 iterations of
   silence, escalate to human.

2. Update agents/implementer.md:
   Add a ## Code Review Awareness section:

   ## Code Review Awareness

   Your completed tasks are reviewed by the Reviewer agent. After you
   emit work.task.completed, the Reviewer will analyze your code changes
   and emit a code.review.submitted event with one of three verdicts:

   - **approve** — Your code passed review. No action needed.
   - **request_changes** — Specific issues were found. The issues list
     in the review event tells you exactly what to fix. Address each
     issue and resubmit the task.
   - **reject** — Fundamental problems. The CTO may issue a directive
     for redesign.

   Take review feedback constructively. The Reviewer is your quality
   partner, not your adversary.

3. Add code.review.* to the implementer's WatchPatterns in agentdef.go
   (if not already present — check Prompt 0 findings for implementer's
   current WatchPatterns).

Read each updated file to verify natural flow.

Commit: "feat: add reviewer awareness to guardian and implementer prompts

- Guardian: silence detection for code.review.submitted events
- Implementer: code review awareness + verdict handling guidance
- Implementer WatchPatterns: add code.review.* if missing

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Prompt 7 — Tests

```
Read pkg/loop/review.go and the enrichment code. Create comprehensive
tests covering the Reviewer's framework integration.

Create or extend pkg/loop/review_test.go:

ENRICHMENT TESTS:

1. TestEnrichReviewObservation_HasPendingTask
   - Set up Loop with reviewer role, add a work.task.completed event
     to pendingEvents
   - Call enrichReviewObservation
   - Output contains "=== CODE REVIEW CONTEXT ==="
   - Output contains task metadata

2. TestEnrichReviewObservation_NoPendingTasks
   - Reviewer role, no pending completed tasks
   - Output contains "No tasks pending review"

3. TestEnrichReviewObservation_SkipsNonReviewer
   - Loop with role "implementer"
   - Returns observation unchanged

4. TestDiffTruncation_Small
   - ≤ 300 lines → full diff included

5. TestDiffTruncation_Medium
   - 500 lines → first 200 + last 50 + omission marker

6. TestDiffTruncation_Large
   - 2000 lines → file list only + "too large" message

EVENT EMISSION:

7. TestReviewCommandToEvent
   - Create ReviewCommand, call emitCodeReview
   - Verify code.review.submitted event on in-memory store
   - Content fields match command fields

8. TestReviewEventContent
   - Verify all 5 fields (TaskID, Verdict, Summary, Issues, Confidence)
     round-trip correctly through emit → store → read

9. TestReviewCausalChain
   - Two sequential reviews → second links causally to first

INTEGRATION:

10. TestReviewerBootsInLegacyMode
    - Start minimal hive with reviewer in StarterAgents
    - Verify agent boots (state change event)
    - Verify agent count = 9

11. TestEnrichmentOrdering
    CRITICAL: Verify enrichReviewObservation runs BEFORE
    enrichKnowledgeObservation. Set up a Loop with both reviewer role
    and a KnowledgeStore with insights. Call observe() (or the
    enrichment chain). Verify === CODE REVIEW CONTEXT === appears
    BEFORE === INSTITUTIONAL KNOWLEDGE === in the output.

Run all tests with -race. Everything must pass.

Commit: "test: add reviewer framework and integration tests

- Enrichment: pending tasks, no tasks, non-reviewer guard, diff truncation
- Event emission: command→event, content round-trip, causal chain
- Integration: boot verification, enrichment ordering
- 11 test cases covering Reviewer framework

Co-Authored-By: transpara-ai (transpara-ai@transpara.com)"
```

---

## Post-Implementation Verification

```
Rebuild the hive binary and restart the service:

cd ~/transpara-ai/repos/lovyou-ai-hive
go build -o /home/transpara/bin/hive ./cmd/hive
systemctl --user restart lovyou-hive.service
sleep 15
systemctl --user status lovyou-hive.service
journalctl --user -u lovyou-hive.service --no-pager -n 80

Verify:
1. Reviewer boots as agent #9 (index 5)
2. Boot order: guardian → sysmon → allocator → cto → spawner → reviewer →
   strategist → planner → implementer
3. Reviewer uses Sonnet model
4. Reviewer's observation includes === CODE REVIEW CONTEXT === block
5. Reviewer's observation includes === INSTITUTIONAL KNOWLEDGE === block
   (if knowledge store has relevant insights)
6. Reviewer does NOT emit /review during first 10 iterations (stabilization)
7. After stabilization, if work.task.completed events exist, Reviewer
   analyzes and emits /review verdicts
8. Guardian observes code.review.submitted events
9. Implementer receives code.review.* events in its observations
10. Total agent count is now 9
11. No existing agent behavior has changed

Report back. If everything checks out, Reviewer is graduated.
```
