package factoryv1

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func testTLC51Plan(t *testing.T, state TLC51InformationState, track, retained *string, obligations []TLC51Obligation) TLC51GatePlan {
	t.Helper()
	subject := map[string]any{"kind": "pull_request", "repository": "transpara-ai/hive", "head_sha": strings.Repeat("1", 40)}
	subjectRaw, err := canonicalTLC51JSON(subject)
	if err != nil {
		t.Fatal(err)
	}
	subjectDigest := fmt.Sprintf("%x", sha256.Sum256(subjectRaw))
	value := map[string]any{
		"schema_version":          TLC51PlanSchema,
		"release_identity":        map[string]any{"version": "5.1.0", "tag": "v5.1.0", "commit": strings.Repeat("a", 40), "manifest_sha256": strings.Repeat("b", 64)},
		"adapter_identity":        map[string]any{"id": "hive-tlc51", "commit": strings.Repeat("c", 40), "sha256": strings.Repeat("d", 64)},
		"repository":              "transpara-ai/hive",
		"change_series_id":        "series-1",
		"subject":                 subject,
		"subject_digest":          subjectDigest,
		"normalized_facts_digest": strings.Repeat("e", 64),
		"information_state":       string(state),
		"track":                   track,
		"retained_floor":          retained,
		"impact_floor":            "high_consequence",
		"required_tests":          []string{"repository-unit-tests"},
		"derived_effects":         []string{"ready"},
		"requested_effects":       []string{"ready"},
		"authority_requirements":  []any{},
		"obligations":             obligations,
		"evidence_rules":          map[string]any{"record_schema": TLC51RecordSchema},
		"unresolved_requests":     []string{},
		"reasons":                 []string{"test fixture"},
		"admitted_fact_records":   []any{},
		"author_lineages":         []string{"author-family"},
		"plan_digest":             "",
	}
	digest, err := tlc51ObjectDigest(value, "plan_digest")
	if err != nil {
		t.Fatal(err)
	}
	value["plan_digest"] = digest
	raw, err := canonicalTLC51JSON(value)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := ParseTLC51GatePlan(raw)
	if err != nil {
		t.Fatalf("ParseTLC51GatePlan: %v", err)
	}
	return plan
}

func testTLC51Obligation(id, kind string, prerequisites []string, family string) TLC51Obligation {
	return TLC51Obligation{
		ID: id, Kind: kind, Prerequisites: prerequisites,
		ExactSubjectDigest:    strings.Repeat("0", 64), // replaced by test plan helper below
		AdmittedActorFamilies: []string{family}, EvidenceContract: json.RawMessage(`{}`),
		RetryPolicy: "same-subject-new-attempt-after-observation", ParallelSafe: true,
	}
}

func TestParseTLC51GatePlanAllowsAdapterSelectedActorFamily(t *testing.T) {
	track := "H"
	obligation := bindTLC51ObligationSubjects(t, []TLC51Obligation{testTLC51Obligation("O001-unit", "repository-unit-tests", nil, "worker-family")})
	obligation[0].AdmittedActorFamilies = nil
	plan := testTLC51Plan(t, TLC51Classified, &track, &track, obligation)
	if len(plan.Obligations[0].AdmittedActorFamilies) != 0 {
		t.Fatalf("actor admissions = %v, want adapter-selected empty set", plan.Obligations[0].AdmittedActorFamilies)
	}
}

func bindTLC51ObligationSubjects(t *testing.T, obligations []TLC51Obligation) []TLC51Obligation {
	t.Helper()
	// Parse once to obtain the canonical subject digest without an obligation
	// identity dependency, then apply it to the final plan.
	track := "H"
	probe := testTLC51PlanWithoutObligations(t, TLC51Classified, &track, &track)
	result := append([]TLC51Obligation(nil), obligations...)
	for index := range result {
		result[index].ExactSubjectDigest = probe.SubjectDigest
	}
	return result
}

func testTLC51PlanWithoutObligations(t *testing.T, state TLC51InformationState, track, retained *string) TLC51GatePlan {
	t.Helper()
	placeholder := testTLC51Obligation("O000-probe", "repository-unit-tests", nil, "worker-family")
	subject := map[string]any{"kind": "pull_request", "repository": "transpara-ai/hive", "head_sha": strings.Repeat("1", 40)}
	subjectRaw, _ := canonicalTLC51JSON(subject)
	placeholder.ExactSubjectDigest = fmt.Sprintf("%x", sha256.Sum256(subjectRaw))
	return testTLC51Plan(t, state, track, retained, []TLC51Obligation{placeholder})
}

