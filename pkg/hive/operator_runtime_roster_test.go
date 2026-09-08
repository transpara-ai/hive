package hive

import (
	"fmt"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func TestRuntimeRosterSurvivesBoundedEventWindow(t *testing.T) {
	s, actorID, _ := newOperatorProjectionStore(t)
	conversation := types.MustConversationID("conv_long_running_daemon")
	appendEvent := func(kind types.EventType, content event.EventContent) {
		appendOperatorProjectionEventWithConversation(t, s, actorID, conversation, kind, content)
	}
	appendEvent(EventTypeRunStarted, RunStartedContent{Idea: "daemon", RepoPath: "/tmp/hive"})
	for _, name := range []string{"guardian", "implementer"} {
		appendEvent(EventTypeAgentSpawned, AgentSpawnedContent{Name: name, Role: name, Model: "test-model", ActorID: "actor_" + name})
	}
	// Enough same-conversation events to push all lifecycle evidence outside
	// the display window, as real daemon reasoning/state events do.
	for i := 0; i < 15; i++ {
		appendEvent(EventTypeFactoryArtifactCreated, FactoryArtifactCreatedContent{RunID: "run_daemon", ArtifactID: fmt.Sprintf("artifact_%d", i), Label: "trace", Title: "progress", MediaType: "text/plain"})
	}
	projection := BuildOperatorProjection(s, 5)
	if len(projection.Errors) != 0 {
		t.Fatal(projection.Errors)
	}
	if got := projection.RuntimeEvidence.AgentEvents.ObservedActive; got != 2 {
		t.Fatalf("active = %d, want 2 after paging beyond the display window", got)
	}
	if len(projection.RuntimeEvidence.RunEvents) > 6 {
		t.Fatal("lifecycle replay expanded the displayed event window")
	}
	appendEvent(EventTypeAgentStopped, AgentStoppedContent{Name: "implementer", Role: "implementer", StopReason: "complete", Iterations: 1})
	projection = BuildOperatorProjection(s, 5)
	if got := projection.RuntimeEvidence.AgentEvents.ActiveAgents; len(got) != 1 || got[0].Name != "guardian" {
		t.Fatalf("active = %+v, want only guardian", got)
	}
	appendEvent(EventTypeRunCompleted, RunCompletedContent{AgentCount: 2, DurationMs: 100})
	projection = BuildOperatorProjection(s, 5)
	if projection.RuntimeEvidence.Status != "completed" || projection.RuntimeEvidence.AgentEvents.ObservedActive != 0 {
		t.Fatalf("completed runtime = %+v", projection.RuntimeEvidence)
	}
	// Reusing a conversation must not resurrect agents from a previous run.
	appendEvent(EventTypeRunStarted, RunStartedContent{Idea: "next run", RepoPath: "/tmp/hive"})
	for i := 0; i < 7; i++ {
		appendEvent(EventTypeFactoryArtifactCreated, FactoryArtifactCreatedContent{RunID: "run_next", ArtifactID: fmt.Sprintf("next_%d", i), Label: "trace", Title: "progress", MediaType: "text/plain"})
	}
	projection = BuildOperatorProjection(s, 5)
	if projection.RuntimeEvidence.Status != "running" || projection.RuntimeEvidence.AgentEvents.ObservedActive != 0 {
		t.Fatalf("new runtime inherited a previous run: %+v", projection.RuntimeEvidence)
	}
}
