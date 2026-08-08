package hive

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
	workpkg "github.com/transpara-ai/work"
)

type repairControlledFactoryV1WorkStore struct {
	factoryv1.WorkStore
	hideLinks bool
	listErr   error
	seedCalls int
}

func (s *repairControlledFactoryV1WorkStore) GetFactoryOrder(ctx context.Context, orderID, version string) (factoryv1.WorkLink, error) {
	if s.hideLinks {
		return factoryv1.WorkLink{}, factoryv1.ErrWorkNotFound
	}
	return s.WorkStore.GetFactoryOrder(ctx, orderID, version)
}

func (s *repairControlledFactoryV1WorkStore) SeedFactoryOrder(ctx context.Context, seed factoryv1.WorkSeed) (factoryv1.WorkLink, error) {
	s.seedCalls++
	link, err := s.WorkStore.SeedFactoryOrder(ctx, seed)
	if err == nil {
		s.hideLinks = false
	}
	return link, err
}

func (s *repairControlledFactoryV1WorkStore) ListFactoryOrders(ctx context.Context) ([]factoryv1.WorkLink, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.hideLinks {
		return nil, nil
	}
	return s.WorkStore.ListFactoryOrders(ctx)
}

func TestFactoryV1DaemonIssueScanAdmission(t *testing.T) {
	eventStore, factory, signer, human, conv := newDecisionTestStore(t)
	workpkg.RegisterWithRegistry(factory.Registry)
	graph, err := NewFactoryV1EventGraphStore(eventStore, factory, signer, human, conv)
	if err != nil {
		t.Fatalf("new EventGraph adapter: %v", err)
	}
	workStore, err := NewFactoryV1WorkStore(eventStore, factory, signer, human, conv)
	if err != nil {
		t.Fatalf("new Work adapter: %v", err)
	}
	controlledWork := &repairControlledFactoryV1WorkStore{WorkStore: workStore}
	intake, err := factoryv1.NewIntake(graph, controlledWork, factoryv1.WallClock{})
	if err != nil {
		t.Fatalf("new intake: %v", err)
	}
	queued, err := QueueIssueScanRunLaunch(eventStore, factory, signer, human, conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Factory V1 Test"),
		Issues: []GitHubIssueCandidate{{
			Repo: "transpara-ai/hive", Number: 441, Title: "Exercise v1 issue normalization",
			URL: "https://github.com/transpara-ai/hive/issues/441", Body: "Implement one bounded and verified daemon behavior.",
			Labels: []string{IssueScanPRReadyLabel},
		}},
		Authority: RunLaunchAuthority{InitialLevel: event.AuthorityLevelRequired, Scope: "bounded test; no merge"},
		Budget:    RunLaunchBudget{MaxIterations: 30, MaxCostUSD: 2},
	}, nil)
	if err != nil {
		t.Fatalf("queue legacy issue request: %v", err)
	}
	normalizer, err := NewFactoryV1IssueNormalizer(eventStore, intake, human.Value())
	if err != nil {
		t.Fatalf("new normalizer: %v", err)
	}
	count, err := normalizer.RunOnce(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("normalize count=%d err=%v", count, err)
	}
	events, err := graph.List(context.Background())
	if err != nil {
		t.Fatalf("list v1 events: %v", err)
	}
	accepted := 0
	for _, candidate := range events {
		if candidate.Type == factoryv1.EventOrderAccepted {
			accepted++
			if len(candidate.Causes) != 1 || candidate.Causes[0] == "" {
				t.Fatalf("accepted causes = %+v", candidate.Causes)
			}
		}
	}
	links, err := workStore.ListFactoryOrders(context.Background())
	if err != nil || accepted != 1 || len(links) != 1 {
		t.Fatalf("accepted=%d links=%+v err=%v queued=%s", accepted, links, err, queued.RunID)
	}
	if links[0].OrderID == "" || links[0].DocumentSHA256 == "" || links[0].AcceptedEventID == "" {
		t.Fatalf("incomplete Work link = %+v", links[0])
	}

	requests, err := factoryV1RequestedEvents(context.Background(), eventStore)
	if err != nil || len(requests) != 1 {
		t.Fatalf("request events=%d err=%v", len(requests), err)
	}
	requestContent, ok := requests[0].Content().(FactoryRunRequestedContent)
	if !ok {
		t.Fatalf("request content = %T", requests[0].Content())
	}
	driftedAdmission, recognized, err := factoryV1IssueAdmission(requests[0].ID(), requestContent, human.Value())
	if err != nil || !recognized {
		t.Fatalf("reconstruct admission recognized=%v err=%v", recognized, err)
	}
	driftedAdmission.Order.ExpectedOutputs = append(driftedAdmission.Order.ExpectedOutputs, "later derived-contract drift")
	if _, err := graph.Append(context.Background(), factoryv1.NewEvent{
		Type: factoryv1.EventStageTransitioned, OrderID: links[0].OrderID,
		Causes: []string{links[0].AcceptedEventID}, IdempotencyKey: "test-human-review:" + links[0].OrderID,
		Payload: factoryv1.StageTransitionPayload{Stage: factoryv1.StageHumanReview, State: factoryv1.TransitionHumanRequired},
	}); err != nil {
		t.Fatalf("append Human Review transition: %v", err)
	}
	if _, err := intake.NormalizeIssue(context.Background(), driftedAdmission); !errors.Is(err, factoryv1.ErrAcceptedTupleConflict) {
		t.Fatalf("derived-contract drift at Human Review err=%v, want accepted tuple conflict", err)
	}
	seedCallsBeforeRepair := controlledWork.seedCalls
	controlledWork.hideLinks = true
	count, err = normalizer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if count != 0 {
		t.Fatalf("idempotent replay count = %d, want 0 already-normalized requests", count)
	}
	if controlledWork.seedCalls <= seedCallsBeforeRepair || controlledWork.hideLinks {
		t.Fatalf("accepted replay did not restore missing Work visibility: seed calls %d -> %d hidden=%v", seedCallsBeforeRepair, controlledWork.seedCalls, controlledWork.hideLinks)
	}
	events, _ = graph.List(context.Background())
	accepted = 0
	for _, candidate := range events {
		if candidate.Type == factoryv1.EventOrderAccepted {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted events after replay = %d, want 1", accepted)
	}

	if _, err := QueueIssueScanRunLaunch(eventStore, factory, signer, human, conv, IssueScanRunLaunchRequest{
		OperatorID: IssueScanOperatorID("Factory V1 Test"),
		Issues: []GitHubIssueCandidate{{
			Repo: "transpara-ai/hive", Number: 442, Title: "Continue after replay repair failure",
			URL: "https://github.com/transpara-ai/hive/issues/442", Body: "Admit a newer bounded request despite a historical repair error.",
			Labels: []string{IssueScanPRReadyLabel},
		}},
		Authority: RunLaunchAuthority{InitialLevel: event.AuthorityLevelRequired, Scope: "bounded test; no merge"},
		Budget:    RunLaunchBudget{MaxIterations: 30, MaxCostUSD: 2},
	}, nil); err != nil {
		t.Fatalf("queue newer issue request: %v", err)
	}
	controlledWork.listErr = errors.New("simulated Work projection outage")
	count, err = normalizer.RunOnce(context.Background())
	if count != 1 || err == nil || !strings.Contains(err.Error(), "simulated Work projection outage") {
		t.Fatalf("newer admission after repair failure count=%d err=%v", count, err)
	}
}

