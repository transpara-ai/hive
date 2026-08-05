package hive

import (
	"context"
	"errors"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
	workpkg "github.com/transpara-ai/work"
)

type factoryV1FailOnceChainStore struct {
	store.Store
	failed bool
}

func (s *factoryV1FailOnceChainStore) Append(ev event.Event) (event.Event, error) {
	if !s.failed {
		s.failed = true
		return event.Event{}, errors.New("chain integrity violation: deterministic concurrent-head test")
	}
	return s.Store.Append(ev)
}

func TestFactoryV1EventGraphAppendRetriesConcurrentHeadRace(t *testing.T) {
	ctx := context.Background()
	base, factory, signer, actor, conversation := newDecisionTestStore(t)
	wrapped := &factoryV1FailOnceChainStore{Store: base}
	graph, err := NewFactoryV1EventGraphStore(wrapped, factory, signer, actor, conversation)
	if err != nil {
		t.Fatal(err)
	}
	input := factoryv1.NewEvent{
		Type: factoryv1.EventIssueAmendmentRecorded, OrderID: "FO-CONCURRENT-HEAD",
		IdempotencyKey: "concurrent-head-retry", OccurredAt: factoryv1.WallClock{}.Now(),
		Payload: map[string]string{"result": "converged"},
	}
	first, err := graph.Append(ctx, input)
	if err != nil {
		t.Fatalf("append after head race: %v", err)
	}
	second, err := graph.Append(ctx, input)
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if first.ID == "" || second.ID != first.ID {
		t.Fatalf("retry/replay identities = (%q,%q)", first.ID, second.ID)
	}
}

func TestFactoryV1DurableAdaptersRestartIdempotency(t *testing.T) {
	ctx := context.Background()
	eventStore, factory, signer, actor, conversation := newDecisionTestStore(t)
	workpkg.RegisterWithRegistry(factory.Registry)

	newBoundaries := func() (*FactoryV1EventGraphStore, *FactoryV1WorkStore, *factoryv1.Intake) {
		t.Helper()
		graph, err := NewFactoryV1EventGraphStore(eventStore, factory, signer, actor, conversation)
		if err != nil {
			t.Fatal(err)
		}
		workStore, err := NewFactoryV1WorkStore(eventStore, factory, signer, actor, conversation)
		if err != nil {
			t.Fatal(err)
		}
		intake, err := factoryv1.NewIntake(graph, workStore, factoryv1.WallClock{})
		if err != nil {
			t.Fatal(err)
		}
		return graph, workStore, intake
	}

	graph, _, intake := newBoundaries()
	order := validFactoryV1APIOrder(factoryv1.ChannelCompletedOrder, "FO-RESTART-IDEMPOTENCY", "transpara-ai/hive")
	first, err := intake.SubmitCompleted(ctx, order, actor.Value(), "credential-test")
	if err != nil {
		t.Fatal(err)
	}
	before, err := graph.List(ctx)
	if err != nil {
		t.Fatal(err)
	}

	restartedGraph, restartedWork, restartedIntake := newBoundaries()
	if err := restartedIntake.ReplayAndRepair(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := restartedIntake.SubmitCompleted(ctx, order, actor.Value(), "credential-test")
	if err != nil {
		t.Fatal(err)
	}
	after, err := restartedGraph.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	links, err := restartedWork.ListFactoryOrders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.AcceptedEventID != second.AcceptedEventID || len(after) != len(before) || len(links) != 1 {
		t.Fatalf("restart duplicated durable state: accepted=(%s,%s) events=(%d,%d) links=%d", first.AcceptedEventID, second.AcceptedEventID, len(before), len(after), len(links))
	}
}

func TestFactoryV1ReplayQuarantinesOrphanWorkOnce(t *testing.T) {
	ctx := context.Background()
	eventStore, factory, signer, actor, conversation := newDecisionTestStore(t)
	workpkg.RegisterWithRegistry(factory.Registry)
	graph, err := NewFactoryV1EventGraphStore(eventStore, factory, signer, actor, conversation)
	if err != nil {
		t.Fatal(err)
	}
	workStore, err := NewFactoryV1WorkStore(eventStore, factory, signer, actor, conversation)
	if err != nil {
		t.Fatal(err)
	}
	head, err := eventStore.Head()
	if err != nil || head.IsNone() {
		t.Fatalf("bootstrap head: %v", err)
	}
	document, err := factoryv1.Canonicalize(validFactoryV1APIOrder(factoryv1.ChannelCompletedOrder, "FO-ORPHAN", "transpara-ai/hive"))
	if err != nil {
		t.Fatal(err)
	}
	link, err := workStore.SeedFactoryOrder(ctx, factoryv1.WorkSeed{
		OrderID: document.Order.DocID, Version: document.Order.Version,
		DocumentSHA256: document.SHA256, Markdown: document.Markdown,
		SourceSHA256:    document.Order.SourceReferences[0].SHA256,
		AcceptedEventID: head.Unwrap().ID().Value(), IdempotencyKey: "orphan-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	intake, err := factoryv1.NewIntake(graph, workStore, factoryv1.WallClock{})
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		if err := intake.ReplayAndRepair(ctx); err != nil {
			t.Fatalf("replay %d: %v", run+1, err)
		}
	}
	quarantined, err := workStore.GetFactoryOrder(ctx, document.Order.DocID, document.Order.Version)
	if err != nil {
		t.Fatal(err)
	}
	taskID, err := types.NewEventID(link.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := workStore.tasks.ListArtifacts(taskID)
	if err != nil {
		t.Fatal(err)
	}
	quarantineCount := 0
	for _, artifact := range artifacts {
		if artifact.Label == factoryV1WorkQuarantineLabel {
			quarantineCount++
		}
	}
	if !quarantined.Quarantined || quarantineCount != 1 {
		t.Fatalf("orphan quarantine projection=%t artifacts=%d, want true and 1", quarantined.Quarantined, quarantineCount)
	}
}
