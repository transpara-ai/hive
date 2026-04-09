# Reboot Survival Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable hive agents to recover intent and mechanical state across reboots using Open Brain for reasoning context and event chain replay as fallback.

**Architecture:** Two-tier recovery. Tier 2 (normal): agents query Open Brain for their last checkpoint thought, warm-start with seeded iteration + intent. Tier 1 (fallback): replay budget/CTO/spawner/reviewer events from chain. Framework-guaranteed boundary captures ensure checkpoints are never missed. Heartbeat events on the chain fill gaps between boundaries.

**Tech Stack:** Go 1.25, eventgraph store API, Open Brain HTTP API, existing loop/runtime infrastructure.

**Spec:** `docs/superpowers/specs/2026-04-09-reboot-survival-design.md`
**Prompts:** `/home/transpara/transpara-ai/repos/docs/designs/reboot-survival-claude-code-prompts-v1.1.0.md`
**Branch:** `feat/checkpoint-recovery`

---

## File Structure

### New files (pkg/checkpoint/)

| File | Responsibility |
|---|---|
| `pkg/checkpoint/thought.go` | LoopSnapshot, BoundaryTrigger, FormatCheckpoint, ParseCheckpoint, FormatHiveSummary |
| `pkg/checkpoint/thought_test.go` | Round-trip format/parse, missing field tolerance, hive summary |
| `pkg/checkpoint/heartbeat.go` | HeartbeatContent, EmitHeartbeat, QueryLatestHeartbeat, event type constant |
| `pkg/checkpoint/heartbeat_test.go` | Emit to in-memory store, query with after filter, nil when empty |
| `pkg/checkpoint/openbrain.go` | ThoughtStore interface, Thought struct, OpenBrainClient, StubThoughtStore |
| `pkg/checkpoint/openbrain_test.go` | Stub round-trip, maxAge filter, empty results |
| `pkg/checkpoint/sink.go` | CheckpointSink interface, DefaultSink, NopSink |
| `pkg/checkpoint/sink_test.go` | Boundary routes to ThoughtStore, heartbeat routes to chain, failure tolerance |
| `pkg/checkpoint/replay.go` | ReplayBudgetFromStore, ReplayCTOFromStore, ReplaySpawnerFromStore, ReplayReviewerFromStore |
| `pkg/checkpoint/replay_test.go` | Per-function: populated store, empty store, multi-event replay |
| `pkg/checkpoint/recover.go` | RecoveryState, RecoveryMode, RecoverAll orchestrator |
| `pkg/checkpoint/recover_test.go` | Warm/cold/mixed modes, ThoughtStore error, staleness, heartbeat merge |

### Modified files

| File | Change |
|---|---|
| `pkg/hive/events.go:10-16` | Add EventTypeAgentHeartbeat constant |
| `pkg/hive/events.go:18-27` | Add to allHiveEventTypes() |
| `pkg/hive/events.go:87-93` | Register HeartbeatContent unmarshaler |
| `pkg/loop/loop.go:54-126` | Add Sink, RecoveryState, HeartbeatInterval to Config |
| `pkg/loop/loop.go:129-160` | Add sink, lastCheckpointIter to Loop struct |
| `pkg/loop/loop.go:164-200` | Seed from RecoveryState in New() |
| `pkg/loop/loop.go:204-393` | Add sink calls after completeTask, signal transitions, heartbeat counter |
| `pkg/loop/loop.go:455-501` | Inject recovery intent into first-iteration prompt |
| `pkg/loop/cto.go:120-127` | Add InitCTOFromRecovery() |
| `pkg/loop/spawner.go:45-51` | Add InitSpawnerFromRecovery() |
| `pkg/loop/review.go:52-58` | Add InitReviewerFromRecovery() |
| `pkg/hive/runtime.go:205-322` | Wire RecoverAll + sinks into boot sequence |
| `pkg/telemetry/schema.go:57-74` | Add reboot_survival column |

---

## Task 1: LoopSnapshot and BoundaryTrigger types

**Files:**
- Create: `pkg/checkpoint/thought.go`
- Test: `pkg/checkpoint/thought_test.go`

- [ ] **Step 1: Create pkg/checkpoint directory and thought.go with types**

```go
// pkg/checkpoint/thought.go
package checkpoint

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LoopSnapshot is a flat, exported view of loop state at a point in time.
// The loop produces these; the checkpoint package consumes them.
// No loop internals leak through this struct.
type LoopSnapshot struct {
	Role          string
	Iteration     int
	MaxIterations int
	TokensUsed    int
	CostUSD       float64
	Signal        string // ACTIVE, IDLE, ESCALATE, HALT
	CurrentTaskID string
	CurrentTask   string // title
	TaskStatus    string // assigned, in-progress, reviewing, blocked
}

// BoundaryTrigger identifies what caused a checkpoint capture.
type BoundaryTrigger string

const (
	TriggerTaskAssigned    BoundaryTrigger = "TaskAssigned"
	TriggerTaskCompleted   BoundaryTrigger = "TaskCompleted"
	TriggerTaskBlocked     BoundaryTrigger = "TaskBlocked"
	TriggerStrategyChange  BoundaryTrigger = "StrategyChange"
	TriggerReviewCompleted BoundaryTrigger = "ReviewCompleted"
	TriggerRoleProposed    BoundaryTrigger = "RoleProposed"
	TriggerRoleDecided     BoundaryTrigger = "RoleDecided"
	TriggerGapEmitted      BoundaryTrigger = "GapEmitted"
	TriggerDirectiveEmitted BoundaryTrigger = "DirectiveEmitted"
	TriggerBudgetAdjusted  BoundaryTrigger = "BudgetAdjusted"
	TriggerHaltSignal      BoundaryTrigger = "HaltSignal"
)
```

- [ ] **Step 2: Add FormatCheckpoint function**

```go
// FormatCheckpoint produces a [CHECKPOINT] thought string from a snapshot.
// The format uses natural language with consistent prefixes for machine parsing.
func FormatCheckpoint(trigger BoundaryTrigger, snap LoopSnapshot, intent, next, context string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[CHECKPOINT] %s agent -- iteration ~%d, %s\n\n",
		snap.Role, snap.Iteration, time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("STATUS: %s\n", snap.Signal))
	sb.WriteString(fmt.Sprintf("BUDGET: %d/%d iterations, %d tokens, $%.2f\n",
		snap.Iteration, snap.MaxIterations, snap.TokensUsed, snap.CostUSD))
	if snap.CurrentTaskID != "" {
		sb.WriteString(fmt.Sprintf("TASK: %s -- %s -- %s\n",
			snap.CurrentTaskID, snap.CurrentTask, snap.TaskStatus))
	}
	if intent != "" {
		sb.WriteString(fmt.Sprintf("INTENT: %s\n", intent))
	}
	if next != "" {
		sb.WriteString(fmt.Sprintf("NEXT: %s\n", next))
	}
	if context != "" {
		sb.WriteString(fmt.Sprintf("CONTEXT: %s\n", context))
	}
	return sb.String()
}
```

- [ ] **Step 3: Add ParsedCheckpoint and ParseCheckpoint**

```go
// ParsedCheckpoint holds fields extracted from a [CHECKPOINT] thought.
type ParsedCheckpoint struct {
	Role           string
	ApproxIteration int
	Timestamp      time.Time
	Status         string
	Budget         string
	Task           string
	Intent         string
	Next           string
	Context        string
}

// ParseCheckpoint extracts structured fields from a [CHECKPOINT] thought.
// Tolerant of missing fields — returns zero values for anything not found.
func ParseCheckpoint(text string) (ParsedCheckpoint, error) {
	var p ParsedCheckpoint
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[CHECKPOINT]") {
			p.Role, p.ApproxIteration, p.Timestamp = parseHeader(line)
		} else if after, ok := strings.CutPrefix(line, "STATUS: "); ok {
			p.Status = after
		} else if after, ok := strings.CutPrefix(line, "BUDGET: "); ok {
			p.Budget = after
		} else if after, ok := strings.CutPrefix(line, "TASK: "); ok {
			p.Task = after
		} else if after, ok := strings.CutPrefix(line, "INTENT: "); ok {
			p.Intent = after
		} else if after, ok := strings.CutPrefix(line, "NEXT: "); ok {
			p.Next = after
		} else if after, ok := strings.CutPrefix(line, "CONTEXT: "); ok {
			p.Context = after
		}
	}
	return p, nil
}

// parseHeader extracts role, iteration, and timestamp from the [CHECKPOINT] line.
func parseHeader(line string) (string, int, time.Time) {
	// [CHECKPOINT] {role} agent -- iteration ~{N}, {timestamp}
	line = strings.TrimPrefix(line, "[CHECKPOINT] ")
	parts := strings.SplitN(line, " agent -- ", 2)
	if len(parts) < 2 {
		return "", 0, time.Time{}
	}
	role := parts[0]

	rest := parts[1] // "iteration ~34, 2026-04-09T14:22:00Z"
	rest = strings.TrimPrefix(rest, "iteration ~")
	iterAndTime := strings.SplitN(rest, ", ", 2)

	var iter int
	if len(iterAndTime) >= 1 {
		iter, _ = strconv.Atoi(iterAndTime[0])
	}

	var ts time.Time
	if len(iterAndTime) >= 2 {
		ts, _ = time.Parse(time.RFC3339, iterAndTime[1])
	}
	return role, iter, ts
}
```

