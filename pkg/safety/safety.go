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
	ActionProductionDeploy           ProtectedAction = "production.deploy"
	ActionRepoCreate                 ProtectedAction = "repo.create"
	ActionRepoDelete                 ProtectedAction = "repo.delete"
	ActionRepoPushDefaultBranch      ProtectedAction = "repo.push.default_branch"
	ActionRepoMergeMain              ProtectedAction = "repo.merge.main"
	ActionRepoMutateCrossRepo        ProtectedAction = "repo.mutate.cross_repo"
	ActionAgentSpawnPersistent       ProtectedAction = "agent.spawn.persistent"
	ActionAgentRetire                ProtectedAction = "agent.retire"
	ActionAgentRevoke                ProtectedAction = "agent.revoke"
	ActionAgentKeyRotate             ProtectedAction = "agent.key.rotate"
	ActionAgentEscalatePermissions   ProtectedAction = "agent.escalate_permissions"
	ActionPolicyChange               ProtectedAction = "policy.change"
	ActionSecretAccess               ProtectedAction = "secret.access"
	ActionExternalCompanyVoice       ProtectedAction = "external_communication.company_voice"
	ActionDataDelete                 ProtectedAction = "data.delete"
	ActionSelfModificationActivate   ProtectedAction = "self_modification.activate"
	ActionBillingSpendAboveThreshold ProtectedAction = "billing.spend_above_threshold"
	ActionLicenseChange              ProtectedAction = "license.change"
	ActionReleaseCertify             ProtectedAction = "release.certify"
	ActionCapabilityPromote          ProtectedAction = "capability.promote"
	ActionCapabilityActivate         ProtectedAction = "capability.activate"
	ActionCapabilityRollback         ProtectedAction = "capability.rollback"
	ActionRuntimeInvokeExternal      ProtectedAction = "runtime.invoke.external"
	ActionMemoryIngestSensitive      ProtectedAction = "memory.ingest.sensitive"
	ActionKnowledgeActivate          ProtectedAction = "knowledge.activate"
	ActionRepoPullRequestCreate      ProtectedAction = "pull_request.create"
	ActionRepoPullRequestMarkReady   ProtectedAction = "pull_request.mark_ready"
)

// ProtectedActions is the DF-SOP-0001 baseline vocabulary. Repos may add
// narrower protected actions, but must not rename or weaken this set.
var ProtectedActions = []ProtectedAction{
	ActionProductionDeploy,
	ActionRepoCreate,
	ActionRepoDelete,
	ActionRepoPushDefaultBranch,
	ActionRepoMergeMain,
	ActionRepoMutateCrossRepo,
	ActionAgentSpawnPersistent,
	ActionAgentRetire,
	ActionAgentRevoke,
	ActionAgentKeyRotate,
	ActionAgentEscalatePermissions,
	ActionPolicyChange,
	ActionSecretAccess,
	ActionExternalCompanyVoice,
	ActionDataDelete,
	ActionSelfModificationActivate,
	ActionBillingSpendAboveThreshold,
	ActionLicenseChange,
	ActionReleaseCertify,
	ActionCapabilityPromote,
	ActionCapabilityActivate,
	ActionCapabilityRollback,
	ActionRuntimeInvokeExternal,
	ActionMemoryIngestSensitive,
	ActionKnowledgeActivate,
	ActionRepoPullRequestCreate,
}

// RepoProtectedActions are hive-narrower protected actions added under the
// DF-SOP-0001 allowance ("repos may add narrower protected actions") — they
// extend the enforcement set without altering the pinned baseline vocabulary.
var RepoProtectedActions = []ProtectedAction{
	ActionRepoPullRequestMarkReady,
}

var protectedActionSet = func() map[ProtectedAction]bool {
	set := make(map[ProtectedAction]bool, len(ProtectedActions)+len(RepoProtectedActions))
	for _, action := range ProtectedActions {
		set[action] = true
	}
	for _, action := range RepoProtectedActions {
		set[action] = true
	}
	return set
}()

// IsProtectedAction returns true only for the known DF-SOP-0001 protected
// action vocabulary. Unknown actions are not autonomous by default.
func IsProtectedAction(action ProtectedAction) bool {
	return protectedActionSet[action]
}

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
	if IsProtectedAction(action) {
		return ApprovalRequired
	}
	return Forbidden
}

// RiskClass returns the conservative Dark Factory risk class for known
// protected actions. Unknown actions are treated as critical for audit wording
// while DefaultOutcome still forbids them.
func RiskClass(action ProtectedAction) string {
	switch action {
	case ActionProductionDeploy,
		ActionRepoPushDefaultBranch,
		ActionRepoMergeMain,
		ActionRepoDelete,
		ActionRepoMutateCrossRepo,
		ActionSecretAccess,
		ActionSelfModificationActivate,
		ActionAgentEscalatePermissions,
		ActionAgentKeyRotate,
		ActionCapabilityActivate,
		ActionCapabilityRollback,
		ActionRuntimeInvokeExternal,
		ActionMemoryIngestSensitive,
		ActionKnowledgeActivate:
		return "critical"
	case ActionRepoCreate,
		ActionAgentSpawnPersistent,
		ActionAgentRetire,
		ActionAgentRevoke,
		ActionPolicyChange,
		ActionExternalCompanyVoice,
		ActionDataDelete,
		ActionBillingSpendAboveThreshold,
		ActionLicenseChange,
		ActionReleaseCertify,
		ActionCapabilityPromote,
		ActionRepoPullRequestCreate:
		return "high"
	default:
		return "critical"
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

// ApprovalAllowsAction returns true only when the approved protected action is
// the same known action being requested. Approval for lifecycle changes, role
// creation, or any other protected action must not leak into self-modification.
func ApprovalAllowsAction(approvedAction, requestedAction ProtectedAction) bool {
	if !IsProtectedAction(approvedAction) || !IsProtectedAction(requestedAction) {
		return false
	}
	return approvedAction == requestedAction
}

// AgentLifecycleOperation names Hive lifecycle operations that map to
// DF-SOP-0001 protected actions.
type AgentLifecycleOperation string

const (
	AgentLifecycleSpawnPersistent     AgentLifecycleOperation = "spawn_persistent"
	AgentLifecycleRetire              AgentLifecycleOperation = "retire"
	AgentLifecycleRevoke              AgentLifecycleOperation = "revoke"
	AgentLifecycleEscalatePermissions AgentLifecycleOperation = "escalate_permissions"
)

// RequiredActionForAgentLifecycleOperation maps lifecycle operations to their
// canonical protected action. Unknown lifecycle operations fail closed.
func RequiredActionForAgentLifecycleOperation(op AgentLifecycleOperation) (ProtectedAction, error) {
	switch op {
	case AgentLifecycleSpawnPersistent:
		return ActionAgentSpawnPersistent, nil
	case AgentLifecycleRetire:
		return ActionAgentRetire, nil
	case AgentLifecycleRevoke:
		return ActionAgentRevoke, nil
	case AgentLifecycleEscalatePermissions:
		return ActionAgentEscalatePermissions, nil
	default:
		return ProtectedAction(""), AuthorityError{Action: ProtectedAction(op), Outcome: Forbidden}
	}
}
