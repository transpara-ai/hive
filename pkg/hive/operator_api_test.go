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

func TestOperatorProjectionServerReturnsConfiguredModelSelection(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)
	modelSelection := testModelSelectionConfigWithRoleDefault("guardian", "api-sonnet")
	handler := NewOperatorProjectionServer(s, "", 50, WithOperatorProjectionModelSelection(modelSelection))

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
	if projection.ModelSelection.CatalogSource != "test-explicit-role-default" {
		t.Fatalf("catalog source = %q, want test-explicit-role-default", projection.ModelSelection.CatalogSource)
	}
	guardian := requireModelAssignment(t, projection.ModelSelection, "guardian")
	if guardian.AuthMode != "api-key" || guardian.Provider != "anthropic" {
		t.Fatalf("guardian assignment = %+v, want explicit anthropic api-key", guardian)
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

func TestOperatorRunLaunchEndpointRequiresBearer(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorRunLaunchWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/runs", bytes.NewReader(validRunLaunchBody(t)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post run launch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated POST status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	assertNoRunLaunchEvents(t, s)
}

func TestOperatorRunLaunchEndpointValidation(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorRunLaunchWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	doPost := func(t *testing.T, rawBody string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/runs", bytes.NewBufferString(rawBody))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post run launch: %v", err)
		}
		resp.Body.Close()
		return resp
	}

	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON body", body: `{not valid json`},
		{name: "missing operator_id", body: `{"intake_id":"intake_127","title":"Launch","brief":{},"sources":[{"type":"issue","ref":"https://github.com/transpara-ai/hive/issues/127"}],"authority":{"initial_level":"required"},"budget":{"max_iterations":4,"max_cost_usd":12.5},"target_repos":["transpara-ai/hive"]}`},
		{name: "missing sources", body: `{"operator_id":"user_127","intake_id":"intake_127","title":"Launch","brief":{},"authority":{"initial_level":"required"},"budget":{"max_iterations":4,"max_cost_usd":12.5},"target_repos":["transpara-ai/hive"]}`},
		{name: "missing authority", body: `{"operator_id":"user_127","intake_id":"intake_127","title":"Launch","brief":{},"sources":[{"type":"issue","ref":"https://github.com/transpara-ai/hive/issues/127"}],"budget":{"max_iterations":4,"max_cost_usd":12.5},"target_repos":["transpara-ai/hive"]}`},
		{name: "missing budget field", body: `{"operator_id":"user_127","intake_id":"intake_127","title":"Launch","brief":{},"sources":[{"type":"issue","ref":"https://github.com/transpara-ai/hive/issues/127"}],"authority":{"initial_level":"required"},"budget":{"max_iterations":4},"target_repos":["transpara-ai/hive"]}`},
		{name: "unsafe target repo", body: `{"operator_id":"user_127","intake_id":"intake_127","title":"Launch","brief":{},"sources":[{"type":"issue","ref":"https://github.com/transpara-ai/hive/issues/127"}],"authority":{"initial_level":"required"},"budget":{"max_iterations":4,"max_cost_usd":12.5},"target_repos":["../site"]}`},
		{name: "dot target repo component", body: `{"operator_id":"user_127","intake_id":"intake_127","title":"Launch","brief":{},"sources":[{"type":"issue","ref":"https://github.com/transpara-ai/hive/issues/127"}],"authority":{"initial_level":"required"},"budget":{"max_iterations":4,"max_cost_usd":12.5},"target_repos":["transpara-ai/.git"]}`},
		{name: "unknown authority level", body: `{"operator_id":"user_127","intake_id":"intake_127","title":"Launch","brief":{},"sources":[{"type":"issue","ref":"https://github.com/transpara-ai/hive/issues/127"}],"authority":{"initial_level":"owner"},"budget":{"max_iterations":4,"max_cost_usd":12.5},"target_repos":["transpara-ai/hive"]}`},
		{name: "brief not object", body: `{"operator_id":"user_127","intake_id":"intake_127","title":"Launch","brief":"do it","sources":[{"type":"issue","ref":"https://github.com/transpara-ai/hive/issues/127"}],"authority":{"initial_level":"required"},"budget":{"max_iterations":4,"max_cost_usd":12.5},"target_repos":["transpara-ai/hive"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := doPost(t, tt.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s: status = %d, want %d", tt.name, resp.StatusCode, http.StatusBadRequest)
			}
			assertNoRunLaunchEvents(t, s)
		})
	}
}

func TestOperatorRunLaunchEndpointRecordsCausalEvents(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorRunLaunchWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/runs", bytes.NewReader(validRunLaunchBody(t)))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post run launch: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	var response operatorRunLaunchResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RunID == "" || response.FirstEventID == "" {
		t.Fatalf("response identifiers must be set, got %+v", response)
	}
	if response.Status != "queued" {
		t.Fatalf("status = %q, want queued", response.Status)
	}

	sourceEvents := requireRunLaunchEvents(t, s, EventTypeSourceIngested, 1)
	briefEvents := requireRunLaunchEvents(t, s, EventTypeBriefDerived, 1)
	runEvents := requireRunLaunchEvents(t, s, EventTypeFactoryRunRequested, 1)
	sourceEvent, briefEvent, runEvent := sourceEvents[0], briefEvents[0], runEvents[0]

	if response.FirstEventID != sourceEvent.ID().Value() {
		t.Fatalf("first_event_id = %q, want source event %q", response.FirstEventID, sourceEvent.ID().Value())
	}
	if got := briefEvent.Causes(); len(got) != 1 || got[0] != sourceEvent.ID() {
		t.Fatalf("brief causes = %+v, want only source event %s", got, sourceEvent.ID())
	}
	if got := runEvent.Causes(); len(got) != 2 || got[0] != sourceEvent.ID() || got[1] != briefEvent.ID() {
		t.Fatalf("run request causes = %+v, want source %s and brief %s", got, sourceEvent.ID(), briefEvent.ID())
	}

	sourceContent, ok := sourceEvent.Content().(SourceIngestedContent)
	if !ok {
		t.Fatalf("source content type = %T", sourceEvent.Content())
	}
	briefContent, ok := briefEvent.Content().(BriefDerivedContent)
	if !ok {
		t.Fatalf("brief content type = %T", briefEvent.Content())
	}
	runContent, ok := runEvent.Content().(FactoryRunRequestedContent)
	if !ok {
		t.Fatalf("run content type = %T", runEvent.Content())
	}
	if sourceContent.RunID != response.RunID || briefContent.RunID != response.RunID || runContent.RunID != response.RunID {
		t.Fatalf("run ids not propagated: source=%q brief=%q run=%q response=%q", sourceContent.RunID, briefContent.RunID, runContent.RunID, response.RunID)
	}
	if briefContent.SourceEventID != sourceEvent.ID() {
		t.Fatalf("brief source_event_id = %s, want %s", briefContent.SourceEventID, sourceEvent.ID())
	}
	if runContent.SourceEventID != sourceEvent.ID() || runContent.BriefEventID != briefEvent.ID() {
		t.Fatalf("run event links source=%s brief=%s, want source=%s brief=%s", runContent.SourceEventID, runContent.BriefEventID, sourceEvent.ID(), briefEvent.ID())
	}
	if runContent.Authority.InitialLevel != event.AuthorityLevelRequired || runContent.Authority.Scope != "operator-launch" {
		t.Fatalf("authority not recorded: %+v", runContent.Authority)
	}
	if runContent.Budget.MaxIterations != 4 || runContent.Budget.MaxCostUSD != 12.5 {
		t.Fatalf("budget not recorded: %+v", runContent.Budget)
	}
	if len(runContent.TargetRepos) != 1 || runContent.TargetRepos[0] != "transpara-ai/hive" {
		t.Fatalf("target repos not recorded: %+v", runContent.TargetRepos)
	}
	if len(runContent.Sources) != 1 || runContent.Sources[0].Ref != "https://github.com/transpara-ai/hive/issues/127" {
		t.Fatalf("sources not recorded: %+v", runContent.Sources)
	}
	if !bytes.Contains(runContent.Brief, []byte(`"goal"`)) {
		t.Fatalf("brief not recorded: %s", string(runContent.Brief))
	}
}

func TestOperatorRunLaunchEndpointRecordsValidatedMeteredModelOverrideWithOptIn(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorRunLaunchWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := validRunLaunchBodyWithOverrides(t, []map[string]any{
		{"role": "guardian", "model": "api-sonnet", "auth_mode": "api-key"},
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post run launch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}

	runEvents := requireRunLaunchEvents(t, s, EventTypeFactoryRunRequested, 1)
	runContent, ok := runEvents[0].Content().(FactoryRunRequestedContent)
	if !ok {
		t.Fatalf("run content type = %T", runEvents[0].Content())
	}
	if len(runContent.ModelOverrides) != 1 {
		t.Fatalf("model overrides = %+v, want one", runContent.ModelOverrides)
	}
	override := runContent.ModelOverrides[0]
	if override.Role != "guardian" || override.Model != "api-sonnet" {
		t.Fatalf("recorded override = %+v, want guardian api-sonnet", override)
	}
	if override.RequestedAuthMode != "api-key" {
		t.Fatalf("requested auth mode = %q, want api-key", override.RequestedAuthMode)
	}
	if override.ResolvedProvider != "anthropic" || override.AuthMode != "api-key" || override.ResolvedModel == "" {
		t.Fatalf("resolved override = %+v, want explicit anthropic api-key", override)
	}
}

func TestOperatorRunLaunchEndpointRejectsMeteredModelOverrideWithoutOptInBeforeWriting(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorRunLaunchWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := validRunLaunchBodyWithOverrides(t, []map[string]any{
		{"role": "guardian", "model": "api-sonnet"},
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post run launch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	assertNoRunLaunchEvents(t, s)
}

func TestOperatorRunLaunchEndpointRejectsProviderModelAuthDesyncBeforeWriting(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorRunLaunchWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := validRunLaunchBodyWithOverrides(t, []map[string]any{
		{"role": "guardian", "provider": "anthropic"},
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post run launch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	assertNoRunLaunchEvents(t, s)
}

func TestOperatorRunLaunchEndpointRejectsUnsafeCanOperateModelOverrideBeforeWriting(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorRunLaunchWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body := validRunLaunchBodyWithOverrides(t, []map[string]any{
		{"role": "implementer", "model": "api-sonnet", "auth_mode": "api-key"},
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post run launch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	assertNoRunLaunchEvents(t, s)
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

// seedPendingActionRequest appends a pending authority request for an arbitrary
// protected action and scope (no decision recorded, so it surfaces in
// PendingApprovals). It mirrors seedPendingDraftPRRequest's anchor+detail
// structure but lets a test choose the action/scope, to exercise the draft-PR
// gate on the decision endpoint.
func seedPendingActionRequest(t *testing.T, s store.Store, factory *event.EventFactory, signer event.Signer, human types.ActorID, conv types.ConversationID, actionName string, scope []string) types.EventID {
	t.Helper()
	head, err := s.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	anchorCauses := []types.EventID{head.Unwrap().ID()}
	anchor, err := factory.Create(event.EventTypeAuthorityRequested, human, event.AuthorityRequestContent{
		Action:        actionName,
		Actor:         human,
		Level:         event.AuthorityLevelRequired,
		Justification: "seed pending request",
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
		RequestID:        requestID,
		RequestingActor:  human,
		RequestingRole:   "guardian",
		ActionName:       actionName,
		Target:           "seed-target",
		Environment:      "production",
		RequestedOutcome: "seed",
		Justification:    "seed pending request",
		RiskSummary:      "seed",
		Scope:            scope,
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

func validRunLaunchBody(t *testing.T) []byte {
	t.Helper()
	body := validRunLaunchMap()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal valid launch body: %v", err)
	}
	return encoded
}

func validRunLaunchBodyWithOverrides(t *testing.T, overrides []map[string]any) []byte {
	t.Helper()
	body := validRunLaunchMap()
	body["model_overrides"] = overrides
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal launch body with overrides: %v", err)
	}
	return encoded
}

func validRunLaunchMap() map[string]any {
	return map[string]any{
		"operator_id": "user_127",
		"intake_id":   "intake_127",
		"title":       "Launch Hive issue 127",
		"brief": map[string]any{
			"goal":  "complete Hive run launch API",
			"issue": "https://github.com/transpara-ai/hive/issues/127",
		},
		"sources": []map[string]string{
			{
				"id":    "issue_127",
				"type":  "issue",
				"ref":   "https://github.com/transpara-ai/hive/issues/127",
				"title": "Hive run launch API",
			},
		},
		"authority": map[string]string{
			"initial_level": "required",
			"scope":         "operator-launch",
			"policy_ref":    "dark-factory/operator-ui-contract-v0.1.2",
			"rationale":     "operator initiated queued launch only",
		},
		"budget": map[string]any{
			"max_iterations": 4,
			"max_cost_usd":   12.5,
		},
		"target_repos": []string{"transpara-ai/hive"},
	}
}

func assertNoRunLaunchEvents(t *testing.T, s store.Store) {
	t.Helper()
	requireRunLaunchEvents(t, s, EventTypeSourceIngested, 0)
	requireRunLaunchEvents(t, s, EventTypeBriefDerived, 0)
	requireRunLaunchEvents(t, s, EventTypeFactoryRunRequested, 0)
}

func requireRunLaunchEvents(t *testing.T, s store.Store, eventType types.EventType, want int) []event.Event {
	t.Helper()
	page, err := s.ByType(eventType, 50, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("query %s: %v", eventType, err)
	}
	items := page.Items()
	if len(items) != want {
		t.Fatalf("%s event count = %d, want %d: %+v", eventType, len(items), want, items)
	}
	return items
}

// TestOperatorDecisionEndpointRejectsNonDraftPR verifies the decision endpoint
// authorizes ONLY the draft-PR create action (P1-a). A pending request for any
// other protected action, or a pull_request.create request whose scope is not a
// valid draft-PR scope, must be refused with 403 and write no decision — so the
// human-decision surface can never approve, e.g., an agent-spawn or deploy.
func TestOperatorDecisionEndpointRejectsNonDraftPR(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorDecisionWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	postApproval := func(t *testing.T, requestID string) *http.Response {
		t.Helper()
		body, _ := json.Marshal(map[string]string{
			"request_id": requestID, "decision": "approved", "approver": human.Value(), "reason": "x",
		})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/operator-decision", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post decision: %v", err)
		}
		resp.Body.Close()
		return resp
	}

	assertNoDecision := func(t *testing.T) {
		t.Helper()
		if proj := BuildOperatorProjection(s, 50); len(proj.AuthorityDecisions) != 0 {
			t.Fatalf("refused decision must not write a decision, got %+v", proj.AuthorityDecisions)
		}
	}

	t.Run("non-draft-PR action", func(t *testing.T) {
		requestID := seedPendingActionRequest(t, s, factory, signer, human, conv,
			"agent.spawn.persistent", []string{"agent.spawn.persistent", "builder"})
		resp := postApproval(t, requestID.Value())
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("non-draft-PR action: status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}
		assertNoDecision(t)
	})

	t.Run("draft-PR action with malformed scope", func(t *testing.T) {
		requestID := seedPendingActionRequest(t, s, factory, signer, human, conv,
			string(safety.ActionRepoPullRequestCreate), []string{string(safety.ActionRepoPullRequestCreate), "transpara-ai/docs"})
		resp := postApproval(t, requestID.Value())
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("malformed draft-PR scope: status = %d, want %d", resp.StatusCode, http.StatusForbidden)
		}
		assertNoDecision(t)
	})
}

// TestOperatorDecisionEndpointRejectsAlreadyDecided verifies a request is
// decided exactly once (P2-a). Once a decision is recorded, a second POST for
// the same request must be refused with 409 — a denial cannot be overwritten by
// a later approval (no latest-wins). The original denial must survive intact.
func TestOperatorDecisionEndpointRejectsAlreadyDecided(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorDecisionWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	requestID := seedPendingDraftPRRequest(t, s, factory, signer, human, conv)

	post := func(t *testing.T, decision string) *http.Response {
		t.Helper()
		body, _ := json.Marshal(map[string]string{
			"request_id": requestID.Value(), "decision": decision, "approver": human.Value(), "reason": decision,
		})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/operator-decision", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post decision: %v", err)
		}
		resp.Body.Close()
		return resp
	}

	// First decision: deny. Recorded normally.
	if resp := post(t, "denied"); resp.StatusCode != http.StatusOK {
		t.Fatalf("first (deny) decision: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Second decision for the same request must be refused — a later approval
	// cannot overwrite the recorded denial.
	if resp := post(t, "approved"); resp.StatusCode != http.StatusConflict {
		t.Fatalf("second (approve) decision: status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}

	// Exactly one decision survives, and it is the original denial.
	proj := BuildOperatorProjection(s, 50)
	if len(proj.AuthorityDecisions) != 1 {
		t.Fatalf("expected exactly one decision, got %d: %+v", len(proj.AuthorityDecisions), proj.AuthorityDecisions)
	}
	if proj.AuthorityDecisions[0].Outcome != "denied" {
		t.Fatalf("decision outcome = %q, want denied (denial must not be overwritten)", proj.AuthorityDecisions[0].Outcome)
	}
}
