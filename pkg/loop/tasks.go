package loop

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/work"
)

// taskDependMu serializes the reverse-edge check and the dependency append in
// execTaskDepend across all agent goroutines in this process. Without it, two
// agents racing opposite edges (A→B and B→A) can both pass the check before
// either append lands, recreating the v11-F1 deadlock as a 2-cycle. Agent
// loops in one daemon are the only concurrent /task depend writers today; a
// store-level atomic guard for cross-process writers routes to G-2.x.
var taskDependMu sync.Mutex

// TaskCommand represents a parsed /task command from an agent's response.
type TaskCommand struct {
	Action  string          // "create", "assign", "complete", "comment", "artifact", "depend"
	Payload json.RawMessage // action-specific JSON
}

// Task command payloads.

type taskCreatePayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

type taskAssignPayload struct {
	TaskID   string `json:"task_id"`
	Assignee string `json:"assignee"` // "self" or actor ID
}

type taskCompletePayload struct {
	TaskID  string `json:"task_id"`
	Summary string `json:"summary"`
}

type taskCommentPayload struct {
	TaskID string `json:"task_id"`
	Body   string `json:"body"`
}

type taskArtifactPayload struct {
	TaskID    string `json:"task_id"`
	Label     string `json:"label"`
	MediaType string `json:"media_type"`
	Body      string `json:"body"`
}

type taskDependPayload struct {
	TaskID    string `json:"task_id"`
	DependsOn string `json:"depends_on"`
}

// parseTaskCommands extracts /task commands from an agent's response.
// Returns parsed commands in order. Invalid lines are silently skipped.
func parseTaskCommands(response string) []TaskCommand {
	var commands []TaskCommand
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/task ") {
			continue
		}
		rest := strings.TrimPrefix(trimmed, "/task ")

		// Split into action and JSON payload.
		idx := strings.Index(rest, " ")
		if idx < 0 {
			continue
		}
		action := strings.ToLower(rest[:idx])
		jsonStr := strings.TrimSpace(rest[idx+1:])

		switch action {
		case "create", "assign", "complete", "comment", "artifact", "depend":
			commands = append(commands, TaskCommand{
				Action:  action,
				Payload: json.RawMessage(jsonStr),
			})
		}
	}
	return commands
}

// executeTaskCommands runs parsed task commands against the TaskStore.
// The agentID is used as the source actor for task operations.
// "self" in assignee fields is replaced with agentID.
// Returns the number of commands successfully executed.
func executeTaskCommands(
	commands []TaskCommand,
	tasks *work.TaskStore,
	agentID types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
	canOperate bool,
) int {
	executed := 0
	for _, cmd := range commands {
		var err error
		switch cmd.Action {
		case "create":
			err = execTaskCreate(cmd.Payload, tasks, agentID, causes, convID)
		case "assign":
			err = execTaskAssign(cmd.Payload, tasks, agentID, causes, convID, canOperate)
		case "complete":
			err = execTaskComplete(cmd.Payload, tasks, agentID, causes, convID)
		case "comment":
			err = execTaskComment(cmd.Payload, tasks, agentID, causes, convID)
		case "artifact":
			err = execTaskArtifact(cmd.Payload, tasks, agentID, causes, convID)
		case "depend":
			err = execTaskDepend(cmd.Payload, tasks, agentID, causes, convID)
		}
		if err != nil {
			fmt.Printf("warning: /task %s failed: %v\n", cmd.Action, err)
		} else {
			executed++
		}
	}
	return executed
}

// metaTaskPatterns are phrases that indicate a /task create is actually trying
// to complete or close an existing task — a meta-task anti-pattern caught at
// parse time (Lesson 137, level 2 structural hardening).
var metaTaskPatterns = []string{
	"op=complete",
	"close task",
	"mark done",
	"close the following",
}

