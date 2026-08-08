package factoryv1

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fixedClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	value := c.now
	c.now = c.now.Add(time.Millisecond)
	return value
}

func testClock() *fixedClock {
	return &fixedClock{now: time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC)}
}

func testOrder(id string, channel Channel) FactoryOrder {
	return FactoryOrder{
		DocID: id, Version: "1.0.0", Status: "approved", Title: "Test " + id,
		Channel: channel, TargetRepository: "transpara-ai/hive",
		SourceReferences:   []SourceReference{{Kind: "test", Identity: "source:" + id, URI: "test://" + id, SHA256: strings.Repeat("a", 64)}},
		Requirements:       []Requirement{{ID: "R1", Statement: "Deliver a bounded change.", Rationale: "Exercise the factory."}},
		AcceptanceCriteria: []AcceptanceCriterion{{ID: "AC1", Statement: "The bounded change passes.", VerificationMethod: "named test", RiskClass: "low"}},
		TestPlan:           []string{"run named test"}, Constraints: []string{"non-production"}, NonGoals: []string{"deployment"}, ExpectedOutputs: []string{"ready pull request"},
		Authority: AuthorityScope{ActorID: "human-actor-1", AllowedActions: []string{"branch", "pull_request"}, TargetRepositories: []string{"transpara-ai/hive"}, NonProductionOnly: true},
		Budget:    BudgetLimit{MaxAttempts: 40, MaxTokens: 1_000_000, MaxCostMicros: 1_000_000},
	}
}

func TestFactoryOrderResolvedIssuesValidationAndCanonicalization(t *testing.T) {
	t.Parallel()
	order := testOrder("FO-RESOLVED-ISSUES", ChannelCompletedOrder)
	withoutIssues, err := Canonicalize(order)
	if err != nil {
		t.Fatal(err)
	}
	order.ResolvedIssues = []ResolvedIssue{
		{Repository: "transpara-ai/docs", Number: 286, Title: "First issue", URI: "https://github.com/transpara-ai/docs/issues/286"},
		{Repository: "transpara-ai/hive", Number: 297, Title: "Second issue", URI: "https://github.com/transpara-ai/hive/issues/297"},
	}
	withIssues, err := Canonicalize(order)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Resolved GitHub issues",
		"- transpara-ai/docs#286 | https://github.com/transpara-ai/docs/issues/286 | First issue",
		"- transpara-ai/hive#297 | https://github.com/transpara-ai/hive/issues/297 | Second issue",
	} {
		if !strings.Contains(withIssues.Markdown, want) {
			t.Fatalf("canonical markdown missing %q:\n%s", want, withIssues.Markdown)
		}
	}
	if strings.Contains(withoutIssues.Markdown, "Resolved GitHub issues") {
		t.Fatalf("empty resolved issues changed legacy canonical markdown:\n%s", withoutIssues.Markdown)
	}

	invalid := []ResolvedIssue{
		{Repository: "docs", Number: 286, Title: "bad repository", URI: "https://github.com/docs/issues/286"},
		{Repository: "transpara-ai/docs", Number: 0, Title: "bad number", URI: "https://github.com/transpara-ai/docs/issues/0"},
		{Repository: "transpara-ai/docs", Number: 286, Title: "", URI: "https://github.com/transpara-ai/docs/issues/286"},
		{Repository: "transpara-ai/docs", Number: 286, Title: "bad URI", URI: "https://example.com/transpara-ai/docs/issues/286"},
	}
	for i, issue := range invalid {
		candidate := testOrder("FO-INVALID-RESOLVED", ChannelCompletedOrder)
		candidate.ResolvedIssues = []ResolvedIssue{issue}
		if err := ValidateFactoryOrder(candidate); err == nil {
			t.Fatalf("invalid resolved issue %d accepted: %+v", i, issue)
		}
	}
	duplicate := testOrder("FO-DUPLICATE-RESOLVED", ChannelCompletedOrder)
	duplicate.ResolvedIssues = []ResolvedIssue{
		{Repository: "transpara-ai/docs", Number: 286, Title: "first", URI: "https://github.com/transpara-ai/docs/issues/286"},
		{Repository: "Transpara-ai/docs", Number: 286, Title: "case-variant duplicate", URI: "https://github.com/Transpara-ai/docs/issues/286"},
	}
	if err := ValidateFactoryOrder(duplicate); err == nil {
		t.Fatal("duplicate resolved issue accepted")
	}
}

func TestFactoryOrderRequiresPositiveTokenAndCostBudgets(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*FactoryOrder)
	}{
		{name: "zero tokens", mutate: func(order *FactoryOrder) { order.Budget.MaxTokens = 0 }},
		{name: "negative tokens", mutate: func(order *FactoryOrder) { order.Budget.MaxTokens = -1 }},
		{name: "zero cost", mutate: func(order *FactoryOrder) { order.Budget.MaxCostMicros = 0 }},
		{name: "negative cost", mutate: func(order *FactoryOrder) { order.Budget.MaxCostMicros = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			order := testOrder("FO-STRICT-BUDGET", ChannelCompletedOrder)
			test.mutate(&order)
			if err := ValidateFactoryOrder(order); err == nil || !strings.Contains(err.Error(), "must be positive") {
				t.Fatalf("validation error = %v, want positive token/cost limit failure", err)
			}
		})
	}
	historical := deriveBudget(BudgetLimit{MaxAttempts: 1}, nil)
	if !historical.Exhausted || historical.RemainingTokens != 0 || historical.RemainingCostMicros != 0 {
		t.Fatalf("historical zero budget projected as unlimited: %+v", historical)
	}
}

