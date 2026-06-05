package hive

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/safety"
)

func TestOperatorProjectionServerRequiresBearerToken(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)
	handler := NewOperatorProjectionServer(s, "secret", 50)

	req := httptest.NewRequest(http.MethodGet, "/api/hive/operator-projection", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status without token = %d, want %d", resp.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/hive/operator-projection", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status with token = %d, want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
}

func TestOperatorProjectionServerReturnsProjectionJSON(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	requestID := newTestEventID(t)
	appendEvent(EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
		RequestID:        requestID,
		RequestingActor:  actorID,
		ActionName:       "agent.spawn.persistent",
		Target:           "builder",
		Environment:      "production",
		RequestedOutcome: "persistent identity",
		Justification:    "operator approved persistence trial",
		RiskSummary:      "persistent agent creation",
	})

	handler := NewOperatorProjectionServer(s, "", 50)
	req := httptest.NewRequest(http.MethodGet, "/api/hive/operator-projection", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var projection OperatorProjection
	if err := json.Unmarshal(resp.Body.Bytes(), &projection); err != nil {
		t.Fatalf("decode projection: %v", err)
	}
	if projection.Source != "eventgraph" {
		t.Fatalf("source = %q, want eventgraph", projection.Source)
	}
	if len(projection.PendingApprovals) != 1 || projection.PendingApprovals[0].RequestID != requestID.Value() {
		t.Fatalf("pending approvals = %+v", projection.PendingApprovals)
	}
	if projection.GeneratedAt.IsZero() {
		t.Fatal("generated_at is zero")
	}
}

func TestOperatorProjectionServerHealth(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)
	handler := NewOperatorProjectionServer(s, "secret", 50)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", resp.Code, http.StatusOK)
	}
	if resp.Body.String() != "ok" {
		t.Fatalf("health body = %q, want ok", resp.Body.String())
	}
}

func TestOperatorDecisionEndpointRecordsApproval(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorDecisionWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)

	// Sanity: the seeded request is pending before any decision.
	pre := BuildOperatorProjection(s, 50)
	if len(pre.PendingApprovals) != 1 || pre.PendingApprovals[0].RequestID != requestID.Value() {
		t.Fatalf("precondition: expected one pending draft-PR request, got %+v", pre.PendingApprovals)
	}

	body, _ := json.Marshal(map[string]string{
		"request_id": requestID.Value(),
		"decision":   "approved",
		"approver":   human.Value(),
		"reason":     "reviewed civic-roles.md",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/operator-decision", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("post decision: err=%v status=%v", err, resp.StatusCode)
	}
	resp.Body.Close()

	proj := BuildOperatorProjection(s, 50)
	if len(proj.PendingApprovals) != 0 {
		t.Fatalf("request should no longer be pending, got %d: %+v", len(proj.PendingApprovals), proj.PendingApprovals)
	}
	if len(proj.AuthorityDecisions) != 1 || proj.AuthorityDecisions[0].Outcome != "approved" {
		t.Fatalf("expected one approved decision, got %+v", proj.AuthorityDecisions)
	}
	decision := proj.AuthorityDecisions[0]
	if decision.RequestID != requestID.Value() {
		t.Fatalf("decision request id = %q, want %q", decision.RequestID, requestID.Value())
	}
	if decision.DeciderRole != "human" {
		t.Fatalf("decider role = %q, want human", decision.DeciderRole)
	}
	if decision.ApprovedAction != string(safety.ActionRepoPullRequestCreate) {
		t.Fatalf("approved action = %q, want %q", decision.ApprovedAction, safety.ActionRepoPullRequestCreate)
	}
}

func TestOperatorDecisionEndpointRejectsWithoutWriter(t *testing.T) {
	s, _, _, human, _ := newDecisionTestStore(t)
	// No writer option => server stays read-only; POST must not be served.
	srv := NewOperatorProjectionServer(s, "secret", 50)
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{
		"request_id": "auth_does_not_matter", "decision": "approved", "approver": human.Value(), "reason": "x",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/operator-decision", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post decision: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("read-only server POST status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestOperatorDecisionEndpointRequiresBearer(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorDecisionWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)
	body, _ := json.Marshal(map[string]string{
		"request_id": requestID.Value(), "decision": "approved", "approver": human.Value(), "reason": "x",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/operator-decision", bytes.NewReader(body))
	// no Authorization header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post decision: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	// The graph must remain unwritten when auth fails (hive stays sole gatekeeper).
	if proj := BuildOperatorProjection(s, 50); len(proj.AuthorityDecisions) != 0 {
		t.Fatalf("unauthenticated POST must not write a decision, got %+v", proj.AuthorityDecisions)
	}
}

// TestOperatorDecisionEndpointValidation exercises all negative-case input
// validation branches of POST /api/hive/operator-decision. Each sub-test
// asserts the correct 4xx status AND that no decision event was written to the
// graph (AuthorityDecisions stays empty).
func TestOperatorDecisionEndpointValidation(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorDecisionWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)

	doPost := func(t *testing.T, rawBody string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/operator-decision", bytes.NewBufferString(rawBody))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post request: %v", err)
		}
		resp.Body.Close()
		return resp
	}

	assertNoDecision := func(t *testing.T) {
		t.Helper()
		if proj := BuildOperatorProjection(s, 50); len(proj.AuthorityDecisions) != 0 {
			t.Fatalf("validation rejection must not write a decision, got %+v", proj.AuthorityDecisions)
		}
	}

	t.Run("malformed JSON body", func(t *testing.T) {
		resp := doPost(t, `{not valid json`)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("malformed JSON: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		assertNoDecision(t)
	})

	t.Run("unknown decision value", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"request_id": requestID.Value(),
			"decision":   "maybe",
			"approver":   human.Value(),
		})
		resp := doPost(t, string(body))
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("unknown decision: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		assertNoDecision(t)
	})

	t.Run("missing request_id", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"decision": "approved",
			"approver": human.Value(),
		})
		resp := doPost(t, string(body))
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("missing request_id: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		assertNoDecision(t)
	})

	t.Run("non-empty invalid approver", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"request_id": requestID.Value(),
			"decision":   "approved",
			"approver":   "not-a-valid-actor-id",
		})
		resp := doPost(t, string(body))
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid approver: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}
		assertNoDecision(t)
	})

	t.Run("request_id matches no pending request", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{
			"request_id": "auth_00000000000000000000000000000000",
			"decision":   "approved",
			"approver":   human.Value(),
		})
		resp := doPost(t, string(body))
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("unknown request_id: status = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
		assertNoDecision(t)
	})
}

