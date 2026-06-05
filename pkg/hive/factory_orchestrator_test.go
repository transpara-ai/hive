package hive

import (
	"context"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

type fakePRClient struct{ calls int }

func (f *fakePRClient) CreateDraftPullRequest(_ context.Context, m work.Epic11DraftPullRequestMutation) (work.Epic11DraftPullRequestResult, error) {
	f.calls++
	return work.Epic11DraftPullRequestResult{
		Repository: m.Repository, Number: 111, URL: "https://github.com/transpara-ai/docs/pull/111",
		GitHubResponseIDOrEquivalent: "node111", BaseRef: m.BaseRef, BaseSHA: m.BaseSHA,
		HeadRef: m.HeadRef, HeadSHA: m.HeadSHA, Draft: true, State: "open",
		CreatedAt: time.Now().UTC(),
	}, nil
}

// newWorkTaskStore constructs a *work.TaskStore backed by an in-memory store
// using only exported APIs. It mirrors the pattern established by
// newDecisionTestStore, adding work.RegisterWithRegistry so the task store
// can emit work event types. It returns the bootstrap event ID as the initial
// cause so callers can satisfy the eventgraph "at least one cause" constraint.
func newWorkTaskStore(t *testing.T) (*work.TaskStore, types.ActorID, types.ConversationID, types.EventID) {
	t.Helper()
	registry := event.DefaultRegistry()
	RegisterWithRegistry(registry)
	work.RegisterWithRegistry(registry)

	s := store.NewInMemoryStore()
	source := types.MustActorID("actor_00000000000000000000000000000042")
	signer := deriveSignerFromID(source)
	bootstrap, err := event.NewBootstrapFactory(registry).Init(source, signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	stored, err := s.Append(bootstrap)
	if err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	factory := event.NewEventFactory(registry)
	conv := types.MustConversationID("conv_00000000000000000000000000000042")
	ts := work.NewTaskStore(s, factory, signer)
	return ts, source, conv, stored.ID()
}

func TestCreateDraftPRFromApprovedDecision(t *testing.T) {
	ts, source, conv, cause := newWorkTaskStore(t)
	client := &fakePRClient{}
	in := DraftPRArtifact{
		Target: DraftPRTarget{
			Repository: "transpara-ai/docs", BaseRef: "main", BaseSHA: "basesha",
			HeadRef: "codex/civic-roles", HeadSHA: "headsha",
			TitleHash: "", BodyHash: "", // authority-scope fields; unused by this orchestrator path
			PolicyBundleID: work.Epic11PolicyBundleID, PolicyBundleHash: work.Epic11DocsDraftPRPolicyBundleHash(),
			SingleUseNonce: "nonce-civic-roles",
		},
		Title: "[codex] Document the civic roles", Body: "## Summary\n",
		ChangedFiles: []string{"dark-factory/civic-roles.md"},
		ActorRole:    "implementer", DeciderActorID: "act_human", DeciderRole: "human",
	}

	run, err := CreateDraftPRFromApprovedDecision(context.Background(), ts, source, conv, client, in, cause)
	if err != nil {
		t.Fatalf("CreateDraftPRFromApprovedDecision: %v", err)
	}
	if client.calls != 1 || run.MutationResult.Number != 111 {
		t.Fatalf("unexpected: calls=%d result=%+v", client.calls, run.MutationResult)
	}
}