// isMetaTaskBody returns true when the combined title+description text matches
// one of the meta-task patterns. The check is case-insensitive.
func isMetaTaskBody(title, description string) bool {
	body := strings.ToLower(title + " " + description)
	for _, pattern := range metaTaskPatterns {
		if strings.Contains(body, pattern) {
			return true
		}
	}
	return false
}

func execTaskCreate(
	payload json.RawMessage,
	tasks *work.TaskStore,
	agentID types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
) error {
	var p taskCreatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	if p.Title == "" {
		return fmt.Errorf("title is required")
	}
	if isMetaTaskBody(p.Title, p.Description) {
		fmt.Printf("warning: rejected meta-task /task create (title: %q) — use /task complete instead\n", p.Title)
		return fmt.Errorf("meta-task rejected: task body describes completing/closing an existing task; use /task complete")
	}
	priority := work.TaskPriority(p.Priority)
	if priority == "" {
		priority = work.DefaultPriority
	}
	task, err := tasks.Create(agentID, p.Title, p.Description, causes, convID, priority)
	if err != nil {
		return err
	}
	fmt.Printf("  → task created: %s — %s\n", task.ID.Value(), p.Title)
	return nil
}

func execTaskAssign(
	payload json.RawMessage,
	tasks *work.TaskStore,
	agentID types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
	canOperate bool,
) error {
	var p taskAssignPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	taskID, err := types.NewEventID(p.TaskID)
	if err != nil {
		return fmt.Errorf("invalid task_id: %w", err)
	}
	assignee := agentID // "self" or empty defaults to self
	if p.Assignee != "" && p.Assignee != "self" {
		assignee, err = types.NewActorID(p.Assignee)
		if err != nil {
			return fmt.Errorf("invalid assignee: %w", err)
		}
	}
	readiness, err := tasks.Readiness(taskID)
	if err != nil {
		return fmt.Errorf("check readiness: %w", err)
	}
	if !readiness.Ready {
		return fmt.Errorf("task is not ready for assignment; missing gates: %s", strings.Join(readiness.MissingGates, ", "))
	}
	// A ready task carries a readiness contract (definition_of_done /
	// acceptance_criteria / test_plan) — it is an implementation task whose
	// deliverable is a committed file. Only a CanOperate agent has filesystem
	// access, so only it can produce and commit that deliverable. A non-Operate
	// agent that takes the task can only "complete" it with a /task comment, which
	// bypasses the commit-verification gate entirely (the round-2 reunification
	// finding: the spawner delivered a catalog as prose, no file, no #131 check).
	// Fail closed: deny the assignment unless the actor can operate.
	if !canOperate {
		return fmt.Errorf("task %s is an implementation task (carries a readiness contract); only a CanOperate agent may take it — a non-Operate agent cannot produce or commit its file deliverable", p.TaskID)
	}
	if err := tasks.Assign(agentID, taskID, assignee, causes, convID); err != nil {
		return err
	}
	fmt.Printf("  → task assigned: %s → %s\n", p.TaskID, assignee.Value())
	return nil
}

func execTaskComplete(
	payload json.RawMessage,
	tasks *work.TaskStore,
	agentID types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
) error {
	var p taskCompletePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	taskID, err := types.NewEventID(p.TaskID)
	if err != nil {
		return fmt.Errorf("invalid task_id: %w", err)
	}
	// An implementation task — one carrying a readiness contract — is completed
	// ONLY through the commit-verified Operate path (handleOperateResult, which
	// attaches the exact Operate commit range and records the completion). A raw
	// /task complete must never complete such a task: from a non-Operate agent it
	// is the comment-only bypass (round-2 finding — the spawner "completed" a write
	// task with prose and no file); from the implementer itself it is a duplicate
	// that overwrites the verified commit range with an unverified ArtifactRef
	// (Codex review on hive#132). The contract signal is ANY required readiness
	// gate present — broader than full readiness, so a partially gated task is
	// caught during the gate-building window too. Fail closed if readiness is
	// unreadable. The FactoryOrder seed and analysis tasks carry no readiness gates
	// (the seed carries fact prerequisites, not gates), so they remain completable.
	readiness, rerr := tasks.Readiness(taskID)
	if rerr != nil {
		return fmt.Errorf("verify readiness before completion: %w", rerr)
	}
	if len(readiness.PresentGates) > 0 {
		return fmt.Errorf("task %s carries an implementation contract (%d readiness gate(s) present); implementation tasks complete only via the commit-verified Operate path, not a raw /task complete", p.TaskID, len(readiness.PresentGates))
	}
	if err := tasks.Complete(agentID, taskID, p.Summary, causes, convID); err != nil {
		return err
	}
	fmt.Printf("  → task completed: %s\n", p.TaskID)
	return nil
}

