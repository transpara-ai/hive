package hive

import (
	"context"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

func TestRuntimeTaskScopeRequiresCurrentConversationAndWorkspace(t *testing.T) {
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "Operator")
	rt, err := New(context.Background(), Config{
		Store:           store.NewInMemoryStore(),
		Actors:          actors,
		HumanID:         humanID,
		RepoPath:        t.TempDir(),
		IsolateRunTasks: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	if !rt.isolateRunTasks {
		t.Fatal("IsolateRunTasks was not wired into Runtime")
	}
	bootstrap, err := event.NewBootstrapFactory(rt.graph.Registry()).Init(rt.humanID, rt.signer)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := rt.store.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}

	head, err := rt.store.Head()
	if err != nil || head.IsNone() {
		t.Fatalf("head = %v, %v", head, err)
	}
	causes := []types.EventID{head.Unwrap().ID()}
	current, err := rt.tasks.CreateInWorkspace(rt.humanID, "current", "", rt.repoPath, causes, rt.convID)
	if err != nil {
		t.Fatalf("create current: %v", err)
	}
	oldConv := types.MustConversationID("conv_00000000000000000000000000000105")
	old, err := rt.tasks.CreateInWorkspace(rt.humanID, "old", "", rt.repoPath, causes, oldConv)
	if err != nil {
		t.Fatalf("create old: %v", err)
	}
	wrongWorkspace, err := rt.tasks.CreateInWorkspace(rt.humanID, "wrong workspace", "", t.TempDir(), causes, rt.convID)
	if err != nil {
		t.Fatalf("create wrong workspace: %v", err)
	}

	if !rt.taskInCurrentRun(current.ID) {
		t.Fatal("current conversation/workspace task rejected")
	}
	if rt.taskInCurrentRun(old.ID) {
		t.Fatal("prior conversation task accepted")
	}
	if rt.taskInCurrentRun(wrongWorkspace.ID) {
		t.Fatal("wrong workspace task accepted")
	}
	currentEvent, err := rt.store.Get(current.ID)
	if err != nil {
		t.Fatalf("get current event: %v", err)
	}
	oldEvent, err := rt.store.Get(old.ID)
	if err != nil {
		t.Fatalf("get old event: %v", err)
	}
	if !rt.eventInCurrentRun(currentEvent) || rt.eventInCurrentRun(oldEvent) {
		t.Fatal("persistent role-event conversation scope does not match current run")
	}
	if rt.oneShotTaskScope() == nil || rt.oneShotTaskWorkspace() != rt.repoPath {
		t.Fatal("one-shot task scope/workspace not wired")
	}

	// Daemon mode deliberately retains the global durable behavior.
	rt.isolateRunTasks = false
	if rt.oneShotTaskScope() != nil || rt.oneShotTaskWorkspace() != "" || !rt.eventInCurrentRun(oldEvent) {
		t.Fatal("daemon mode unexpectedly filters durable task or role state")
	}
}

func TestOneShotAuthorityRequestDedupIgnoresPriorConversation(t *testing.T) {
	actors := actor.NewInMemoryActorStore()
	humanID := registerTestHuman(t, actors, "Operator")
	rt, err := New(context.Background(), Config{
		Store:           store.NewInMemoryStore(),
		Actors:          actors,
		HumanID:         humanID,
		RepoPath:        t.TempDir(),
		IsolateRunTasks: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = rt.graph.Close() })
	bootstrap, err := event.NewBootstrapFactory(rt.graph.Registry()).Init(rt.humanID, rt.signer)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := rt.store.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}

	currentConv := rt.convID
	oldConv := types.MustConversationID("conv_00000000000000000000000000000106")
	req := protectedActionRequest{
		Action:            safety.ActionAgentSpawnPersistent,
		Target:            "agent:reviewer",
		Justification:     "test prior-run dedup",
		RequestedOutcome:  "create persistent agent",
		ProposedOperation: "spawnDynamicAgent",
	}
	rt.convID = oldConv
	if _, err := rt.recordAuthorityRequest(req); err != nil {
		t.Fatalf("record old request: %v", err)
	}
	rt.convID = currentConv
	if rt.hasAuthorityRequest(req.Action, req.Target) {
		t.Fatal("prior-conversation authority request suppressed a current one-shot request")
	}

	rt.isolateRunTasks = false
	if !rt.hasAuthorityRequest(req.Action, req.Target) {
		t.Fatal("daemon mode did not retain global authority-request deduplication")
	}
	rt.isolateRunTasks = true
	if _, err := rt.recordAuthorityRequest(req); err != nil {
		t.Fatalf("record current request: %v", err)
	}
	if !rt.hasAuthorityRequest(req.Action, req.Target) {
		t.Fatal("current-conversation authority request not found")
	}
}

func TestOneShotPublicIssueScanProgressIsDisabled(t *testing.T) {
	rt := &Runtime{isolateRunTasks: true}
	if _, err := rt.ProgressIssueScanRunLifecycleContext(context.Background(), ""); err != nil {
		t.Fatalf("isolated local lifecycle entry point returned error: %v", err)
	}
	if _, err := rt.ProgressIssueScanRunLifecycleWithConfiguredRunners(context.Background(), ""); err != nil {
		t.Fatalf("isolated configured-runner entry point returned error: %v", err)
	}

	rt.isolateRunTasks = false
	if _, err := rt.ProgressIssueScanRunLifecycleContext(context.Background(), ""); err == nil {
		t.Fatal("daemon lifecycle entry point unexpectedly accepted empty run_id")
	}
}

func TestOneShotDynamicRoleLookupsIgnorePriorConversation(t *testing.T) {
	rt := newIdentityTestRuntime(t)
	rt.isolateRunTasks = true
	currentConv := rt.convID
	rt.convID = types.MustConversationID("conv_00000000000000000000000000000107")
	recordRoleProposalApprovalAndBudget(t, rt, "reviewer")
	rt.convID = currentConv

	if _, found := rt.findRoleProposal("reviewer"); found {
		t.Fatal("prior-conversation role proposal entered one-shot lookup")
	}
	if _, found := rt.findApproval("reviewer"); found {
		t.Fatal("prior-conversation role approval entered one-shot lookup")
	}
	if _, found := rt.findBudgetForRole("reviewer"); found {
		t.Fatal("prior-conversation role budget entered one-shot lookup")
	}

	rt.isolateRunTasks = false
	if _, found := rt.findRoleProposal("reviewer"); !found {
		t.Fatal("daemon mode did not retain global role-proposal lookup")
	}
	if _, found := rt.findApproval("reviewer"); !found {
		t.Fatal("daemon mode did not retain global role-approval lookup")
	}
	if _, found := rt.findBudgetForRole("reviewer"); !found {
		t.Fatal("daemon mode did not retain global role-budget lookup")
	}
}

func TestNilThoughtStoreDisablesCheckpointSink(t *testing.T) {
	if sink := buildCheckpointSink(nil, "reviewer"); sink != nil {
		t.Fatalf("buildCheckpointSink(nil) = %T, want nil", sink)
	}
}