func testTLC51Evidence(recordID string) TLC51ExactJSON {
	raw, err := canonicalTLC51JSON(map[string]any{"schema_version": TLC51RecordSchema, "record_id": recordID})
	if err != nil {
		panic(err)
	}
	return TLC51ExactJSON{SchemaVersion: TLC51RecordSchema, CanonicalJSON: string(raw), SHA256: fmt.Sprintf("%x", sha256.Sum256(raw))}
}

type testTLC51WorkLinker struct {
	mu       sync.Mutex
	items    map[uint64]TLC51WorkArtifact
	failOnce bool
}

func newTestTLC51WorkLinker() *testTLC51WorkLinker {
	return &testTLC51WorkLinker{items: map[uint64]TLC51WorkArtifact{}}
}

func (linker *testTLC51WorkLinker) LinkTLC51Event(_ context.Context, entry TLC51HistoryEntry) error {
	linker.mu.Lock()
	defer linker.mu.Unlock()
	if linker.failOnce {
		linker.failOnce = false
		return fmt.Errorf("injected Work split")
	}
	wanted := TLC51WorkArtifactFromEntry(entry)
	if existing, ok := linker.items[wanted.EventOrdinal]; ok && !tlc51WorkArtifactsEqual(existing, wanted) {
		return ErrTLC51HistoryConflict
	}
	linker.items[wanted.EventOrdinal] = wanted
	return nil
}

func (linker *testTLC51WorkLinker) TLC51WorkArtifacts(context.Context, string, string) ([]TLC51WorkArtifact, error) {
	linker.mu.Lock()
	defer linker.mu.Unlock()
	ordinals := make([]int, 0, len(linker.items))
	for ordinal := range linker.items {
		ordinals = append(ordinals, int(ordinal))
	}
	sort.Ints(ordinals)
	result := make([]TLC51WorkArtifact, 0, len(ordinals))
	for _, ordinal := range ordinals {
		result = append(result, linker.items[uint64(ordinal)])
	}
	return result, nil
}

type testTLC51Runner struct {
	mu              sync.Mutex
	active          int
	maxActive       int
	executeCalls    []string
	reconcileCalls  []string
	failFirst       bool
	reconcileState  TLC51ExternalState
	parallelStarted chan struct{}
	parallelTarget  int
}

func (runner *testTLC51Runner) Execute(_ context.Context, request TLC51ObligationRequest) (TLC51ObligationResult, error) {
	runner.mu.Lock()
	runner.executeCalls = append(runner.executeCalls, request.Obligation.ID)
	runner.active++
	if runner.active > runner.maxActive {
		runner.maxActive = runner.active
	}
	if runner.parallelTarget > 0 && runner.active == runner.parallelTarget {
		close(runner.parallelStarted)
	}
	fail := runner.failFirst
	runner.failFirst = false
	runner.mu.Unlock()
	if runner.parallelTarget > 0 {
		select {
		case <-runner.parallelStarted:
		case <-time.After(time.Second):
		}
	}
	runner.mu.Lock()
	runner.active--
	runner.mu.Unlock()
	if fail {
		return TLC51ObligationResult{}, fmt.Errorf("injected crash after possible effect")
	}
	record := testTLC51Evidence("record-" + request.Obligation.ID)
	return TLC51ObligationResult{
		Outcome: TLC51ObligationPassed, Reason: "exact test evidence", EvidenceRecordID: "record-" + request.Obligation.ID,
		EvidenceRecord: &record, Usage: Usage{Tokens: 10, CostMicros: 20}, Provider: request.Provider,
	}, nil
}

func (runner *testTLC51Runner) Reconcile(_ context.Context, request TLC51ObligationRequest) (TLC51ReconcileResult, error) {
	runner.mu.Lock()
	runner.reconcileCalls = append(runner.reconcileCalls, request.Obligation.ID)
	runner.mu.Unlock()
	state := runner.reconcileState
	if state == "" {
		state = TLC51ExternalAbsent
	}
	return TLC51ReconcileResult{
		ExternalState: state, ObservationID: "observation-1", ObservationSHA256: strings.Repeat("f", 64),
		ObservedAt: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC), Result: TLC51ObligationResult{Provider: request.Provider},
	}, nil
}

