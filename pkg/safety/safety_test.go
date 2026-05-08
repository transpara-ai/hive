package safety

import "testing"

func TestDefaultOutcomeRequiresApprovalForProtectedActions(t *testing.T) {
	for _, action := range ProtectedActions {
		if got := DefaultOutcome(action); got != ApprovalRequired {
			t.Fatalf("DefaultOutcome(%q) = %s, want %s", action, got, ApprovalRequired)
		}
		if err := RequireAuthorized(action); err == nil {
			t.Fatalf("RequireAuthorized(%q) returned nil, want authority error", action)
		}
	}
}

func TestProtectedActionsMatchDFSOPVocabulary(t *testing.T) {
	want := []ProtectedAction{
		"production.deploy",
		"repo.create",
		"repo.delete",
		"repo.push.default_branch",
		"repo.merge.main",
		"repo.mutate.cross_repo",
		"agent.spawn.persistent",
		"agent.retire",
		"agent.revoke",
		"agent.escalate_permissions",
		"policy.change",
		"secret.access",
		"external_communication.company_voice",
		"data.delete",
		"self_modification.activate",
		"billing.spend_above_threshold",
		"license.change",
	}
	if len(ProtectedActions) != len(want) {
		t.Fatalf("ProtectedActions len = %d, want %d", len(ProtectedActions), len(want))
	}
	for i, action := range want {
		if ProtectedActions[i] != action {
			t.Fatalf("ProtectedActions[%d] = %q, want %q", i, ProtectedActions[i], action)
		}
	}
}

func TestDefaultOutcomeFailsClosedForUnknownActions(t *testing.T) {
	const action ProtectedAction = "patch.proposed"
	if got := DefaultOutcome(action); got != Forbidden {
		t.Fatalf("DefaultOutcome(%q) = %s, want %s", action, got, Forbidden)
	}
	if err := RequireAuthorized(action); err == nil {
		t.Fatalf("RequireAuthorized(%q) returned nil, want fail-closed authority error", action)
	}
}

func TestAgentLifecycleOperationsRequireCanonicalProtectedActions(t *testing.T) {
	tests := []struct {
		op   AgentLifecycleOperation
		want ProtectedAction
	}{
		{AgentLifecycleSpawnPersistent, ActionAgentSpawnPersistent},
		{AgentLifecycleRetire, ActionAgentRetire},
		{AgentLifecycleRevoke, ActionAgentRevoke},
		{AgentLifecycleEscalatePermissions, ActionAgentEscalatePermissions},
	}
	for _, tt := range tests {
		t.Run(string(tt.op), func(t *testing.T) {
			got, err := RequiredActionForAgentLifecycleOperation(tt.op)
			if err != nil {
				t.Fatalf("RequiredActionForAgentLifecycleOperation(%q): %v", tt.op, err)
			}
			if got != tt.want {
				t.Fatalf("RequiredActionForAgentLifecycleOperation(%q) = %q, want %q", tt.op, got, tt.want)
			}
			if DefaultOutcome(got) != ApprovalRequired {
				t.Fatalf("%q must require approval", got)
			}
		})
	}
}

func TestUnknownLifecycleOperationFailsClosed(t *testing.T) {
	_, err := RequiredActionForAgentLifecycleOperation("promote_without_policy")
	if err == nil {
		t.Fatal("unknown lifecycle operation returned nil error, want fail-closed authority error")
	}
	authErr, ok := err.(AuthorityError)
	if !ok {
		t.Fatalf("error type = %T, want AuthorityError", err)
	}
	if authErr.Outcome != Forbidden {
		t.Fatalf("outcome = %q, want %q", authErr.Outcome, Forbidden)
	}
}

func TestLifecycleApprovalDoesNotGrantSelfModification(t *testing.T) {
	if ApprovalAllowsAction(ActionAgentSpawnPersistent, ActionSelfModificationActivate) {
		t.Fatal("agent.spawn.persistent approval must not grant self_modification.activate")
	}
	if ApprovalAllowsAction(ActionAgentRetire, ActionSelfModificationActivate) {
		t.Fatal("agent.retire approval must not grant self_modification.activate")
	}
	if !ApprovalAllowsAction(ActionSelfModificationActivate, ActionSelfModificationActivate) {
		t.Fatal("matching self_modification.activate approval should authorize only that action")
	}
}
