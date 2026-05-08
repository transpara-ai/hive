package hive

import (
	"errors"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/modelconfig"
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
