package hive

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
	workpkg "github.com/transpara-ai/work"
)

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
	intake, err := factoryv1.NewIntake(graph, workStore, factoryv1.WallClock{})
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

	if _, err := normalizer.RunOnce(context.Background()); err != nil {
		t.Fatalf("idempotent replay: %v", err)
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