- [ ] **Step 4: Add FormatHiveSummary and supporting types**

```go
// AgentSummary is a minimal view of one agent for the hive summary thought.
type AgentSummary struct {
	Role  string
	State string // active, idle, stopped
}

// TaskStats summarizes task counts for the hive summary thought.
type TaskStats struct {
	Open      int
	Completed int
	Details   string // e.g., "task-77 in-review, task-80 in-progress"
}

// BudgetStats summarizes budget for the hive summary thought.
type BudgetStats struct {
	TotalSpend   float64
	DailyCap     float64
}

// FormatHiveSummary produces a [HIVE SUMMARY] thought string.
func FormatHiveSummary(agents []AgentSummary, tasks TaskStats, budget BudgetStats) string {
	var sb strings.Builder

	dynamicCount := 0 // placeholder — caller can compute if needed
	sb.WriteString(fmt.Sprintf("[HIVE SUMMARY] -- %d agents active, %d dynamic, %s\n\n",
		len(agents), dynamicCount, time.Now().UTC().Format(time.RFC3339)))

	sb.WriteString("AGENTS: ")
	for i, a := range agents {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("%s(%s)", a.Role, a.State))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("TASKS: %d open (%s), %d completed\n",
		tasks.Open, tasks.Details, tasks.Completed))
	sb.WriteString(fmt.Sprintf("BUDGET: $%.2f total spend, $%.2f remaining daily cap\n",
		budget.TotalSpend, budget.DailyCap-budget.TotalSpend))

	return sb.String()
}
```

- [ ] **Step 5: Write thought_test.go**

```go
// pkg/checkpoint/thought_test.go
package checkpoint

import (
	"strings"
	"testing"
	"time"
)

func TestFormatParseCheckpoint_RoundTrip(t *testing.T) {
	snap := LoopSnapshot{
		Role: "implementer", Iteration: 34, MaxIterations: 50,
		TokensUsed: 142000, CostUSD: 0.83, Signal: "ACTIVE",
		CurrentTaskID: "task-77", CurrentTask: "Add error handling", TaskStatus: "in-progress",
	}
	text := FormatCheckpoint(TriggerTaskCompleted, snap, "Working on retries", "Write tests", "2 commits on feat/api")
	parsed, err := ParseCheckpoint(text)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Role != "implementer" {
		t.Errorf("Role = %q, want implementer", parsed.Role)
	}
	if parsed.ApproxIteration != 34 {
		t.Errorf("ApproxIteration = %d, want 34", parsed.ApproxIteration)
	}
	if parsed.Status != "ACTIVE" {
		t.Errorf("Status = %q, want ACTIVE", parsed.Status)
	}
	if parsed.Intent != "Working on retries" {
		t.Errorf("Intent = %q, want 'Working on retries'", parsed.Intent)
	}
	if parsed.Next != "Write tests" {
		t.Errorf("Next = %q, want 'Write tests'", parsed.Next)
	}
	if parsed.Context != "2 commits on feat/api" {
		t.Errorf("Context = %q, want '2 commits on feat/api'", parsed.Context)
	}
	if !strings.Contains(parsed.Task, "task-77") {
		t.Errorf("Task should contain task-77: %q", parsed.Task)
	}
	if parsed.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestParseCheckpoint_MissingFields(t *testing.T) {
	text := "[CHECKPOINT] cto agent -- iteration ~10, 2026-04-09T14:00:00Z\n\nSTATUS: IDLE\n"
	parsed, err := ParseCheckpoint(text)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Role != "cto" {
		t.Errorf("Role = %q, want cto", parsed.Role)
	}
	if parsed.Intent != "" {
		t.Errorf("Intent should be empty, got %q", parsed.Intent)
	}
	if parsed.Context != "" {
		t.Errorf("Context should be empty, got %q", parsed.Context)
	}
	if parsed.Task != "" {
		t.Errorf("Task should be empty, got %q", parsed.Task)
	}
}

func TestParseCheckpoint_ExtraWhitespace(t *testing.T) {
	text := "  [CHECKPOINT] reviewer agent -- iteration ~5, 2026-04-09T12:00:00Z  \n\n  STATUS: ACTIVE  \n  INTENT: Reviewing code  \n\n\n"
	parsed, err := ParseCheckpoint(text)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Role != "reviewer" {
		t.Errorf("Role = %q, want reviewer", parsed.Role)
	}
	if parsed.Status != "ACTIVE" {
		t.Errorf("Status = %q, want ACTIVE", parsed.Status)
	}
}

func TestFormatHiveSummary(t *testing.T) {
	agents := []AgentSummary{
		{Role: "guardian", State: "idle"},
		{Role: "implementer", State: "active"},
	}
	tasks := TaskStats{Open: 2, Completed: 5, Details: "task-77 in-progress"}
	budget := BudgetStats{TotalSpend: 1.50, DailyCap: 10.00}

	text := FormatHiveSummary(agents, tasks, budget)
	if !strings.Contains(text, "[HIVE SUMMARY]") {
		t.Error("missing [HIVE SUMMARY] header")
	}
	if !strings.Contains(text, "guardian(idle)") {
		t.Error("missing guardian agent")
	}
	if !strings.Contains(text, "2 open") {
		t.Error("missing task count")
	}
	if !strings.Contains(text, "$1.50") {
		t.Error("missing spend")
	}
}

func TestParseHeader_Timestamp(t *testing.T) {
	line := "[CHECKPOINT] spawner agent -- iteration ~12, 2026-04-09T10:30:00Z"
	role, iter, ts := parseHeader(line)
	if role != "spawner" {
		t.Errorf("role = %q, want spawner", role)
	}
	if iter != 12 {
		t.Errorf("iter = %d, want 12", iter)
	}
	expected, _ := time.Parse(time.RFC3339, "2026-04-09T10:30:00Z")
	if !ts.Equal(expected) {
		t.Errorf("timestamp = %v, want %v", ts, expected)
	}
}
```

- [ ] **Step 6: Run tests**

Run: `go test ./pkg/checkpoint/... -v -count=1`
Expected: All pass. If compilation fails (missing eventgraph imports), that's expected at this stage — thought.go has no external deps.

- [ ] **Step 7: Commit**

```bash
git add pkg/checkpoint/thought.go pkg/checkpoint/thought_test.go
git commit -m "feat(checkpoint): add thought format/parse types and round-trip tests"
```

---

## Task 2: Heartbeat event type

**Files:**
- Create: `pkg/checkpoint/heartbeat.go`
- Create: `pkg/checkpoint/heartbeat_test.go`
- Modify: `pkg/hive/events.go:10-16,18-27,87-93`

- [ ] **Step 1: Write heartbeat_test.go with failing tests**

