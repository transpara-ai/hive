package loop

import (
	"errors"
	"strings"
	"testing"
	"time"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/checkpoint"
	"github.com/transpara-ai/work"
)

// The review→fix return edge (run findings v12-F1). Round 2 of the slice-1
// convergence run proved a request_changes verdict had no consumer: the task
// stayed completed, the producer stayed parked, the cycle cap blocked every
// further verdict including approve, and "escalating" was a stdout line no
// agent could observe — the reviewer burned its entire duration budget
// re-reviewing one un-settleable task. These tests pin the closed loop:
// request_changes reopens (while a verdict slot remains), the cap escalates
// ON THE CHAIN exactly once, and a capped task never pends again.

// newReturnEdgeFixture builds a loop with reviewer state over a shared
// chain: one task, gated, assigned to the loop's own agent, completed.
func newReturnEdgeFixture(t *testing.T) (*Loop, *work.TaskStore, *hiveagent.Agent, types.ConversationID, []types.EventID, work.Task) {
	t.Helper()
	l, ts, agent, convID := newRecheckLoop(t, true, 10*time.Millisecond)
	l.reviewerState = newReviewerState()
	var causes []types.EventID
	if !agent.LastEvent().IsZero() {
		causes = []types.EventID{agent.LastEvent()}
	}
	task, err := ts.Create(agent.ID(), "impl: roles catalog", "write the catalog", causes, convID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ts.AddArtifact(agent.ID(), task.ID, "result", "text/plain", "commit: abc", causes, convID); err != nil {
		t.Fatalf("AddArtifact: %v", err)
	}
	if err := ts.Assign(agent.ID(), task.ID, agent.ID(), causes, convID); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := ts.Complete(agent.ID(), task.ID, "done", causes, convID); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return l, ts, agent, convID, causes, task
}

func openIDs(t *testing.T, ts *work.TaskStore) map[types.EventID]bool {
	t.Helper()
	open, err := ts.ListOpen()
	if err != nil {
		t.Fatalf("ListOpen: %v", err)
	}
	ids := make(map[types.EventID]bool, len(open))
	for _, task := range open {
		ids[task.ID] = true
	}
	return ids
}

func escalationComments(t *testing.T, ts *work.TaskStore, taskID types.EventID) int {
	t.Helper()
	comments, err := ts.ListComments(taskID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	n := 0
	for _, c := range comments {
		if strings.HasPrefix(c.Body, reviewEscalationMarker) {
			n++
		}
	}
	return n
}

func TestRouteReviewVerdict_RequestChangesReopens(t *testing.T) {
	l, ts, _, _, _, task := newReturnEdgeFixture(t)

	cmd := &ReviewCommand{
		TaskID:     task.ID.Value(),
		Verdict:    "request_changes",
		Summary:    "Tier rows cite the wrong source",
		Issues:     []string{"fix the Tier citation", "reword the Notes claim"},
		Confidence: 0.97,
	}
	l.reviewerState.recordReview(cmd.TaskID, cmd.Verdict, cmd.Issues)
	l.routeReviewVerdict(cmd)

	if !openIDs(t, ts)[task.ID] {
		t.Fatal("request_changes must reopen the task")
	}
	reopens, err := ts.ListReopens(task.ID)
	if err != nil {
		t.Fatalf("ListReopens: %v", err)
	}
	if len(reopens) != 1 {
		t.Fatalf("got %d reopens, want 1", len(reopens))
	}
	if !strings.Contains(reopens[0].Reason, "request_changes") || !strings.Contains(reopens[0].Reason, "Tier rows") {
		t.Fatalf("reopen reason = %q, want the verdict and summary", reopens[0].Reason)
	}
	if len(reopens[0].Issues) != 2 {
		t.Fatalf("reopen issues = %v, want the review's 2 issues", reopens[0].Issues)
	}
}

func TestRouteReviewVerdict_ApproveDoesNotReopen(t *testing.T) {
	l, ts, _, _, _, task := newReturnEdgeFixture(t)

	cmd := &ReviewCommand{
		TaskID:     task.ID.Value(),
		Verdict:    "approve",
		Summary:    "all good",
		Issues:     []string{},
		Confidence: 0.9,
	}
	l.reviewerState.recordReview(cmd.TaskID, cmd.Verdict, cmd.Issues)
	l.routeReviewVerdict(cmd)

	if openIDs(t, ts)[task.ID] {
		t.Fatal("approve must not reopen the task")
	}
	reopens, err := ts.ListReopens(task.ID)
	if err != nil {
		t.Fatalf("ListReopens: %v", err)
	}
	if len(reopens) != 0 {
		t.Fatalf("got %d reopens, want 0", len(reopens))
	}
}

func TestRouteReviewVerdict_ThirdRequestChangesEscalates(t *testing.T) {
	l, ts, agent, convID, causes, task := newReturnEdgeFixture(t)

	rc := func(summary string) *ReviewCommand {
		return &ReviewCommand{
			TaskID:     task.ID.Value(),
			Verdict:    "request_changes",
			Summary:    summary,
			Issues:     []string{"still wrong"},
			Confidence: 0.9,
		}
	}

	// Round 1: reopen, fix, re-complete.
	cmd := rc("round 1")
	l.reviewerState.recordReview(cmd.TaskID, cmd.Verdict, cmd.Issues)
	l.routeReviewVerdict(cmd)
	if err := ts.Complete(agent.ID(), task.ID, "fixed once", causes, convID); err != nil {
		t.Fatalf("re-Complete 1: %v", err)
	}

	// Round 2: reopen, fix, re-complete.
	cmd = rc("round 2")
	l.reviewerState.recordReview(cmd.TaskID, cmd.Verdict, cmd.Issues)
	l.routeReviewVerdict(cmd)
	if err := ts.Complete(agent.ID(), task.ID, "fixed twice", causes, convID); err != nil {
		t.Fatalf("re-Complete 2: %v", err)
	}

	// Round 3: the last verdict slot — escalate instead of reopen. A fix
	// that could never be re-approved must not be requested.
	cmd = rc("round 3")
	l.reviewerState.recordReview(cmd.TaskID, cmd.Verdict, cmd.Issues)
	l.routeReviewVerdict(cmd)

	reopens, err := ts.ListReopens(task.ID)
	if err != nil {
		t.Fatalf("ListReopens: %v", err)
	}
	if len(reopens) != 2 {
		t.Fatalf("got %d reopens, want 2 — the third request_changes must not reopen", len(reopens))
	}
	if openIDs(t, ts)[task.ID] {
		t.Fatal("after the cap trip the deliverable must stay completed for the human to judge")
	}
	if got := escalationComments(t, ts, task.ID); got != 1 {
		t.Fatalf("got %d escalation comments, want exactly 1 on the chain", got)
	}

	// A fourth attempt (the blocked-emission branch) must not re-post.
	l.emitReviewEscalationOnce(task.ID.Value(), []string{"still wrong"})
	if got := escalationComments(t, ts, task.ID); got != 1 {
		t.Fatalf("escalation must be emit-once; got %d comments", got)
	}
}

func TestFindPendingReviews_CappedTaskNotPending(t *testing.T) {
	s := newReviewerState()
	s.completedTasks["task-1"] = work.TaskCompletedContent{}
	s.recordReview("task-1", "request_changes", []string{"a"})
	s.recordReview("task-1", "request_changes", []string{"b"})
	if got := len(s.findPendingReviews()); got != 1 {
		t.Fatalf("below the cap the task must pend; got %d", got)
	}
	s.recordReview("task-1", "request_changes", []string{"c"})
	if got := len(s.findPendingReviews()); got != 0 {
		t.Fatalf("a capped task must never pend (the v12-F1 burn); got %d", got)
	}
}

func TestFindPendingReviews_RecoverySeededCapNotPending(t *testing.T) {
	s := newReviewerState()
	s.InitReviewerFromRecovery(&checkpoint.ReviewerRecoveredState{
		ReviewCounts: map[string]int{"task-1": 3},
	})
	// The completion folds in from the chain after recovery.
	s.completedTasks["task-1"] = work.TaskCompletedContent{}
	if got := len(s.findPendingReviews()); got != 0 {
		t.Fatalf("a recovery-seeded capped task must not pend (restart re-burn); got %d", got)
	}
}

func TestCatchUp_ReopenEvictsPending(t *testing.T) {
	l, ts, agent, convID, causes, task := newReturnEdgeFixture(t)

	if !l.catchUpReviewProjection() {
		t.Fatal("catch-up failed")
	}
	if got := len(l.reviewerState.findPendingReviews()); got != 1 {
		t.Fatalf("completed task must pend; got %d", got)
	}

	if err := ts.Reopen(agent.ID(), task.ID, "request_changes: fix it", []string{"issue"}, causes, convID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if !l.catchUpReviewProjection() {
		t.Fatal("catch-up after reopen failed")
	}
	if got := len(l.reviewerState.findPendingReviews()); got != 0 {
		t.Fatalf("a reopened task is being fixed and must not pend; got %d", got)
	}
}

func TestCatchUp_OwnEscalationCommentRearmsOnce(t *testing.T) {
	l, ts, agent, convID, causes, task := newReturnEdgeFixture(t)

	// A prior process posted the escalation; this process replays the chain.
	body := reviewEscalationMarker + " review cycle limit (3) reached for task " + task.ID.Value()
	if err := ts.AddComment(task.ID, body, agent.ID(), causes, convID); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if !l.catchUpReviewProjection() {
		t.Fatal("catch-up failed")
	}

	rec := l.reviewerState.reviewHistory[task.ID.Value()]
	if rec == nil || !rec.escalated {
		t.Fatal("replaying the reviewer's own escalation comment must re-arm emit-once state")
	}
	l.emitReviewEscalationOnce(task.ID.Value(), []string{"x"})
	if got := escalationComments(t, ts, task.ID); got != 1 {
		t.Fatalf("a restarted reviewer must not re-post the escalation; got %d comments", got)
	}
}

func TestTaskIsOperable_ReopenedAssignedTaskOperableAgain(t *testing.T) {
	l, ts, agent, convID, causes, task := newReturnEdgeFixture(t)

	if l.taskIsOperable(task.ID) {
		t.Fatal("a completed task must not be operable")
	}
	if l.hasAssignedTask() {
		t.Fatal("a completed assigned task must not count as assigned work")
	}

	if err := ts.Reopen(agent.ID(), task.ID, "request_changes: fix it", []string{"issue"}, causes, convID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	if !l.taskIsOperable(task.ID) {
		t.Fatal("a reopened task must be operable again — this IS the return edge")
	}
	if !l.hasAssignedTask() {
		t.Fatal("the sticky assignment must make the reopened task the producer's work again")
	}
}

func TestOperateInstruction_IncludesReopenFeedback(t *testing.T) {
	l, ts, agent, convID, causes, task := newReturnEdgeFixture(t)

	if err := ts.Reopen(agent.ID(), task.ID, "request_changes (review 1 of 3): Tier rows cite the wrong source", []string{"fix the Tier citation"}, causes, convID); err != nil {
		t.Fatalf("Reopen: %v", err)
	}

	instr, err := l.operateInstruction(task)
	if err != nil {
		t.Fatalf("operateInstruction: %v", err)
	}
	if !strings.Contains(instr, "REVIEW FEEDBACK") {
		t.Fatal("instruction must carry the review feedback section for a reopened task")
	}
	if !strings.Contains(instr, "fix the Tier citation") {
		t.Fatal("instruction must carry the reviewer's issues")
	}
	if !strings.Contains(instr, "Tier rows cite the wrong source") {
		t.Fatal("instruction must carry the reopen reason")
	}
}

// failingReopenLister satisfies taskContextLister with artifacts that load and
// reopens that fail — the instruction must fail closed rather than Operate
// blind to review feedback that provably exists but cannot be read.
type failingReopenLister struct{}

func (failingReopenLister) ListArtifacts(types.EventID) ([]work.ArtifactEvent, error) {
	return nil, nil
}

func (failingReopenLister) ListReopens(types.EventID) ([]work.ReopenEvent, error) {
	return nil, errors.New("reopen scan unavailable")
}

func TestOperateInstruction_FailsClosedOnReopenListError(t *testing.T) {
	task := work.Task{Title: "T", Description: "D"}
	if _, err := operateInstructionFrom(failingReopenLister{}, task); err == nil {
		t.Fatal("an unreadable reopen list must fail the instruction closed")
	}
}
