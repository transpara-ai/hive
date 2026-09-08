package hive

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	civ "github.com/transpara-ai/hive/pkg/hive/civilization"
)

type civilizationFailOnceChainStore struct {
	store.Store
	failed bool
}

func (s *civilizationFailOnceChainStore) Append(candidate event.Event) (event.Event, error) {
	if !s.failed {
		s.failed = true
		return event.Event{}, errors.New("chain integrity violation: concurrent append")
	}
	return s.Store.Append(candidate)
}

func TestCivilizationEventGraphRestartAndIdempotency(t *testing.T) {
	ctx := context.Background()
	eventStore, factory, signer, actor, conversation := newDecisionTestStore(t)
	newStore := func() *CivilizationEventGraphStore {
		result, err := NewCivilizationEventGraphStore(eventStore, factory, signer, actor, conversation)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	firstStore := newStore()
	first, err := firstStore.Append(ctx, civ.NewEvent{
		Type: civ.EventIntakeAccepted, WorkID: "work-restart", IdempotencyKey: "intake:restart",
		Payload: civ.Intake{Text: "Persist me"},
	})
	if err != nil {
		t.Fatal(err)
	}
	restarted := newStore()
	second, err := restarted.Append(ctx, civ.NewEvent{
		Type: civ.EventIntakeAccepted, WorkID: "work-restart", IdempotencyKey: "intake:restart",
		Payload: civ.Intake{Text: "Persist me"},
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := restarted.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || second.ID != first.ID || len(listed) != 1 {
		t.Fatalf("restart replay ids=(%q,%q) events=%d", first.ID, second.ID, len(listed))
	}
	_, err = restarted.Append(ctx, civ.NewEvent{
		Type: civ.EventIntakeAccepted, WorkID: "work-restart", IdempotencyKey: "intake:restart",
		Payload: civ.Intake{Text: "Different"},
	})
	if !errors.Is(err, civ.ErrIdempotencyConflict) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestCivilizationEventGraphRetriesConcurrentHeadRace(t *testing.T) {
	ctx := context.Background()
	base, factory, signer, actor, conversation := newDecisionTestStore(t)
	wrapper := &civilizationFailOnceChainStore{Store: base}
	graph, err := NewCivilizationEventGraphStore(wrapper, factory, signer, actor, conversation)
	if err != nil {
		t.Fatal(err)
	}
	appended, err := graph.Append(ctx, civ.NewEvent{
		Type: civ.EventStateChanged, WorkID: "work-race", IdempotencyKey: "state:race",
		Payload: civ.StateChange{To: civ.StateQueued, Summary: "Queued", NextAction: "Run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if appended.ID == "" || !wrapper.failed {
		t.Fatalf("retry result = %#v failed=%t", appended, wrapper.failed)
	}
}

func TestCivilizationHumanOwnerEventSurvivesSignedStoreRestart(t *testing.T) {
	eventStore, factory, signer, actor, conversation := newDecisionTestStore(t)
	first, err := NewCivilizationEventGraphStore(eventStore, factory, signer, actor, conversation)
	if err != nil {
		t.Fatal(err)
	}
	assignment := civ.HumanOwnerAssignment{OwnerID: "alice", AssignedBy: "bob"}
	written, err := first.Append(context.Background(), civ.NewEvent{Type: civ.EventHumanOwnerAssigned, WorkID: "work-assignment", IdempotencyKey: "owner:initial", Payload: assignment})
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewCivilizationEventGraphStore(eventStore, factory, signer, actor, conversation)
	if err != nil {
		t.Fatal(err)
	}
	items, err := restarted.List(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != written.ID || items[0].Type != civ.EventHumanOwnerAssigned {
		t.Fatalf("events=%+v error=%v", items, err)
	}
	var read civ.HumanOwnerAssignment
	if err := json.Unmarshal(items[0].Payload, &read); err != nil || read != assignment {
		t.Fatalf("assignment=%+v error=%v", read, err)
	}
}

func TestCivilizationResultReviewSurvivesSignedStoreRestart(t *testing.T) {
	eventStore, factory, signer, actor, conversation := newDecisionTestStore(t)
	first, err := NewCivilizationEventGraphStore(eventStore, factory, signer, actor, conversation)
	if err != nil {
		t.Fatal(err)
	}
	review := civ.ResultReview{ResultReviewRequest: civ.ResultReviewRequest{ResultID: "prepared-event", WorkspaceDigest: "exact-digest", Decision: "approve", ReviewedBy: "alice"}}
	written, err := first.Append(context.Background(), civ.NewEvent{Type: civ.EventResultReviewed, WorkID: "work-reviewed", IdempotencyKey: "review:exact-result", Payload: review})
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := NewCivilizationEventGraphStore(eventStore, factory, signer, actor, conversation)
	if err != nil {
		t.Fatal(err)
	}
	items, err := restarted.List(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != written.ID || items[0].Type != civ.EventResultReviewed {
		t.Fatalf("events=%+v %v", items, err)
	}
	var read civ.ResultReview
	if err = json.Unmarshal(items[0].Payload, &read); err != nil || read != review {
		t.Fatalf("review=%+v %v", read, err)
	}
}
