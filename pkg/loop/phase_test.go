package loop

import (
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

var (
	phaseTestActor = types.MustActorID("actor_00000000000000000000000000000011")
	phaseTestConv  = types.MustConversationID("conv_00000000000000000000000000000011")
)

func newPhaseCommandStore(t *testing.T) (*work.PhaseGateStore, []types.EventID) {
	t.Helper()
	_, g := agentWithGraph(t, newMockProvider(`/signal {"signal":"IDLE"}`))
	factory := event.NewEventFactory(g.Registry())
	head, err := g.Store().Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head.IsNone() {
		t.Fatal("expected agent boot event cause")
	}
	return work.NewPhaseGateStore(g.Store(), factory, &testSigner{}), []types.EventID{head.Unwrap().ID()}
}

func TestParsePhaseCommands(t *testing.T) {
	response := `notes
/phase gate {"phase":"design","title":"Approve design","criteria":["brief"]}
/phase approve {"gate_id":"evt_00000000000000000000000000000001","summary":"ok"}`

	got := parsePhaseCommands(response)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Action != "gate" || got[1].Action != "approve" {
		t.Fatalf("actions = %#v", got)
	}
}

func TestExecutePhaseCommandsDeclareApprove(t *testing.T) {
	gates, causes := newPhaseCommandStore(t)

	executed := executePhaseCommands(parsePhaseCommands(`/phase gate {"phase":"design","title":"Approve design","criteria":["brief"]}`), gates, phaseTestActor, causes, phaseTestConv)
	if executed != 1 {
		t.Fatalf("executed = %d, want 1", executed)
	}
	list, err := gates.List(10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Status != work.PhaseGatePending {
		t.Fatalf("gates = %#v", list)
	}

	cmd := `/phase approve {"gate_id":"` + list[0].ID.Value() + `","summary":"accepted"}`
	executed = executePhaseCommands(parsePhaseCommands(cmd), gates, phaseTestActor, causes, phaseTestConv)
	if executed != 1 {
		t.Fatalf("approve executed = %d, want 1", executed)
	}
	state, ok, err := gates.Get(list[0].ID)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if state.Status != work.PhaseGateApproved || state.Summary != "accepted" {
		t.Fatalf("state = %#v", state)
	}
}