```go
// pkg/checkpoint/heartbeat_test.go
package checkpoint

import (
	"testing"
	"time"
)

func TestHeartbeatContent_Fields(t *testing.T) {
	hb := HeartbeatContent{
		Role: "implementer", Iteration: 34, MaxIterations: 50,
		TokensUsed: 142000, CostUSD: 0.83, Signal: "ACTIVE",
		CurrentTaskID: "task-77",
	}
	if hb.Role != "implementer" {
		t.Errorf("Role = %q, want implementer", hb.Role)
	}
	if hb.Iteration != 34 {
		t.Errorf("Iteration = %d, want 34", hb.Iteration)
	}
}

func TestHeartbeatFromSnapshot(t *testing.T) {
	snap := LoopSnapshot{
		Role: "cto", Iteration: 28, MaxIterations: 50,
		TokensUsed: 95000, CostUSD: 0.55, Signal: "ACTIVE",
		CurrentTaskID: "",
	}
	hb := HeartbeatFromSnapshot(snap)
	if hb.Role != "cto" {
		t.Errorf("Role = %q, want cto", hb.Role)
	}
	if hb.Iteration != 28 {
		t.Errorf("Iteration = %d, want 28", hb.Iteration)
	}
	if hb.Signal != "ACTIVE" {
		t.Errorf("Signal = %q, want ACTIVE", hb.Signal)
	}
}

func TestQueryLatestHeartbeat_NoResults(t *testing.T) {
	// QueryLatestHeartbeat with nil store or empty results returns nil, nil
	hb, err := QueryLatestHeartbeat(nil, "implementer", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hb != nil {
		t.Error("expected nil heartbeat for nil store")
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

Run: `go test ./pkg/checkpoint/... -run TestHeartbeat -v`
Expected: FAIL — HeartbeatContent not defined

- [ ] **Step 3: Write heartbeat.go**

```go
// pkg/checkpoint/heartbeat.go
package checkpoint

