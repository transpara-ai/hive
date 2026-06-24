package hive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

type fakePRClient struct {
	calls          int
	preflightHead  string
	preflightFiles []string
	resultRepo     string
	resultURL      string
	resultDraft    *bool
	resultState    string
	resultBaseSHA  string
	forceBaseSHA   bool
}

func (f *fakePRClient) CreateDraftPullRequest(_ context.Context, m work.Epic11DraftPullRequestMutation) (work.Epic11DraftPullRequestResult, error) {
	f.calls++
	repo := m.Repository
	if f.resultRepo != "" {
		repo = f.resultRepo
	}
	url := "https://github.com/" + repo + "/pull/111"
	if f.resultURL != "" {
		url = f.resultURL
	}
	draft := true
	if f.resultDraft != nil {
		draft = *f.resultDraft
	}
	state := "open"
	if f.resultState != "" {
		state = f.resultState
	}
	baseSHA := m.BaseSHA
	if f.forceBaseSHA {
		baseSHA = f.resultBaseSHA
	} else if f.resultBaseSHA != "" {
		baseSHA = f.resultBaseSHA
	}
	return work.Epic11DraftPullRequestResult{
		Repository: repo, Number: 111, URL: url,
		GitHubResponseIDOrEquivalent: "node111", BaseRef: m.BaseRef, BaseSHA: baseSHA,
		HeadRef: m.HeadRef, HeadSHA: m.HeadSHA, Draft: draft, State: state,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// PreflightHead satisfies the work#44 Epic11PullRequestCreator preflight: it
// reports the approved head SHA and a single in-scope dark-factory/ file so
// epic11ValidateRemoteHead passes. It is not a mutation, so it does not count
// toward calls.
func (f *fakePRClient) PreflightHead(_ context.Context, m work.Epic11DraftPullRequestMutation) (work.Epic11RemoteHeadState, error) {
	head := m.HeadSHA
	if f.preflightHead != "" {
		head = f.preflightHead
	}
	files := f.preflightFiles
	if len(files) == 0 {
		files = []string{"dark-factory/civic-roles.md"}
	}
	return work.Epic11RemoteHeadState{HeadSHA: head, ChangedFiles: files}, nil
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

func TestCreateTransparaAIDraftPRFromApprovedDecision(t *testing.T) {
	ts, source, conv, cause := newWorkTaskStore(t)
	client := &fakePRClient{preflightFiles: []string{"src/App.tsx", "package.json"}}
	title := "[codex] Display Civilization runtime evidence"
	body := "## Summary\nDisplay runtime evidence for Transpara-AI operators.\n"
	target := DraftPRTarget{
		Repository:       "transpara-ai/site",
		BaseRef:          "main",
		BaseSHA:          "basesha",
		HeadRef:          "codex/site-civilization-runtime",
		HeadSHA:          "headsha",
		TitleHash:        sha256HexPrefixed([]byte(title)),
		BodyHash:         sha256HexPrefixed([]byte(body)),
		PolicyBundleID:   TransparaAIDraftPRPolicyBundleID,
		PolicyBundleHash: TransparaAIDraftPRPolicyBundleHash(),
		SingleUseNonce:   "nonce-site",
	}

	run, err := CreateTransparaAIDraftPRFromApprovedDecision(context.Background(), ts, source, conv, client, DraftPRArtifact{
		Target:         target,
		Title:          title,
		Body:           body,
		ChangedFiles:   []string{"package.json", "src/App.tsx"},
		ActorRole:      "implementer",
		DeciderActorID: "act_human",
		DeciderRole:    "human",
	}, cause)
	if err != nil {
		t.Fatalf("CreateTransparaAIDraftPRFromApprovedDecision: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("client calls = %d, want 1", client.calls)
	}
	if run.MutationResult.Repository != "transpara-ai/site" || run.MutationResult.URL != "https://github.com/transpara-ai/site/pull/111" {
		t.Fatalf("mutation result = %+v", run.MutationResult)
	}
	artifacts, err := ts.ListArtifacts(run.WorkTask.ID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].Label != TransparaAIDraftPRReceiptArtifactLabel {
		t.Fatalf("artifacts = %+v, want one draft PR receipt", artifacts)
	}
	var receipt TransparaAIDraftPRReceipt
	if err := json.Unmarshal([]byte(artifacts[0].Body), &receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	if receipt.Repository != "transpara-ai/site" || receipt.PRURL != "https://github.com/transpara-ai/site/pull/111" || !receipt.HumanApprovalRequired || !receipt.NoMergeOrDeployClaim || !receipt.ReadyForReviewRequired {
		t.Fatalf("receipt = %+v", receipt)
	}
	if strings.Join(receipt.ChangedFiles, ",") != "package.json,src/App.tsx" {
		t.Fatalf("changed files = %+v, want sorted package/src files", receipt.ChangedFiles)
	}
}

func TestCreateTransparaAIDraftPRAcceptsAdvancedBaseSHA(t *testing.T) {
	ts, source, conv, cause := newWorkTaskStore(t)
	client := &fakePRClient{
		preflightFiles: []string{"src/App.tsx"},
		resultBaseSHA:  "live-base-sha",
	}
	title := "[codex] Display Civilization runtime evidence"
	body := "## Summary\nDisplay runtime evidence for Transpara-AI operators.\n"
	target := DraftPRTarget{
		Repository:       "transpara-ai/site",
		BaseRef:          "main",
		BaseSHA:          "approved-base-sha",
		HeadRef:          "codex/site-civilization-runtime",
		HeadSHA:          "headsha",
		TitleHash:        sha256HexPrefixed([]byte(title)),
		BodyHash:         sha256HexPrefixed([]byte(body)),
		PolicyBundleID:   TransparaAIDraftPRPolicyBundleID,
		PolicyBundleHash: TransparaAIDraftPRPolicyBundleHash(),
		SingleUseNonce:   "nonce-site",
	}

	run, err := CreateTransparaAIDraftPRFromApprovedDecision(context.Background(), ts, source, conv, client, DraftPRArtifact{
		Target:       target,
		Title:        title,
		Body:         body,
		ChangedFiles: []string{"src/App.tsx"},
	}, cause)
	if err != nil {
		t.Fatalf("CreateTransparaAIDraftPRFromApprovedDecision with advanced base SHA: %v", err)
	}
	if run.MutationResult.BaseSHA != "live-base-sha" {
		t.Fatalf("mutation result base SHA = %q, want live base", run.MutationResult.BaseSHA)
	}
	if run.Receipt.BaseSHA != "approved-base-sha" {
		t.Fatalf("receipt base SHA = %q, want approved base evidence", run.Receipt.BaseSHA)
	}
}

func TestCreateTransparaAIDraftPRRejectsEmptyResultBaseSHA(t *testing.T) {
	ts, source, conv, cause := newWorkTaskStore(t)
	client := &fakePRClient{
		preflightFiles: []string{"src/App.tsx"},
		forceBaseSHA:   true,
	}
	title := "[codex] Display Civilization runtime evidence"
	body := "## Summary\nDisplay runtime evidence for Transpara-AI operators.\n"
	target := DraftPRTarget{
		Repository:       "transpara-ai/site",
		BaseRef:          "main",
		BaseSHA:          "approved-base-sha",
		HeadRef:          "codex/site-civilization-runtime",
		HeadSHA:          "headsha",
		TitleHash:        sha256HexPrefixed([]byte(title)),
		BodyHash:         sha256HexPrefixed([]byte(body)),
		PolicyBundleID:   TransparaAIDraftPRPolicyBundleID,
		PolicyBundleHash: TransparaAIDraftPRPolicyBundleHash(),
		SingleUseNonce:   "nonce-site",
	}

	_, err := CreateTransparaAIDraftPRFromApprovedDecision(context.Background(), ts, source, conv, client, DraftPRArtifact{
		Target:       target,
		Title:        title,
		Body:         body,
		ChangedFiles: []string{"src/App.tsx"},
	}, cause)
	if err == nil || !strings.Contains(err.Error(), "created PR base_sha is empty") {
		t.Fatalf("CreateTransparaAIDraftPRFromApprovedDecision error = %v, want empty base_sha refusal", err)
	}
}

func TestCreateTransparaAIDraftPRRejectsUnsafeRemoteChangedFile(t *testing.T) {
	ts, source, conv, cause := newWorkTaskStore(t)
	client := &fakePRClient{preflightFiles: []string{"../secrets.env"}}
	title := "[codex] Display Civilization runtime evidence"
	body := "## Summary\nDisplay runtime evidence.\n"
	target := DraftPRTarget{
		Repository:       "transpara-ai/site",
		BaseRef:          "main",
		BaseSHA:          "basesha",
		HeadRef:          "codex/site-civilization-runtime",
		HeadSHA:          "headsha",
		TitleHash:        sha256HexPrefixed([]byte(title)),
		BodyHash:         sha256HexPrefixed([]byte(body)),
		PolicyBundleID:   TransparaAIDraftPRPolicyBundleID,
		PolicyBundleHash: TransparaAIDraftPRPolicyBundleHash(),
		SingleUseNonce:   "nonce-site",
	}

	_, err := CreateTransparaAIDraftPRFromApprovedDecision(context.Background(), ts, source, conv, client, DraftPRArtifact{
		Target:       target,
		Title:        title,
		Body:         body,
		ChangedFiles: []string{"../secrets.env"},
	}, cause)
	if err == nil || !strings.Contains(err.Error(), "escapes repository root") {
		t.Fatalf("error = %v, want repository-root refusal", err)
	}
	if client.calls != 0 {
		t.Fatalf("client create calls = %d, want 0 before refused mutation", client.calls)
	}
}