func testTLC51Binding(t *testing.T) TLC51OrderBinding {
	t.Helper()
	order := testOrder("FO-TLC51-TEST", ChannelCompletedOrder)
	order.TargetRepository = "transpara-ai/hive"
	binding, err := NewTLC51OrderBinding(order, "accepted-event-1")
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func testTLC51Provider(family string) ProviderBinding {
	return ProviderBinding{ProviderID: "provider-" + family, Family: family, ExecutableRealpath: "/bin/true", ExecutableSHA256: strings.Repeat("a", 64), ModelID: "model-1", CredentialSourceID: "credential-source-1"}
}

func TestTLC51PlanExactDigestAndSubjectValidation(t *testing.T) {
	track := "H"
	plan := testTLC51PlanWithoutObligations(t, TLC51Classified, &track, &track)
	if plan.InformationState != TLC51Classified || plan.PlanDigest == "" || len(plan.Raw) == 0 {
		t.Fatalf("plan = %+v", plan)
	}
	forged := append([]byte(nil), plan.Raw...)
	forged = bytes.Replace(forged, []byte(`"plan_digest":"`+plan.PlanDigest), []byte(`"plan_digest":"`+strings.Repeat("0", 64)), 1)
	if _, err := ParseTLC51GatePlan(forged); err == nil {
		t.Fatal("forged plan digest accepted")
	}
}

func TestTLC51SchedulerParallelDAGAndHumanWait(t *testing.T) {
	track := "H"
	obligations := bindTLC51ObligationSubjects(t, []TLC51Obligation{
		testTLC51Obligation("O001-unit", "repository-unit-tests", nil, "worker-family"),
		testTLC51Obligation("O002-e2e", "product-e2e", nil, "worker-family"),
		testTLC51Obligation("O003-human", "human-design-review", []string{"O001-unit", "O002-e2e"}, "human"),
	})
	plan := testTLC51Plan(t, TLC51Classified, &track, &track, obligations)
	binding := testTLC51Binding(t)
	journal := NewInMemoryTLC51Journal()
	work := newTestTLC51WorkLinker()
	runner := &testTLC51Runner{parallelStarted: make(chan struct{}), parallelTarget: 2}
	scheduler, err := NewTLC51Scheduler(journal, work, runner, testClock(), TLC51SchedulerConfig{
		WorkerCount: 2,
		Providers:   map[string]ProviderBinding{"O001-unit": testTLC51Provider("worker-family"), "O002-e2e": testTLC51Provider("worker-family")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.RecordPlan(context.Background(), binding, plan); err != nil {
		t.Fatalf("RecordPlan: %v", err)
	}
	if err := scheduler.RunOnce(context.Background(), binding, plan); err != nil {
		t.Fatalf("RunOnce parallel wave: %v", err)
	}
	if runner.maxActive != 2 {
		t.Fatalf("max active = %d, want 2 parallel obligations", runner.maxActive)
	}
	if err := scheduler.RunOnce(context.Background(), binding, plan); err != nil {
		t.Fatalf("RunOnce Human wave: %v", err)
	}
	history, err := journal.TLC51History(context.Background(), binding.FactoryOrderID, plan.ChangeSeriesID)
	if err != nil {
		t.Fatal(err)
	}
	states, err := projectTLC51Obligations(plan, history)
	if err != nil {
		t.Fatal(err)
	}
	if states["O001-unit"].Terminal != TLC51ObligationPassed || states["O002-e2e"].Terminal != TLC51ObligationPassed || !states["O003-human"].HumanWaiting {
		t.Fatalf("states = %+v", states)
	}
	if len(runner.executeCalls) != 2 {
		t.Fatalf("Human obligation reached runner: calls=%v", runner.executeCalls)
	}
	artifacts, _ := work.TLC51WorkArtifacts(context.Background(), binding.FactoryOrderID, plan.ChangeSeriesID)
	if len(artifacts) != len(history) {
		t.Fatalf("EventGraph/Work twins = %d/%d", len(history), len(artifacts))
	}
	row, err := ProjectTLC51MissionControl(binding, plan, history, artifacts, time.Date(2026, 8, 27, 12, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ProjectTLC51MissionControl: %v", err)
	}
	if row.ProtocolVersion != TLC51ProtocolVersion || row.WorkReconciliation != "match" || row.Decision != "unknown" || row.AuthorityGranted {
		t.Fatalf("projection identity/state = %+v", row)
	}
	if len(row.HumanWaits) != 1 || row.HumanWaits[0].Status != "waiting" || len(row.Obligations) != 3 || row.Obligations[2].Status != "human_required" {
		t.Fatalf("projection waits/obligations = waits=%+v obligations=%+v", row.HumanWaits, row.Obligations)
	}
}

func TestTLC51RestartReconcilesBeforeRetryingSameAttempt(t *testing.T) {
	track := "I"
	obligations := bindTLC51ObligationSubjects(t, []TLC51Obligation{testTLC51Obligation("O001-impl", "implementation", nil, "worker-family")})
	plan := testTLC51Plan(t, TLC51Classified, &track, &track, obligations)
	binding := testTLC51Binding(t)
	journal := NewInMemoryTLC51Journal()
	work := newTestTLC51WorkLinker()
	firstRunner := &testTLC51Runner{failFirst: true}
	first, err := NewTLC51Scheduler(journal, work, firstRunner, testClock(), TLC51SchedulerConfig{WorkerCount: 1, Providers: map[string]ProviderBinding{"O001-impl": testTLC51Provider("worker-family")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.RecordPlan(context.Background(), binding, plan); err != nil {
		t.Fatal(err)
	}
	if err := first.RunOnce(context.Background(), binding, plan); err == nil || !strings.Contains(err.Error(), "durable running") {
		t.Fatalf("injected crash = %v", err)
	}
	secondRunner := &testTLC51Runner{reconcileState: TLC51ExternalAbsent}
	restarted, err := NewTLC51Scheduler(journal, work, secondRunner, testClock(), TLC51SchedulerConfig{WorkerCount: 1, Providers: map[string]ProviderBinding{"O001-impl": testTLC51Provider("worker-family")}})
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.RunOnce(context.Background(), binding, plan); err != nil {
		t.Fatalf("restart RunOnce: %v", err)
	}
	if len(secondRunner.reconcileCalls) != 1 || len(secondRunner.executeCalls) != 1 {
		t.Fatalf("restart order reconcile=%v execute=%v", secondRunner.reconcileCalls, secondRunner.executeCalls)
	}
	history, _ := journal.TLC51History(context.Background(), binding.FactoryOrderID, plan.ChangeSeriesID)
	states, _ := projectTLC51Obligations(plan, history)
	if states["O001-impl"].Terminal != TLC51ObligationPassed || states["O001-impl"].MaxAttempt != 1 {
		t.Fatalf("same attempt not settled after observation: %+v", states["O001-impl"])
	}
}

func TestTLC51RepairsMissingWorkTwinAndRejectsLoweredReplan(t *testing.T) {
	high := "H"
	low := "M"
	obligation := bindTLC51ObligationSubjects(t, []TLC51Obligation{testTLC51Obligation("O001-unit", "repository-unit-tests", nil, "worker-family")})
	plan := testTLC51Plan(t, TLC51Classified, &high, &high, obligation)
	binding := testTLC51Binding(t)
	journal := NewInMemoryTLC51Journal()
	work := newTestTLC51WorkLinker()
	work.failOnce = true
	scheduler, err := NewTLC51Scheduler(journal, work, &testTLC51Runner{}, testClock(), TLC51SchedulerConfig{Providers: map[string]ProviderBinding{"O001-unit": testTLC51Provider("worker-family")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.RecordPlan(context.Background(), binding, plan); err == nil || !strings.Contains(err.Error(), "committed before Work twin") {
		t.Fatalf("split not surfaced: %v", err)
	}
	if _, err := scheduler.RecordPlan(context.Background(), binding, plan); err != nil {
		t.Fatalf("missing Work twin not repaired on replay: %v", err)
	}
	lowered := testTLC51Plan(t, TLC51Classified, &low, &low, obligation)
	if _, err := scheduler.RecordPlan(context.Background(), binding, lowered); err == nil || !strings.Contains(err.Error(), "lowered") {
		t.Fatalf("lowered retained floor accepted: %v", err)
	}
}

func TestTLC51UnclassifiedPlanNeverSchedules(t *testing.T) {
	high := "H"
	obligation := bindTLC51ObligationSubjects(t, []TLC51Obligation{testTLC51Obligation("O001-unit", "repository-unit-tests", nil, "worker-family")})
	plan := testTLC51Plan(t, TLC51Unclassified, nil, &high, obligation)
	binding := testTLC51Binding(t)
	scheduler, err := NewTLC51Scheduler(NewInMemoryTLC51Journal(), newTestTLC51WorkLinker(), &testTLC51Runner{}, testClock(), TLC51SchedulerConfig{Providers: map[string]ProviderBinding{"O001-unit": testTLC51Provider("worker-family")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.RecordPlan(context.Background(), binding, plan); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.RunOnce(context.Background(), binding, plan); err == nil || !strings.Contains(err.Error(), "UNCLASSIFIED") {
		t.Fatalf("unclassified plan scheduled: %v", err)
	}
	history, _ := scheduler.journal.TLC51History(context.Background(), binding.FactoryOrderID, plan.ChangeSeriesID)
	work, _ := scheduler.work.TLC51WorkArtifacts(context.Background(), binding.FactoryOrderID, plan.ChangeSeriesID)
	row, err := ProjectTLC51MissionControl(binding, plan, history, work, time.Date(2026, 8, 27, 12, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if row.InformationState != TLC51Unclassified || row.Track != nil || !contains(row.Blockers, "information_state:UNCLASSIFIED") {
		t.Fatalf("unclassified projection invented legacy defaults: %+v", row)
	}
}

type testTLC51EffectBoundary struct {
	calls         int
	subjectDigest string
}

func (boundary *testTLC51EffectBoundary) CheckEffect(_ context.Context, request json.RawMessage) (json.RawMessage, error) {
	boundary.calls++
	var input struct {
		Effect         string `json:"effect"`
		OperationID    string `json:"operation_id"`
		IdempotencyKey string `json:"idempotency_key"`
		Receipt        struct {
			ReceiptDigest string `json:"receipt_digest"`
		} `json:"receipt"`
		ProviderObservation struct {
			RecordID     string `json:"record_id"`
			RecordDigest string `json:"record_digest"`
		} `json:"provider_observation"`
	}
	if err := json.Unmarshal(request, &input); err != nil {
		return nil, err
	}
	tuple := map[string]string{
		"receipt_digest": input.Receipt.ReceiptDigest, "effect": input.Effect,
		"subject_digest": boundary.subjectDigest, "operation_id": input.OperationID, "idempotency_key": input.IdempotencyKey,
	}
	tupleRaw, _ := canonicalTLC51JSON(tuple)
	value := map[string]any{
		"schema_version": TLC51EffectDecisionSchema, "decision": "pass", "checks": []any{},
		"provider_observation_id": input.ProviderObservation.RecordID, "provider_observation_digest": input.ProviderObservation.RecordDigest,
		"authority_observation_id": "authority-observation-1", "authority_observation_digest": strings.Repeat("d", 64),
		"reservation_tuple": tuple, "reservation_digest": fmt.Sprintf("%x", sha256.Sum256(tupleRaw)),
		"effect_invoked": false, "authority_granted": []any{}, "decision_digest": "",
	}
	digest, _ := tlc51ObjectDigest(value, "decision_digest")
	value["decision_digest"] = digest
	return canonicalTLC51JSON(value)
}

type testTLC51EffectDriver struct {
	states         []TLC51ExternalState
	receipt        TLC51ExactJSON
	failExecute    bool
	wrongOperation bool
	observeCalls   int
	executeCalls   int
}

func (driver *testTLC51EffectDriver) ObserveEffect(_ context.Context, operation TLC51EffectOperation) (TLC51EffectObservation, error) {
	index := driver.observeCalls
	driver.observeCalls++
	state := TLC51ExternalUnknown
	if index < len(driver.states) {
		state = driver.states[index]
	}
	operationID := operation.OperationID
	if driver.wrongOperation {
		operationID = "different-operation"
	}
	observationID := fmt.Sprintf("provider-observation-%d", index+1)
	digest := strings.Repeat(string(rune('e'+index)), 64)
	request := map[string]any{
		"schema_version": TLC51EffectRequestSchema, "effect": operation.Effect,
		"operation_id": operationID, "idempotency_key": operation.IdempotencyKey,
		"receipt":               map[string]any{"receipt_digest": operation.ReceiptDigest},
		"external_effect_state": state,
		"provider_observation":  map[string]any{"record_id": observationID, "record_digest": digest},
	}
	raw, _ := json.Marshal(request)
	observation := TLC51EffectObservation{
		ExternalState: state, ObservationID: observationID, ObservationSHA256: digest,
		ObservedAt: time.Date(2026, 8, 27, 12, 0, index, 0, time.UTC), BoundaryRequest: raw,
	}
	if state == TLC51ExternalExact {
		receipt := driver.receipt
		observation.EffectReceipt = &receipt
	}
	return observation, nil
}

func (driver *testTLC51EffectDriver) ExecuteEffect(context.Context, TLC51EffectOperation) (TLC51ExactJSON, error) {
	driver.executeCalls++
	if driver.failExecute {
		return TLC51ExactJSON{}, fmt.Errorf("injected crash")
	}
	return driver.receipt, nil
}

func testTLC51GateReceipt(t *testing.T, plan TLC51GatePlan, effect string) TLC51GateReceipt {
	t.Helper()
	var planValue map[string]any
	if err := json.Unmarshal(plan.Raw, &planValue); err != nil {
		t.Fatal(err)
	}
	value := map[string]any{
		"schema_version": TLC51ReceiptSchema, "release_identity": planValue["release_identity"],
		"adapter_identity": planValue["adapter_identity"], "repository": plan.Repository,
		"change_series_id": plan.ChangeSeriesID, "plan_digest": plan.PlanDigest,
		"subject": planValue["subject"], "subject_digest": plan.SubjectDigest,
		"information_state": string(plan.InformationState), "retained_floor": plan.RetainedFloor,
		"predicate_results":       []any{map[string]any{"id": "P001-test", "status": "true", "reason": "exact test receipt", "record_digests": []any{}}},
		"admitted_record_digests": []any{}, "reviewers": []any{},
		"authority_references": []any{map[string]any{"effect": effect, "provider_record_id": "authority-1", "record_digest": strings.Repeat("c", 64)}},
		"evaluation_clock":     map[string]any{"record_id": "clock-1", "record_digest": strings.Repeat("d", 64), "provider_time": "2026-08-27T12:00:00Z", "observed_time": "2026-08-27T12:00:00Z", "freshness_seconds": 300, "expires_at": "2026-08-27T12:05:00Z"},
		"decision":             "pass", "reasons": []any{},
		"enforcer_provenance": map[string]any{"test": true}, "authority_granted": []any{},
		"mutation_effects_invoked": []any{}, "receipt_digest": "",
	}
	digest, err := tlc51ObjectDigest(value, "receipt_digest")
	if err != nil {
		t.Fatal(err)
	}
	value["receipt_digest"] = digest
	raw, err := canonicalTLC51JSON(value)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ParseTLC51GateReceipt(raw, plan)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func testTLC51EffectReceipt(plan TLC51GatePlan, operation TLC51EffectOperation) TLC51ExactJSON {
	value := map[string]any{
		"schema_version": TLC51EffectReceiptSchema, "effect": operation.Effect,
		"subject_digest": plan.SubjectDigest, "gate_receipt_digest": operation.ReceiptDigest,
		"operation_id": operation.OperationID, "idempotency_key": operation.IdempotencyKey,
		"attempt_ordinal": operation.AttemptOrdinal, "provider_effect_id": "provider-effect-1",
		"provider_record_url": "https://provider.example/effects/1", "invoked_at": "2026-08-27T12:00:00Z",
		"outcome": "succeeded", "receipt_digest": "",
	}
	digest, _ := tlc51ObjectDigest(value, "receipt_digest")
	value["receipt_digest"] = digest
	raw, _ := canonicalTLC51JSON(value)
	return TLC51ExactJSON{SchemaVersion: TLC51EffectReceiptSchema, CanonicalJSON: string(raw), SHA256: fmt.Sprintf("%x", sha256.Sum256(raw))}
}

func testTLC51EffectScheduler(t *testing.T) (*TLC51Scheduler, TLC51OrderBinding, TLC51GatePlan, *InMemoryTLC51Journal, TLC51GateReceipt) {
	t.Helper()
	track := "H"
	obligation := bindTLC51ObligationSubjects(t, []TLC51Obligation{testTLC51Obligation("O001-unit", "repository-unit-tests", nil, "worker-family")})
	plan := testTLC51Plan(t, TLC51Classified, &track, &track, obligation)
	binding := testTLC51Binding(t)
	journal := NewInMemoryTLC51Journal()
	scheduler, err := NewTLC51Scheduler(journal, newTestTLC51WorkLinker(), &testTLC51Runner{}, testClock(), TLC51SchedulerConfig{Providers: map[string]ProviderBinding{"O001-unit": testTLC51Provider("worker-family")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.RecordPlan(context.Background(), binding, plan); err != nil {
		t.Fatal(err)
	}
	receipt := testTLC51GateReceipt(t, plan, "ready")
	if _, err := scheduler.RecordDecision(context.Background(), binding, plan, receipt); err != nil {
		t.Fatal(err)
	}
	return scheduler, binding, plan, journal, receipt
}

func TestTLC51ProtectedEffectFreshBoundaryAndPostObservation(t *testing.T) {
	scheduler, binding, plan, journal, gateReceipt := testTLC51EffectScheduler(t)
	operation := TLC51EffectOperation{Effect: "ready", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: gateReceipt.ReceiptDigest, AttemptOrdinal: 1}
	boundary := &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}
	driver := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent, TLC51ExternalExact}, receipt: testTLC51EffectReceipt(plan, operation)}
	receipt, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, operation, boundary, driver)
	if err != nil {
		t.Fatalf("ExecuteProtectedEffect: %v", err)
	}
	if receipt.SHA256 != driver.receipt.SHA256 || boundary.calls != 1 || driver.executeCalls != 1 || driver.observeCalls != 2 {
		t.Fatalf("effect calls receipt=%+v boundary=%d execute=%d observe=%d", receipt, boundary.calls, driver.executeCalls, driver.observeCalls)
	}
	history, _ := journal.TLC51History(context.Background(), binding.FactoryOrderID, plan.ChangeSeriesID)
	var effectTypes []TLC51EventType
	for _, entry := range history {
		if strings.HasPrefix(string(entry.Type), "factory.tlc51.effect.") {
			effectTypes = append(effectTypes, entry.Type)
		}
	}
	want := []TLC51EventType{TLC51EffectObserved, TLC51EffectProposed, TLC51EffectObserved, TLC51EffectTerminal}
	if !reflect.DeepEqual(effectTypes, want) {
		t.Fatalf("effect history = %v, want %v", effectTypes, want)
	}
}

func TestTLC51ProtectedEffectCrashRecoversExactWithoutDuplicate(t *testing.T) {
	scheduler, binding, plan, _, gateReceipt := testTLC51EffectScheduler(t)
	operation := TLC51EffectOperation{Effect: "ready", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: gateReceipt.ReceiptDigest, AttemptOrdinal: 1}
	boundary := &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}
	crashed := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent}, receipt: testTLC51EffectReceipt(plan, operation), failExecute: true}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, operation, boundary, crashed); err == nil || !strings.Contains(err.Error(), "durable proposal") {
		t.Fatalf("crash = %v", err)
	}
	recovered := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalExact}, receipt: testTLC51EffectReceipt(plan, operation)}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, operation, boundary, recovered); err != nil {
		t.Fatalf("recovery: %v", err)
	}
	if recovered.executeCalls != 0 || recovered.observeCalls != 1 {
		t.Fatalf("recovery duplicated effect: execute=%d observe=%d", recovered.executeCalls, recovered.observeCalls)
	}
}

func TestTLC51ProtectedEffectReceiptCannotReserveDifferentOperation(t *testing.T) {
	scheduler, binding, plan, _, gateReceipt := testTLC51EffectScheduler(t)
	first := TLC51EffectOperation{Effect: "ready", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: gateReceipt.ReceiptDigest, AttemptOrdinal: 1}
	crashed := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent}, receipt: testTLC51EffectReceipt(plan, first), failExecute: true}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, first, &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}, crashed); err == nil || !strings.Contains(err.Error(), "durable proposal") {
		t.Fatalf("first operation did not leave a reservation: %v", err)
	}
	second := first
	second.OperationID = "operation-2"
	second.IdempotencyKey = "key-2"
	driver := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent}, receipt: testTLC51EffectReceipt(plan, second)}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, second, &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}, driver); err == nil || !strings.Contains(err.Error(), "already bound") {
		t.Fatalf("receipt reserved a second operation: %v", err)
	}
	if driver.observeCalls != 0 || driver.executeCalls != 0 {
		t.Fatalf("provider reached after cross-operation receipt reuse: observe=%d execute=%d", driver.observeCalls, driver.executeCalls)
	}
}

func TestTLC51MissionControlProposalUsesUnknownExternalState(t *testing.T) {
	scheduler, binding, plan, journal, gateReceipt := testTLC51EffectScheduler(t)
	operation := TLC51EffectOperation{Effect: "ready", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: gateReceipt.ReceiptDigest, AttemptOrdinal: 1}
	if err := scheduler.recordEffectProposal(context.Background(), binding, plan, operation); err != nil {
		t.Fatal(err)
	}
	history, _ := journal.TLC51History(context.Background(), binding.FactoryOrderID, plan.ChangeSeriesID)
	work, _ := scheduler.work.TLC51WorkArtifacts(context.Background(), binding.FactoryOrderID, plan.ChangeSeriesID)
	row, err := ProjectTLC51MissionControl(binding, plan, history, work, time.Date(2026, 8, 27, 12, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(row.Effects) != 1 || row.Effects[0].ExternalState != TLC51ExternalUnknown {
		t.Fatalf("proposal external state = %+v, want explicit unknown", row.Effects)
	}
}

func TestTLC51ProtectedEffectRejectsCrossOperationObservationReuse(t *testing.T) {
	scheduler, binding, plan, _, gateReceipt := testTLC51EffectScheduler(t)
	operation := TLC51EffectOperation{Effect: "ready", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: gateReceipt.ReceiptDigest, AttemptOrdinal: 1}
	driver := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent}, receipt: testTLC51EffectReceipt(plan, operation), wrongOperation: true}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, operation, &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}, driver); err == nil || !strings.Contains(err.Error(), "reused") {
		t.Fatalf("cross-operation observation accepted: %v", err)
	}
	if driver.executeCalls != 0 {
		t.Fatalf("effect invoked after cross-operation observation: %d", driver.executeCalls)
	}
}

