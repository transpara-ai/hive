package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive"
	"github.com/transpara-ai/hive/pkg/social"
	"github.com/transpara-ai/work"
)

func TestRegisterOpsAPIEventTypesHandlesSharedStoreEvents(t *testing.T) {
	registerOpsAPIEventTypes()
	t.Cleanup(func() { event.SetFallbackUnmarshaler(nil) })

	got, err := event.UnmarshalContent(social.EventTypePostCreated.Value(), []byte(`{"Author":"actor_00000000000000000000000000000077","Body":"hello"}`))
	if err != nil {
		t.Fatalf("unmarshal social post: %v", err)
	}
	if _, ok := got.(social.PostCreatedContent); !ok {
		t.Fatalf("social post content type = %T, want social.PostCreatedContent", got)
	}

	raw, err := event.UnmarshalContent("foreign.event.type", []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("unmarshal unknown shared-store event: %v", err)
	}
	if _, ok := raw.(event.RawContent); !ok {
		t.Fatalf("unknown event content type = %T, want event.RawContent", raw)
	}

	if _, err := event.UnmarshalContent(social.EventTypePostCreated.Value(), []byte(`{`)); err == nil {
		t.Fatal("malformed registered social event decoded successfully; fallback must only handle unknown event types")
	}
}

func TestRegisterOpsAPIEventTypesProjectsWorkFactoryOrderThroughRoute(t *testing.T) {
	registerOpsAPIEventTypes()
	t.Cleanup(func() { event.SetFallbackUnmarshaler(nil) })

	registry := event.DefaultRegistry()
	hive.RegisterWithRegistry(registry)
	work.RegisterWithRegistry(registry)
	s := store.NewInMemoryStore()
	actorID := types.MustActorID("actor_00000000000000000000000000000079")
	signer := newOpsSigner("ops-api-work-factory-order-test")
	bootstrap, err := event.NewBootstrapFactory(registry).Init(actorID, signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	factory := event.NewEventFactory(registry)
	head, err := s.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	taskEvent, err := factory.Create(work.EventTypeTaskCreated, actorID, work.TaskCreatedContent{
		Title:                  "Route-visible FactoryOrder",
		Description:            "Ops API must decode Work task content through production registration.",
		CreatedBy:              actorID,
		FactoryOrderID:         "fo_ops_api_route_001",
		RequirementIDs:         []string{"req_ops_api_route_001"},
		AcceptanceCriterionIDs: []string{"ac_ops_api_route_001"},
		RiskClass:              "high",
	}, []types.EventID{head.Unwrap().ID()}, types.MustConversationID("conv_00000000000000000000000000000079"), s, signer)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := s.Append(taskEvent); err != nil {
		t.Fatalf("append task: %v", err)
	}

	handler := hive.NewOperatorProjectionServer(opsAPIRoundTripStore{Store: s}, "", 50)
	req := httptest.NewRequest(http.MethodGet, "/api/hive/civilization/assembly-projection", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var projection hive.CivilizationAssemblyProjection
	if err := json.Unmarshal(resp.Body.Bytes(), &projection); err != nil {
		t.Fatalf("decode projection: %v", err)
	}
	if len(projection.FactoryOrderSummary) != 1 {
		t.Fatalf("factory orders = %+v, want one", projection.FactoryOrderSummary)
	}
	order := projection.FactoryOrderSummary[0]
	if order.ID != "fo_ops_api_route_001" || order.Status != "work_task_seeded" {
		t.Fatalf("factory order = %+v, want decoded seeded FactoryOrder", order)
	}
	if len(order.TaskRefs) != 1 || order.TaskRefs[0] != taskEvent.ID().Value() {
		t.Fatalf("task refs = %+v, want %s", order.TaskRefs, taskEvent.ID().Value())
	}
}

type opsAPIRoundTripStore struct {
	store.Store
}

func (s opsAPIRoundTripStore) Get(id types.EventID) (event.Event, error) {
	ev, err := s.Store.Get(id)
	if err != nil {
		return ev, err
	}
	return roundTripOpsAPIEventContent(ev)
}

func (s opsAPIRoundTripStore) Head() (types.Option[event.Event], error) {
	head, err := s.Store.Head()
	if err != nil || !head.IsSome() {
		return head, err
	}
	ev, err := roundTripOpsAPIEventContent(head.Unwrap())
	if err != nil {
		return types.None[event.Event](), err
	}
	return types.Some(ev), nil
}

func (s opsAPIRoundTripStore) Recent(limit int, after types.Option[types.Cursor]) (types.Page[event.Event], error) {
	page, err := s.Store.Recent(limit, after)
	if err != nil {
		return page, err
	}
	return roundTripOpsAPIEventPage(page)
}

func (s opsAPIRoundTripStore) ByType(eventType types.EventType, limit int, after types.Option[types.Cursor]) (types.Page[event.Event], error) {
	page, err := s.Store.ByType(eventType, limit, after)
	if err != nil {
		return page, err
	}
	return roundTripOpsAPIEventPage(page)
}

func roundTripOpsAPIEventPage(page types.Page[event.Event]) (types.Page[event.Event], error) {
	events := make([]event.Event, 0, len(page.Items()))
	for _, ev := range page.Items() {
		next, err := roundTripOpsAPIEventContent(ev)
		if err != nil {
			return types.Page[event.Event]{}, err
		}
		events = append(events, next)
	}
	return types.NewPage(events, page.Cursor(), page.HasMore()), nil
}

func roundTripOpsAPIEventContent(ev event.Event) (event.Event, error) {
	contentJSON, err := json.Marshal(ev.Content())
	if err != nil {
		return event.Event{}, err
	}
	content, err := event.UnmarshalContent(ev.Type().Value(), contentJSON)
	if err != nil {
		return event.Event{}, err
	}
	if ev.IsBootstrap() {
		bootstrap, ok := content.(event.BootstrapContent)
		if !ok {
			return event.Event{}, fmt.Errorf("bootstrap content type = %T, want event.BootstrapContent", content)
		}
		return event.NewBootstrapEvent(ev.Version(), ev.ID(), ev.Type(), ev.Timestamp(), ev.Source(), bootstrap, ev.ConversationID(), ev.Hash(), ev.Signature()), nil
	}
	return event.NewEvent(ev.Version(), ev.ID(), ev.Type(), ev.Timestamp(), ev.Source(), content, ev.Causes(), ev.ConversationID(), ev.Hash(), ev.PrevHash(), ev.Signature()), nil
}
