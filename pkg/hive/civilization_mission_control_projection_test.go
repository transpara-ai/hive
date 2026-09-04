package hive

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

type missionControlClock struct {
	now time.Time
}

func (c *missionControlClock) Now() time.Time { return c.now }

func TestMissionControlCompletesWithoutEmbeddedFactoryRuntime(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)
	now := time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
	projector, err := NewCivilizationMissionControlProjector(s, MissionControlProjectorConfig{
		Clock: &missionControlClock{now: now}, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	projection := projector.Build(context.Background())
	if !projection.Completeness.Complete {
		t.Fatalf("projection incomplete without retired runtime: %+v", projection.Completeness.Reasons)
	}
	if len(projection.Sources) != 3 {
		t.Fatalf("sources = %+v, want three EventGraph sources", projection.Sources)
	}
	for _, source := range projection.Sources {
		if strings.Contains(source.SourceID, "factory") {
			t.Fatalf("retired Factory source remains required: %+v", source)
		}
	}
	for _, service := range projection.Services {
		if strings.Contains(service.ServiceID, "factory") {
			t.Fatalf("retired Factory service remains active: %+v", service)
		}
	}
	if projection.OperationalStatus != "healthy" {
		t.Fatalf("operational status = %q, want healthy", projection.OperationalStatus)
	}
}

func TestMissionControlProjectsWorkWithoutInferringTLCPolicy(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	now := time.Now().UTC().Add(time.Hour)
	task := appendEvent(work.EventTypeTaskCreated, work.TaskCreatedContent{
		Title: "Canonical Workbench task", CreatedBy: actorID,
		FactoryOrderID: "historical-order-reference",
	})
	projector, err := NewCivilizationMissionControlProjector(s, MissionControlProjectorConfig{
		Clock: &missionControlClock{now: now}, PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	projection := projector.Build(context.Background())
	if len(projection.WIP) != 1 {
		t.Fatalf("WIP = %+v, want one Work task", projection.WIP)
	}
	row := projection.WIP[0]
	if row.WorkTaskID != task.ID().Value() || row.FactoryOrderID != "historical-order-reference" {
		t.Fatalf("row identity = %+v", row)
	}
	if row.Kind != "independent_work_task" || row.EngineProtocol.Value != "work-v3.9" {
		t.Fatalf("row contract = %+v", row)
	}
}

type missionFailStore struct {
	store.Store
	mu        sync.Mutex
	fail      bool
	eventType types.EventType
}

func (s *missionFailStore) setFail(value bool) {
	s.mu.Lock()
	s.fail = value
	s.mu.Unlock()
}

func (s *missionFailStore) ByType(eventType types.EventType, limit int, after types.Option[types.Cursor]) (types.Page[event.Event], error) {
	s.mu.Lock()
	fail := s.fail
	s.mu.Unlock()
	if fail && eventType == s.eventType {
		return types.Page[event.Event]{}, errors.New("injected source read failure")
	}
	return s.Store.ByType(eventType, limit, after)
}

func TestMissionControlRetainsBoundedStaleEvidence(t *testing.T) {
	base, _, _ := newOperatorProjectionStore(t)
	now := time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
	failing := &missionFailStore{Store: base, eventType: work.EventTypeTaskCreated}
	clock := &missionControlClock{now: now}
	projector, err := NewCivilizationMissionControlProjector(failing, MissionControlProjectorConfig{
		Clock: clock, PageSize: 10, Retention: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := projector.Build(context.Background())
	if !first.Completeness.Complete {
		t.Fatalf("initial projection incomplete: %+v", first.Completeness.Reasons)
	}
	failing.setFail(true)
	clock.now = now.Add(30 * time.Second)
	stale := projector.Build(context.Background())
	if stale.Sources[0].Mark.State != StateStale {
		t.Fatalf("cached WIP source state = %q, want stale", stale.Sources[0].Mark.State)
	}
	clock.now = now.Add(2 * time.Minute)
	expired := projector.Build(context.Background())
	if expired.Sources[0].Mark.State != StateUnavailable {
		t.Fatalf("expired WIP source state = %q, want unavailable", expired.Sources[0].Mark.State)
	}
}