func TestTLC51ProtectedEffectRejectsUnplannedEffect(t *testing.T) {
	scheduler, binding, plan, _, gateReceipt := testTLC51EffectScheduler(t)
	operation := TLC51EffectOperation{Effect: "deployment", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: gateReceipt.ReceiptDigest, AttemptOrdinal: 1}
	driver := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent}, receipt: testTLC51EffectReceipt(plan, operation)}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, operation, &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}, driver); err == nil || !strings.Contains(err.Error(), "not derived or requested") {
		t.Fatalf("unplanned effect accepted: %v", err)
	}
	if driver.observeCalls != 0 || driver.executeCalls != 0 {
		t.Fatalf("provider called for unplanned effect: observe=%d execute=%d", driver.observeCalls, driver.executeCalls)
	}
}

func TestTLC51ProtectedEffectRequiresRecordedExactPassingDecision(t *testing.T) {
	scheduler, binding, plan, _, gateReceipt := testTLC51EffectScheduler(t)
	operation := TLC51EffectOperation{Effect: "ready", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: strings.Repeat("b", 64), AttemptOrdinal: 1}
	if operation.ReceiptDigest == gateReceipt.ReceiptDigest {
		t.Fatal("test receipt digest unexpectedly collided")
	}
	driver := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent}, receipt: testTLC51EffectReceipt(plan, operation)}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, operation, &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}, driver); err == nil || !strings.Contains(err.Error(), "exact passing TLC decision receipt is not recorded") {
		t.Fatalf("unrecorded decision receipt accepted: %v", err)
	}
	if driver.executeCalls != 0 {
		t.Fatalf("effect invoked with unrecorded decision receipt: %d", driver.executeCalls)
	}
}

