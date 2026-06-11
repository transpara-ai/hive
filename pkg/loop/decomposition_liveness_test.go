package loop

import (
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
