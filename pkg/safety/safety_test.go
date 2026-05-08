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

func TestDefaultOutcomeAllowsUnknownActions(t *testing.T) {
	const action ProtectedAction = "patch.proposed"
	if got := DefaultOutcome(action); got != Autonomous {
		t.Fatalf("DefaultOutcome(%q) = %s, want %s", action, got, Autonomous)
	}
	if err := RequireAuthorized(action); err != nil {
		t.Fatalf("RequireAuthorized(%q) returned %v, want nil", action, err)
	}
}