import (
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// EventTypeAgentHeartbeat is the event type for periodic heartbeat snapshots.
var EventTypeAgentHeartbeat = types.MustEventType("hive.agent.heartbeat")

// HeartbeatContent is the structured payload for heartbeat events on the chain.
type HeartbeatContent struct {
	Role          string  `json:"role"`
	Iteration     int     `json:"iteration"`
	MaxIterations int     `json:"max_iterations"`
	TokensUsed    int     `json:"tokens_used"`
	CostUSD       float64 `json:"cost_usd"`
	Signal        string  `json:"signal"`
	CurrentTaskID string  `json:"current_task_id,omitempty"`
}

// Accept implements event.Content.
func (HeartbeatContent) Accept(event.ContentVisitor) {}

// HeartbeatFromSnapshot converts a LoopSnapshot to a HeartbeatContent.
func HeartbeatFromSnapshot(snap LoopSnapshot) HeartbeatContent {
	return HeartbeatContent{
		Role:          snap.Role,
		Iteration:     snap.Iteration,
		MaxIterations: snap.MaxIterations,
		TokensUsed:    snap.TokensUsed,
		CostUSD:       snap.CostUSD,
		Signal:        snap.Signal,
		CurrentTaskID: snap.CurrentTaskID,
	}
}

// QueryLatestHeartbeat finds the most recent heartbeat for a role after a given time.
// Returns nil, nil if no heartbeat found or store is nil.
func QueryLatestHeartbeat(s store.Store, role string, after time.Time) (*HeartbeatContent, error) {
	if s == nil {
		return nil, nil
	}
	cursor := types.None[types.Cursor]()
	page, err := s.ByType(EventTypeAgentHeartbeat, 100, cursor)
	if err != nil {
		return nil, err
	}
	// ByType returns reverse-chrono. Find the first match for this role after the timestamp.
	for _, ev := range page.Items() {
		if ev.CreatedAt().Before(after) {
			break // past our window, stop
		}
		content, ok := ev.Content().(HeartbeatContent)
		if ok && content.Role == role {
			return &content, nil
		}
	}
	return nil, nil
}
```

Note: The `event.Content` import and `Accept` method need to match the eventgraph pattern. The implementing agent should check how other content types (e.g., `ProgressContent` in `events.go:78-83`) implement the Content interface and follow that exact pattern.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/checkpoint/... -run TestHeartbeat -v`
Expected: All pass

- [ ] **Step 5: Register heartbeat event type in events.go**

Modify `pkg/hive/events.go`. Add to constants (after line 15):

```go
EventTypeAgentHeartbeat = types.MustEventType("hive.agent.heartbeat")
```

Add to `allHiveEventTypes()` (after line 22, before closing brace):

```go
EventTypeAgentHeartbeat,
```

Add to `RegisterEventTypes()` (after line 92):

```go
event.RegisterContentUnmarshaler("hive.agent.heartbeat", event.Unmarshal[checkpoint.HeartbeatContent])
```

Note: This creates an import of `pkg/checkpoint` from `pkg/hive`. If this creates a circular dependency (checkpoint imports hive types), move `EventTypeAgentHeartbeat` to the checkpoint package and have events.go reference it. Write deviation to notes file.

- [ ] **Step 6: Run full build**

Run: `go build ./...`
Expected: Compiles. If circular import, follow the deviation note in step 5.

- [ ] **Step 7: Commit**

```bash
git add pkg/checkpoint/heartbeat.go pkg/checkpoint/heartbeat_test.go pkg/hive/events.go
git commit -m "feat(checkpoint): add heartbeat event type and chain query"
```

---

## Task 3: ThoughtStore interface and Open Brain adapter

**Files:**
- Create: `pkg/checkpoint/openbrain.go`
- Create: `pkg/checkpoint/openbrain_test.go`

- [ ] **Step 1: Write openbrain_test.go**

```go
// pkg/checkpoint/openbrain_test.go
package checkpoint

import (
	"testing"
	"time"
)

func TestStubThoughtStore_CaptureAndSearch(t *testing.T) {
	s := NewStubThoughtStore()
	if err := s.Capture("test thought"); err != nil {
		t.Fatal(err)
	}
	results, err := s.SearchRecent("test", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Content != "test thought" {
		t.Errorf("content = %q, want 'test thought'", results[0].Content)
	}
}

func TestStubThoughtStore_MaxAgeFilter(t *testing.T) {
	s := NewStubThoughtStore()
	// Insert a thought with an old timestamp.
	s.thoughts = append(s.thoughts, Thought{
		Content:    "old thought",
		CapturedAt: time.Now().Add(-3 * time.Hour),
	})
	if err := s.Capture("new thought"); err != nil {
		t.Fatal(err)
	}
	results, err := s.SearchRecent("thought", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (only new)", len(results))
	}
	if results[0].Content != "new thought" {
		t.Errorf("content = %q, want 'new thought'", results[0].Content)
	}
}

func TestStubThoughtStore_EmptyResults(t *testing.T) {
	s := NewStubThoughtStore()
	results, err := s.SearchRecent("nonexistent", 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

Run: `go test ./pkg/checkpoint/... -run TestStub -v`
Expected: FAIL — types not defined

- [ ] **Step 3: Write openbrain.go**

```go
// pkg/checkpoint/openbrain.go
package checkpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ThoughtStore abstracts Open Brain for checkpoint recovery.
// Real implementation wraps the HTTP API. Tests use StubThoughtStore.
type ThoughtStore interface {
	SearchRecent(query string, maxAge time.Duration) ([]Thought, error)
	Capture(content string) error
}

// Thought is a single Open Brain thought with its capture time.
type Thought struct {
	Content    string
	CapturedAt time.Time
}

// OpenBrainClient implements ThoughtStore via Open Brain's HTTP API.
type OpenBrainClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenBrainClient creates a ThoughtStore backed by Open Brain.
func NewOpenBrainClient(baseURL, apiKey string) *OpenBrainClient {
	return &OpenBrainClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Capture stores a thought in Open Brain.
func (c *OpenBrainClient) Capture(content string) error {
	body, _ := json.Marshal(map[string]string{"content": content})
	req, err := http.NewRequest("POST", c.baseURL+"/capture_thought", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("capture: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("capture: status %d", resp.StatusCode)
	}
	return nil
}

// SearchRecent queries Open Brain for thoughts matching a query within maxAge.
func (c *OpenBrainClient) SearchRecent(query string, maxAge time.Duration) ([]Thought, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"query": query,
		"limit": 5,
	})
	req, err := http.NewRequest("POST", c.baseURL+"/search_thoughts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("search: status %d", resp.StatusCode)
	}

	var results []struct {
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("search decode: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	var thoughts []Thought
	for _, r := range results {
		ts, _ := time.Parse(time.RFC3339, r.CreatedAt)
		if ts.After(cutoff) {
			thoughts = append(thoughts, Thought{Content: r.Content, CapturedAt: ts})
		}
	}
	return thoughts, nil
}

// doWithRetry executes an HTTP request, retrying once on transient failure.
func (c *OpenBrainClient) doWithRetry(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Retry once.
		resp, err = c.httpClient.Do(req)
	}
	return resp, err
}

// --- Stub for tests ---

// StubThoughtStore is an in-memory ThoughtStore for testing.
type StubThoughtStore struct {
	thoughts []Thought
}

// NewStubThoughtStore creates an empty stub.
func NewStubThoughtStore() *StubThoughtStore {
	return &StubThoughtStore{}
}

// Capture appends a thought with the current timestamp.
func (s *StubThoughtStore) Capture(content string) error {
	s.thoughts = append(s.thoughts, Thought{
		Content:    content,
		CapturedAt: time.Now(),
	})
	return nil
}

// SearchRecent returns thoughts matching the query within maxAge.
// Uses simple substring matching (not semantic search).
func (s *StubThoughtStore) SearchRecent(query string, maxAge time.Duration) ([]Thought, error) {
	cutoff := time.Now().Add(-maxAge)
	var results []Thought
	for _, t := range s.thoughts {
		if t.CapturedAt.After(cutoff) && strings.Contains(strings.ToLower(t.Content), strings.ToLower(query)) {
			results = append(results, t)
		}
	}
	return results, nil
}
```

Note: The Open Brain HTTP API shape is assumed. The implementing agent MUST recon the actual endpoints (check for MCP server definition or existing integration code). If the API differs, adapt the `OpenBrainClient` methods and write findings to the notes file. The `StubThoughtStore` and `ThoughtStore` interface are stable regardless of API shape.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/checkpoint/... -run TestStub -v`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add pkg/checkpoint/openbrain.go pkg/checkpoint/openbrain_test.go
git commit -m "feat(checkpoint): add ThoughtStore interface and Open Brain adapter"
```

---

## Task 4: CheckpointSink

**Files:**
- Create: `pkg/checkpoint/sink.go`
- Create: `pkg/checkpoint/sink_test.go`

- [ ] **Step 1: Write sink_test.go**

```go
// pkg/checkpoint/sink_test.go
package checkpoint

import (
	"testing"
)

func TestDefaultSink_OnBoundary_CallsCapture(t *testing.T) {
	stub := NewStubThoughtStore()
	sink := NewDefaultSink(stub, nil, "", "implementer")
	snap := LoopSnapshot{
		Role: "implementer", Iteration: 10, MaxIterations: 50,
		Signal: "ACTIVE", CurrentTaskID: "task-1", CurrentTask: "Do thing", TaskStatus: "in-progress",
	}
	sink.OnBoundary(TriggerTaskCompleted, snap)
	results, _ := stub.SearchRecent("checkpoint", 1*60*60*1e9) // 1 hour
	if len(results) == 0 {
		t.Error("OnBoundary should have captured a thought")
	}
}

func TestDefaultSink_OnBoundary_CaptureFailure(t *testing.T) {
	sink := NewDefaultSink(&failingThoughtStore{}, nil, "", "implementer")
	snap := LoopSnapshot{Role: "implementer", Iteration: 10, Signal: "ACTIVE"}
	// Should not panic.
	sink.OnBoundary(TriggerTaskCompleted, snap)
}

func TestNopSink(t *testing.T) {
	var sink NopSink
	snap := LoopSnapshot{Role: "test", Iteration: 1}
	// Should not panic.
	sink.OnBoundary(TriggerTaskCompleted, snap)
	sink.OnHeartbeat(snap)
}

type failingThoughtStore struct{}

func (f *failingThoughtStore) Capture(string) error {
	return fmt.Errorf("connection refused")
}
func (f *failingThoughtStore) SearchRecent(string, time.Duration) ([]Thought, error) {
	return nil, fmt.Errorf("connection refused")
}
```

Note: Add missing imports (`fmt`, `time`) to the test file.

- [ ] **Step 2: Run tests — verify they fail**

Run: `go test ./pkg/checkpoint/... -run TestDefaultSink -v`
Expected: FAIL — CheckpointSink not defined

- [ ] **Step 3: Write sink.go**

```go
// pkg/checkpoint/sink.go
package checkpoint

import (
	"fmt"
	"os"

	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// CheckpointSink receives boundary and heartbeat signals from the loop.
// The loop checks for nil before calling — a nil sink means no checkpointing.
type CheckpointSink interface {
	OnBoundary(trigger BoundaryTrigger, snap LoopSnapshot)
	OnHeartbeat(snap LoopSnapshot)
}

// DefaultSink routes boundaries to Open Brain and heartbeats to the event chain.
type DefaultSink struct {
	thoughts ThoughtStore
	store    store.Store
	actorID  types.ActorID
	role     string
}

// NewDefaultSink creates a sink that writes boundary thoughts to Open Brain
// and heartbeat events to the event chain.
func NewDefaultSink(thoughts ThoughtStore, s store.Store, actorID types.ActorID, role string) *DefaultSink {
	return &DefaultSink{
		thoughts: thoughts,
		store:    s,
		actorID:  actorID,
		role:     role,
	}
}

// OnBoundary formats a checkpoint thought and captures it to Open Brain.
// On failure, logs a warning and continues — never blocks.
func (s *DefaultSink) OnBoundary(trigger BoundaryTrigger, snap LoopSnapshot) {
	thought := FormatCheckpoint(trigger, snap, "", "", "")
	if err := s.thoughts.Capture(thought); err != nil {
		fmt.Fprintf(os.Stderr, "[checkpoint] boundary capture failed for %s: %v\n", s.role, err)
	}
}

// OnHeartbeat emits a heartbeat event to the event chain.
// On failure, logs a warning and continues — never blocks.
func (s *DefaultSink) OnHeartbeat(snap LoopSnapshot) {
	if s.store == nil {
		return
	}
	hb := HeartbeatFromSnapshot(snap)
	// EmitHeartbeat would go here — but it needs event factory and signer
	// from the agent. For now, log. The implementing agent should wire this
	// through the agent's event emission path (see how emitHealthReport works
	// in loop.go for the pattern).
	_ = hb
	fmt.Fprintf(os.Stderr, "[checkpoint] heartbeat: %s iter %d\n", s.role, snap.Iteration)
}

// NopSink is a no-op CheckpointSink for tests and disabled checkpointing.
type NopSink struct{}

func (NopSink) OnBoundary(BoundaryTrigger, LoopSnapshot) {}
func (NopSink) OnHeartbeat(LoopSnapshot)                  {}
```

Note: The `OnHeartbeat` implementation is a placeholder — the actual event emission requires the agent's signer and event factory. The implementing agent should wire this using the same pattern as `emitHealthReport` in `loop.go`. Write the pattern to the notes file.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/checkpoint/... -run "TestDefaultSink|TestNopSink" -v`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add pkg/checkpoint/sink.go pkg/checkpoint/sink_test.go
git commit -m "feat(checkpoint): add CheckpointSink interface with DefaultSink and NopSink"
```

---

## Task 5: Chain replay functions

**Files:**
- Create: `pkg/checkpoint/replay.go`
- Create: `pkg/checkpoint/replay_test.go`

- [ ] **Step 1: Write replay_test.go with budget replay test**

```go
// pkg/checkpoint/replay_test.go
package checkpoint

import (
	"testing"
)

func TestReplayBudgetFromStore_Empty(t *testing.T) {
	// nil store returns empty map, no error
	result, err := ReplayBudgetFromStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestReplayCTOFromStore_Empty(t *testing.T) {
	result, err := ReplayCTOFromStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil state")
	}
	if len(result.GapByCategory) != 0 {
		t.Error("expected empty gap map")
	}
}

func TestReplaySpawnerFromStore_Empty(t *testing.T) {
	result, err := ReplaySpawnerFromStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil state")
	}
}

func TestReplayReviewerFromStore_Empty(t *testing.T) {
	result, err := ReplayReviewerFromStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil state")
	}
}
```

Note: Tests with populated in-memory stores require constructing events with the correct content types from eventgraph. The implementing agent should look at how `knowledge/replay_test.go` or `budget_integration_test.go` construct test events and follow that pattern. Add those tests after verifying the empty-store tests pass.

- [ ] **Step 2: Run tests — verify they fail**

Run: `go test ./pkg/checkpoint/... -run TestReplay -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Write replay.go**

```go
// pkg/checkpoint/replay.go
package checkpoint

import (
	"fmt"
	"sort"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// BudgetState holds recovered budget state for one agent.
type BudgetState struct {
	AgentName       string
	CurrentBudget   int
	AdjustmentCount int
}

// CTORecoveredState holds recovered CTO cooldown state.
type CTORecoveredState struct {
	GapByCategory     map[string]int  // category -> last emission iteration (approx)
	DirectiveByTarget map[string]int  // target -> last emission iteration (approx)
	EmittedGaps       map[string]bool // role -> already emitted
}

// SpawnerRecoveredState holds recovered spawner state.
type SpawnerRecoveredState struct {
	RecentRejections map[string]bool // role name -> was rejected
	ProcessedGaps    map[string]bool // gap ID -> already processed
	PendingProposal  string          // role currently proposed
}

// ReviewerRecoveredState holds recovered reviewer state.
type ReviewerRecoveredState struct {
	ReviewCounts   map[string]int // task ID -> review round count
	CompletedTasks map[string]bool // task ID -> completed
}

// ReplayBudgetFromStore replays agent.budget.adjusted events from the chain.
func ReplayBudgetFromStore(s store.Store) (map[string]BudgetState, error) {
	if s == nil {
		return map[string]BudgetState{}, nil
	}
	events, err := fetchAllByType(s, types.MustEventType("agent.budget.adjusted"))
	if err != nil {
		return nil, fmt.Errorf("replay budget: %w", err)
	}
	sortChronological(events)

	result := make(map[string]BudgetState)
	for _, ev := range events {
		content, ok := ev.Content().(event.AgentBudgetAdjustedContent)
		if !ok {
			continue
		}
		bs := result[content.AgentName]
		bs.AgentName = content.AgentName
		bs.CurrentBudget = content.NewBudget
		bs.AdjustmentCount++
		result[content.AgentName] = bs
	}
	return result, nil
}

// ReplayCTOFromStore replays gap and directive events from the chain.
func ReplayCTOFromStore(s store.Store) (*CTORecoveredState, error) {
	state := &CTORecoveredState{
		GapByCategory:     make(map[string]int),
		DirectiveByTarget: make(map[string]int),
		EmittedGaps:       make(map[string]bool),
	}
	if s == nil {
		return state, nil
	}

	// Replay gap events.
	gaps, err := fetchAllByType(s, types.MustEventType("hive.gap.detected"))
	if err != nil {
		return nil, fmt.Errorf("replay cto gaps: %w", err)
	}
	sortChronological(gaps)
	for _, ev := range gaps {
		content, ok := ev.Content().(event.GapDetectedContent)
		if !ok {
			continue
		}
		state.GapByCategory[content.Category] = len(gaps) // approximate iteration
		state.EmittedGaps[content.MissingRole] = true
	}

	// Replay directive events.
	directives, err := fetchAllByType(s, types.MustEventType("hive.directive.issued"))
	if err != nil {
		return nil, fmt.Errorf("replay cto directives: %w", err)
	}
	sortChronological(directives)
	for _, ev := range directives {
		content, ok := ev.Content().(event.DirectiveIssuedContent)
		if !ok {
			continue
		}
		state.DirectiveByTarget[content.Target] = len(directives) // approximate
	}
	return state, nil
}

// ReplaySpawnerFromStore replays role proposal and rejection events.
func ReplaySpawnerFromStore(s store.Store) (*SpawnerRecoveredState, error) {
	state := &SpawnerRecoveredState{
		RecentRejections: make(map[string]bool),
		ProcessedGaps:    make(map[string]bool),
	}
	if s == nil {
		return state, nil
	}

	// Replay rejections.
	rejected, err := fetchAllByType(s, types.MustEventType("hive.role.rejected"))
	if err != nil {
		return nil, fmt.Errorf("replay spawner rejections: %w", err)
	}
	for _, ev := range rejected {
		content, ok := ev.Content().(event.RoleRejectedContent)
		if !ok {
			continue
		}
		state.RecentRejections[content.Name] = true
	}

	// Check for pending proposal (proposed but not approved or rejected).
	proposed, err := fetchAllByType(s, types.MustEventType("hive.role.proposed"))
	if err != nil {
		return nil, fmt.Errorf("replay spawner proposals: %w", err)
	}
	approved, err := fetchAllByType(s, types.MustEventType("hive.role.approved"))
	if err != nil {
		return nil, fmt.Errorf("replay spawner approvals: %w", err)
	}

	approvedSet := make(map[string]bool)
	for _, ev := range approved {
		content, ok := ev.Content().(event.RoleApprovedContent)
		if !ok {
			continue
		}
		approvedSet[content.Name] = true
	}
	rejectedSet := make(map[string]bool)
	for _, ev := range rejected {
		content, ok := ev.Content().(event.RoleRejectedContent)
		if !ok {
			continue
		}
		rejectedSet[content.Name] = true
	}

	for _, ev := range proposed {
		content, ok := ev.Content().(event.RoleProposedContent)
		if !ok {
			continue
		}
		if !approvedSet[content.Name] && !rejectedSet[content.Name] {
			state.PendingProposal = content.Name
		}
	}
	return state, nil
}

// ReplayReviewerFromStore replays task completion and review events.
func ReplayReviewerFromStore(s store.Store) (*ReviewerRecoveredState, error) {
	state := &ReviewerRecoveredState{
		ReviewCounts:   make(map[string]int),
		CompletedTasks: make(map[string]bool),
	}
	if s == nil {
		return state, nil
	}

	// Replay code review events.
	reviews, err := fetchAllByType(s, types.MustEventType("code.review"))
	if err != nil {
		// code.review may not exist yet — not fatal
		return state, nil
	}
	for _, ev := range reviews {
		content, ok := ev.Content().(event.CodeReviewContent)
		if !ok {
			continue
		}
		state.ReviewCounts[content.TaskID]++
	}

	// Replay task completions.
	completions, err := fetchAllByType(s, types.MustEventType("work.task.completed"))
	if err != nil {
		return state, nil
	}
	for _, ev := range completions {
		content, ok := ev.Content().(work.TaskCompletedContent)
		if !ok {
			continue
		}
		state.CompletedTasks[content.TaskID] = true
	}
	return state, nil
}

// --- Helpers (following knowledge.ReplayFromStore pattern) ---

func fetchAllByType(s store.Store, et types.EventType) ([]event.Event, error) {
	const pageSize = 1000
	var all []event.Event
	cursor := types.None[types.Cursor]()

	for {
		page, err := s.ByType(et, pageSize, cursor)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items()...)
		if !page.HasMore() {
			break
		}
		cursor = page.Cursor()
	}
	return all, nil
}

func sortChronological(events []event.Event) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt().Before(events[j].CreatedAt())
	})
}
```

Note: The exact content type names (`event.AgentBudgetAdjustedContent`, `event.GapDetectedContent`, `event.DirectiveIssuedContent`, `event.RoleProposedContent`, `event.RoleApprovedContent`, `event.RoleRejectedContent`, `event.CodeReviewContent`, `work.TaskCompletedContent`) are imported from the eventgraph and work packages. The implementing agent MUST verify these names by reading the actual type definitions in those packages. If names differ, adapt and note the deviation.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/checkpoint/... -run TestReplay -v`
Expected: All pass (empty-store tests). Add populated-store tests following the integration test patterns in `pkg/loop/budget_integration_test.go`.

- [ ] **Step 5: Commit**

```bash
git add pkg/checkpoint/replay.go pkg/checkpoint/replay_test.go
git commit -m "feat(checkpoint): add chain replay functions for budget, CTO, spawner, reviewer"
```

---

## Task 6: Recovery orchestrator

**Files:**
- Create: `pkg/checkpoint/recover.go`
- Create: `pkg/checkpoint/recover_test.go`

- [ ] **Step 1: Write recover_test.go**

```go
// pkg/checkpoint/recover_test.go
package checkpoint

import (
	"testing"
	"time"
)

func TestRecoverAll_WarmStart(t *testing.T) {
	stub := NewStubThoughtStore()
	thought := FormatCheckpoint(TriggerTaskCompleted, LoopSnapshot{
		Role: "implementer", Iteration: 34, Signal: "ACTIVE",
		CurrentTaskID: "task-77", CurrentTask: "API work", TaskStatus: "in-progress",
	}, "Working on retries", "Write tests", "")
	stub.Capture(thought)

	states, err := RecoverAll([]string{"implementer"}, stub, nil, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s := states["implementer"]
	if s == nil {
		t.Fatal("expected recovery state for implementer")
	}
	if s.Mode != ModeWarm {
		t.Errorf("Mode = %v, want ModeWarm", s.Mode)
	}
	if s.Iteration != 34 {
		t.Errorf("Iteration = %d, want 34", s.Iteration)
	}
	if s.Intent != "Working on retries" {
		t.Errorf("Intent = %q, want 'Working on retries'", s.Intent)
	}
}

func TestRecoverAll_ColdStart_NoThoughts(t *testing.T) {
	stub := NewStubThoughtStore()
	states, err := RecoverAll([]string{"cto"}, stub, nil, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s := states["cto"]
	if s == nil {
		t.Fatal("expected recovery state for cto")
	}
	if s.Mode != ModeCold {
		t.Errorf("Mode = %v, want ModeCold", s.Mode)
	}
}

func TestRecoverAll_Mixed(t *testing.T) {
	stub := NewStubThoughtStore()
	thought := FormatCheckpoint(TriggerTaskCompleted, LoopSnapshot{
		Role: "implementer", Iteration: 20, Signal: "ACTIVE",
	}, "Coding", "", "")
	stub.Capture(thought)

	states, err := RecoverAll([]string{"implementer", "cto"}, stub, nil, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if states["implementer"].Mode != ModeWarm {
		t.Error("implementer should be warm")
	}
	if states["cto"].Mode != ModeCold {
		t.Error("cto should be cold")
	}
}

func TestRecoverAll_ThoughtStoreError(t *testing.T) {
	states, err := RecoverAll([]string{"implementer"}, &failingThoughtStore{}, nil, 2*time.Hour)
	if err != nil {
		t.Fatal("should not return error, should degrade to cold")
	}
	if states["implementer"].Mode != ModeCold {
		t.Error("should cold-start on ThoughtStore error")
	}
}

func TestRecoverAll_StaleThought(t *testing.T) {
	stub := NewStubThoughtStore()
	// Insert old thought.
	stub.thoughts = append(stub.thoughts, Thought{
		Content:    FormatCheckpoint(TriggerTaskCompleted, LoopSnapshot{Role: "implementer", Iteration: 5}, "", "", ""),
		CapturedAt: time.Now().Add(-5 * time.Hour),
	})
	states, err := RecoverAll([]string{"implementer"}, stub, nil, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if states["implementer"].Mode != ModeCold {
		t.Error("should cold-start on stale thought")
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

Run: `go test ./pkg/checkpoint/... -run TestRecoverAll -v`
Expected: FAIL — RecoverAll not defined

- [ ] **Step 3: Write recover.go**

```go
// pkg/checkpoint/recover.go
package checkpoint

import (
	"fmt"
	"os"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/store"
)

// RecoveryMode indicates how an agent was recovered.
type RecoveryMode int

const (
	ModeCold RecoveryMode = iota // chain replay or no recovery
	ModeWarm                      // Open Brain thought found
)

func (m RecoveryMode) String() string {
	if m == ModeWarm {
		return "warm"
	}
	return "cold"
}

// RecoveryState holds the recovered state for one agent.
type RecoveryState struct {
	Role          string
	Mode          RecoveryMode
	Iteration     int
	Intent        string
	HiveSummary   string
	CurrentTaskID string
	BudgetState   *BudgetState
	CTOState      *CTORecoveredState
	SpawnerState  *SpawnerRecoveredState
	ReviewerState *ReviewerRecoveredState
}

// RecoverAll recovers state for all agents. Queries Open Brain first (Tier 2),
// falls back to chain replay (Tier 1) per agent. Tolerant of all failures.
func RecoverAll(agents []string, thoughts ThoughtStore, s store.Store, staleness time.Duration) (map[string]*RecoveryState, error) {
	states := make(map[string]*RecoveryState, len(agents))

	// Initialize all agents as cold-start.
	for _, role := range agents {
		states[role] = &RecoveryState{Role: role, Mode: ModeCold}
	}

	// Try Tier 2: Open Brain thoughts.
	for _, role := range agents {
		state := states[role]
		thought, err := searchForCheckpoint(thoughts, role, staleness)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[recovery] Open Brain search failed for %s: %v\n", role, err)
			continue
		}
		if thought == nil {
			continue
		}

		parsed, parseErr := ParseCheckpoint(thought.Content)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "[recovery] parse failed for %s: %v\n", role, parseErr)
			continue
		}

		state.Mode = ModeWarm
		state.Iteration = parsed.ApproxIteration
		state.Intent = parsed.Intent
		if parsed.Task != "" {
			// Extract task ID from "task-77 -- title -- status" format.
			state.CurrentTaskID = extractTaskID(parsed.Task)
		}

		// Check for newer heartbeat on chain.
		if s != nil {
			hb, hbErr := QueryLatestHeartbeat(s, role, thought.CapturedAt)
			if hbErr == nil && hb != nil && hb.Iteration > state.Iteration {
				state.Iteration = hb.Iteration
			}
		}
	}

	// Tier 1: Chain replay for cold-started agents.
	if s != nil {
		budgets, _ := ReplayBudgetFromStore(s)
		ctoState, _ := ReplayCTOFromStore(s)
		spawnerState, _ := ReplaySpawnerFromStore(s)
		reviewerState, _ := ReplayReviewerFromStore(s)

		for _, role := range agents {
			state := states[role]
			if bs, ok := budgets[role]; ok {
				state.BudgetState = &bs
			}
			if role == "cto" {
				state.CTOState = ctoState
			}
			if role == "spawner" {
				state.SpawnerState = spawnerState
			}
			if role == "reviewer" {
				state.ReviewerState = reviewerState
			}
		}
	}

	// Try hive summary.
	summary, err := searchForHiveSummary(thoughts, staleness)
	if err == nil && summary != nil {
		for _, state := range states {
			state.HiveSummary = summary.Content
		}
	}

	return states, nil
}

func searchForCheckpoint(thoughts ThoughtStore, role string, staleness time.Duration) (*Thought, error) {
	if thoughts == nil {
		return nil, nil
	}
	results, err := thoughts.SearchRecent(fmt.Sprintf("checkpoint %s", role), staleness)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

func searchForHiveSummary(thoughts ThoughtStore, staleness time.Duration) (*Thought, error) {
	if thoughts == nil {
		return nil, nil
	}
	results, err := thoughts.SearchRecent("hive summary", staleness)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

func extractTaskID(taskField string) string {
	// "task-77 -- title -- status" -> "task-77"
	parts := strings.SplitN(taskField, " -- ", 2)
	if len(parts) >= 1 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}
```

Note: Add `"strings"` to imports.

- [ ] **Step 4: Run tests**

Run: `go test ./pkg/checkpoint/... -run TestRecoverAll -v`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add pkg/checkpoint/recover.go pkg/checkpoint/recover_test.go
git commit -m "feat(checkpoint): add recovery orchestrator with warm/cold fallback"
```

---

## Task 7: Loop integration — Config, init, sink calls

**Files:**
- Modify: `pkg/loop/loop.go`

- [ ] **Step 1: Add checkpoint imports and Config fields**

Add to imports (`loop.go:13-31`):

```go
"github.com/lovyou-ai/hive/pkg/checkpoint"
```

Add to Config struct (after `KnowledgeStore` field, around line 122):

```go
// RecoveryState holds recovered state from a prior run. When set and Mode
// is ModeWarm, the loop seeds iteration counter, skips stabilization, and
// injects intent into the first iteration. Nil means first boot.
RecoveryState *checkpoint.RecoveryState

// Sink receives boundary and heartbeat signals for checkpointing.
// Nil means no checkpointing.
Sink checkpoint.CheckpointSink

// HeartbeatInterval is the number of iterations between heartbeat emissions
// when no boundary trigger has fired. Default 10.
HeartbeatInterval int
```

- [ ] **Step 2: Add sink and heartbeat tracking to Loop struct**

Add to Loop struct (after `reviewerState` field, around line 159):

```go
// sink receives checkpoint signals. Nil-safe — callers check before use.
sink checkpoint.CheckpointSink

// lastCheckpointIter tracks the iteration of the last boundary or heartbeat.
lastCheckpointIter int

// heartbeatInterval is iterations between heartbeats. 0 means disabled.
heartbeatInterval int
```

- [ ] **Step 3: Seed from RecoveryState in New()**

In `New()` function (after line 184, before the CTO/spawner/reviewer init block):

```go
if cfg.Sink != nil {
	l.sink = cfg.Sink
}
l.heartbeatInterval = cfg.HeartbeatInterval
if l.heartbeatInterval <= 0 {
	l.heartbeatInterval = 10
}
```

- [ ] **Step 4: Seed iteration from warm recovery in Run()**

Replace the iteration initialization at line 214:

Before:
```go
iteration := 0
```

After:
```go
iteration := 0
if l.config.RecoveryState != nil && l.config.RecoveryState.Mode == checkpoint.ModeWarm {
	iteration = l.config.RecoveryState.Iteration
	fmt.Fprintf(os.Stderr, "[%s] warm-started at iteration %d\n", l.agent.Name(), iteration)
}
```

- [ ] **Step 5: Inject recovery intent into first-iteration prompt**

In `buildPrompt()` (around line 461, after the iteration header and before the Task block):

```go
if iteration == 1 && l.config.RecoveryState != nil && l.config.RecoveryState.Mode == checkpoint.ModeWarm {
	sb.WriteString("## Recovery Context\nYou are resuming after a restart. Your last checkpoint:\n")
	sb.WriteString(l.config.RecoveryState.Intent)
	sb.WriteString("\n\n")
	if l.config.RecoveryState.HiveSummary != "" {
		sb.WriteString("Hive context:\n")
		sb.WriteString(l.config.RecoveryState.HiveSummary)
		sb.WriteString("\n\n")
	}
	sb.WriteString("Resume from where you left off. Do not restart completed work.\n\n")
}
```

Note: For warm-started agents, `iteration` is seeded > 0, so `iteration == 1` won't fire. Use a flag instead:

```go
// In Run(), after iteration seeding:
isFirstIteration := true

// In the loop, after iteration++:
if isFirstIteration {
	isFirstIteration = false
	// ... inject recovery context
}
```

The implementing agent should choose the cleanest approach for the existing code structure.

- [ ] **Step 6: Add framework-guaranteed boundary calls**

After `l.completeTask(task, result.Summary)` at line 272:

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerTaskCompleted, l.currentSnapshot())
}
```

After signal transitions — in `checkResponse()` at the HALT case (line 618) and ESCALATE case (line 621), before the return:

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerHaltSignal, l.currentSnapshot())
}
```

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerTaskBlocked, l.currentSnapshot())
}
```

After auto-assignment succeeds in `autoAssignOpenTask()` (line 815):

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerTaskAssigned, l.currentSnapshot())
}
```

- [ ] **Step 7: Add heartbeat counter**

After the `OnIteration` callback (line 288), add:

```go
// Heartbeat: if no boundary fired recently, emit a heartbeat.
if l.sink != nil && l.heartbeatInterval > 0 {
	if iteration-l.lastCheckpointIter >= l.heartbeatInterval {
		l.sink.OnHeartbeat(l.currentSnapshot())
		l.lastCheckpointIter = iteration
	}
}
```

Reset the counter on boundary calls — in each `sink.OnBoundary` call above, add:

```go
l.lastCheckpointIter = iteration
```

- [ ] **Step 8: Add currentSnapshot() helper**

Add as a method on Loop (after `completeTask`, around line 930):

```go
// currentSnapshot builds a LoopSnapshot from current loop state.
func (l *Loop) currentSnapshot() checkpoint.LoopSnapshot {
	snap := checkpoint.LoopSnapshot{
		Role:          string(l.agent.Role()),
		Iteration:     l.iteration,
		MaxIterations: l.config.Budget.MaxIterations,
		Signal:        "ACTIVE",
	}
	if bs := l.budget.Snapshot(); bs != nil {
		snap.TokensUsed = bs.Tokens
		snap.CostUSD = bs.CostUSD
	}
	if task := l.nextAssignedTask(); task.ID != (types.TaskID{}) {
		snap.CurrentTaskID = task.ID.Value()
		snap.CurrentTask = task.Title
		snap.TaskStatus = "in-progress"
	}
	return snap
}
```

Note: Check the actual `Budget.Snapshot()` return type and field names. The implementing agent should read `pkg/resources/budget.go` for the exact struct.

- [ ] **Step 9: Run tests**

Run: `go test ./pkg/loop/... -v -count=1`
Expected: All existing tests pass. New checkpoint calls should be nil-safe (existing tests don't set Sink).

Run: `go build ./...`
Expected: Compiles.

- [ ] **Step 10: Commit**

```bash
git add pkg/loop/loop.go
git commit -m "feat(checkpoint): integrate recovery state and sink into loop"
```

---

## Task 8: Runtime wiring

**Files:**
- Modify: `pkg/hive/runtime.go`

- [ ] **Step 1: Add checkpoint import**

Add to imports (`runtime.go:8-32`):

```go
"github.com/lovyou-ai/hive/pkg/checkpoint"
```

- [ ] **Step 2: Wire recovery into Run() boot sequence**

After knowledge replay (line 212) and before agent registration (line 244), add:

```go
// --- Checkpoint recovery ---
var thoughtStore checkpoint.ThoughtStore
var recoveryStates map[string]*checkpoint.RecoveryState

openBrainURL := os.Getenv("OPEN_BRAIN_URL")
openBrainKey := os.Getenv("OPEN_BRAIN_KEY")
if openBrainURL != "" {
	thoughtStore = checkpoint.NewOpenBrainClient(openBrainURL, openBrainKey)
}

staleness := 2 * time.Hour
if s := os.Getenv("CHECKPOINT_STALENESS"); s != "" {
	if d, err := time.ParseDuration(s); err == nil {
		staleness = d
	}
}

heartbeatInterval := 10
if s := os.Getenv("CHECKPOINT_HEARTBEAT_INTERVAL"); s != "" {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		heartbeatInterval = n
	}
}

// Collect role names for recovery.
var roleNames []string
for _, def := range r.defs {
	roleNames = append(roleNames, def.Name)
}

recoveryStates, recoverErr := checkpoint.RecoverAll(roleNames, thoughtStore, r.store, staleness)
if recoverErr != nil {
	fmt.Fprintf(os.Stderr, "WARNING: checkpoint recovery: %v\n", recoverErr)
}

// Log recovery summary.
warmCount := 0
for _, rs := range recoveryStates {
	if rs.Mode == checkpoint.ModeWarm {
		warmCount++
	}
}
if len(recoveryStates) > 0 {
	fmt.Fprintf(os.Stderr, "Checkpoint: %d/%d agents warm-started\n", warmCount, len(recoveryStates))
}
```

Add `"strconv"` to imports.

- [ ] **Step 3: Attach RecoveryState and Sink to each loop config**

In the loop config building block (around line 274-305), add these fields:

```go
RecoveryState:     recoveryStates[def.Name],
Sink:              buildSink(thoughtStore, r.store, agent.ID(), def.Name),
HeartbeatInterval: heartbeatInterval,
```

Add the helper function:

```go
func buildSink(thoughts checkpoint.ThoughtStore, s store.Store, actorID types.ActorID, role string) checkpoint.CheckpointSink {
	if thoughts == nil {
		return nil
	}
	return checkpoint.NewDefaultSink(thoughts, s, actorID, role)
}
```

- [ ] **Step 4: Wire hive summary capture triggers**

The spec lists hive summary triggers: agent spawned/stopped, task completed, budget threshold crossed, HALT/ESCALATE from any agent. Subscribe to the bus in runtime.go and capture a hive summary thought when these events arrive:

```go
// After telemetry bus subscription (line 314), add:
if thoughtStore != nil {
	r.graph.Bus().Subscribe(types.MustSubscriptionPattern("hive.agent.*"), func(ev event.Event) {
		summary := checkpoint.FormatHiveSummary(
			r.collectAgentSummaries(),
			r.collectTaskStats(),
			r.collectBudgetStats(),
		)
		if err := thoughtStore.Capture(summary); err != nil {
			fmt.Fprintf(os.Stderr, "[checkpoint] hive summary capture failed: %v\n", err)
		}
	})
}
```

The implementing agent should add `collectAgentSummaries()`, `collectTaskStats()`, and `collectBudgetStats()` helper methods to the Runtime that build the summary structs from the budget registry and task store. These are straightforward queries against existing runtime state. Don't over-subscribe — filter to only the trigger events listed above, not every bus event.

- [ ] **Step 5: Run tests and build**

Run: `go test ./pkg/hive/... ./pkg/loop/... ./pkg/checkpoint/... -v -count=1`
Expected: All pass.

Run: `go build ./...`
Expected: Compiles.

- [ ] **Step 6: Commit**

```bash
git add pkg/hive/runtime.go
git commit -m "feat(checkpoint): wire recovery and sink into runtime boot sequence"
```

---

## Task 9: Per-agent state recovery

**Files:**
- Modify: `pkg/loop/cto.go`
- Modify: `pkg/loop/spawner.go`
- Modify: `pkg/loop/review.go`
- Modify: `pkg/loop/loop.go` (call init functions)

- [ ] **Step 1: Add InitCTOFromRecovery to cto.go**

After `NewCTOCooldowns()` (line 127):

```go
// InitCTOFromRecovery seeds CTO cooldown state from chain replay.
func (c *CTOCooldowns) InitCTOFromRecovery(state *checkpoint.CTORecoveredState) {
	if state == nil {
		return
	}
	for k, v := range state.GapByCategory {
		c.gapByCategory[k] = v
	}
	for k, v := range state.DirectiveByTarget {
		c.directiveByTarget[k] = v
	}
	for k, v := range state.EmittedGaps {
		c.emittedGaps[k] = v
	}
}
```

Add `"github.com/lovyou-ai/hive/pkg/checkpoint"` to cto.go imports.

- [ ] **Step 2: Add InitSpawnerFromRecovery to spawner.go**

After `newSpawnerState()` (line 51):

```go
// InitSpawnerFromRecovery seeds spawner state from chain replay.
func (s *spawnerState) InitSpawnerFromRecovery(state *checkpoint.SpawnerRecoveredState) {
	if state == nil {
		return
	}
	for k, v := range state.RecentRejections {
		s.recentRejections[k] = 0 // iteration unknown, but presence prevents re-proposal
		_ = v
	}
	for k, v := range state.ProcessedGaps {
		s.processedGaps[k] = v
	}
	s.pendingProposal = state.PendingProposal
}
```

Add `"github.com/lovyou-ai/hive/pkg/checkpoint"` to spawner.go imports.

- [ ] **Step 3: Add InitReviewerFromRecovery to review.go**

After `newReviewerState()` (line 58):

```go
// InitReviewerFromRecovery seeds reviewer state from chain replay.
func (s *reviewerState) InitReviewerFromRecovery(state *checkpoint.ReviewerRecoveredState) {
	if state == nil {
		return
	}
	for taskID, count := range state.ReviewCounts {
		s.reviewHistory[taskID] = &taskReviewRecord{
			reviewCount: count,
		}
	}
}
```

Add `"github.com/lovyou-ai/hive/pkg/checkpoint"` to review.go imports.

- [ ] **Step 4: Call init functions from loop New()**

In `New()` (loop.go), after the existing role-specific init blocks (lines 187-198), add:

```go
// Seed recovered state into role-specific structs.
if cfg.RecoveryState != nil {
	if l.ctoCooldowns != nil && cfg.RecoveryState.CTOState != nil {
		l.ctoCooldowns.InitCTOFromRecovery(cfg.RecoveryState.CTOState)
	}
	if l.spawnerState != nil && cfg.RecoveryState.SpawnerState != nil {
		l.spawnerState.InitSpawnerFromRecovery(cfg.RecoveryState.SpawnerState)
	}
	if l.reviewerState != nil && cfg.RecoveryState.ReviewerState != nil {
		l.reviewerState.InitReviewerFromRecovery(cfg.RecoveryState.ReviewerState)
	}
}
```

- [ ] **Step 5: Add role-specific boundary triggers**

In the CTO command processing block (loop.go, after line 314, after `validateAndEmitGap` succeeds):

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerGapEmitted, l.currentSnapshot())
	l.lastCheckpointIter = l.iteration
}
```

After `validateAndEmitDirective` succeeds (after line 319):

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerDirectiveEmitted, l.currentSnapshot())
	l.lastCheckpointIter = l.iteration
}
```

In the Spawner block, after `emitRoleProposed` succeeds (after line 331):

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerRoleProposed, l.currentSnapshot())
	l.lastCheckpointIter = l.iteration
}
```

