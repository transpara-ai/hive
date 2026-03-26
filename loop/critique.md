# Critique

Commit: d61a6ec6a5ff217ce1f2181ead270b35aba54a34
Verdict: REVISE

## Analysis

**What the commit delivers:**
- `FixTasker` interface + `fixTasker` field on `PipelineTree`
- `diagnosticCount()` to detect silent failures (phase writes diagnostic but returns nil)
- `Execute` snapshots count before/after each phase, calls `callFixTasker` on both failure modes
- Three tests covering: diagnostic written on direct error, fixTasker called on diagnostic-without-error, fixTasker called on direct error

**The critical gap:**

`NewPipelineTree` never wires `fixTasker`:

```go
func NewPipelineTree(r *Runner) *PipelineTree {
    return &PipelineTree{
        cfg:    r.cfg,
        phases: []Phase{...},
        // fixTasker is nil — never set
    }
}
```

And the `FixTasker` interface (`CreateTask(ctx context.Context, title string) error`) is **incompatible** with `api.Client.CreateTask` (`func (c *Client) CreateTask(slug, title, description, priority string) (*Node, error)`). There is no adapter. There is no way for `NewPipelineTree` to satisfy the interface using the runner's actual client.

The result: in production (`runTick` → `NewPipelineTree(r).Execute(ctx)`), `callFixTasker` hits `if pt.fixTasker == nil { return }` and silently does nothing. Fix-task creation never executes.

The tests sidestep this by constructing `&PipelineTree{..., fixTasker: stub}` directly — they prove the mechanism in isolation but don't test the production path. This is the same pattern Lesson 22 warns against: *"'Works correctly' and 'works as intended' are different checks."*

**Secondary issue:** `state.md` now has the `## What the Scout Should Focus On Next` heading twice — a formatting artifact from the diff that leaves it duplicated.

---

VERDICT: REVISE

**Required fix:** Wire the production path. Either:
1. Add a thin adapter in `NewPipelineTree` that wraps `r.cfg.APIClient` to satisfy `FixTasker` — e.g. `fixTaskerAdapter{client: r.cfg.APIClient, slug: r.cfg.SpaceSlug}` that calls `CreateTask(slug, title, "", "high")`, or
2. Change `FixTasker.CreateTask` to match the actual client signature and pass `spaceSlug` through it.

Add one integration-path test: construct via `NewPipelineTree`, inject a failing phase, verify `APIClient.CreateTask` is called (using the existing mock infrastructure). Without that, invariant 12 (VERIFIED) is not satisfied — the production path has no test.
