package safety

import "testing"

func TestDefaultOutcomeRequiresApprovalForProtectedActions(t *testing.T) {
	actions := []ProtectedAction{
		ActionProductionDeploy,
		ActionRepoCreate,
		ActionRepoDelete,
		ActionRepoPushDefaultBranch,
		ActionRepoMutateCrossRepo,
		ActionAgentSpawnPersistent,
		ActionAgentRetire,
		ActionAgentEscalatePermissions,
		ActionPolicyChange,
		ActionSecretAccess,
		ActionExternalCompanyVoice,
		ActionDataDelete,
		ActionSelfModificationActivate,
		ActionBillingSpendAboveThreshold,
		ActionLicenseChange,
		ActionRepoMergeMain,
	}

	for _, action := range actions {
		if got := DefaultOutcome(action); got != ApprovalRequired {
			t.Fatalf("DefaultOutcome(%q) = %s, want %s", action, got, ApprovalRequired)
		}
		if err := RequireAuthorized(action); err == nil {
			t.Fatalf("RequireAuthorized(%q) returned nil, want authority error", action)
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