func TestTLC51ProtectedEffectRejectsReceiptForDifferentOperationTuple(t *testing.T) {
	scheduler, binding, plan, _, gateReceipt := testTLC51EffectScheduler(t)
	operation := TLC51EffectOperation{Effect: "ready", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: gateReceipt.ReceiptDigest, AttemptOrdinal: 1}
	other := operation
	other.IdempotencyKey = "different-key"
	driver := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent}, receipt: testTLC51EffectReceipt(plan, other)}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, operation, &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}, driver); err == nil || !strings.Contains(err.Error(), "exact plan and operation tuple") {
		t.Fatalf("cross-operation effect receipt accepted: %v", err)
	}
}

func TestTLC51ProtectedEffectTerminalCannotReplayAcrossTuple(t *testing.T) {
	scheduler, binding, plan, _, gateReceipt := testTLC51EffectScheduler(t)
	operation := TLC51EffectOperation{Effect: "ready", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: gateReceipt.ReceiptDigest, AttemptOrdinal: 1}
	driver := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent, TLC51ExternalExact}, receipt: testTLC51EffectReceipt(plan, operation)}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, operation, &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}, driver); err != nil {
		t.Fatal(err)
	}
	replayed := operation
	replayed.IdempotencyKey = "key-2"
	replayed.ReceiptDigest = strings.Repeat("b", 64)
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, replayed, &testTLC51EffectBoundary{subjectDigest: plan.SubjectDigest}, driver); err == nil || !strings.Contains(err.Error(), "tuple conflicts") {
		t.Fatalf("terminal effect replayed across tuple: %v", err)
	}
}

func TestTLC51ProtectedEffectRejectsBoundaryDecisionForDifferentSubject(t *testing.T) {
	scheduler, binding, plan, _, gateReceipt := testTLC51EffectScheduler(t)
	operation := TLC51EffectOperation{Effect: "ready", OperationID: "operation-1", IdempotencyKey: "key-1", ReceiptDigest: gateReceipt.ReceiptDigest, AttemptOrdinal: 1}
	driver := &testTLC51EffectDriver{states: []TLC51ExternalState{TLC51ExternalAbsent}, receipt: testTLC51EffectReceipt(plan, operation)}
	boundary := &testTLC51EffectBoundary{subjectDigest: strings.Repeat("f", 64)}
	if _, err := scheduler.ExecuteProtectedEffect(context.Background(), binding, plan, operation, boundary, driver); err == nil || !strings.Contains(err.Error(), "subject_digest") {
		t.Fatalf("cross-subject boundary decision accepted: %v", err)
	}
	if driver.executeCalls != 0 {
		t.Fatalf("effect invoked after cross-subject boundary decision: %d", driver.executeCalls)
	}
}
