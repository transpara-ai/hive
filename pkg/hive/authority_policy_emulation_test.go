package hive

import (
	"strings"
	"testing"
	"time"

	v39 "github.com/transpara-ai/eventgraph/go/pkg/darkfactory/v39"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

func TestRecordProtectedActionLocalEmulationReceiptsApprovedPolicyPath(t *testing.T) {
	rt := newIdentityTestRuntime(t)
	rt.approveRequests = true
	req := validProtectedActionLocalEmulationRequest(rt, safety.ActionRepoMergeMain)

	result, err := rt.recordProtectedActionLocalEmulation(req)
	if err != nil {
		t.Fatalf("recordProtectedActionLocalEmulation: %v", err)
	}
	if result.RequestID.IsZero() || result.DecisionEventID.IsZero() ||
		result.PolicyAdapterDecisionID.IsZero() || result.ExecutionReceiptID.IsZero() {
		t.Fatalf("result missing evidence IDs: %#v", result)
	}
	if result.RealSideEffectExecuted || result.RepositoryMutationExecuted {
		t.Fatalf("local emulation reported real side effects: %#v", result)
	}

	requests := authorityRequestsByType[AuthorityRequestRecordedContent](t, rt, EventTypeAuthorityRequestRecorded)
	if len(requests) != 1 {
		t.Fatalf("authority.request.recorded count = %d, want 1", len(requests))
	}
	if requests[0].RequestID != result.RequestID {
		t.Fatalf("request ID = %s, want %s", requests[0].RequestID, result.RequestID)
	}
	if requests[0].ActionName != string(safety.ActionRepoMergeMain) {
		t.Fatalf("request action = %q, want %q", requests[0].ActionName, safety.ActionRepoMergeMain)
	}
	if !containsString(requests[0].Scope, protectedActionEmulationModeSideEffectFreeLocal) {
		t.Fatalf("request scope missing local emulation mode: %#v", requests[0].Scope)
	}

	decisions := authorityRequestsByType[AuthorityDecisionRecordedContent](t, rt, EventTypeAuthorityDecisionRecorded)
	if len(decisions) != 1 {
		t.Fatalf("authority.decision.recorded count = %d, want 1", len(decisions))
	}
	if decisions[0].RequestID != result.RequestID {
		t.Fatalf("decision request ID = %s, want %s", decisions[0].RequestID, result.RequestID)
	}
	if decisions[0].ApprovedAction != string(safety.ActionRepoMergeMain) {
		t.Fatalf("approved action = %q, want %q", decisions[0].ApprovedAction, safety.ActionRepoMergeMain)
	}

	policies := authorityRequestsByType[PolicyEngineAdapterDecisionContent](t, rt, EventTypePolicyEngineAdapterDecision)
	if len(policies) != 1 {
		t.Fatalf("policy.engine.adapter.decision count = %d, want 1", len(policies))
	}
	if policies[0].AuthorityDecisionRef == nil || *policies[0].AuthorityDecisionRef != result.DecisionEventID {
		t.Fatalf("policy authority decision ref = %v, want %s", policies[0].AuthorityDecisionRef, result.DecisionEventID)
	}
	if policies[0].PolicyBundleHash != req.ExpectedPolicyBundleHash {
		t.Fatalf("policy bundle hash = %q, want %q", policies[0].PolicyBundleHash, req.ExpectedPolicyBundleHash)
	}
	if policies[0].CanonicalDecision != policyCanonicalApprovalRequired {
		t.Fatalf("canonical decision = %q, want %q", policies[0].CanonicalDecision, policyCanonicalApprovalRequired)
	}

	receipts := authorityRequestsByType[AuthorityExecutionReceiptContent](t, rt, EventTypeAuthorityExecutionReceipt)
	if len(receipts) != 1 {
		t.Fatalf("authority.execution.receipt count = %d, want 1", len(receipts))
	}
	if receipts[0].RequestID != result.RequestID || receipts[0].DecisionEventID != result.DecisionEventID {
		t.Fatalf("receipt authority refs = (%s, %s), want (%s, %s)", receipts[0].RequestID, receipts[0].DecisionEventID, result.RequestID, result.DecisionEventID)
	}
	if !strings.Contains(receipts[0].Operation, protectedActionEmulationModeSideEffectFreeLocal) {
		t.Fatalf("receipt operation does not record local emulation mode: %q", receipts[0].Operation)
	}
	if receipts[0].TargetStateBefore == "" || receipts[0].TargetStateAfter == "" {
		t.Fatalf("receipt missing before/after state: %#v", receipts[0])
	}
	if receipts[0].ResultStatus != "succeeded" {
		t.Fatalf("receipt result = %q, want succeeded", receipts[0].ResultStatus)
	}
	if !containsString(receipts[0].ProducedResourceIDs, "policy.adapter.decision:"+result.PolicyAdapterDecisionID.Value()) {
		t.Fatalf("receipt missing policy produced resource: %#v", receipts[0].ProducedResourceIDs)
	}

	validateV39PolicyAndReceipt(t, req, result, policies[0])
}

func TestProtectedActionLocalEmulationNegativeTrialsBlockReceipts(t *testing.T) {
	tests := []struct {
		name          string
		approve       bool
		mutate        func(*protectedActionLocalEmulationRequest)
		want          string
		wantRequests  int
		wantDecisions int
		wantPolicies  int
	}{
		{
			name:    "missing policy adapter decision",
			approve: true,
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.PolicyDecision = PolicyEngineAdapterDecisionContent{}
			},
			want: "missing policy adapter decision evidence",
		},
		{
			name:    "missing policy bundle",
			approve: true,
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.PolicyDecision.PolicyBundleID = ""
			},
			want: "missing policy bundle evidence",
		},
		{
			name:    "stale policy bundle hash",
			approve: true,
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.PolicyDecision.PolicyBundleHash = "sha256:stale-policy-bundle"
			},
			want: "stale or mismatched policy_bundle_hash",
		},
		{
			name:    "forbidden policy decision",
			approve: true,
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.PolicyDecision.CanonicalDecision = policyCanonicalForbidden
				req.PolicyDecision.RawDecision = "deny real or emulated protected action"
			},
			want:         "policy canonical decision forbidden",
			wantRequests: 1,
			wantPolicies: 1,
		},
		{
			name:    "missing authority decision",
			approve: false,
			want:    "missing authority decision",
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.PolicyDecision.RawDecision = "approval required but no operator decision exists"
			},
			wantRequests: 1,
		},
		{
			name:    "cross-action approval",
			approve: true,
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.PolicyDecision.ProtectedActionType = string(safety.ActionProductionDeploy)
			},
			want:         "cross-action approval blocked",
			wantRequests: 1,
		},
		{
			name:    "receipt without local emulation",
			approve: true,
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.Mode = ""
			},
			want: "real protected side effects are forbidden",
		},
		{
			name:    "attempted real default branch push",
			approve: true,
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.Action = safety.ActionRepoPushDefaultBranch
				req.Mode = "real_default_branch_push"
			},
			want: "real protected side effects are forbidden",
		},
		{
			name:    "attempted real worktree merge",
			approve: true,
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.Mode = "real_worktree_merge_to_main"
			},
			want: "real protected side effects are forbidden",
		},
		{
			name:    "attempted production deploy",
			approve: true,
			mutate: func(req *protectedActionLocalEmulationRequest) {
				req.Action = safety.ActionProductionDeploy
				req.PolicyDecision.ProtectedActionType = string(safety.ActionProductionDeploy)
			},
			want: "outside the authorized local emulation seam",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := newIdentityTestRuntime(t)
			rt.approveRequests = tc.approve
			req := validProtectedActionLocalEmulationRequest(rt, safety.ActionRepoMergeMain)
			if tc.mutate != nil {
				tc.mutate(&req)
			}

			result, err := rt.recordProtectedActionLocalEmulation(req)
			if err == nil {
				t.Fatal("recordProtectedActionLocalEmulation returned nil error, want block")
			}
			if !strings.Contains(err.Error(), tc.want) || !strings.Contains(result.Blocker, tc.want) {
				t.Fatalf("error/blocker = %q / %q, want substring %q", err, result.Blocker, tc.want)
			}
			if !result.ExecutionReceiptID.IsZero() {
				t.Fatalf("blocked path produced receipt ID: %#v", result)
			}
			if result.RealSideEffectExecuted || result.RepositoryMutationExecuted {
				t.Fatalf("blocked path reported side effects: %#v", result)
			}

			assertEventCount(t, rt, EventTypeAuthorityRequestRecorded, tc.wantRequests)
			assertEventCount(t, rt, EventTypeAuthorityDecisionRecorded, tc.wantDecisions)
			assertEventCount(t, rt, EventTypePolicyEngineAdapterDecision, tc.wantPolicies)
			assertEventCount(t, rt, EventTypeAuthorityExecutionReceipt, 0)
		})
	}
}