func TestFactoryV1GateStateUsesLatestRequiredGateTruth(t *testing.T) {
	t.Parallel()
	zero := 0
	passed := func(stage Stage) StageTransitionPayload {
		evidence := Evidence{Kind: "review", Reference: "test:" + string(stage), BlockerCount: &zero}
		switch stage {
		case StageIADA, StageCFADA:
			evidence.DesignBlobSHA = strings.Repeat("a", 40)
		case StageIAR, StageCFAR:
			evidence.PRHeadSHA = strings.Repeat("b", 40)
			evidence.ReviewedHeadSHA = evidence.PRHeadSHA
		}
		if stage == StageCFADA || stage == StageCFAR {
			evidence.AuthorFamily = "OpenAI/Codex"
			evidence.ReviewerFamily = "Anthropic/Claude"
		}
		return StageTransitionPayload{Stage: stage, State: TransitionPassed, Evidence: []Evidence{evidence}}
	}
	state := func(stage Stage, value TransitionState) StageTransitionPayload {
		return StageTransitionPayload{Stage: stage, State: value}
	}
	allPassed := []StageTransitionPayload{passed(StageIADA), passed(StageCFADA), passed(StageIAR), passed(StageCFAR)}
	tests := []struct {
		name        string
		transitions []StageTransitionPayload
		want        string
	}{
		{name: "none", want: "unavailable"},
		{name: "one valid pass", transitions: []StageTransitionPayload{passed(StageIADA)}, want: "current_gate_passed_later_gates_pending"},
		{name: "all valid passes", transitions: allPassed, want: "all_required_gates_passed"},
		{name: "running outranks partial", transitions: []StageTransitionPayload{passed(StageIADA), state(StageCFADA, TransitionRunning)}, want: "running"},
		{name: "furthest human required", transitions: []StageTransitionPayload{state(StageIADA, TransitionBlocked), state(StageCFADA, TransitionHumanRequired)}, want: "human_required"},
		{name: "furthest blocked", transitions: []StageTransitionPayload{state(StageIADA, TransitionHumanRequired), state(StageCFADA, TransitionBlocked)}, want: "blocked"},
		{name: "later repair replaces block", transitions: []StageTransitionPayload{state(StageIADA, TransitionBlocked), passed(StageIADA)}, want: "current_gate_passed_later_gates_pending"},
		{name: "later block replaces pass", transitions: []StageTransitionPayload{passed(StageIADA), state(StageIADA, TransitionBlocked)}, want: "blocked"},
		{name: "invalid pass", transitions: []StageTransitionPayload{{Stage: StageIADA, State: TransitionPassed, Evidence: []Evidence{{Kind: "review", Reference: "missing exact blob", BlockerCount: &zero}}}}, want: "unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := deriveGateState(test.transitions); got != test.want {
				t.Fatalf("gate state = %q, want %q", got, test.want)
			}
		})
	}
}

func testProvider() ProviderBinding {
	return ProviderBinding{
		ProviderID: "claude-cli", Family: "Claude/Anthropic", ExecutableRealpath: "/usr/bin/claude",
		ExecutableSHA256: strings.Repeat("d", 64), ModelID: "claude-sonnet-test", CredentialSourceID: "oauth:test",
	}
}

func testSchedulerConfig() SchedulerConfig {
	providers := make(map[Stage]ProviderBinding, len(TLCStages))
	for _, stage := range TLCStages {
		providers[stage] = testProvider()
	}
	return SchedulerConfig{
		WorkerCount: 3, StageProviders: providers, AuthorFamily: "Codex/OpenAI",
		RepositoryRoot: func(order FactoryOrder) string { return "/work/" + order.DocID },
		StandingApproval: &StandingApprovalBinding{
			ActorID: "human-actor-1", CredentialKeyID: "operator-key-1", SourceSHA256: strings.Repeat("a", 64),
			FactoryOrderBlobSHA: strings.Repeat("b", 64), ApprovalSentence: "The Human approves this exact non-production v1 demonstration scope.",
			ApprovalSourceEventID: "human-source-event-1",
		},
	}
}

type orderingWorkStore struct {
	*InMemoryWorkStore
	events Store
	t      *testing.T
}

func (s *orderingWorkStore) SeedFactoryOrder(ctx context.Context, seed WorkSeed) (WorkLink, error) {
	events, err := s.events.List(ctx)
	if err != nil {
		return WorkLink{}, err
	}
	found := false
	for _, event := range events {
		if event.ID == seed.AcceptedEventID && event.Type == EventOrderAccepted {
			found = true
		}
	}
	if !found {
		s.t.Error("Work seed occurred before accepted EventGraph event committed")
	}
	return s.InMemoryWorkStore.SeedFactoryOrder(ctx, seed)
}