In the Guardian block, after `emitRoleApproved` succeeds (after line 362):

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerRoleDecided, l.currentSnapshot())
	l.lastCheckpointIter = l.iteration
}
```

In the Reviewer block, after `recordReview` (after line 351):

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerReviewCompleted, l.currentSnapshot())
	l.lastCheckpointIter = l.iteration
}
```

In the Budget block, after `applyBudgetAdjustment` succeeds (after line 304):

```go
if l.sink != nil {
	l.sink.OnBoundary(checkpoint.TriggerBudgetAdjusted, l.currentSnapshot())
	l.lastCheckpointIter = l.iteration
}
```

- [ ] **Step 6: Run tests and build**

Run: `go test ./pkg/loop/... -v -count=1`
Expected: All pass. Existing tests don't set Sink, so boundary calls are nil-safe no-ops.

Run: `go build ./...`
Expected: Compiles.

- [ ] **Step 7: Commit**

```bash
git add pkg/loop/cto.go pkg/loop/spawner.go pkg/loop/review.go pkg/loop/loop.go
git commit -m "feat(checkpoint): add per-agent state recovery and role-specific boundary triggers"
```

---

## Task 10: Telemetry reboot_survival column

**Files:**
- Modify: `pkg/telemetry/schema.go`
- Modify: `pkg/hive/runtime.go`