func TestProtectedActionLocalEmulationMissingDependenciesBlock(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Runtime)
		want   string
	}{
		{
			name:   "missing audit store",
			mutate: func(rt *Runtime) { rt.store = nil },
			want:   "missing audit store dependency",
		},
		{
			name:   "missing graph",
			mutate: func(rt *Runtime) { rt.factory = nil },
			want:   "missing graph dependency",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newIdentityTestRuntime(t)
			rt.approveRequests = true
			req := validProtectedActionLocalEmulationRequest(rt, safety.ActionRepoMergeMain)
			tc.mutate(rt)

			result, err := rt.recordProtectedActionLocalEmulation(req)
			if err == nil {
				t.Fatal("recordProtectedActionLocalEmulation returned nil error, want block")
			}
			if !strings.Contains(err.Error(), tc.want) || !strings.Contains(result.Blocker, tc.want) {
				t.Fatalf("error/blocker = %q / %q, want substring %q", err, result.Blocker, tc.want)
			}
			if !result.ExecutionReceiptID.IsZero() || result.RealSideEffectExecuted || result.RepositoryMutationExecuted {
				t.Fatalf("missing dependency path produced side-effect evidence: %#v", result)
			}
		})
	}
}

func validProtectedActionLocalEmulationRequest(rt *Runtime, action safety.ProtectedAction) protectedActionLocalEmulationRequest {
	policyBundleHash := "sha256:dark-factory-epic-10-local-policy-bundle"
	target := "repo:transpara-ai/hive"
	return protectedActionLocalEmulationRequest{
		Action:              action,
		RequestingActor:     rt.humanID,
		RequestingRole:      "operator",
		Target:              target,
		Environment:         "local-test",
		Justification:       "prove bounded Epic 10 local emulation evidence without protected side effects",
		RiskSummary:         "critical repository mutation remains blocked outside side-effect-free local emulation",
		Mode:                protectedActionEmulationModeSideEffectFreeLocal,
		TargetStateBefore:   "no repository mutation requested",
		TargetStateAfter:    "local emulation evidence recorded; repository unchanged",
		ProducedResourceIDs: []string{"local-evidence:dark-factory:epic-10:protected-action-emulation"},
		PolicyDecision: PolicyEngineAdapterDecisionContent{
			DecisionID:          "policy-decision-" + strings.ReplaceAll(string(action), ".", "-"),
			AdapterID:           "hive.local-policy-emulator",
			AdapterVersion:      "1.0.0",
			PolicyBundleID:      "dark-factory-epic-10-local-emulation",
			PolicyBundleHash:    policyBundleHash,
			ProtectedActionType: string(action),
			ActorID:             rt.humanID.Value(),
			ResourceRefs:        []string{target, "path:pkg/runner/worktree.go", "path:pkg/runner/runner.go"},
			InputFacts: map[string]any{
				"mode":                     protectedActionEmulationModeSideEffectFreeLocal,
				"protected_action":         string(action),
				"real_side_effect_allowed": false,
			},
			RawDecision:       "approval required; side-effect-free local emulation only",
			CanonicalDecision: policyCanonicalApprovalRequired,
			ReasonCodes:       []string{"protected_action", "local_emulation_only", "no_repository_mutation"},
			LatencyMS:         1,
		},
		ExpectedPolicyBundleHash: policyBundleHash,
	}
}