func TestFactoryV1AllChannelsCanonicalize(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := &orderingWorkStore{InMemoryWorkStore: NewInMemoryWorkStore(), events: events, t: t}
	intake, err := NewIntake(events, work, clock)
	if err != nil {
		t.Fatal(err)
	}

	issue, err := intake.NormalizeIssue(ctx, IssueAdmission{
		LaunchEventID: "factory-run-requested-1", Repository: "transpara-ai/hive", IssueNumber: 101,
		Title: "Bounded issue", Body: "Implement the accepted slice.", Order: testOrder("FO-ISSUE", ChannelIssueScan), ActorID: "scanner-actor",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(issue.Document.Order.ResolvedIssues) != 1 {
		t.Fatalf("issue scan resolved issues = %+v, want one", issue.Document.Order.ResolvedIssues)
	}
	resolved := issue.Document.Order.ResolvedIssues[0]
	if resolved.Repository != "transpara-ai/hive" || resolved.Number != 101 || resolved.Title != "Bounded issue" || resolved.URI != "https://github.com/transpara-ai/hive/issues/101" {
		t.Fatalf("issue scan resolved issue = %+v", resolved)
	}
	if !strings.Contains(issue.Document.Markdown, "- transpara-ai/hive#101 | https://github.com/transpara-ai/hive/issues/101 | Bounded issue") {
		t.Fatalf("issue scan canonical markdown lacks resolved issue:\n%s", issue.Document.Markdown)
	}
	ideaOrder := testOrder("FO-IDEA", ChannelHumanIdea)
	if _, err := intake.RecordIdea(ctx, IdeaInput{IdeaID: "idea-1", Note: "initial", Candidate: ideaOrder, ActorID: "human-actor-1"}); err != nil {
		t.Fatal(err)
	}
	ideaOrder.Title = "Refined bounded idea"
	if _, err := intake.RefineIdea(ctx, IdeaInput{IdeaID: "idea-1", Note: "refinement", Candidate: ideaOrder, ActorID: "human-actor-1"}); err != nil {
		t.Fatal(err)
	}
	idea, err := intake.SubmitIdea(ctx, "idea-1", true, "human-actor-1", "operator-key-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	completedOrder := testOrder("FO-COMPLETED", ChannelCompletedOrder)
	completed, err := intake.SubmitCompleted(ctx, completedOrder, "human-actor-1", "operator-key-1", nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, receipt := range []AcceptanceReceipt{issue, idea, completed} {
		if receipt.AcceptedEventID == "" || receipt.Work.TaskID == "" || receipt.Work.ArtifactID == "" {
			t.Fatalf("channel %s did not produce both accepted event and Work linkage: %+v", receipt.Channel, receipt)
		}
		if receipt.Work.DocumentSHA256 != receipt.DocumentSHA256 || HashText(receipt.Document.Markdown) != receipt.DocumentSHA256 {
			t.Fatalf("channel %s diverged from canonical markdown truth", receipt.Channel)
		}
	}
	listed, err := events.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accepted := 0
	for _, event := range listed {
		if event.Type == EventOrderAccepted {
			accepted++
			payload, decodeErr := decodeEvent[OrderAcceptedPayload](event)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if payload.Document.Markdown == "" || payload.Document.SHA256 == "" || len(payload.SourceEventIDs) == 0 {
				t.Fatalf("accepted event lacks canonical fields: %+v", payload)
			}
		}
	}
	if accepted != 3 {
		t.Fatalf("accepted event count = %d, want 3", accepted)
	}

	// A byte-identical retry is idempotent; a conflicting immutable tuple fails closed.
	again, err := intake.SubmitCompleted(ctx, completedOrder, "human-actor-1", "operator-key-1", nil)
	if err != nil || again.AcceptedEventID != completed.AcceptedEventID {
		t.Fatalf("idempotent retry = (%s, %v), want original %s", again.AcceptedEventID, err, completed.AcceptedEventID)
	}
	completedOrder.Title = "Conflicting title"
	if _, err := intake.AcceptCompleted(ctx, completedOrder, AcceptOptions{SourceIdentity: "file:completed-order", SourceEventIDs: []string{"upload-event-2"}, ActorID: "human-actor-1"}); !errors.Is(err, ErrAcceptedTupleConflict) {
		t.Fatalf("conflicting tuple error = %v, want %v", err, ErrAcceptedTupleConflict)
	}
}

func TestFactoryV1CompletedOrderAdmission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	intake, _ := NewIntake(events, work, clock)
	receipt, err := intake.SubmitCompleted(ctx, testOrder("FO-DIRECT", ChannelCompletedOrder), "human-actor-1", "operator-key-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	listed, _ := events.List(ctx)
	var submitted, accepted Event
	for _, event := range listed {
		switch event.Type {
		case EventOrderSubmitted:
			submitted = event
		case EventOrderAccepted:
			accepted = event
		}
	}
	if submitted.ID == "" || accepted.ID != receipt.AcceptedEventID || len(accepted.Causes) != 1 || accepted.Causes[0] != submitted.ID {
		t.Fatalf("direct submission causality is incomplete: submitted=%+v accepted=%+v", submitted, accepted)
	}
}

type concurrencyRunner struct {
	provider ProviderBinding
	entered  atomic.Int32
	peak     atomic.Int32
	release  chan struct{}
}

func (r *concurrencyRunner) Execute(ctx context.Context, request RunRequest) (RunResult, error) {
	active := r.entered.Add(1)
	for {
		peak := r.peak.Load()
		if active <= peak || r.peak.CompareAndSwap(peak, active) {
			break
		}
	}
	select {
	case <-ctx.Done():
		return RunResult{}, ctx.Err()
	case <-r.release:
	}
	r.entered.Add(-1)
	return RunResult{Status: RunnerBlocked, Evidence: []Evidence{{Kind: "bounded_test_wait", Reference: request.AttemptID}}, Blocker: "deliberate bounded wait", NextAction: "resolve test intervention", Provider: r.provider}, nil
}

func (r *concurrencyRunner) Reconcile(context.Context, RunRequest) (ReconcileResult, error) {
	return ReconcileResult{}, errors.New("unexpected reconcile")
}

func TestFactoryV1SchedulesThreeConcurrently(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	intake, _ := NewIntake(events, work, clock)
	for n := 1; n <= 3; n++ {
		id := "FO-CONCURRENT-" + string(rune('0'+n))
		if _, err := intake.AcceptCompleted(ctx, testOrder(id, ChannelCompletedOrder), AcceptOptions{SourceIdentity: "test:" + id, SourceEventIDs: []string{"source:" + id}, ActorID: "human-actor-1"}); err != nil {
			t.Fatal(err)
		}
	}
	runner := &concurrencyRunner{provider: testProvider(), release: make(chan struct{})}
	scheduler, err := NewScheduler(events, work, runner, clock, testSchedulerConfig())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- scheduler.RunOnce(ctx) }()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for runner.peak.Load() < 3 {
		select {
		case <-deadline.C:
			t.Fatalf("peak active executions = %d, want 3", runner.peak.Load())
		case <-time.After(time.Millisecond):
		}
	}
	if scheduler.ActiveWorkers() != 3 {
		t.Fatalf("active scheduler workers = %d, want 3", scheduler.ActiveWorkers())
	}
	runtime := scheduler.RuntimeSnapshot()
	if runtime.ConfiguredWorkers != 3 || runtime.ActiveWorkers != 3 || runtime.AvailableWorkers != 0 || runtime.QueuedOrders != 3 || runtime.SchedulableOrders != 3 || len(runtime.Assignments) != 3 {
		t.Fatalf("HIVE-MC-T2 runtime snapshot = %+v", runtime)
	}
	for _, assignment := range runtime.Assignments {
		if assignment.Stage != StageIngestWork || assignment.AttemptID == "" || assignment.ProviderID != runner.provider.ProviderID || assignment.ModelID != runner.provider.ModelID || len(assignment.DocumentSHA256) != 64 || assignment.AssignedAt.IsZero() {
			t.Fatalf("HIVE-MC-T2 assignment is incomplete: %+v", assignment)
		}
	}
	close(runner.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if runner.peak.Load() != 3 {
		t.Fatalf("peak concurrent runner calls = %d, want 3", runner.peak.Load())
	}
	if scheduler.ActiveWorkers() != 0 {
		t.Fatal("blocked orders did not release worker slots")
	}
	released := scheduler.RuntimeSnapshot()
	if released.ActiveWorkers != 0 || released.AvailableWorkers != 3 || len(released.Assignments) != 0 || released.Sequence <= runtime.Sequence {
		t.Fatalf("HIVE-MC-T2 released runtime snapshot = %+v; prior sequence=%d", released, runtime.Sequence)
	}
}

type lifecycleRunner struct {
	provider ProviderBinding
}

func (r *lifecycleRunner) Execute(_ context.Context, request RunRequest) (RunResult, error) {
	result := RunResult{Status: RunnerPassed, Provider: r.provider, Evidence: []Evidence{{Kind: "stage_receipt", Reference: "evidence:" + string(request.Stage) + ":" + request.AttemptID}}}
	zero := 0
	switch request.Stage {
	case StageDesign:
		result.Evidence = []Evidence{{Kind: "design", Reference: "docs/design.md", DesignBlobSHA: strings.Repeat("1", 40)}}
	case StageIADA, StageIAR:
		item := Evidence{Kind: "gate", Reference: "gate:" + string(request.Stage), BlockerCount: &zero, AuthorFamily: "Codex/OpenAI", ReviewerFamily: "Codex/OpenAI"}
		if request.Stage == StageIADA {
			item.DesignBlobSHA = strings.Repeat("1", 40)
		} else {
			item.PRHeadSHA, item.ReviewedHeadSHA = strings.Repeat("c", 40), strings.Repeat("c", 40)
		}
		result.Evidence = []Evidence{item}
	case StageCFADA, StageCFAR:
		provider := request.Provider
		item := Evidence{Kind: "cross_family_gate", Reference: "gate:" + string(request.Stage), BlockerCount: &zero, AuthorFamily: "Codex/OpenAI", ReviewerFamily: provider.Family, Provider: &provider}
		if request.Stage == StageCFADA {
			item.DesignBlobSHA = strings.Repeat("1", 40)
		} else {
			item.PRHeadSHA, item.ReviewedHeadSHA = strings.Repeat("c", 40), strings.Repeat("c", 40)
		}
		result.Evidence = []Evidence{item}
	case StageHumanDesignReview:
		result.Evidence = []Evidence{{Kind: "human_approval", Reference: "approval:event-1", Approval: &HumanApprovalReceipt{
			Basis: ApprovalStandingScoped, ActorID: "human-actor-1", CredentialKeyID: "operator-key-1",
			SourceSHA256: strings.Repeat("a", 64), FactoryOrderBlobSHA: strings.Repeat("b", 64),
			OrderID: request.Order.DocID, OrderVersion: request.Order.Version, DocumentSHA256: request.DocumentSHA256,
			ApprovalSentence: "The Human approves this exact non-production v1 demonstration scope.", ApprovalSourceEventID: "human-source-event-1",
			IssuedAt: time.Date(2026, 8, 4, 19, 0, 0, 0, time.UTC),
		}}}
	case StageWriteCode:
		result.Evidence = []Evidence{{Kind: "code", Reference: "branch:factory/fo-tlc", PRHeadSHA: strings.Repeat("c", 40), Metadata: map[string]string{"branch": "factory/fo-tlc", "tests_passing": "true"}}}
	case StageCreateDraftPR:
		result.Evidence = []Evidence{{Kind: "draft_pr", Reference: "https://github.com/transpara-ai/hive/pull/999", PR: &PREvidence{Repository: "transpara-ai/hive", Number: 999, URL: "https://github.com/transpara-ai/hive/pull/999", HeadSHA: strings.Repeat("c", 40), Open: true, Draft: true}}}
	case StageMarkPRReady:
		head := strings.Repeat("c", 40)
		result.Evidence = []Evidence{{Kind: "ready_pr", Reference: "https://github.com/transpara-ai/hive/pull/999", PR: &PREvidence{Repository: "transpara-ai/hive", Number: 999, URL: "https://github.com/transpara-ai/hive/pull/999", HeadSHA: head, ReviewedHeadSHA: head, Open: true, Draft: false, ChecksPassing: true}}}
	case StageHumanReview:
		result.Status = RunnerHumanRequired
		result.Evidence = []Evidence{{Kind: "human_review_boundary", Reference: "https://github.com/transpara-ai/hive/pull/999"}}
		result.NextAction = "Human reviews pull request 999"
	}
	return result, nil
}

func (r *lifecycleRunner) Reconcile(_ context.Context, request RunRequest) (ReconcileResult, error) {
	result, err := r.Execute(context.Background(), request)
	return ReconcileResult{EffectExists: true, Result: result}, err
}

func TestFactoryV1TLCOrderAndEvidence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	intake, _ := NewIntake(events, work, clock)
	receipt, err := intake.AcceptCompleted(ctx, testOrder("FO-TLC", ChannelCompletedOrder), AcceptOptions{SourceIdentity: "test:tlc", SourceEventIDs: []string{"source:tlc"}, ActorID: "human-actor-1"})
	if err != nil {
		t.Fatal(err)
	}
	runner := &lifecycleRunner{provider: testProvider()}
	scheduler, err := NewScheduler(events, work, runner, clock, testSchedulerConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	listed, _ := events.List(ctx)
	transitions, _, _, err := orderTransitions(listed, receipt.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != len(TLCStages)*2 {
		t.Fatalf("transition count = %d, want %d", len(transitions), len(TLCStages)*2)
	}
	for index, stage := range TLCStages {
		running, terminal := transitions[index*2], transitions[index*2+1]
		if running.Stage != stage || terminal.Stage != stage || running.State != TransitionRunning || len(terminal.Evidence) == 0 {
			t.Fatalf("stage %d ledger mismatch: running=%+v terminal=%+v", index, running, terminal)
		}
		if terminal.AttemptID != running.AttemptID {
			t.Fatalf("stage %s terminal attempt differs from running attempt", stage)
		}
	}
	projector, _ := NewProjector(events, work, clock, ServiceProjection{InstanceID: "test-instance", Healthy: true})
	projection, err := projector.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order, err := projection.Order(receipt.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != "human_review" || order.TLCStage != StageHumanReview || order.PR == nil || !order.PR.ChecksPassing {
		t.Fatalf("terminal projection is not exact-head Human Review: %+v", order)
	}
}

func TestStandingApprovalAcceptsGovernedGitBlobIdentity(t *testing.T) {
	document, err := Canonicalize(testOrder("FO-GIT-BLOB-APPROVAL", ChannelCompletedOrder))
	if err != nil {
		t.Fatal(err)
	}
	binding := StandingApprovalBinding{
		ActorID: "human-actor-1", CredentialKeyID: "operator-key-1",
		SourceSHA256: strings.Repeat("a", 64), FactoryOrderBlobSHA: strings.Repeat("b", 40),
		ApprovalSentence: "Human approved the exact governed source blob.", ApprovalSourceEventID: "human-source-event-1",
	}
	receipt, err := binding.Bind(document, time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("bind 40-hex Git blob identity: %v", err)
	}
	if receipt.FactoryOrderBlobSHA != strings.Repeat("b", 40) {
		t.Fatalf("FactoryOrder blob identity = %q", receipt.FactoryOrderBlobSHA)
	}
	if err := ValidateApprovalReceipt(document, receipt); err != nil {
		t.Fatalf("validate 40-hex Git blob identity: %v", err)
	}
	receipt.FactoryOrderBlobSHA = strings.Repeat("b", 39)
	if err := ValidateApprovalReceipt(document, receipt); err == nil {
		t.Fatal("39-hex FactoryOrder blob identity was accepted")
	}
}

type interventionRunner struct {
	provider ProviderBinding
	mu       sync.Mutex
	byStage  map[Stage]int
}

func (r *interventionRunner) Execute(_ context.Context, request RunRequest) (RunResult, error) {
	r.mu.Lock()
	r.byStage[request.Stage]++
	count := r.byStage[request.Stage]
	r.mu.Unlock()
	if request.Stage == StageIngestWork && count == 1 {
		return RunResult{Status: RunnerBlocked, Provider: r.provider, Evidence: []Evidence{{Kind: "blocker", Reference: "bounded:blocker"}}, Blocker: "bounded test blocker", NextAction: "Human acknowledges bounded blocker"}, nil
	}
	if request.Stage == StageCraftFactoryOrder {
		return RunResult{Status: RunnerBlocked, Provider: r.provider, Evidence: []Evidence{{Kind: "stop", Reference: "bounded:stop"}}, Blocker: "stop after resume", NextAction: "no action in unit test"}, nil
	}
	return RunResult{Status: RunnerPassed, Provider: r.provider, Evidence: []Evidence{{Kind: "pass", Reference: request.AttemptID}}}, nil
}

func (r *interventionRunner) Reconcile(context.Context, RunRequest) (ReconcileResult, error) {
	return ReconcileResult{}, errors.New("unexpected reconcile")
}

func TestFactoryV1InterventionResume(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	intake, _ := NewIntake(events, work, clock)
	receipt, err := intake.AcceptCompleted(ctx, testOrder("FO-INTERVENTION", ChannelCompletedOrder), AcceptOptions{SourceIdentity: "test:intervention", SourceEventIDs: []string{"source:intervention"}, ActorID: "human-actor-1"})
	if err != nil {
		t.Fatal(err)
	}
	runner := &interventionRunner{provider: testProvider(), byStage: make(map[Stage]int)}
	scheduler, _ := NewScheduler(events, work, runner, clock, testSchedulerConfig())
	if err := scheduler.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	projection, _ := NewProjector(events, work, clock, ServiceProjection{Healthy: true})
	before, err := projection.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	orderBefore, _ := before.Order(receipt.OrderID)
	if orderBefore.Status != "human_required" || len(before.Interventions) != 1 || before.Interventions[0].Status != InterventionOpen {
		t.Fatalf("bounded intervention not visible: order=%+v interventions=%+v", orderBefore, before.Interventions)
	}
	resolution, err := ResolveIntervention(ctx, events, clock, InterventionResolution{InterventionID: before.Interventions[0].InterventionID, Resolution: "resume this exact order", ActorID: "human-actor-1", CredentialKeyID: "operator-key-1"})
	if err != nil || resolution.Type != EventInterventionResolved {
		t.Fatalf("resolution = (%+v, %v)", resolution, err)
	}
	if err := scheduler.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	listed, _ := events.List(ctx)
	transitions, _, _, _ := orderTransitions(listed, receipt.OrderID)
	var ingestPassed bool
	var resumedRunningCitesResolution bool
	for _, transition := range transitions {
		if transition.Stage == StageIngestWork && transition.State == TransitionPassed {
			ingestPassed = true
			if transition.Ordinal != 2 {
				t.Fatalf("resumed ordinal = %d, want 2", transition.Ordinal)
			}
		}
	}
	for _, event := range listed {
		if event.Type != EventStageTransitioned {
			continue
		}
		transition, _ := decodeEvent[StageTransitionPayload](event)
		if transition.Stage == StageIngestWork && transition.State == TransitionRunning && transition.Ordinal == 2 && contains(event.Causes, resolution.ID) {
			resumedRunningCitesResolution = true
		}
	}
	if !ingestPassed {
		t.Fatal("resolved intervention did not resume the waiting order")
	}
	if !resumedRunningCitesResolution {
		t.Fatal("resumed running transition is not causally linked to the Human resolution event")
	}
}

type recoveryRunner struct {
	provider       ProviderBinding
	executeByStage map[Stage]int
	reconciles     int
}

func (r *recoveryRunner) Execute(_ context.Context, request RunRequest) (RunResult, error) {
	r.executeByStage[request.Stage]++
	return RunResult{Status: RunnerBlocked, Provider: r.provider, Evidence: []Evidence{{Kind: "bounded_stop", Reference: request.AttemptID}}, Blocker: "stop after recovery", NextAction: "unit test complete"}, nil
}

func (r *recoveryRunner) Reconcile(_ context.Context, request RunRequest) (ReconcileResult, error) {
	r.reconciles++
	return ReconcileResult{EffectExists: true, Result: RunResult{Status: RunnerPassed, Provider: r.provider, Evidence: []Evidence{{Kind: "reconciled_effect", Reference: request.AttemptID}}}}, nil
}

func TestFactoryV1RestartRecoversWithoutDuplicateEffect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	intake, _ := NewIntake(events, work, clock)
	receipt, err := intake.AcceptCompleted(ctx, testOrder("FO-RECOVERY", ChannelCompletedOrder), AcceptOptions{SourceIdentity: "test:recovery", SourceEventIDs: []string{"source:recovery"}, ActorID: "human-actor-1"})
	if err != nil {
		t.Fatal(err)
	}
	attempt, _ := AttemptID(receipt.DocumentSHA256, StageIngestWork, 1)
	running := StageTransitionPayload{TLCVersion: TLCVersion, Stage: StageIngestWork, StageIndex: 0, State: TransitionRunning, AttemptID: attempt, Ordinal: 1, Peers: PeersForStage(StageIngestWork), Runner: testProvider()}
	runningEvent, err := AppendTyped(ctx, events, EventStageTransitioned, receipt.OrderID, "stage-running:"+receipt.OrderID+":"+attempt, []string{receipt.AcceptedEventID}, running)
	if err != nil {
		t.Fatal(err)
	}
	firstRecovery, err := AppendTyped(ctx, events, EventRecoveryRecorded, receipt.OrderID, "recovery:"+receipt.OrderID+":"+attempt+":1", []string{runningEvent.ID}, RecoveryPayload{
		OrderID: receipt.OrderID, Stage: StageIngestWork, AttemptID: attempt, Observation: 1,
		EffectFound: false, Evidence: []Evidence{{Kind: "effect_absent", Reference: attempt}}, RecoveredFrom: "running_without_terminal", Result: RunnerBlocked,
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recoveryRunner{provider: testProvider(), executeByStage: make(map[Stage]int)}
	scheduler, _ := NewScheduler(events, work, runner, clock, testSchedulerConfig())
	if err := scheduler.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if runner.reconciles != 1 || runner.executeByStage[StageIngestWork] != 0 {
		t.Fatalf("reconcile=%d ingest executes=%d, want 1 and 0", runner.reconciles, runner.executeByStage[StageIngestWork])
	}
	listed, _ := events.List(ctx)
	recoveryCount, ingestTerminal := 0, 0
	secondRecoveryCitesFirst := false
	for _, event := range listed {
		if event.Type == EventRecoveryRecorded {
			recoveryCount++
			payload, _ := decodeEvent[RecoveryPayload](event)
			if payload.Observation == 2 && contains(event.Causes, firstRecovery.ID) {
				secondRecoveryCitesFirst = true
			}
		}
		if event.Type == EventStageTransitioned {
			transition, _ := decodeEvent[StageTransitionPayload](event)
			if transition.Stage == StageIngestWork && transition.State == TransitionPassed {
				ingestTerminal++
				if transition.AttemptID != attempt || !transition.Recovered {
					t.Fatalf("recovered transition did not preserve attempt: %+v", transition)
				}
			}
		}
	}
	if recoveryCount != 2 || ingestTerminal != 1 || !secondRecoveryCitesFirst {
		t.Fatalf("recovery events=%d ingest terminals=%d chained=%t, want two observations, one terminal, and causal chain", recoveryCount, ingestTerminal, secondRecoveryCitesFirst)
	}
}

func TestFactoryV1ReadyPRExactHead(t *testing.T) {
	t.Parallel()
	head := strings.Repeat("e", 40)
	valid := PREvidence{Repository: "transpara-ai/hive", Number: 42, URL: "https://github.com/transpara-ai/hive/pull/42", HeadSHA: head, ReviewedHeadSHA: head, Open: true, Draft: false, ChecksPassing: true}
	if err := ValidateReadyPR(valid); err != nil {
		t.Fatalf("valid exact-head PR rejected: %v", err)
	}
	invalid := valid
	invalid.ReviewedHeadSHA = strings.Repeat("f", 40)
	if err := ValidateReadyPR(invalid); err == nil {
		t.Fatal("mismatched reviewed head was accepted")
	}
	invalid = valid
	invalid.Draft = true
	if err := ValidateReadyPR(invalid); err == nil {
		t.Fatal("draft PR was accepted as ready")
	}
	invalid = valid
	invalid.ChecksPassing = false
	if err := ValidateReadyPR(invalid); err == nil {
		t.Fatal("failing required checks were accepted")
	}
}

func TestFactoryV1ProjectionHonesty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	intake, _ := NewIntake(events, work, clock)
	receipt, err := intake.AcceptCompleted(ctx, testOrder("FO-HONESTY", ChannelCompletedOrder), AcceptOptions{SourceIdentity: "test:honesty", SourceEventIDs: []string{"source:honesty"}, ActorID: "human-actor-1"})
	if err != nil {
		t.Fatal(err)
	}
	attempt, _ := AttemptID(receipt.DocumentSHA256, StageIngestWork, 1)
	unsupported := StageTransitionPayload{TLCVersion: TLCVersion, Stage: StageIngestWork, StageIndex: 0, State: TransitionPassed, AttemptID: attempt, Ordinal: 1, Peers: PeersForStage(StageIngestWork), Evidence: nil, Runner: testProvider()}
	if _, err := AppendTyped(ctx, events, EventStageTransitioned, receipt.OrderID, "malformed-passed", []string{receipt.AcceptedEventID}, unsupported); err != nil {
		t.Fatal(err)
	}
	projector, _ := NewProjector(events, work, clock, ServiceProjection{ServiceID: "hive-factory-v1", InstanceID: "instance-1", Healthy: true})
	projection, err := projector.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order, _ := projection.Order(receipt.OrderID)
	if order.Status != "blocked" || order.GateState != "unavailable" || !strings.Contains(order.Blocker, "durable evidence") {
		t.Fatalf("projection rendered unsupported success as healthy: %+v", order)
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"schema_version"`, `"generated_at"`, `"service"`, `"orders"`, `"ideas"`, `"interventions"`, `"actively_executing"`, `"human_approval_basis"`, `"stages"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("projection JSON lacks stable key %s: %s", key, raw)
		}
	}
}

func TestFactoryV1IssueSingleActiveClaim(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	intake, _ := NewIntake(events, work, clock)
	admission := IssueAdmission{LaunchEventID: "launch-1", Repository: "transpara-ai/hive", IssueNumber: 77, Title: "Original", Body: "Body", Order: testOrder("FO-ISSUE-CLAIM", ChannelIssueScan), ActorID: "scanner"}
	if _, err := intake.NormalizeIssue(ctx, admission); err != nil {
		t.Fatal(err)
	}
	admission.LaunchEventID = "launch-2"
	admission.Title = "Edited"
	if _, err := intake.NormalizeIssue(ctx, admission); !errors.Is(err, ErrIssueAmendmentBlocked) {
		t.Fatalf("edited active issue error = %v, want amendment blocked", err)
	}
	projection, _ := NewProjector(events, work, clock, ServiceProjection{Healthy: true})
	view, err := projection.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Orders) != 1 || len(view.Interventions) != 1 || view.Interventions[0].Kind != "issue_source_amendment" {
		t.Fatalf("single claim projection = orders %d interventions %+v", len(view.Orders), view.Interventions)
	}
}

func TestFactoryV1ReplayTwiceKeepsOneOrphanIntervention(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	if _, err := work.SeedFactoryOrder(ctx, WorkSeed{
		OrderID: "FO-ORPHAN", Version: "1.0.0", DocumentSHA256: strings.Repeat("a", 64), Markdown: "# orphan\n",
		SourceSHA256: strings.Repeat("b", 64), AcceptedEventID: "missing-accepted-event", IdempotencyKey: "orphan-seed",
	}); err != nil {
		t.Fatal(err)
	}
	intake, _ := NewIntake(events, work, clock)
	if err := intake.ReplayAndRepair(ctx); err != nil {
		t.Fatalf("first replay: %v", err)
	}
	if err := intake.ReplayAndRepair(ctx); err != nil {
		t.Fatalf("second replay must be idempotent: %v", err)
	}
	listed, _ := events.List(ctx)
	requests := 0
	for _, event := range listed {
		if event.Type == EventInterventionRequested {
			requests++
		}
	}
	if requests != 1 {
		t.Fatalf("orphan intervention requests = %d, want exactly one", requests)
	}
}

func TestFactoryV1ReplayRestoresInterventionForQuarantinedOrphan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	link, err := work.SeedFactoryOrder(ctx, WorkSeed{
		OrderID: "FO-ORPHAN-CRASH", Version: "1.0.0", DocumentSHA256: strings.Repeat("c", 64), Markdown: "# orphan after crash\n",
		SourceSHA256: strings.Repeat("d", 64), AcceptedEventID: "missing-accepted-event", IdempotencyKey: "orphan-crash-seed",
	})
	if err != nil {
		t.Fatal(err)
	}
	reason := "Work FactoryOrder has no matching accepted EventGraph event"
	if err := work.QuarantineFactoryOrder(ctx, link, reason); err != nil {
		t.Fatal(err)
	}

	intake, _ := NewIntake(events, work, clock)
	for replay := 1; replay <= 2; replay++ {
		if err := intake.ReplayAndRepair(ctx); err != nil {
			t.Fatalf("replay %d: %v", replay, err)
		}
	}
	listed, err := events.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	for _, event := range listed {
		if event.Type != EventInterventionRequested {
			continue
		}
		payload, err := decodeEvent[InterventionRequestedPayload](event)
		if err != nil {
			t.Fatal(err)
		}
		if payload.OrderID == link.OrderID && payload.Kind == "orphan_work" && payload.Prompt == reason {
			requests++
		}
	}
	if requests != 1 {
		t.Fatalf("recovered orphan intervention requests = %d, want exactly one", requests)
	}
}

func TestFactoryV1ReplayIsolatesAcceptedTupleConflictPerOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := testClock()
	events := NewInMemoryStore(clock)
	work := NewInMemoryWorkStore()
	intake, _ := NewIntake(events, work, clock)
	first, err := intake.AcceptCompleted(ctx, testOrder("FO-CONFLICT", ChannelCompletedOrder), AcceptOptions{SourceIdentity: "test:conflict", SourceEventIDs: []string{"source:conflict"}, ActorID: "human-actor-1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := intake.AcceptCompleted(ctx, testOrder("FO-INDEPENDENT", ChannelCompletedOrder), AcceptOptions{SourceIdentity: "test:independent", SourceEventIDs: []string{"source:independent"}, ActorID: "human-actor-1"})
	if err != nil {
		t.Fatal(err)
	}

	work.mu.Lock()
	conflicting := work.links[workTuple(first.OrderID, first.Version)]
	conflicting.DocumentSHA256 = strings.Repeat("f", 64)
	work.links[workTuple(first.OrderID, first.Version)] = conflicting
	delete(work.links, workTuple(second.OrderID, second.Version))
	work.mu.Unlock()

	for replay := 1; replay <= 2; replay++ {
		if err := intake.ReplayAndRepair(ctx); err != nil {
			t.Fatalf("replay %d: %v", replay, err)
		}
	}
	conflictLink, err := work.GetFactoryOrder(ctx, first.OrderID, first.Version)
	if err != nil || !conflictLink.Quarantined || conflictLink.Metadata["quarantine_reason"] != "Work FactoryOrder conflicts with accepted EventGraph tuple" {
		t.Fatalf("conflicting link = %+v, err=%v", conflictLink, err)
	}
	independentLink, err := work.GetFactoryOrder(ctx, second.OrderID, second.Version)
	if err != nil || independentLink.DocumentSHA256 != second.DocumentSHA256 || independentLink.Quarantined {
		t.Fatalf("independent link was not repaired after conflict: %+v, err=%v", independentLink, err)
	}
	listed, err := events.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	for _, event := range listed {
		if event.Type != EventInterventionRequested {
			continue
		}
		payload, err := decodeEvent[InterventionRequestedPayload](event)
		if err != nil {
			t.Fatal(err)
		}
		if payload.OrderID == first.OrderID && payload.Kind == "accepted_tuple_conflict" {
			requests++
		}
	}
	if requests != 1 {
		t.Fatalf("tuple-conflict intervention requests = %d, want exactly one", requests)
	}
}