// newDecisionTestStore builds an in-memory eventgraph store plus the
// factory/signer/human/conversation a decision writer needs, mirroring
// newOperatorProjectionStore but exposing the construction primitives.
func newDecisionTestStore(t *testing.T) (*store.InMemoryStore, *event.EventFactory, event.Signer, types.ActorID, types.ConversationID) {
	t.Helper()
	registry := event.DefaultRegistry()
	RegisterWithRegistry(registry)

	s := store.NewInMemoryStore()
	human := types.MustActorID("actor_00000000000000000000000000000077")
	signer := deriveSignerFromID(human)
	bootstrap, err := event.NewBootstrapFactory(registry).Init(human, signer)
	if err != nil {
		t.Fatalf("bootstrap init: %v", err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		t.Fatalf("append bootstrap: %v", err)
	}
	factory := event.NewEventFactory(registry)
	conv := types.MustConversationID("conv_00000000000000000000000000000077")
	return s, factory, signer, human, conv
}

// seedPendingDraftPRRequest appends an authority.request.recorded event for a
// draft-PR target (reusing the H2 DraftPRTarget.Scope() encoding) directly via
// the factory, then returns its RequestID. No decision is recorded, so the
// request surfaces in BuildOperatorProjection(...).PendingApprovals.
func seedPendingDraftPRRequest(t *testing.T, s store.Store, factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID) types.EventID {
	t.Helper()
	target := DraftPRTarget{
		Repository: "transpara-ai/docs", BaseRef: "main", BaseSHA: "basesha",
		HeadRef: "codex/civic-roles", HeadSHA: "headsha",
		TitleHash: "sha256:aaa", BodyHash: "sha256:bbb",
		PolicyBundleID: "df-v3.9.20-docs-draft-pr-create-only", PolicyBundleHash: "sha256:ccc",
		SingleUseNonce: "nonce-1",
	}
	// Mirror recordAuthorityRequest exactly: first append the authority.requested
	// anchor and capture its real event id, then append the authority.request.recorded
	// detail whose RequestID points back to that anchor. The projection keys the
	// pending request by content.RequestID, and the later decision causes that same
	// (real) anchor event — so the decision's causal link is valid.
	head, err := s.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	anchorCauses := []types.EventID{head.Unwrap().ID()}
	anchor, err := factory.Create(event.EventTypeAuthorityRequested, human, event.AuthorityRequestContent{
		Action:        string(safety.ActionRepoPullRequestCreate),
		Actor:         human,
		Level:         event.AuthorityLevelRequired,
		Justification: "Draft PR for civic-roles.md",
		Causes:        types.MustNonEmpty(anchorCauses),
	}, anchorCauses, conv, s, signer)
	if err != nil {
		t.Fatalf("create authority.requested: %v", err)
	}
	storedAnchor, err := s.Append(anchor)
	if err != nil {
		t.Fatalf("append authority.requested: %v", err)
	}
	requestID := storedAnchor.ID()

	content := AuthorityRequestRecordedContent{
		RequestID:         requestID,
		RequestingActor:   human,
		RequestingRole:    "guardian",
		ActionName:        string(safety.ActionRepoPullRequestCreate),
		Target:            target.Repository + " " + target.HeadRef,
		Environment:       "production",
		RiskClass:         safety.RiskClass(safety.ActionRepoPullRequestCreate),
		RequestedOutcome:  "create draft PR",
		Justification:     "Draft PR for civic-roles.md",
		RiskSummary:       "creates one reversible draft PR; no branch push, merge, or deploy",
		Scope:             target.Scope(),
		ProposedOperation: "createDraftPR",
	}
	detail, err := factory.Create(EventTypeAuthorityRequestRecorded, human, content, []types.EventID{requestID}, conv, s, signer)
	if err != nil {
		t.Fatalf("create authority.request.recorded: %v", err)
	}
	if _, err := s.Append(detail); err != nil {
		t.Fatalf("append authority.request.recorded: %v", err)
	}
	return requestID
}