func execTaskComment(
	payload json.RawMessage,
	tasks *work.TaskStore,
	agentID types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
) error {
	var p taskCommentPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	taskID, err := types.NewEventID(p.TaskID)
	if err != nil {
		return fmt.Errorf("invalid task_id: %w", err)
	}
	if err := tasks.AddComment(taskID, p.Body, agentID, causes, convID); err != nil {
		return err
	}
	fmt.Printf("  → task comment: %s\n", p.TaskID)
	return nil
}

func execTaskArtifact(
	payload json.RawMessage,
	tasks *work.TaskStore,
	agentID types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
) error {
	var p taskArtifactPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	taskID, err := types.NewEventID(p.TaskID)
	if err != nil {
		return fmt.Errorf("invalid task_id: %w", err)
	}
	if err := tasks.AddArtifact(agentID, taskID, p.Label, p.MediaType, p.Body, causes, convID); err != nil {
		return err
	}
	fmt.Printf("  → task artifact: %s — %s\n", p.TaskID, p.Label)
	return nil
}

func execTaskDepend(
	payload json.RawMessage,
	tasks *work.TaskStore,
	agentID types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
) error {
	var p taskDependPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	taskID, err := types.NewEventID(p.TaskID)
	if err != nil {
		return fmt.Errorf("invalid task_id: %w", err)
	}
	dependsOnID, err := types.NewEventID(p.DependsOn)
	if err != nil {
		return fmt.Errorf("invalid depends_on: %w", err)
	}
	// Fail closed on the one cycle shape the contracts can produce between a
	// pair: a reverse edge (depends_on already depends on task_id) would make
	// both tasks aggregates AND both ListOpen-blocked — the v11-F1 deadlock as
	// a 2-cycle. Refuse on read error too: an unverifiable direction is not a
	// permitted one. Transitive cycle detection routes to G-2.x. The mutex
	// makes check+append atomic within this process (see taskDependMu).
	taskDependMu.Lock()
	defer taskDependMu.Unlock()
	existing, err := tasks.GetDependencies(dependsOnID)
	if err != nil {
		return fmt.Errorf("verify dependency direction: %w", err)
	}
	for _, dep := range existing {
		if dep == taskID {
			return fmt.Errorf("dependency refused: %s already depends on %s — the reverse edge would deadlock both tasks (run findings v11-F1); decomposition direction is parent depends_on subtask", p.DependsOn, p.TaskID)
		}
	}
	if err := tasks.AddDependency(agentID, taskID, dependsOnID, causes, convID); err != nil {
		return err
	}
	// Duplicate-sibling dedup (SupersedeDuplicateDirectChildren) was removed
	// here: under parent-depends_on-subtask decomposition (v11-F1) its old
	// pivot resolved to "tasks depending on the subtask" — i.e. PARENTS — and
	// could waive-and-complete a parent as a "duplicate". Sibling dedup under
	// the corrected vocabulary routes to G-2.x; the planner's no-re-decompose
	// guard is the live defense.
	fmt.Printf("  → task dependency: %s depends on %s\n", p.TaskID, p.DependsOn)
	return nil
}
