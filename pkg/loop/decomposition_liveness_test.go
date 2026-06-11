package loop

import (
	"strings"
	"sync"
	"testing"
	"time"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// newDecomposedGraph builds the slice-1 factory shape on a fresh loop fixture:
// an order task and an implementation subtask, both carrying the three required
// readiness gates, no dependency edge yet. Callers add the edge under test.
func newDecomposedGraph(t *testing.T) (*Loop, *work.TaskStore, *hiveagent.Agent, types.ConversationID, []types.EventID, work.Task, work.Task) {
	t.Helper()
	l, ts, agent, convID := newRecheckLoop(t, true, 10*time.Millisecond)
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	order, err := ts.Create(agent.ID(), "fo_roles_catalog", "the order task", causes, convID)
	if err != nil {
		t.Fatalf("Create order: %v", err)
	}
	sub, err := ts.Create(agent.ID(), "implement fo_roles_catalog document", "the implementation subtask", causes, convID)
	if err != nil {
		t.Fatalf("Create subtask: %v", err)
	}
	for _, label := range work.RequiredReadinessGateLabels() {
		if err := ts.AddArtifact(agent.ID(), order.ID, label, "text/markdown", "gate body", causes, convID); err != nil {
			t.Fatalf("AddArtifact order %s: %v", label, err)
		}
		if err := ts.AddArtifact(agent.ID(), sub.ID, label, "text/markdown", "gate body", causes, convID); err != nil {
			t.Fatalf("AddArtifact sub %s: %v", label, err)
		}
	}
	return l, ts, agent, convID, causes, order, sub
}

// TestFirstAssignableOpenTask_DecomposedGraphLiveness locks in the v11-F1 fix:
// run findings v11-F1 found decomposed task graphs permanently unassignable by
// the auto-assign path, because ListOpen reads "X depends_on Y" as a
// prerequisite (X hidden until Y completes) while the old childless-leaf skip
// read the same edge as decomposition (Y skipped for having dependent X) —
// zero assignable tasks in either edge direction, neutralizing the
// hive#135/#148 re-check exactly when a dropped wake edge needed it.
//
// The fixed predicate skips AGGREGATES (tasks that declare dependencies)
// instead of tasks with dependents. Over the full edge-shape input domain the
// planner can produce, at least one task stays auto-assignable, and an
// aggregate is never auto-assigned — including after its pieces complete (an
// unblocked order's remaining work is its governed completion path, never
// another Operate).
func TestFirstAssignableOpenTask_DecomposedGraphLiveness(t *testing.T) {
	t.Run("no_edge_both_assignable_oldest_first", func(t *testing.T) {
		l, _, _, _, _, order, _ := newDecomposedGraph(t)
		got, ok := l.firstAssignableOpenTask()
		if !ok {
			t.Fatal("no-edge graph: firstAssignableOpenTask = false; want the oldest ready task")
		}
		if got.ID != order.ID {
			t.Fatalf("no-edge graph: assignable = %s; want oldest task %s", got.ID.Value(), order.ID.Value())
		}
	})

	t.Run("corrected_direction_subtask_assignable_aggregate_never", func(t *testing.T) {
		l, ts, agent, convID, causes, order, sub := newDecomposedGraph(t)
		// Corrected decomposition: the PARENT depends on its piece.
		if err := ts.AddDependency(agent.ID(), order.ID, sub.ID, causes, convID); err != nil {
			t.Fatalf("AddDependency: %v", err)
		}
		got, ok := l.firstAssignableOpenTask()
		if !ok {
			t.Fatal("corrected direction: firstAssignableOpenTask = false; want the subtask (v11-F1 liveness)")
		}
		if got.ID != sub.ID {
			t.Fatalf("corrected direction: assignable = %s; want subtask %s", got.ID.Value(), sub.ID.Value())
		}

		// Complete the subtask: the order unblocks in ListOpen, but as an
		// aggregate it must STILL never be auto-assigned — its remaining work
		// is the governed completion path, not another Operate.
		if err := ts.Assign(agent.ID(), sub.ID, agent.ID(), causes, convID); err != nil {
			t.Fatalf("Assign sub: %v", err)
		}
		if err := ts.AddArtifact(agent.ID(), sub.ID, "result", "text/plain", "done", causes, convID); err != nil {
			t.Fatalf("AddArtifact result: %v", err)
		}
		if err := ts.Complete(agent.ID(), sub.ID, "subtask done", causes, convID); err != nil {
			t.Fatalf("Complete sub: %v", err)
		}
		if got, ok := l.firstAssignableOpenTask(); ok {
			t.Fatalf("unblocked aggregate auto-assigned: got %s; aggregates must never be auto-assigned", got.ID.Value())
		}
	})

	t.Run("reverse_edge_refused_fail_closed", func(t *testing.T) {
		_, ts, agent, convID, causes, order, sub := newDecomposedGraph(t)
		// Corrected decomposition: the parent depends on its piece.
		if err := ts.AddDependency(agent.ID(), order.ID, sub.ID, causes, convID); err != nil {
			t.Fatalf("AddDependency: %v", err)
		}
		// A reverse edge (subtask depends on parent) would make both tasks
		// aggregates AND both ListOpen-blocked — the v11-F1 deadlock as a
		// 2-cycle. The command layer must refuse it fail-closed.
		payload := []byte(`{"task_id":"` + sub.ID.Value() + `","depends_on":"` + order.ID.Value() + `"}`)
		err := execTaskDepend(payload, ts, agent.ID(), causes, convID)
		if err == nil {
			t.Fatal("execTaskDepend accepted a reverse edge; want fail-closed refusal (v11-F1 2-cycle)")
		}
		if !strings.Contains(err.Error(), "refused") {
			t.Fatalf("refusal error = %q; want it to name the refusal", err)
		}
		deps, depErr := ts.GetDependencies(sub.ID)
		if depErr != nil {
			t.Fatalf("GetDependencies: %v", depErr)
		}
		if len(deps) != 0 {
			t.Fatalf("reverse edge persisted (%d deps on subtask); refusal must leave the store unchanged", len(deps))
		}
	})

	t.Run("concurrent_opposite_edges_never_form_cycle", func(t *testing.T) {
		// Round-2 review blocker: the reverse-edge guard was check-then-append,
		// so two agent goroutines racing opposite edges could both pass the
		// check (TOCTOU) and land a 2-cycle. taskDependMu makes the pair
		// atomic in-process. Race fresh pairs repeatedly: a cycle must never
		// persist. (Without the mutex this catches the interleave
		// probabilistically; with it, never.)
		_, ts, agent, convID, causes, _, _ := newDecomposedGraph(t)
		for i := 0; i < 50; i++ {
			a, err := ts.Create(agent.ID(), "pair-a", "race", causes, convID)
			if err != nil {
				t.Fatalf("Create a: %v", err)
			}
			b, err := ts.Create(agent.ID(), "pair-b", "race", causes, convID)
			if err != nil {
				t.Fatalf("Create b: %v", err)
			}
			fwd := []byte(`{"task_id":"` + a.ID.Value() + `","depends_on":"` + b.ID.Value() + `"}`)
			rev := []byte(`{"task_id":"` + b.ID.Value() + `","depends_on":"` + a.ID.Value() + `"}`)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); _ = execTaskDepend(fwd, ts, agent.ID(), causes, convID) }()
			go func() { defer wg.Done(); _ = execTaskDepend(rev, ts, agent.ID(), causes, convID) }()
			wg.Wait()
			aDeps, err := ts.GetDependencies(a.ID)
			if err != nil {
				t.Fatalf("GetDependencies a: %v", err)
			}
			bDeps, err := ts.GetDependencies(b.ID)
			if err != nil {
				t.Fatalf("GetDependencies b: %v", err)
			}
			aOnB := false
			for _, d := range aDeps {
				if d == b.ID {
					aOnB = true
				}
			}
			bOnA := false
			for _, d := range bDeps {
				if d == a.ID {
					bOnA = true
				}
			}
			if aOnB && bOnA {
				t.Fatalf("iteration %d: both opposite edges landed — 2-cycle persisted (v11-F1 deadlock)", i)
			}
		}
	})

	t.Run("legacy_direction_order_assignable_not_deadlocked", func(t *testing.T) {
		l, ts, agent, convID, causes, order, sub := newDecomposedGraph(t)
		// Legacy decomposition direction (pre-v11-F1 stores): the subtask
		// depends on its parent. The subtask is prerequisite-hidden by ListOpen
		// AND an aggregate; the gated order must remain assignable — the
		// literal v11 deadlock (zero assignable tasks) must not recur.
		if err := ts.AddDependency(agent.ID(), sub.ID, order.ID, causes, convID); err != nil {
			t.Fatalf("AddDependency: %v", err)
		}
		got, ok := l.firstAssignableOpenTask()
		if !ok {
			t.Fatal("legacy direction: firstAssignableOpenTask = false — the v11-F1 deadlock recurred")
		}
		if got.ID != order.ID {
			t.Fatalf("legacy direction: assignable = %s; want order %s", got.ID.Value(), order.ID.Value())
		}
	})
}
