// Package safety defines Dark Factory protected actions and default authority gates.
package safety

import "fmt"

// AuthorityOutcome is the result of evaluating whether an actor may perform an action.
type AuthorityOutcome string

const (
	Autonomous       AuthorityOutcome = "Autonomous"
	Notify           AuthorityOutcome = "Notify"
	ApprovalRequired AuthorityOutcome = "ApprovalRequired"
	Forbidden        AuthorityOutcome = "Forbidden"
)

// ProtectedAction identifies an action with side effects that may require authority.
type ProtectedAction string

const (
	ActionProductionDeploy          ProtectedAction = "production.deploy"
	ActionRepoCreate                ProtectedAction = "repo.create"
	ActionRepoDelete                ProtectedAction = "repo.delete"
	ActionRepoPushDefaultBranch     ProtectedAction = "repo.push.default_branch"
	ActionRepoMutateCrossRepo       ProtectedAction = "repo.mutate.cross_repo"
	ActionAgentSpawnPersistent      ProtectedAction = "agent.spawn.persistent"
	ActionAgentRetire               ProtectedAction = "agent.retire"
	ActionAgentEscalatePermissions  ProtectedAction = "agent.escalate_permissions"
	ActionPolicyChange              ProtectedAction = "policy.change"
	ActionSecretAccess              ProtectedAction = "secret.access"
	ActionExternalCompanyVoice      ProtectedAction = "external_communication.company_voice"
	ActionDataDelete                ProtectedAction = "data.delete"
	ActionSelfModificationActivate  ProtectedAction = "self_modification.activate"
	ActionBillingSpendAboveThreshold ProtectedAction = "billing.spend_above_threshold"
	ActionLicenseChange             ProtectedAction = "license.change"
	ActionRepoMergeMain             ProtectedAction = "repo.merge.main"
)

// AuthorityError is returned when an action must not execute without approval.
type AuthorityError struct {
	Action  ProtectedAction
	Outcome AuthorityOutcome
}

func (e AuthorityError) Error() string {
	return fmt.Sprintf("authority required for %s: %s", e.Action, e.Outcome)
}

// DefaultOutcome returns the safety-first default for protected actions.
// The default posture is conservative until EventGraph authority policy is wired in.
func DefaultOutcome(action ProtectedAction) AuthorityOutcome {
	switch action {
	case ActionProductionDeploy,
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
		ActionRepoMergeMain:
		return ApprovalRequired
	default:
		return Autonomous
	}
}

// RequireAuthorized returns nil only for actions that are autonomous by default.
// Callers must convert non-nil results into proposal or authority-request flows.
func RequireAuthorized(action ProtectedAction) error {
	outcome := DefaultOutcome(action)
	if outcome == Autonomous || outcome == Notify {
		return nil
	}
	return AuthorityError{Action: action, Outcome: outcome}
}