type retryingFactoryV1Normalizer struct {
	mu    sync.Mutex
	calls int
}

func (n *retryingFactoryV1Normalizer) RunOnce(context.Context) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls++
	return 0, nil
}

type retryingFactoryV1Scheduler struct {
	mu     sync.Mutex
	calls  int
	cancel context.CancelFunc
}

func (s *retryingFactoryV1Scheduler) RunOnce(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.calls == 1 {
		return errors.New("simulated interrupted execute")
	}
	s.cancel()
	return nil
}

func TestFactoryV1DaemonRestartsCycleAfterSchedulerFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	normalizer := &retryingFactoryV1Normalizer{}
	scheduler := &retryingFactoryV1Scheduler{cancel: cancel}
	var reported int
	daemon, err := NewFactoryV1Daemon(normalizer, scheduler, FactoryV1DaemonConfig{
		PollInterval: 100 * time.Millisecond,
		OnError: func(error) {
			reported++
		},
	})
	if err != nil {
		t.Fatalf("new daemon: %v", err)
	}
	if err := daemon.Run(ctx); err != nil {
		t.Fatalf("run daemon: %v", err)
	}
	if scheduler.calls != 2 || normalizer.calls != 2 || reported != 1 {
		t.Fatalf("calls normalizer=%d scheduler=%d reported=%d", normalizer.calls, scheduler.calls, reported)
	}
}
