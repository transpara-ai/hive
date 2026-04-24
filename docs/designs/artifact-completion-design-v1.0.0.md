# Artifact-Referenced Task Completion — Final Design v1.0.0

## Date: 2026-04-09

## Problem

Three problems, one root cause — tasks complete without verifiable deliverables:

1. **No proof of work.** `/task complete` accepts a `Summary` string but no artifact reference. An agent can claim completion without producing anything.
2. **Reviewer race condition.** The Reviewer diffs `HEAD~1` instead of the specific commit that completed the task. Concurrent completions = wrong diff reviewed. **(Fixed in PR #38, merged.)**
3. **Non-code agents have no artifact path.** The Researcher, Strategist, and Planner produce knowledge artifacts but there was no convention for how to attach them. **(Fixed in PR #37, merged — non-code agents use `/task comment` to attach deliverables.)**

What remains: thread a structured `ArtifactRef` through `TaskCompletedContent` so every completion points to its proof-of-work event on the chain.

## Prior Art (Already Merged)

| Feature | PR | What it solved |
|---------|-----|---------------|
| A — Output convention | #37 | Non-code agents attach deliverables via `/task comment` |
| B — Reviewer diff fix | #38 | `resolveCommitForTask()` extracts commit hash from summary |

Feature C builds on both: the artifact event provides a reliable, structured reference that replaces the text-scanning heuristic.

## Architecture

The artifact gate infrastructure already exists in `work`. The `TaskStore.Complete()` method at `store.go:271` enforces that a `work.task.artifact` or `work.task.artifact.waived` event must exist before a task can be marked complete. The gate uses `hasEventForTask()` which returns a boolean — it finds the artifact but throws away the event ID.

Feature C captures that ID and embeds it in the completion event.

## Change Surface

### Repo: work (2 files)

#### 1. `events.go` — Add ArtifactRef to TaskCompletedContent

```go
// BEFORE (line 78):
type TaskCompletedContent struct {
    workContent
    TaskID      types.EventID `json:"TaskID"`
    CompletedBy types.ActorID `json:"CompletedBy"`
    Summary     string        `json:"Summary,omitempty"`
}

// AFTER:
type TaskCompletedContent struct {
    workContent
    TaskID      types.EventID `json:"TaskID"`
    CompletedBy types.ActorID `json:"CompletedBy"`
    Summary     string        `json:"Summary,omitempty"`
    ArtifactRef types.EventID `json:"ArtifactRef,omitempty"`
}
```

`ArtifactRef` points to the `work.task.artifact` or `work.task.artifact.waived` event that satisfied the gate. Optional (`omitempty`) for backward compatibility — existing events on the chain without it deserialize fine.

#### 2. `store.go` — findEventForTask() and Complete() auto-population

**Replace `hasEventForTask()` with `findEventForTask()`:**

```go
// BEFORE (line 853):
func (ts *TaskStore) hasEventForTask(eventType types.EventType, taskID types.EventID) (bool, error) {
    page, err := ts.store.ByType(eventType, 1000, types.None[types.Cursor]())
    if err != nil { return false, ... }
    for _, ev := range page.Items() {
        switch c := ev.Content().(type) {
        case TaskArtifactContent:
            if c.TaskID == taskID { return true, nil }
        case TaskArtifactWaivedContent:
            if c.TaskID == taskID { return true, nil }
        }
    }
    return false, nil
}

// AFTER:
func (ts *TaskStore) findEventForTask(eventType types.EventType, taskID types.EventID) (types.EventID, bool, error) {
    page, err := ts.store.ByType(eventType, 1000, types.None[types.Cursor]())
    if err != nil { return types.EventID{}, false, ... }
    for _, ev := range page.Items() {
        switch c := ev.Content().(type) {
        case TaskArtifactContent:
            if c.TaskID == taskID { return ev.ID(), true, nil }
        case TaskArtifactWaivedContent:
            if c.TaskID == taskID { return ev.ID(), true, nil }
        }
    }
    return types.EventID{}, false, nil
}
```

**Update `Complete()` to auto-populate ArtifactRef:**

```go
func (ts *TaskStore) Complete(
    source types.ActorID,
    taskID types.EventID,
    summary string,
    causes []types.EventID,
    convID types.ConversationID,
) error {
    // --- Artifact gate (returns event ID now) ---
    artifactRef, hasArtifact, err := ts.findEventForTask(EventTypeTaskArtifact, taskID)
    if err != nil { return ... }
    if !hasArtifact {
        waiverRef, hasWaiver, err := ts.findEventForTask(EventTypeTaskArtifactWaived, taskID)
        if err != nil { return ... }
        if !hasWaiver {
            return ErrArtifactRequired
        }
        artifactRef = waiverRef
    }

    content := TaskCompletedContent{
        TaskID:      taskID,
        CompletedBy: source,
        Summary:     summary,
        ArtifactRef: artifactRef,  // <-- auto-populated from gate
    }
    // ... create and append event (unchanged)
}
```

**Key design decision:** The `Complete()` signature does NOT change. No new parameters. `ArtifactRef` is auto-populated from the same query the gate already performs. Every caller benefits without modification.

**Update `HasWaiver()` and any callers of `hasEventForTask`** to use the new 3-return signature.

### Repo: hive (2 files)

#### 3. `pkg/loop/loop.go` — Wire AddArtifact() in the Operate path

The Operate path (lines 248-261) currently calls `completeTask()` without attaching an artifact. The artifact gate in `Complete()` would reject this.

**Add artifact attachment between Operate and Complete:**

```go
// After Operate returns successfully (line 256):
result, opErr := l.agent.Operate(ctx, l.config.RepoPath, instruction)
if opErr != nil { ... }

// NEW: attach artifact before completing (satisfies gate).
l.attachOperateArtifact(task, result.Summary)

// Auto-complete the task.
l.completeTask(task, result.Summary)
```

**New helper `attachOperateArtifact()`:**

```go
func (l *Loop) attachOperateArtifact(task work.Task, summary string) {
    if l.config.TaskStore == nil {
        return
    }
    var causes []types.EventID
    if lastEv := l.agent.LastEvent(); !lastEv.IsZero() {
        causes = []types.EventID{lastEv}
    }

    // Build artifact body: commit hash + changed files.
    body := buildOperateArtifactBody(l.config.RepoPath)

    err := l.config.TaskStore.AddArtifact(
        l.agent.ID(), task.ID,
        "Operate result",   // label
        "text/plain",       // mediaType
        body,               // body
        causes, l.config.ConvID,
    )
    if err != nil {
        fmt.Printf("[%s] warning: attach artifact failed: %v\n", l.agent.Name(), err)
    }
}

// buildOperateArtifactBody captures the current git state as artifact content.
func buildOperateArtifactBody(repoPath string) string {
    if repoPath == "" {
        return "(no repo path configured)"
    }
    hash := gitCommand(repoPath, "log", "-1", "--format=%H")
    stat := gitCommand(repoPath, "diff", "HEAD~1", "--stat")
    if hash == "" {
        return "(no commits)"
    }
    return fmt.Sprintf("commit: %s\n\n%s", hash, stat)
}
```

Note: `gitCommand` is already defined in `review.go` in the same package. `buildOperateArtifactBody` uses `HEAD~1` here, which is correct — it runs immediately after the Operate call, so HEAD is the agent's commit. This is different from the Reviewer's diff (which runs later, after multiple agents may have committed).

#### 4. `pkg/loop/review.go` — Use ArtifactRef when present (enhancement)

Add Strategy 0 to `resolveCommitForTask()`:

```go
func (l *Loop) resolveCommitForTask(task work.TaskCompletedContent, taskFound bool) (string, string) {
    repo := l.config.RepoPath

    // Strategy 0: use ArtifactRef → fetch artifact body → extract commit hash.
    if taskFound && !task.ArtifactRef.IsZero() {
        if body := l.fetchArtifactBody(task.ArtifactRef); body != "" {
            if hash := extractCommitHash(body, repo); hash != "" {
                return hash, hash + "^.." + hash
            }
        }
    }

    // Strategy 1: extract hash from summary text (existing).
    ...
}
```

`fetchArtifactBody()` reads the artifact event from the store by ID and returns its `Body` field. This is the most reliable path — the artifact body is structured (`"commit: abc123\n\nfile.go | 5 ++"`) and was captured at Operate time.

### Repos NOT Changed

- **eventgraph**: `OperateResult` stays `{Summary, Usage}`. The artifact is a separate graph event, not part of the Operate return path.
- **agent**: `Agent.Operate()` is a pass-through. No changes.

## Backward Compatibility

- `ArtifactRef` is `omitempty` — existing `TaskCompletedContent` events without it deserialize without error
- `Complete()` signature is unchanged — all callers compile without modification
- The Reviewer's Strategy 1 (hash from summary) remains as a fallback when `ArtifactRef` is empty
- The artifact gate behavior is preserved — `findEventForTask` returns the same truth value as `hasEventForTask`, just with the ID attached

## Implementation Sequence

| Step | Repo | What | Depends on |
|------|------|------|-----------|
| 1 | work | `findEventForTask()` + `ArtifactRef` field + auto-populate in `Complete()` | Nothing |
| 2 | hive | `attachOperateArtifact()` + `buildOperateArtifactBody()` + wire in Operate path | Step 1 merged |
| 3 | hive | Reviewer Strategy 0 using `ArtifactRef` | Step 2 merged |

Steps 1 and 2 can be developed in parallel. Step 2 needs Step 1 merged before final testing (go.sum will pick up the new field).

## Test Plan

### Step 1 (work):
- Extend `store_artifact_test.go`: after `AddArtifact()` + `Complete()`, verify `ArtifactRef` on the completed event matches the artifact event ID
- Extend `store_artifact_test.go`: after `WaiveArtifact()` + `Complete()`, verify `ArtifactRef` matches the waiver event ID
- Verify backward compat: `Complete()` with a pre-existing artifact (from old code without ArtifactRef) still works
- Verify `HasWaiver()` still returns correct boolean after signature change

### Step 2 (hive):
- New test: mock Operate path → verify `work.task.artifact` event appears on chain before `work.task.completed`
- New test: verify artifact body contains commit hash and file stat
- Integration: run hive with implementer, verify completed tasks have non-zero `ArtifactRef`

### Step 3 (hive):
- New test: `resolveCommitForTask()` with `ArtifactRef` set → verify Strategy 0 fires and returns correct hash
- Verify fallback: `ArtifactRef` empty → Strategy 1 still works

## Open Questions (None)

All design questions resolved by recon:
- Artifact gate exists → no new gate logic needed
- `Complete()` signature unchanged → no caller updates
- `OperateResult` unchanged → no cross-repo coupling
- Non-code agents use `/task comment` → covered by Feature A
