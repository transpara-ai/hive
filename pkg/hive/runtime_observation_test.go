package hive

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/loop"
)

func TestRuntimeObservationLifecycleAndExpiry(t *testing.T) {
	r := &Runtime{}
	observe := r.observeAgentRuntime("actor_live", "guardian")
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "runtime.json")
	rows := []RoleAgentRow{{ActorID: "actor_live", Role: "guardian"}, {ActorID: "actor_old", Role: "guardian"}, {Role: "guardian"}}
	for _, state := range []loop.RuntimeState{loop.RuntimeWorking, loop.RuntimeWaiting, loop.RuntimeBudgetWait, loop.RuntimeStopped} {
		observe(state)
		if err := writeRuntimeObservation(path, r.runtimeObservation(now)); err != nil {
			t.Fatal(err)
		}
		service := applyRuntimeObservations(rows, path, now)
		if service.OperationalStatus != "healthy" || rows[0].Running.Value != (state != loop.RuntimeStopped) || rows[0].Status.Value != string(state) {
			t.Fatalf("bad %s projection: %+v", state, rows)
		}
		if rows[0].Running.Mark.Basis != BasisProjectedOnly || rows[1].Running.Value != false {
			t.Fatalf("runtime evidence confused with durable identity: %+v", rows)
		}
		count := 1
		if state == loop.RuntimeStopped {
			count = 0
		}
		if rows[2].Running.Value != count {
			t.Fatalf("role rollup wrong: %+v", rows[2])
		}
	}
	service := applyRuntimeObservations(rows, path, now.Add(runtimeObservationMaxAge+time.Second))
	if service.OperationalStatus != "unavailable" || rows[0].Running.Value != nil || rows[0].Running.Mark.Freshness != FreshnessUnavailable {
		t.Fatal("expired process report stayed live")
	}
}

func TestRuntimeObservationRejectsInvalidIdentityAndClock(t *testing.T) {
	now := time.Now().UTC()
	base := RuntimeObservation{Schema: runtimeObservationSchema, ObservedAt: now, Agents: []RuntimeAgentObservation{{ActorID: "actor_one", Role: "guardian", State: loop.RuntimeWaiting}}}
	path := filepath.Join(t.TempDir(), "runtime.json")
	for _, change := range []func(*RuntimeObservation){
		func(o *RuntimeObservation) { o.ObservedAt = now.Add(time.Minute) },
		func(o *RuntimeObservation) { o.Schema = "unknown" },
		func(o *RuntimeObservation) { o.Agents = append(o.Agents, o.Agents[0]) },
		func(o *RuntimeObservation) {
			o.Agents = []RuntimeAgentObservation{{ActorID: "actor_one", Role: "guardian", State: "invented"}}
		},
	} {
		observation := base
		change(&observation)
		if err := writeRuntimeObservation(path, observation); err != nil {
			t.Fatal(err)
		}
		if _, err := readRuntimeObservation(path, now); err == nil {
			t.Fatal("invalid runtime report accepted")
		}
	}
	if err := writeRuntimeObservation(path, base); err != nil {
		t.Fatal(err)
	}
	rows := []RoleAgentRow{{ActorID: "actor_one", Role: "implementer"}}
	applyRuntimeObservations(rows, path, now)
	if rows[0].Running.Value != nil {
		t.Fatal("cross-role identity mismatch granted liveness")
	}
}