- [ ] **Step 1: Add column to schema**

In `pkg/telemetry/schema.go`, in the `telemetry_role_definitions` CREATE TABLE statement (around line 72, before `updated_at`):

```sql
reboot_survival TEXT NOT NULL DEFAULT 'none',
```

- [ ] **Step 2: Add migration for existing tables**

In the `EnsureTables` function (or wherever ALTER TABLE statements are handled — check the existing pattern), add:

```sql
ALTER TABLE telemetry_role_definitions ADD COLUMN IF NOT EXISTS reboot_survival TEXT NOT NULL DEFAULT 'none';
```

- [ ] **Step 3: Update reboot_survival after recovery in runtime.go**

After the recovery summary log (the block added in Task 8), add:

```go
// Update telemetry with recovery results.
if r.telemetryWriter != nil {
	for role, rs := range recoveryStates {
		var survival string
		switch rs.Mode {
		case checkpoint.ModeWarm:
			survival = "full"
		default:
			survival = "role-only"
		}
		r.telemetryWriter.UpdateRebootSurvival(role, survival)
	}
}
```

The implementing agent should add `UpdateRebootSurvival(role, survival string)` to the telemetry writer — a simple SQL UPDATE on `telemetry_role_definitions SET reboot_survival = $2 WHERE role = $1`.

