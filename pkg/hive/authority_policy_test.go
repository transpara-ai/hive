package hive

import (
	"errors"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

func TestAuthorizeProtectedActionFailsClosedForUnknownAction(t *testing.T) {
	rt := newIdentityTestRuntime(t)

	_, err := rt.authorizeProtectedAction(protectedActionRequest{
		Action:        safety.ProtectedAction("agent.promote_without_policy"),
		Target:        "agent:builder",
		Justification: "unknown action must not execute",
	})
	if err == nil {
		t.Fatal("unknown protected action returned nil error, want fail-closed authority error")
	}
	var authErr safety.AuthorityError
	if !errors.As(err, &authErr) {
		t.Fatalf("error type = %T, want safety.AuthorityError", err)
	}
	if authErr.Outcome != safety.Forbidden {
		t.Fatalf("outcome = %q, want %q", authErr.Outcome, safety.Forbidden)
	}
}

func TestAuthorizeProtectedActionRecordsRequestAndBlocksWithoutApproval(t *testing.T) {
	rt := newIdentityTestRuntime(t)

	requestID, err := rt.authorizeProtectedAction(protectedActionRequest{
		Action:            safety.ActionAgentSpawnPersistent,
		Target:            "agent:builder",
		Environment:       string(AgentIdentityEnvironmentProduction),
		RequestedOutcome:  "create persistent agent",
		Justification:     "persistent builder requested",
		RiskSummary:       "long-lived authority",
		ProposedOperation: "spawnDynamicAgent",
	})
	if err == nil {
		t.Fatal("protected action without approval returned nil error")
	}
	var authErr safety.AuthorityError
	if !errors.As(err, &authErr) {
		t.Fatalf("error type = %T, want safety.AuthorityError", err)
	}
	if authErr.Action != safety.ActionAgentSpawnPersistent {
		t.Fatalf("action = %q, want %q", authErr.Action, safety.ActionAgentSpawnPersistent)
	}
	if authErr.Outcome != safety.ApprovalRequired {
		t.Fatalf("outcome = %q, want %q", authErr.Outcome, safety.ApprovalRequired)
	}
	if requestID.IsZero() {
		t.Fatal("requestID is zero, want recorded authority request")
	}

	requests := authorityRequestsByType[event.AuthorityRequestContent](t, rt, event.EventTypeAuthorityRequested)
	if len(requests) != 1 {
		t.Fatalf("authority.requested count = %d, want 1", len(requests))
	}
	if requests[0].Action != string(safety.ActionAgentSpawnPersistent) {
		t.Fatalf("request action = %q, want %q", requests[0].Action, safety.ActionAgentSpawnPersistent)
	}

	details := authorityRequestsByType[AuthorityRequestRecordedContent](t, rt, EventTypeAuthorityRequestRecorded)
	if len(details) != 1 {
		t.Fatalf("authority.request.recorded count = %d, want 1", len(details))
	}
	if details[0].RequestID != requestID {
		t.Fatalf("detail RequestID = %s, want %s", details[0].RequestID, requestID)
	}
	if details[0].ActionName != string(safety.ActionAgentSpawnPersistent) {
		t.Fatalf("detail action = %q, want %q", details[0].ActionName, safety.ActionAgentSpawnPersistent)
	}
	if details[0].RiskClass != safety.RiskClass(safety.ActionAgentSpawnPersistent) {
		t.Fatalf("detail risk class = %q, want %q", details[0].RiskClass, safety.RiskClass(safety.ActionAgentSpawnPersistent))
	}
	if len(details[0].Scope) != 1 || details[0].Scope[0] != string(safety.ActionAgentSpawnPersistent) {
		t.Fatalf("detail scope = %#v, want [%s]", details[0].Scope, safety.ActionAgentSpawnPersistent)
	}
}

func TestAuthorizeProtectedActionWithAutoApprovalRecordsDecision(t *testing.T) {
	rt := newIdentityTestRuntime(t)
	rt.approveRequests = true

	requestID, err := rt.authorizeProtectedAction(protectedActionRequest{
		Action:            safety.ActionAgentRetire,
		Target:            "agent:builder",
		Environment:       string(AgentIdentityEnvironmentProduction),
		RequestedOutcome:  "retire persistent agent",
		Justification:     "agent reached planned end of life",
		RiskSummary:       "open work must be drained",
		ProposedOperation: "retireAgent",
	})
	if err != nil {
		t.Fatalf("authorize protected action with approveRequests: %v", err)
	}
	if requestID.IsZero() {
		t.Fatal("requestID is zero")
	}

	decisions := authorityRequestsByType[AuthorityDecisionRecordedContent](t, rt, EventTypeAuthorityDecisionRecorded)
	if len(decisions) != 1 {
		t.Fatalf("authority.decision.recorded count = %d, want 1", len(decisions))
	}
	if decisions[0].RequestID != requestID {
		t.Fatalf("decision RequestID = %s, want %s", decisions[0].RequestID, requestID)
	}
	if decisions[0].ApprovedAction != string(safety.ActionAgentRetire) {
		t.Fatalf("approved action = %q, want %q", decisions[0].ApprovedAction, safety.ActionAgentRetire)
	}
	if decisions[0].DeciderRole != "operator" {
		t.Fatalf("decider role = %q, want operator", decisions[0].DeciderRole)
	}
	if len(decisions[0].Scope) != 1 || decisions[0].Scope[0] != string(safety.ActionAgentRetire) {
		t.Fatalf("decision scope = %#v, want [%s]", decisions[0].Scope, safety.ActionAgentRetire)
	}
}

func TestAuthorizeProtectedActionPreservesExplicitScopeAndRiskClass(t *testing.T) {
	rt := newIdentityTestRuntime(t)

	_, err := rt.authorizeProtectedAction(protectedActionRequest{
		Action:           safety.ActionRuntimeInvokeExternal,
		RequestingRole:   "runner",
		Target:           "runtime:external",
		Environment:      "test",
		RiskClass:        "critical",
		Scope:            []string{"runtime.invoke.external", "runtime:test-only"},
		RequestedOutcome: "blocked external runtime invocation",
		Justification:    "prove Epic 3 records explicit scope",
	})
	if err == nil {
		t.Fatal("protected action without approval returned nil error")
	}

	details := authorityRequestsByType[AuthorityRequestRecordedContent](t, rt, EventTypeAuthorityRequestRecorded)
	if len(details) != 1 {
		t.Fatalf("authority.request.recorded count = %d, want 1", len(details))
	}
	if details[0].RequestingRole != "runner" {
		t.Fatalf("requesting role = %q, want runner", details[0].RequestingRole)
	}
	if details[0].RiskClass != "critical" {
		t.Fatalf("risk class = %q, want critical", details[0].RiskClass)
	}
	wantScope := []string{"runtime.invoke.external", "runtime:test-only"}
	if len(details[0].Scope) != len(wantScope) {
		t.Fatalf("scope len = %d, want %d: %#v", len(details[0].Scope), len(wantScope), details[0].Scope)
	}
	for i := range wantScope {
		if details[0].Scope[i] != wantScope[i] {
			t.Fatalf("scope[%d] = %q, want %q", i, details[0].Scope[i], wantScope[i])
		}
	}
}

func TestLifecycleApprovalDoesNotAuthorizeSelfModificationRuntime(t *testing.T) {
	rt := newIdentityTestRuntime(t)
	rt.approveRequests = true
	if _, err := rt.authorizeProtectedAction(protectedActionRequest{
		Action:        safety.ActionAgentSpawnPersistent,
		Target:        "agent:builder",
		Justification: "persistent spawn approved",
	}); err != nil {
		t.Fatalf("authorize spawn persistent: %v", err)
	}

	rt.approveRequests = false
	_, err := rt.authorizeProtectedAction(protectedActionRequest{
		Action:        safety.ActionSelfModificationActivate,
		Target:        "agent:builder",
		Justification: "try self modification after lifecycle approval",
	})
	if err == nil {
		t.Fatal("self-modification after lifecycle approval returned nil error")
	}
	var authErr safety.AuthorityError
	if !errors.As(err, &authErr) {
		t.Fatalf("error type = %T, want safety.AuthorityError", err)
	}
	if authErr.Action != safety.ActionSelfModificationActivate {
		t.Fatalf("action = %q, want %q", authErr.Action, safety.ActionSelfModificationActivate)
	}
}

func TestProcessApprovedRolesRequiresPersistentSpawnAuthority(t *testing.T) {
	rt := newIdentityTestRuntime(t)
	rt.dynamic = newDynamicAgentTracker()
	rt.resolver = modelconfig.DefaultResolver()

	recordRoleProposalApprovalAndBudget(t, rt, "builder")

	rt.processApprovedRoles(t.Context())

	if rt.dynamic.IsTracked("builder") {
		t.Fatal("builder was spawned without agent.spawn.persistent authority")
	}
	details := authorityRequestsByType[AuthorityRequestRecordedContent](t, rt, EventTypeAuthorityRequestRecorded)
	if len(details) != 1 {
		t.Fatalf("authority.request.recorded count = %d, want 1", len(details))
	}
	if details[0].ActionName != string(safety.ActionAgentSpawnPersistent) {
		t.Fatalf("action = %q, want %q", details[0].ActionName, safety.ActionAgentSpawnPersistent)
	}
	if details[0].Target != "agent:builder" {
		t.Fatalf("target = %q, want agent:builder", details[0].Target)
	}
}

func authorityRequestsByType[T any](t *testing.T, rt *Runtime, eventType types.EventType) []T {
	t.Helper()
	page, err := rt.store.ByType(eventType, 100, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(%s): %v", eventType, err)
	}
	out := make([]T, 0, len(page.Items()))
	for _, ev := range page.Items() {
		content, ok := ev.Content().(T)
		if !ok {
			t.Fatalf("%s content type = %T", eventType, ev.Content())
		}
		out = append(out, content)
	}
	return out
}

func recordRoleProposalApprovalAndBudget(t *testing.T, rt *Runtime, name string) {
	t.Helper()
	proposal := event.NewRoleProposedContent(
		name,
		"haiku",
		[]string{"work.task.created"},
		false,
		3,
		"build carefully",
		"test role",
		"spawner",
	)
	proposalEv := recordHiveTestEvent(t, rt, roleProposedType, proposal, nil)

	approval := event.RoleApprovedContent{
		Name:       name,
		ApprovedBy: "operator",
		Reason:     "lifecycle approved",
	}
	approvalEv := recordHiveTestEvent(t, rt, roleApprovedType, approval, []types.EventID{proposalEv.ID()})

	budget := event.AgentBudgetAdjustedContent{
		AgentID:   rt.humanID,
		AgentName: name,
		Action:    "set",
		NewBudget: 3,
		Reason:    "test budget",
	}
	recordHiveTestEvent(t, rt, budgetAdjustedType, budget, []types.EventID{approvalEv.ID()})
}

func recordHiveTestEvent(t *testing.T, rt *Runtime, eventType types.EventType, content event.EventContent, causes []types.EventID) event.Event {
	t.Helper()
	if len(causes) == 0 {
		head, err := rt.store.Head()
		if err != nil {
			t.Fatalf("head: %v", err)
		}
		if !head.IsSome() {
			t.Fatal("no chain head")
		}
		causes = []types.EventID{head.Unwrap().ID()}
	}
	ev, err := rt.graph.Record(eventType, rt.humanID, content, causes, rt.convID, rt.signer)
	if err != nil {
		t.Fatalf("record %s: %v", eventType, err)
	}
	return ev
}

// TestRecordAuthorityDecisionRuntimePublishesToBus is a regression test
// pinning that Runtime.recordAuthorityDecision publishes the
// authority.decision.recorded event to the in-process bus.
//
// This test FAILS against the no-publish regression (where recordAuthorityDecision
// delegates to appendAuthorityDecisionRecorded, which only appends to the store
// without calling graph.Record/Publish). It PASSES once the runtime path uses
// r.graph.Record, which publishes to all bus subscribers.
//
// The telemetry writer and agent loop subscribe to the bus with a "* " pattern;
// they depend on receiving this event to function correctly.
func TestRecordAuthorityDecisionRuntimePublishesToBus(t *testing.T) {
	rt := newIdentityTestRuntime(t)
	rt.approveRequests = true

	// Subscribe to the bus with a catch-all pattern before driving any decision.
	// The bus delivers events asynchronously from per-subscriber goroutines.
	received := make(chan event.Event, 16)
	pattern := types.MustSubscriptionPattern("*")
	subID := rt.graph.Bus().Subscribe(pattern, func(ev event.Event) {
		received <- ev
	})
	t.Cleanup(func() { rt.graph.Bus().Unsubscribe(subID) })

	// Drive a decision through the runtime path. authorizeProtectedAction with
	// approveRequests=true calls recordAuthorityDecision, which must publish.
	requestID, err := rt.authorizeProtectedAction(protectedActionRequest{
		Action:            safety.ActionAgentRetire,
		Target:            "agent:regression-bus-test",
		Environment:       string(AgentIdentityEnvironmentProduction),
		RequestedOutcome:  "retire agent for bus-publish regression test",
		Justification:     "regression: runtime recordAuthorityDecision must publish to bus",
		RiskSummary:       "retire is reversible",
		ProposedOperation: "retireAgent",
	})
	if err != nil {
		t.Fatalf("authorizeProtectedAction: %v", err)
	}
	if requestID.IsZero() {
		t.Fatal("requestID is zero")
	}

	// Wait for the authority.decision.recorded event to arrive on the bus.
	// The bus is async (goroutine delivery), so allow a short timeout.
	const busDeliveryTimeout = 2 * time.Second
	deadline := time.After(busDeliveryTimeout)
	for {
		select {
		case ev := <-received:
			if ev.Type() == EventTypeAuthorityDecisionRecorded {
				content, ok := ev.Content().(AuthorityDecisionRecordedContent)
				if !ok {
					t.Fatalf("authority.decision.recorded content type = %T, want AuthorityDecisionRecordedContent", ev.Content())
				}
				if content.RequestID != requestID {
					t.Fatalf("published decision RequestID = %s, want %s", content.RequestID, requestID)
				}
				// PASS: the event was published to the bus.
				return
			}
			// Other events (authority.requested, authority.request.recorded, etc.) — keep waiting.
		case <-deadline:
			t.Fatal("authority.decision.recorded was NOT published to the bus within timeout; " +
				"Runtime.recordAuthorityDecision must use r.graph.Record, not the store-only helper")
		}
	}
}
