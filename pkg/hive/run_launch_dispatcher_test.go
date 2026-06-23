package hive

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

func TestDispatchQueuedRunLaunchesSeedsFactoryOrderWithModelOverrides(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	maxCost := 3.75
	requestEvent := appendValidatedRunLaunch(t, rt.store, writer, []ModelOverrideRequest{
		{Role: "guardian", Model: "api-sonnet", AuthMode: "api-key", MaxCostPerCallUSD: &maxCost},
	})

	result, err := rt.DispatchQueuedRunLaunches(10)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunches: %v", err)
	}
	if result.Scanned != 1 || result.Dispatched != 1 || result.Failed != 0 {
		t.Fatalf("dispatch result = %+v, want one dispatched and no failures", result)
	}
	if len(result.DispatchedTaskIDs) != 1 || len(result.DispatchedOrderIDs) != 1 {
		t.Fatalf("dispatch identifiers = %+v", result)
	}

	tasks, err := rt.tasks.List(10)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1: %+v", len(tasks), tasks)
	}
	task := tasks[0]
	if task.ID != result.DispatchedTaskIDs[0] {
		t.Fatalf("task id = %s, want dispatch result %s", task.ID, result.DispatchedTaskIDs[0])
	}
	if task.FactoryOrderID != result.DispatchedOrderIDs[0] {
		t.Fatalf("factory order id = %q, want %q", task.FactoryOrderID, result.DispatchedOrderIDs[0])
	}
	storedTask, err := rt.store.Get(task.ID)
	if err != nil {
		t.Fatalf("get task event: %v", err)
	}
	if causes := storedTask.Causes(); len(causes) != 1 || causes[0] != requestEvent.ID() {
		t.Fatalf("task causes = %+v, want original run request %s", causes, requestEvent.ID())
	}

	projection, err := rt.tasks.ProjectTask(task.ID)
	if err != nil {
		t.Fatalf("ProjectTask: %v", err)
	}
	if len(projection.ModelOverrides) != 1 {
		t.Fatalf("model overrides = %+v, want one", projection.ModelOverrides)
	}
	override := projection.ModelOverrides[0]
	if override.Role != "guardian" || override.Model != "api-sonnet" || override.RequestedAuthMode != "api-key" {
		t.Fatalf("stored override = %+v, want guardian api-sonnet api-key", override)
	}
	if override.ResolvedProvider != "anthropic" || override.AuthMode != "api-key" || override.ResolvedModel == "" {
		t.Fatalf("stored resolved override = %+v, want resolved anthropic api-key", override)
	}
	if override.MaxCostPerCallUSD == nil || *override.MaxCostPerCallUSD != maxCost {
		t.Fatalf("max cost = %v, want %v", override.MaxCostPerCallUSD, maxCost)
	}

	def := AgentDef{
		Role:           "guardian",
		CanOperate:     false,
		RoleDefinition: StarterRoleDefinitions()["guardian"],
	}
	if _, _, applied, err := rt.resolveTaskModelOverride(task, def, "guardian"); err != nil || !applied {
		t.Fatalf("resolveTaskModelOverride applied=%v err=%v, want Operate-time override validation to succeed", applied, err)
	}

	again, err := rt.DispatchQueuedRunLaunches(10)
	if err != nil {
		t.Fatalf("second DispatchQueuedRunLaunches: %v", err)
	}
	if again.Dispatched != 0 || again.AlreadyDispatched != 1 {
		t.Fatalf("second dispatch result = %+v, want no duplicate dispatch", again)
	}
	tasks, err = rt.tasks.List(10)
	if err != nil {
		t.Fatalf("List tasks after second dispatch: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("task count after second dispatch = %d, want 1", len(tasks))
	}
}

