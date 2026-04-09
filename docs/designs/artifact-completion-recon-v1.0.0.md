# Artifact-Referenced Task Completion — Recon Findings v1.0.0

## Date: 2026-04-09

## Summary

Recon for adding a structured `ArtifactRef` field to `TaskCompletedContent` so completed tasks reference their deliverables (commit hashes, document bodies, URLs).

## Key Finding: Artifact Gate Already Exists

`TaskStore.Complete()` in `lovyou-ai-work/store.go:279-291` already enforces that a `work.task.artifact` or `work.task.artifact.waived` event must exist before a task can be marked complete. The `hasEventForTask()` helper scans for these events but does **not** return the artifact event ID — it only returns a boolean.

This means the infrastructure is 80% built. The remaining work is:
1. Make `Complete()` capture and embed the artifact event ID
2. Wire `AddArtifact()` into the Operate path before `Complete()`
3. Add `ArtifactRef` to the content struct

## Change Surface by Repo

### lovyou-ai-work (2 files)

| File | Line | What | Change needed |
|------|------|------|---------------|
| `events.go` | 78 | `TaskCompletedContent` struct | Add `ArtifactRef types.EventID` field |
| `store.go` | 271 | `Complete()` method | Auto-populate ArtifactRef from gate query |
| `store.go` | 853 | `hasEventForTask()` | Return event ID, not just bool |

### lovyou-ai-eventgraph (0 files)

No changes. `OperateResult` only carries `Summary` and `Usage`. The artifact is a separate event on the graph, not part of the Operate return path.

### lovyou-ai-agent (0 files)

No changes. `Agent.Operate()` is a pass-through for `OperateResult`.

### lovyou-ai-hive (3 files)

| File | Line | What | Change needed |
|------|------|------|---------------|
| `pkg/loop/loop.go` | 248-261 | Operate path | Call `AddArtifact()` before `Complete()` |
| `pkg/loop/loop.go` | 841 | `completeTask()` | Accept optional artifact body/hash |
| `pkg/loop/tasks.go` | 32 | `taskCompletePayload` | Add `artifact_id` field (optional) |
| `pkg/loop/review.go` | 316 | `resolveCommitForTask()` | Use `ArtifactRef` when present (enhancement) |

## Existing Patterns

- `TaskArtifactContent` (`events.go:95`): has `TaskID`, `Body`, `AuthorID` — the artifact body is a free-text string
- `TaskCommentContent` (`events.go:109`): has `TaskID`, `Body`, `AuthorID` — same pattern
- `AddArtifact()` (`store.go`): exists, takes `source, taskID, body, causes, convID`

## What OperateResult Contains

```go
type OperateResult struct {
    Summary string     // plain-text output from Claude CLI
    Usage   TokenUsage
}
```

The Summary is the full agentic output. It may contain commit hashes (Feature B's `extractCommitHash` parses these). It does NOT contain a structured file list or diff.

## Current Operate Path Flow (loop.go:248-261)

```
1. nextAssignedTask() → work.Task
2. l.agent.Operate(ctx, repoPath, instruction) → OperateResult
3. l.completeTask(task, result.Summary) → calls TaskStore.Complete()
```

Step 3 currently has NO artifact attachment. The artifact gate in `Complete()` would reject this unless a waiver exists. This is the gap Feature C fills.

## Design-Changing Findings

1. **Only 2 repos need changes** (work + hive), not 4 as originally assumed
2. **The artifact gate already enforces proof-of-work** — we just need to wire it
3. **`hasEventForTask()` is the integration point** — change it to return the event ID
4. **Non-code agents solved by Feature A** — they use `/task comment` for deliverables, which is already a graph event. The Reviewer can find these via the task ID.