func validateV39PolicyAndReceipt(t *testing.T, req protectedActionLocalEmulationRequest, result protectedActionLocalEmulationResult, policy PolicyEngineAdapterDecisionContent) {
	t.Helper()

	authorityDecisionRef := result.DecisionEventID.Value()
	policyRecord := &v39.PolicyEngineAdapterDecision{
		CommonNode: v39CommonNode("padc_"+result.PolicyAdapterDecisionID.Value(), v39.TypePolicyEngineAdapterDecision, policy.ActorID),
		DecisionID: policy.DecisionID, AdapterID: policy.AdapterID, AdapterVersion: policy.AdapterVersion,
		PolicyBundleID: policy.PolicyBundleID, PolicyBundleHash: policy.PolicyBundleHash,
		ProtectedActionType: policy.ProtectedActionType, ActorID: policy.ActorID, ResourceRefs: policy.ResourceRefs,
		InputFacts: policy.InputFacts, RawDecision: policy.RawDecision, CanonicalDecision: policy.CanonicalDecision,
		ReasonCodes: policy.ReasonCodes, EvidenceRefs: policy.EvidenceRefs, LatencyMS: policy.LatencyMS,
		AuthorityDecisionRef: &authorityDecisionRef,
	}
	if err := policyRecord.Validate(); err != nil {
		t.Fatalf("v39 PolicyEngineAdapterDecision validation: %v", err)
	}

	receiptRecord := &v39.ExecutionReceipt{
		CommonNode:          v39CommonNode("exec_"+result.ExecutionReceiptID.Value(), v39.TypeExecutionReceipt, policy.ActorID),
		AuthorityDecisionID: result.DecisionEventID.Value(),
		Action:              string(req.Action),
		TargetID:            req.Target,
		Result:              "succeeded",
		EvidenceRefs:        []string{result.RequestID.Value(), result.DecisionEventID.Value(), result.PolicyAdapterDecisionID.Value()},
	}
	if err := receiptRecord.Validate(); err != nil {
		t.Fatalf("v39 ExecutionReceipt validation: %v", err)
	}
}

func v39CommonNode(id, typ, createdBy string) v39.CommonNode {
	return v39.CommonNode{
		ID:             id,
		Type:           typ,
		CreatedAt:      time.Now().UTC(),
		CreatedBy:      createdBy,
		IdempotencyKey: "idem-" + id,
		CorrelationID:  "corr-dark-factory-epic-10",
	}
}

func assertEventCount(t *testing.T, rt *Runtime, eventType types.EventType, want int) {
	t.Helper()
	page, err := rt.store.ByType(eventType, 100, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("ByType(%s): %v", eventType, err)
	}
	if len(page.Items()) != want {
		t.Fatalf("%s count = %d, want %d", eventType, len(page.Items()), want)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