func TestDispatchQueuedRunLaunchesRejectsStoredResolutionDrift(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	requestEvent := appendStaleRunLaunchRequest(t, rt.store, writer)

	result, err := rt.DispatchQueuedRunLaunches(10)
	if err == nil {
		t.Fatal("DispatchQueuedRunLaunches error = nil, want stored resolution drift failure")
	}
	if !strings.Contains(err.Error(), "stored resolved_model") {
		t.Fatalf("error = %q, want stored resolved_model drift", err.Error())
	}
	if result.Scanned != 1 || result.Dispatched != 0 || result.Failed != 1 {
		t.Fatalf("dispatch result = %+v, want one failed and no dispatch", result)
	}

	tasks, listErr := rt.tasks.List(10)
	if listErr != nil {
		t.Fatalf("List tasks: %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("dispatch created task(s) despite stale override on %s: %+v", requestEvent.ID(), tasks)
	}
}

func TestDispatchQueuedRunLaunchOnlyDispatchesRequestedRun(t *testing.T) {
	rt, writer := newRunLaunchDispatchRuntime(t)
	appendValidatedRunLaunch(t, rt.store, writer, nil)
	second := appendValidatedRunLaunch(t, rt.store, writer, nil)
	secondRunID := second.Content().(FactoryRunRequestedContent).RunID

	result, err := rt.DispatchQueuedRunLaunch(secondRunID)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunch: %v", err)
	}
	if result.Dispatched != 1 || result.Failed != 0 {
		t.Fatalf("dispatch result = %+v, want only requested run dispatched", result)
	}
	tasks, err := rt.tasks.List(10)
	if err != nil {
		t.Fatalf("List tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want one targeted dispatch", len(tasks))
	}
	storedTask, err := rt.store.Get(tasks[0].ID)
	if err != nil {
		t.Fatalf("get task event: %v", err)
	}
	if causes := storedTask.Causes(); len(causes) != 1 || causes[0] != second.ID() {
		t.Fatalf("task causes = %+v, want selected run request %s", causes, second.ID())
	}

	remaining, err := rt.DispatchQueuedRunLaunches(10)
	if err != nil {
		t.Fatalf("DispatchQueuedRunLaunches after targeted dispatch: %v", err)
	}
	if remaining.Dispatched != 1 || remaining.AlreadyDispatched != 1 {
		t.Fatalf("remaining dispatch = %+v, want first dispatched and selected already dispatched", remaining)
	}
	tasks, err = rt.tasks.List(10)
	if err != nil {
		t.Fatalf("List tasks after remaining dispatch: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("task count after remaining dispatch = %d, want 2", len(tasks))
	}
}

func newRunLaunchDispatchRuntime(t *testing.T) (*Runtime, *operatorRunLaunchWriter) {
	t.Helper()
	registry := event.DefaultRegistry()
	RegisterWithRegistry(registry)
	work.RegisterWithRegistry(registry)

	s := store.NewInMemoryStore()
	human := types.MustActorID("actor_00000000000000000000000000000078")
	signer := deriveSignerFromID(human)
	bootstrap, err := event.NewBootstrapFactory(registry).Init(human, signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	factory := event.NewEventFactory(registry)
	conv := types.MustConversationID("conv_00000000000000000000000000000078")
	rt := &Runtime{
		store:   s,
		humanID: human,
		factory: factory,
		signer:  signer,
		convID:  conv,
		tasks:   work.NewTaskStore(s, factory, signer),
	}
	rt.setResolver(modelconfig.DefaultResolver())
	return rt, &operatorRunLaunchWriter{factory: factory, signer: signer, human: human, conv: conv}
}

func appendValidatedRunLaunch(t *testing.T, s store.Store, writer *operatorRunLaunchWriter, overrides []ModelOverrideRequest) event.Event {
	t.Helper()
	raw := operatorRunLaunchRequest{
		OperatorID: "user_142",
		IntakeID:   "intake_142",
		Title:      "Launch Hive issue 142",
		Brief:      json.RawMessage(`{"goal":"dispatch queued run launch","issue":"https://github.com/transpara-ai/hive/issues/142"}`),
		Sources: []RunLaunchSource{
			{ID: "issue_142", Type: "issue", Ref: "https://github.com/transpara-ai/hive/issues/142", Title: "Queued run dispatcher"},
		},
		Authority: RunLaunchAuthority{
			InitialLevel: event.AuthorityLevelRequired,
			Scope:        "operator-launch",
			PolicyRef:    "dark-factory/operator-ui-contract-v0.1.2",
			Rationale:    "operator initiated queued launch",
		},
		Budget:         runLaunchBudgetRequest{MaxIterations: intPtr(4), MaxCostUSD: floatPtr(12.5)},
		ModelOverrides: overrides,
		TargetRepos:    []string{"transpara-ai/hive"},
	}
	launch, err := validateRunLaunchRequest(raw, nil)
	if err != nil {
		t.Fatalf("validateRunLaunchRequest: %v", err)
	}
	result, err := appendRunLaunchEvents(s, writer, launch)
	if err != nil {
		t.Fatalf("appendRunLaunchEvents: %v", err)
	}
	page, err := s.ByType(EventTypeFactoryRunRequested, 50, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("query factory.run.requested: %v", err)
	}
	for _, ev := range page.Items() {
		if ev.Content().(FactoryRunRequestedContent).RunID == result.RunID {
			return ev
		}
	}
	t.Fatalf("missing factory.run.requested for run %s", result.RunID)
	return event.Event{}
}

func appendStaleRunLaunchRequest(t *testing.T, s store.Store, writer *operatorRunLaunchWriter) event.Event {
	t.Helper()
	head, err := s.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	content := FactoryRunRequestedContent{
		RunID:      "run_stale_dispatch",
		IntakeID:   "intake_stale",
		OperatorID: "user_stale",
		Title:      "Stale queued run",
		Status:     "queued",
		Authority: RunLaunchAuthority{
			InitialLevel: event.AuthorityLevelRequired,
			Scope:        "operator-launch",
		},
		Budget:      RunLaunchBudget{MaxIterations: 4, MaxCostUSD: 12.5},
		TargetRepos: []string{"transpara-ai/hive"},
		Sources:     []RunLaunchSource{{Type: "issue", Ref: "https://github.com/transpara-ai/hive/issues/142"}},
		Brief:       json.RawMessage(`{"goal":"stale dispatch must fail closed"}`),
		ModelOverrides: []RunLaunchModelOverride{
			{
				Role:              "guardian",
				Model:             "api-sonnet",
				RequestedAuthMode: "api-key",
				ResolvedModel:     "api-claude-opus-4-6",
				ResolvedProvider:  "anthropic",
				AuthMode:          "api-key",
			},
		},
	}
	ev, err := writer.factory.Create(EventTypeFactoryRunRequested, writer.human, content, []types.EventID{head.Unwrap().ID()}, writer.conv, s, writer.signer)
	if err != nil {
		t.Fatalf("create stale factory.run.requested: %v", err)
	}
	stored, err := s.Append(ev)
	if err != nil {
		t.Fatalf("append stale factory.run.requested: %v", err)
	}
	return stored
}

func intPtr(value int) *int {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}
