package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	hiveagent "github.com/transpara-ai/agent"
	"github.com/transpara-ai/hive/pkg/resources"
)

type organicGovernanceFixture struct {
	cto, spawner, guardian, allocator *hiveagent.Agent
	spawnerLoop, guardianLoop         *Loop
	allocatorLoop                     *Loop
	resolver                          func(types.ActorID) string
}

func newOrganicGovernanceFixture(t *testing.T) *organicGovernanceFixture {
	t.Helper()
	g := testSharedGraph(t)
	convID := types.MustConversationID("conv_organic_governance_" + strings.ReplaceAll(t.Name(), "/", "_"))
	names := make(map[types.ActorID]string)
	newAgent := func(role string) *hiveagent.Agent {
		a, err := hiveagent.New(context.Background(), hiveagent.Config{
			Role: hiveagent.Role(role), Name: role, Graph: g,
			Provider: newMockProvider("ok"), ConversationID: convID,
		})
		if err != nil {
			t.Fatalf("new %s agent: %v", role, err)
		}
		names[a.ID()] = role
		return a
	}
	f := &organicGovernanceFixture{
		cto: newAgent("cto"), spawner: newAgent("spawner"),
		guardian: newAgent("guardian"), allocator: newAgent("allocator"),
	}
	f.resolver = func(id types.ActorID) string { return names[id] }
	newLoop := func(agent *hiveagent.Agent) *Loop {
		l, err := New(Config{
			Agent: agent, HumanID: humanID(), ActorResolver: f.resolver,
			EnforceOrganicGovernanceCausality: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	f.spawnerLoop = newLoop(f.spawner)
	f.guardianLoop = newLoop(f.guardian)
	reg := resources.NewBudgetRegistry()
	for _, name := range []string{"cto", "spawner", "guardian", "allocator"} {
		reg.Register(name, resources.NewBudget(resources.BudgetConfig{MaxIterations: 100}), 100, "mock")
	}
	allocatorLoop, err := New(Config{
		Agent: f.allocator, HumanID: humanID(), ActorResolver: f.resolver,
		BudgetRegistry: reg, AllowPreAdmissionRoleBudget: true,
		EnforceOrganicGovernanceCausality: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.allocatorLoop = allocatorLoop
	return f
}

func (f *organicGovernanceFixture) emitGap(t *testing.T, missingRole string) event.Event {
	t.Helper()
	if err := f.cto.EmitGapDetected(event.NewGapDetectedContent(
		event.GapCategoryCapability, missingRole, "repeated unresolved dependency failures", event.SeverityLevelSerious,
	)); err != nil {
		t.Fatalf("emit gap: %v", err)
	}
	page, err := f.cto.Graph().Store().ByType(types.MustEventType("hive.gap.detected"), 100, types.None[types.Cursor]())
	if err != nil || len(page.Items()) == 0 {
		t.Fatalf("read gap: %v", err)
	}
	return page.Items()[0]
}

func dependencyOwnerSpawnCommand() *SpawnCommand {
	return &SpawnCommand{
		Name: "dependency-remediation-owner", Model: "haiku",
		WatchPatterns: []string{"work.task.created"}, CanOperate: false, MaxIterations: 40,
		Prompt: "You own dependency remediation evidence, preserve the soul statement, and report bounded findings without operating on files or changing authority.",
		Reason: "the CTO identified a durable dependency-remediation ownership gap",
	}
}

func TestOrganicGovernanceDirectEdgesSurviveInterveningAgentEvaluations(t *testing.T) {
	f := newOrganicGovernanceFixture(t)
	gap := f.emitGap(t, "Dependency Remediation Owner")
	if _, err := f.spawner.Evaluate(context.Background(), "gap", "consider the gap"); err != nil {
		t.Fatal(err)
	}
	cmd := dependencyOwnerSpawnCommand()
	if err := f.spawnerLoop.emitRoleProposed(cmd); err != nil {
		t.Fatal(err)
	}
	if f.spawnerLoop.spawnerState.pendingProposal != cmd.Name {
		t.Fatalf("successful proposal was not synchronously marked pending")
	}
	state, err := f.guardianLoop.readOrganicGovernanceState()
	if err != nil {
		t.Fatal(err)
	}
	if !exactSingleCause(state.proposal, gap.ID()) {
		t.Fatalf("proposal causes = %v, want only %s", state.proposal.Causes(), gap.ID())
	}
	if _, err := f.guardian.Evaluate(context.Background(), "proposal", "review the proposal"); err != nil {
		t.Fatal(err)
	}
	if err := f.guardianLoop.emitRoleApproved(&ApproveCommand{Name: cmd.Name, Reason: "bounded and non-operating"}); err != nil {
		t.Fatal(err)
	}
	state, err = f.allocatorLoop.readOrganicGovernanceState()
	if err != nil {
		t.Fatal(err)
	}
	if !exactSingleCause(state.approval, state.proposal.ID()) {
		t.Fatalf("approval causes = %v, want only %s", state.approval.Causes(), state.proposal.ID())
	}
	if _, err := f.allocator.Evaluate(context.Background(), "approval", "prepare the budget"); err != nil {
		t.Fatal(err)
	}
	budgetCmd := &BudgetCommand{Agent: cmd.Name, Action: "set", Resource: "iterations", Amount: cmd.MaxIterations, Reason: "bounded pre-admission grant"}
	if err := f.allocatorLoop.applyBudgetAdjustment(budgetCmd, 20); err != nil {
		t.Fatal(err)
	}
	state, err = f.allocatorLoop.readOrganicGovernanceState()
	if err != nil {
		t.Fatal(err)
	}
	if !exactSingleCause(state.budget, state.approval.ID()) {
		t.Fatalf("budget causes = %v, want only %s", state.budget.Causes(), state.approval.ID())
	}
}

func TestOrganicGovernanceFreezesEarliestNormalizedGap(t *testing.T) {
	f := newOrganicGovernanceFixture(t)
	first := f.emitGap(t, "Dependency Remediation Owner")
	_ = f.emitGap(t, "dependency_remediation_owner")
	if err := f.spawnerLoop.emitRoleProposed(dependencyOwnerSpawnCommand()); err != nil {
		t.Fatal(err)
	}
	state, err := f.spawnerLoop.readOrganicGovernanceState()
	if err != nil {
		t.Fatal(err)
	}
	if state.gap.ID() != first.ID() || !exactSingleCause(state.proposal, first.ID()) {
		t.Fatalf("frozen gap/proposal = %s/%v, want earliest %s", state.gap.ID(), state.proposal.Causes(), first.ID())
	}
}

func TestOrganicGovernanceRejectsMismatchedAndRepeatedProposalsWithoutEmission(t *testing.T) {
	t.Run("mismatch", func(t *testing.T) {
		f := newOrganicGovernanceFixture(t)
		f.emitGap(t, "Dependency Remediation Owner")
		cmd := dependencyOwnerSpawnCommand()
		cmd.Name = "dependency-remediation-scout"
		if err := f.spawnerLoop.emitRoleProposed(cmd); err == nil {
			t.Fatal("mismatched proposal unexpectedly emitted")
		}
		state, err := f.spawnerLoop.readOrganicGovernanceState()
		if err != nil || state.proposalCount != 0 {
			t.Fatalf("proposal count after rejection = %d, err=%v", state.proposalCount, err)
		}
	})
	for _, tc := range []struct{ name, second string }{{"different-role", "dependency-remediation-scout"}, {"same-role-refinement", "dependency-remediation-owner"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOrganicGovernanceFixture(t)
			f.emitGap(t, "Dependency Remediation Owner")
			if err := f.spawnerLoop.emitRoleProposed(dependencyOwnerSpawnCommand()); err != nil {
				t.Fatal(err)
			}
			second := dependencyOwnerSpawnCommand()
			second.Name = tc.second
			second.Reason = "attempted second proposal"
			if err := f.spawnerLoop.emitRoleProposed(second); err == nil {
				t.Fatal("second proposal unexpectedly emitted")
			}
			state, err := f.spawnerLoop.readOrganicGovernanceState()
			if err != nil || state.proposalCount != 1 {
				t.Fatalf("proposal count = %d, err=%v", state.proposalCount, err)
			}
		})
	}
}

func TestOrganicGovernanceContextCarriesExactProposalAndApprovalTuple(t *testing.T) {
	f := newOrganicGovernanceFixture(t)
	f.emitGap(t, "Dependency Remediation Owner")
	cmd := dependencyOwnerSpawnCommand()
	if err := f.spawnerLoop.emitRoleProposed(cmd); err != nil {
		t.Fatal(err)
	}
	guardianContext, err := f.guardianLoop.enrichOrganicGovernanceObservation("base")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{cmd.Name, cmd.Model, cmd.Prompt, cmd.Reason, "work.task.created", `"can_operate":false`, `"decision_state":"pending"`} {
		if !strings.Contains(guardianContext, want) {
			t.Fatalf("Guardian context missing %q: %s", want, guardianContext)
		}
	}
	if err := f.guardianLoop.emitRoleApproved(&ApproveCommand{Name: cmd.Name, Reason: "approved"}); err != nil {
		t.Fatal(err)
	}
	state, err := f.allocatorLoop.readOrganicGovernanceState()
	if err != nil {
		t.Fatal(err)
	}
	allocatorContext, err := f.allocatorLoop.enrichOrganicGovernanceObservation("base")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{state.proposal.ID().Value(), state.approval.ID().Value(), cmd.Name, `"proposed_iteration_budget":40`, `"budget_state":"absent"`} {
		if !strings.Contains(allocatorContext, want) {
			t.Fatalf("Allocator context missing %q: %s", want, allocatorContext)
		}
	}
}

func TestOrganicGovernanceRejectsWrongSourceAnchor(t *testing.T) {
	f := newOrganicGovernanceFixture(t)
	if err := f.spawner.EmitGapDetected(event.NewGapDetectedContent(
		event.GapCategoryCapability, "Dependency Remediation Owner", "spoofed", event.SeverityLevelSerious,
	)); err != nil {
		t.Fatal(err)
	}
	if err := f.spawnerLoop.emitRoleProposed(dependencyOwnerSpawnCommand()); err == nil || !strings.Contains(err.Error(), "unverified CTO source") {
		t.Fatalf("wrong-source gap result = %v", err)
	}
}