- [ ] **Step 4: Run tests and build**

Run: `go test ./pkg/telemetry/... ./pkg/hive/... -v -count=1`
Expected: All pass.

Run: `go build ./...`
Expected: Compiles.

- [ ] **Step 5: Commit**

```bash
git add pkg/telemetry/schema.go pkg/hive/runtime.go
git commit -m "feat(telemetry): add reboot_survival column to role definitions"
```

---

## Task 11: End-to-end validation

**Files:** No new code — verification only.

- [ ] **Step 1: Verify clean build**

Run: `go build ./...`
Expected: Compiles with zero errors.

- [ ] **Step 2: Run full test suite**

Run: `go test ./... -count=1`
Expected: All tests pass.

- [ ] **Step 3: Verify first-boot behavior**

Start hive without `OPEN_BRAIN_URL` set. Confirm:
- All agents cold-start (log shows "Checkpoint: 0/N agents warm-started" or no checkpoint log)
- Hive functions normally — identical to pre-checkpoint behavior

- [ ] **Step 4: Verify with Open Brain**

Set `OPEN_BRAIN_URL` and `OPEN_BRAIN_KEY`. Run hive for 20+ iterations. Verify:
- Boundary thoughts appear in Open Brain (search for "checkpoint")
- Heartbeat events on chain (query for `hive.agent.heartbeat`)
- At least one framework-guaranteed boundary fires

- [ ] **Step 5: Verify warm restart**

Stop and restart hive. Verify:
- Log shows warm-started agents with seeded iteration
- Intent injection visible in first agent output
- Agents skip stabilization (act immediately, don't wait 15 iterations)

- [ ] **Step 6: Verify graceful degradation**

Set `OPEN_BRAIN_URL` to unreachable address. Restart. Verify:
- Warning logged
- All agents cold-start
- Hive functions normally

- [ ] **Step 7: Document results**

Write validation results to `/home/transpara/transpara-ai/repos/docs/designs/reboot-survival-validation-v1.0.0.md`

- [ ] **Step 8: Final commit**

```bash
git add -A
git commit -m "docs: reboot survival validation results"
```
