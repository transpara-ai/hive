package hive

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/modelconfig"
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

func TestOperatorProjectionServerReturnsCivilizationAssemblyProjectionJSON(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	appendEvent(EventTypeAgentIdentityRegistered, AgentIdentityRegisteredContent{
		ActorID:          actorID,
		DisplayName:      "Guardian",
		Role:             "guardian",
		PublicKey:        types.MustPublicKey(make([]byte, 32)),
		KeyProvenance:    "generated",
		Environment:      "review",
		IdentityMode:     "persistent",
		LifecycleStatus:  "active",
		AuthorityScope:   "hive:governance",
		RegistrationPath: "generated",
	})

	handler := NewOperatorProjectionServer(s, "secret", 50)
	req := httptest.NewRequest(http.MethodGet, "/api/hive/civilization/assembly-projection", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status without token = %d, want %d", resp.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/hive/civilization/assembly-projection", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status with token = %d, want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var projection CivilizationAssemblyProjection
	if err := json.Unmarshal(resp.Body.Bytes(), &projection); err != nil {
		t.Fatalf("decode projection: %v", err)
	}
	if projection.ProjectionSubject != civilizationAssemblyProjectionSubject {
		t.Fatalf("subject = %q, want %q", projection.ProjectionSubject, civilizationAssemblyProjectionSubject)
	}
	if len(projection.ActorRoster) != 1 || projection.ActorRoster[0].ActorID != actorID.Value() {
		t.Fatalf("actor roster = %+v, want one guardian actor", projection.ActorRoster)
	}
	if projection.SiteConsumerStatus.Status != civilizationAssemblyFieldAvailable {
		t.Fatalf("site consumer status = %+v, want available", projection.SiteConsumerStatus)
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

func TestOperatorModelRolePolicyEndpointRecordsHiveOwnedPolicy(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorModelRolePolicyWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	maxCost := 2.75
	body, _ := json.Marshal(map[string]any{
		"operator_id":           "user_139",
		"reason":                "operator selected metered guardian model",
		"role":                  "guardian",
		"model":                 "api-sonnet",
		"auth_mode":             "api-key",
		"max_cost_per_call_usd": maxCost,
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/model-selection/role-policy", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post model role policy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.StatusCode, http.StatusOK, respBody(t, resp))
	}

	var response operatorModelRolePolicyResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Role != "guardian" || response.AuthMode != string(modelconfig.AuthAPIKey) || response.ResolvedProvider != "anthropic" || response.EventID == "" {
		t.Fatalf("response = %+v, want recorded guardian anthropic api-key policy", response)
	}

	events := requireModelRolePolicyEvents(t, s, 1)
	content, ok := events[0].Content().(ModelRolePolicyUpdatedContent)
	if !ok {
		t.Fatalf("policy content type = %T", events[0].Content())
	}
	if content.Role != "guardian" || content.Model != "api-sonnet" || content.RequestedAuthMode != "api-key" {
		t.Fatalf("recorded policy = %+v, want guardian api-sonnet api-key request", content)
	}
	if content.MaxCostPerCallUSD == nil || *content.MaxCostPerCallUSD != maxCost {
		t.Fatalf("recorded max cost = %v, want %v", content.MaxCostPerCallUSD, maxCost)
	}
	if content.OperatorID != "user_139" || content.Reason == "" {
		t.Fatalf("operator metadata = %+v, want recorded operator and reason", content)
	}

	projection := BuildOperatorProjection(s, 50)
	guardian := requireModelAssignment(t, projection.ModelSelection, "guardian")
	if guardian.Source != "hive-model-policy-event" || guardian.PolicyEventID != events[0].ID().Value() {
		t.Fatalf("guardian assignment source/event = %q/%q, want hive policy event %s", guardian.Source, guardian.PolicyEventID, events[0].ID())
	}
	if guardian.Model != "api-claude-sonnet-4-6" || guardian.Provider != "anthropic" || guardian.AuthMode != "api-key" {
		t.Fatalf("guardian assignment = %+v, want projected metered model", guardian)
	}
	if guardian.MaxCostPerCallUSD == nil || *guardian.MaxCostPerCallUSD != maxCost {
		t.Fatalf("guardian projected max cost = %v, want %v", guardian.MaxCostPerCallUSD, maxCost)
	}
}

func TestOperatorModelRolePolicyEndpointRejectsUnsafeCanOperatePolicyBeforeWriting(t *testing.T) {
	s, factory, signer, human, conv := newDecisionTestStore(t)
	srv := NewOperatorProjectionServer(s, "secret", 50, WithOperatorModelRolePolicyWriter(factory, signer, human, conv))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	body, _ := json.Marshal(map[string]any{
		"operator_id": "user_139",
		"role":        "implementer",
		"model":       "api-sonnet",
		"auth_mode":   "api-key",
	})
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/hive/model-selection/role-policy", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post model role policy: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	requireModelRolePolicyEvents(t, s, 0)
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

func requireModelRolePolicyEvents(t *testing.T, s store.Store, want int) []event.Event {
	t.Helper()
	page, err := s.ByType(EventTypeModelRolePolicyUpdated, 50, types.None[types.Cursor]())
	if err != nil {
		t.Fatalf("query %s: %v", EventTypeModelRolePolicyUpdated, err)
	}
	items := page.Items()
	if len(items) != want {
		t.Fatalf("%s event count = %d, want %d: %+v", EventTypeModelRolePolicyUpdated, len(items), want, items)
	}
	return items
}

func respBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
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

// TestOperatorProjectionServerServesRecentIssueScanRunsEndToEnd covers D1-D3 +
// the handler leg of the D2 TDD plan: a seeded store containing an issue-scan
// parked event must be visible in the assembly-projection endpoint's
// recent_issue_scan_runs section with the expected run/state, over a real
// httptest server using the same handler-mounting pattern as every other
// operator API test in this file.
func TestOperatorProjectionServerServesRecentIssueScanRunsEndToEnd(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
		RunID:             "run_ops_api_e2e_parked_001",
		Repository:        "transpara-ai/hive",
		IssueNumber:       777,
		LifecycleVersion:  IssueScanParkLifecycleLevel1Canary,
		EvidenceClass:     IssueScanParkEvidenceClassLevel1Canary,
		AuthorityBoundary: IssueScanParkAuthorityBoundaryLevel1Canary,
		BlockerType:       IssueScanParkBlockerHumanScope,
		Detail:            "transpara-ai/hive#777 is labeled cc:needs-human-scope",
		RequiredAction:    "human must clarify scope before Hive may continue",
		SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/777"},
		ParkedBy:          actorID,
		TargetIssueState:  "open",
		TargetIssueLabels: []string{"cc:intake", "cc:needs-human-scope"},
	})

	handler := NewOperatorProjectionServer(s, "secret", 50)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/hive/civilization/assembly-projection", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get assembly projection: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var projection CivilizationAssemblyProjection
	if err := json.NewDecoder(resp.Body).Decode(&projection); err != nil {
		t.Fatalf("decode projection: %v", err)
	}
	rail := projection.RecentIssueScanRuns
	if rail.Status != civilizationAssemblyFieldAvailable {
		t.Fatalf("recent_issue_scan_runs.status = %q, want available", rail.Status)
	}
	if len(rail.Runs) != 1 {
		t.Fatalf("recent_issue_scan_runs.runs = %+v, want one", rail.Runs)
	}
	run := rail.Runs[0]
	if run.RunID != "run_ops_api_e2e_parked_001" {
		t.Fatalf("run_id = %q, want run_ops_api_e2e_parked_001", run.RunID)
	}
	if run.State != "human_action" {
		t.Fatalf("state = %q, want human_action", run.State)
	}
	if run.Repo != "transpara-ai/hive" || run.IssueNumber != 777 {
		t.Fatalf("run issue ref = %+v, want transpara-ai/hive#777", run)
	}
}

// gatedCountingStore wraps a store.Store, counting every ByType call for a
// single tracked event type and, on the FIRST such call only, blocking on a
// gate channel until it is closed. Paired with gateAfterNHandlerEntries
// (which holds the HTTP handler itself, at the server, until N requests have
// arrived), this makes the singleflight-collapse assertion deterministic
// rather than racy: no tracked store read can complete until every one of
// the N requests has actually been received and dispatched by the server —
// not merely "sent" by the client — and had the opportunity to join the
// shared flight.
type gatedCountingStore struct {
	store.Store
	trackedType types.EventType
	gate        chan struct{}

	count int32
	gated int32
}

func (s *gatedCountingStore) ByType(eventType types.EventType, limit int, after types.Option[types.Cursor]) (types.Page[event.Event], error) {
	if eventType == s.trackedType {
		atomic.AddInt32(&s.count, 1)
		if atomic.AddInt32(&s.gated, 1) == 1 && s.gate != nil {
			// Only the first call blocks on the gate; this proves at most one
			// underlying computation is ever actually running the tracked
			// query at a time when singleflight is collapsing concurrent
			// requests. Later calls (a fresh flight after the first
			// completes) do not re-block.
			<-s.gate
		}
	}
	return s.Store.ByType(eventType, limit, after)
}

func (s *gatedCountingStore) callCount() int {
	return int(atomic.LoadInt32(&s.count))
}

// gateAfterNHandlerEntries wraps an http.Handler so that the Nth request to
// actually reach the server (ServeHTTP invoked — i.e. accepted, routed, and
// dispatched, not merely sent by a client) closes gate exactly once. This is
// the server-side arrival signal the singleflight-collapse test needs: it
// guarantees the underlying store's gated tracked read (blocked on the same
// channel) cannot complete until every concurrent request has been
// dispatched to the mux and had the opportunity to call singleflight.Do —
// removing any dependency on client-side goroutine/network scheduling.
// releaseGate is idempotent so BOTH the normal path (Nth arrival) and the
// test's timeout path can release the gate: httptest.Server.Close waits for
// active handlers, so a timed-out attempt must unblock the gated store read
// BEFORE closing the server or the failure would hang instead of reporting.
func gateAfterNHandlerEntries(next http.Handler, n int, releaseGate func()) http.Handler {
	var arrived int32
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if int(atomic.AddInt32(&arrived, 1)) >= n {
			releaseGate()
		}
		next.ServeHTTP(w, r)
	})
}

// TestOperatorProjectionServerSingleflightCollapsesConcurrentRequests covers
// D2's singleflight requirement: N=4 concurrent GETs to the civilization
// assembly-projection endpoint must collapse to fewer than N fresh
// computations of the tracked query (hive.issuescan.run.parked, fetched
// exactly once per BuildCivilizationAssemblyProjection call), while every
// response is still 200 with valid JSON carrying the same run data.
// Determinism of ARRIVAL comes from a SERVER-SIDE gate: the underlying
// store's tracked read blocks until the Nth request has actually been
// dispatched to the handler (gateAfterNHandlerEntries), so the one live
// computation cannot complete before every concurrent request has had the
// chance to reach the mux. This gate maximizes collapse probability but does
// NOT make collapse deterministic in the strict sense: the gate fires at the
// outer mux wrapper, which runs BEFORE auth and BEFORE singleflight.Do. A
// "late" sibling can in principle clear auth and reach singleflight.Do after
// the leader's flight has already completed and its key been evicted,
// causing it to start a fresh second flight — there is no hook to observe
// "inside Do" without instrumenting production code, which is out of scope
// (the production singleflight code is verified correct independently).
//
// Two consequences follow:
//
//  1. Response bodies are NOT asserted byte-identical. Two requests landing
//     in different flights get different BuildCivilizationAssemblyProjection
//     calls and therefore different GeneratedAt timestamps — that is
//     flight-splitting, not a singleflight correctness violation. What must
//     hold regardless of flight-splitting is: every response is 200, is
//     valid JSON, and carries the identical run STATUS/content (same
//     RunID, same rail status) — i.e., every caller observes a consistent
//     view of the store, whether they shared a flight or not.
//  2. The trackedComputations < N check is run inside a bounded retry (up to
//     3 attempts), passing as soon as one attempt observes collapse. This is
//     statistically airtight, not merely hopeful: if singleflight were
//     absent (or broken), computations == N deterministically on EVERY
//     attempt (each request always starts its own flight), so a regression
//     still fails reliably — a broken build cannot get lucky across 3
//     attempts. If singleflight is present and correct, the measured
//     late-sibling split rate is 0.2% under a plain test run and 1.5-4%
//     under -race; the probability of every one of 3 independent attempts
//     splitting ALL 4 followers into separate flights is bounded by
//     (0.04)^3 = 6.4e-5 in the worst (race) case per-follower, and the
//     actual all-followers-split event is rarer still — negligible (<1e-6)
//     for the purpose of this test. Only if every attempt shows
//     computations == N do we fail, which is the correct behavior for an
//     actual regression.
func TestOperatorProjectionServerSingleflightCollapsesConcurrentRequests(t *testing.T) {
	baseStore, actorID, appendEvent := newOperatorProjectionStore(t)
	appendEvent(EventTypeIssueScanRunParked, IssueScanRunParkedContent{
		RunID:             "run_ops_api_singleflight_001",
		Repository:        "transpara-ai/hive",
		IssueNumber:       778,
		LifecycleVersion:  IssueScanParkLifecycleLevel1Canary,
		EvidenceClass:     IssueScanParkEvidenceClassLevel1Canary,
		AuthorityBoundary: IssueScanParkAuthorityBoundaryLevel1Canary,
		BlockerType:       IssueScanParkBlockerHumanScope,
		Detail:            "transpara-ai/hive#778 is labeled cc:needs-human-scope",
		RequiredAction:    "human must clarify scope before Hive may continue",
		SourceRefs:        []string{"https://github.com/transpara-ai/hive/issues/778"},
		ParkedBy:          actorID,
		TargetIssueState:  "open",
		TargetIssueLabels: []string{"cc:intake", "cc:needs-human-scope"},
	})

	// Compute the per-build tracked-call count sequentially first, exactly as
	// the task brief requires, so the concurrent assertion below is relative
	// to ground truth rather than an assumed constant.
	sequential := &gatedCountingStore{Store: baseStore, trackedType: EventTypeIssueScanRunParked}
	_ = BuildCivilizationAssemblyProjection(sequential, 50)
	perBuildCount := sequential.callCount()
	if perBuildCount < 1 {
		t.Fatalf("perBuildCount = %d, want >= 1 tracked call per build", perBuildCount)
	}

	const concurrency = 4
	const maxAttempts = 3
	const waitTimeout = 30 * time.Second

	var lastTotal int
	collapsed := false

	for attempt := 1; attempt <= maxAttempts && !collapsed; attempt++ {
		gate := make(chan struct{})
		gated := &gatedCountingStore{
			Store:       baseStore,
			trackedType: EventTypeIssueScanRunParked,
			gate:        gate,
		}
		// gateAfterNHandlerEntries closes gate only once the SERVER has actually
		// dispatched the Nth request to the mux — the store's tracked read (also
		// blocked on gate) therefore cannot complete until every one of the 4
		// concurrent requests has been received and had the chance to reach the
		// mux and attempt to join the shared singleflight.Do call, regardless of
		// client/network scheduling. See the function doc for why this still
		// does not guarantee every request reaches Do before the leader's
		// flight completes.
		var gateOnce sync.Once
		releaseGate := func() { gateOnce.Do(func() { close(gate) }) }
		handler := gateAfterNHandlerEntries(NewOperatorProjectionServer(gated, "secret", 50), concurrency, releaseGate)
		ts := httptest.NewServer(handler)

		var wg sync.WaitGroup
		responses := make([]*http.Response, concurrency)
		bodies := make([][]byte, concurrency)
		errs := make([]error, concurrency)

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/hive/civilization/assembly-projection", nil)
				if err != nil {
					errs[i] = err
					return
				}
				req.Header.Set("Authorization", "Bearer secret")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					errs[i] = err
					return
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					errs[i] = err
					return
				}
				responses[i] = resp
				bodies[i] = body
			}(i)
		}

		// wg.Wait() has no built-in timeout; if the gate or a request hangs,
		// an unbounded wait would hang the whole test run. Bound it so a
		// stuck attempt fails with a diagnosable message instead of hanging
		// CI indefinitely.
		waitDone := make(chan struct{})
		go func() {
			wg.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(waitTimeout):
			// Release the gate FIRST: a handler may still be blocked on the
			// gated store read, and httptest.Server.Close waits for active
			// handlers — closing without releasing would hang the test run
			// instead of failing with this message.
			releaseGate()
			ts.Close()
			t.Fatalf("attempt %d: concurrent requests did not complete within %s (possible deadlock in gate/handler)", attempt, waitTimeout)
		}

		ts.Close()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("attempt %d: request %d: %v", attempt, i, err)
			}
		}

		for i := 0; i < concurrency; i++ {
			if responses[i].StatusCode != http.StatusOK {
				t.Fatalf("attempt %d: response %d status = %d, want %d body=%s", attempt, i, responses[i].StatusCode, http.StatusOK, bodies[i])
			}
			var decoded CivilizationAssemblyProjection
			if err := json.Unmarshal(bodies[i], &decoded); err != nil {
				t.Fatalf("attempt %d: decode response %d: %v body=%s", attempt, i, err, bodies[i])
			}
			// Schema/content validity, NOT byte-identical bodies: flight-split
			// responses legitimately differ in GeneratedAt (see function doc).
			// Every caller must still observe the same run data and status.
			if decoded.RecentIssueScanRuns.Status != civilizationAssemblyFieldAvailable {
				t.Fatalf("attempt %d: response %d recent_issue_scan_runs.status = %q, want %q", attempt, i, decoded.RecentIssueScanRuns.Status, civilizationAssemblyFieldAvailable)
			}
			if len(decoded.RecentIssueScanRuns.Runs) != 1 || decoded.RecentIssueScanRuns.Runs[0].RunID != "run_ops_api_singleflight_001" {
				t.Fatalf("attempt %d: response %d recent_issue_scan_runs = %+v, want run_ops_api_singleflight_001", attempt, i, decoded.RecentIssueScanRuns)
			}
		}

		total := gated.callCount()
		lastTotal = total
		if total < perBuildCount {
			t.Fatalf("attempt %d: tracked ByType calls = %d, want >= %d (at least one real computation must have run)", attempt, total, perBuildCount)
		}
		if total < concurrency*perBuildCount {
			// Collapse observed: strictly fewer fresh computations than one
			// per request, i.e. at least two requests shared a flight.
			collapsed = true
			t.Logf("attempt %d: tracked ByType calls = %d < %d (perBuildCount=%d * concurrency=%d): singleflight collapsed concurrent requests", attempt, total, concurrency*perBuildCount, perBuildCount, concurrency)
		} else {
			t.Logf("attempt %d: tracked ByType calls = %d == %d (perBuildCount=%d * concurrency=%d): no collapse observed this attempt, retrying", attempt, total, concurrency*perBuildCount, perBuildCount, concurrency)
		}
	}

	if !collapsed {
		t.Fatalf("tracked ByType calls stayed at %d == %d across all %d attempts (perBuildCount=%d * concurrency=%d): singleflight did not collapse concurrent requests in any attempt", lastTotal, concurrency*perBuildCount, maxAttempts, perBuildCount, concurrency)
	}
}
