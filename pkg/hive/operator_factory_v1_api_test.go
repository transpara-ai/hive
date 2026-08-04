package hive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
	workpkg "github.com/transpara-ai/work"
)

func newFactoryV1OperatorTestServer(t *testing.T) (http.Handler, *FactoryV1EventGraphStore, *FactoryV1WorkStore) {
	t.Helper()
	eventStore, factory, signer, human, conv := newDecisionTestStore(t)
	workpkg.RegisterWithRegistry(factory.Registry)
	graph, err := NewFactoryV1EventGraphStore(eventStore, factory, signer, human, conv)
	if err != nil {
		t.Fatalf("new v1 EventGraph adapter: %v", err)
	}
	workStore, err := NewFactoryV1WorkStore(eventStore, factory, signer, human, conv)
	if err != nil {
		t.Fatalf("new v1 Work adapter: %v", err)
	}
	intake, err := factoryv1.NewIntake(graph, workStore, factoryv1.WallClock{})
	if err != nil {
		t.Fatalf("new v1 intake: %v", err)
	}
	projector, err := factoryv1.NewProjector(graph, workStore, factoryv1.WallClock{}, factoryv1.ServiceProjection{ServiceID: "hive-factory-v1", InstanceID: "test-instance", StartedAt: time.Now().UTC(), Healthy: true})
	if err != nil {
		t.Fatalf("new v1 projector: %v", err)
	}
	service, err := NewFactoryV1OperatorService(intake, projector, graph, factoryv1.WallClock{}, human.Value(), "credential-test")
	if err != nil {
		t.Fatalf("new v1 operator service: %v", err)
	}
	return NewOperatorProjectionServer(eventStore, "secret", 100, WithOperatorFactoryV1(service)), graph, workStore
}

func doFactoryV1Request(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestFactoryV1IdeaRefinementSubmission(t *testing.T) {
	handler, _, workStore := newFactoryV1OperatorTestServer(t)
	created := doFactoryV1Request(t, handler, http.MethodPost, "/api/hive/factory/v1/ideas", map[string]any{
		"title": "Document cursor recovery", "idea": "Add one bounded cursor recovery example.", "target_repository": "transpara-ai/eventgraph",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var idea factoryv1.IdeaCandidate
	if err := json.Unmarshal(created.Body.Bytes(), &idea); err != nil {
		t.Fatalf("decode idea: %v", err)
	}
	for index, instruction := range []string{"Keep the change documentation-only.", "Verify the example in repository tests."} {
		response := doFactoryV1Request(t, handler, http.MethodPost, fmt.Sprintf("/api/hive/factory/v1/ideas/%s/refine", idea.IdeaID), map[string]string{"instruction": instruction})
		if response.Code != http.StatusOK {
			t.Fatalf("refinement %d status=%d body=%s", index+1, response.Code, response.Body.String())
		}
		if err := json.Unmarshal(response.Body.Bytes(), &idea); err != nil {
			t.Fatalf("decode refinement %d: %v", index+1, err)
		}
	}
	if len(idea.Revisions) != 3 || idea.Revisions[2].CandidateSHA256 == "" {
		t.Fatalf("refined idea lacks exact candidate binding: %+v", idea)
	}
	stale := doFactoryV1Request(t, handler, http.MethodPost, fmt.Sprintf("/api/hive/factory/v1/ideas/%s/submit", idea.IdeaID), map[string]any{
		"approved": true, "revision": 2, "candidate_sha256": idea.Revisions[1].CandidateSHA256,
	})
	if stale.Code != http.StatusBadRequest {
		t.Fatalf("stale approval status=%d, want 400; body=%s", stale.Code, stale.Body.String())
	}
	submitted := doFactoryV1Request(t, handler, http.MethodPost, fmt.Sprintf("/api/hive/factory/v1/ideas/%s/submit", idea.IdeaID), map[string]any{
		"approved": true, "revision": idea.CurrentRevision, "candidate_sha256": idea.Revisions[2].CandidateSHA256,
	})
	if submitted.Code != http.StatusCreated {
		t.Fatalf("submit status=%d body=%s", submitted.Code, submitted.Body.String())
	}
	projectionResponse := doFactoryV1Request(t, handler, http.MethodGet, "/api/hive/factory/v1/projection", nil)
	if projectionResponse.Code != http.StatusOK {
		t.Fatalf("projection status=%d body=%s", projectionResponse.Code, projectionResponse.Body.String())
	}
	var projection factoryv1.Projection
	if err := json.Unmarshal(projectionResponse.Body.Bytes(), &projection); err != nil {
		t.Fatalf("decode projection: %v", err)
	}
	if len(projection.Ideas) != 1 || projection.Ideas[0].CurrentRevision != 3 || projection.Ideas[0].Status != "submitted" {
		t.Fatalf("idea projection=%+v", projection.Ideas)
	}
	if len(projection.Orders) != 1 || projection.Orders[0].Channel != factoryv1.ChannelHumanIdea || projection.Orders[0].HumanApprovalBasis != factoryv1.ApprovalFreshScoped {
		t.Fatalf("order projection=%+v", projection.Orders)
	}
	links, err := workStore.ListFactoryOrders(context.Background())
	if err != nil || len(links) != 1 || links[0].AcceptedEventID == "" || links[0].ArtifactID == "" {
		t.Fatalf("Work links=%+v err=%v", links, err)
	}
}

func TestFactoryV1CompletedOrderAdmission(t *testing.T) {
	handler, _, _ := newFactoryV1OperatorTestServer(t)
	order := validFactoryV1APIOrder(factoryv1.ChannelCompletedOrder, "FO-DIRECT-001", "transpara-ai/work")
	response := doFactoryV1Request(t, handler, http.MethodPost, "/api/hive/factory/v1/orders", map[string]any{"factory_order": order})
	if response.Code != http.StatusCreated {
		t.Fatalf("direct admission status=%d body=%s", response.Code, response.Body.String())
	}
	var receipt factoryv1.AcceptanceReceipt
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	if receipt.Channel != factoryv1.ChannelCompletedOrder || receipt.AcceptedEventID == "" || receipt.Work.TaskID == "" || receipt.Work.ArtifactID == "" {
		t.Fatalf("receipt=%+v", receipt)
	}
}

func TestFactoryV1InterventionPOST(t *testing.T) {
	handler, graph, _ := newFactoryV1OperatorTestServer(t)
	requested, err := factoryv1.AppendTyped(context.Background(), graph, factoryv1.EventInterventionRequested, "FO-INTERVENTION-001", "intervention-requested:test", nil, factoryv1.InterventionRequestedPayload{
		InterventionID: "int-test-001", OrderID: "FO-INTERVENTION-001", Kind: "bounded_demo",
		Prompt: "Confirm the bounded recovery action.", Stage: factoryv1.StageHumanDesignReview, RequestedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("append intervention: %v", err)
	}
	response := doFactoryV1Request(t, handler, http.MethodPost, "/api/hive/factory/v1/interventions/int-test-001/resolve", map[string]string{
		"resolution": "resume within the recorded scope", "actor_id": "actor_00000000000000000000000000000077", "operator_principal_id": "site-human-operator-1",
	})
	if response.Code != http.StatusOK {
		t.Fatalf("resolve status=%d body=%s", response.Code, response.Body.String())
	}
	events, err := graph.List(context.Background())
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var resolved factoryv1.Event
	for _, candidate := range events {
		if candidate.Type == factoryv1.EventInterventionResolved {
			resolved = candidate
		}
	}
	if resolved.ID == "" || len(resolved.Causes) != 1 || resolved.Causes[0] != requested.ID {
		t.Fatalf("resolved event=%+v request=%s", resolved, requested.ID)
	}
	var payload factoryv1.InterventionResolvedPayload
	if err := json.Unmarshal(resolved.Payload, &payload); err != nil {
		t.Fatalf("decode resolved payload: %v", err)
	}
	if payload.ActorID != "actor_00000000000000000000000000000077" || payload.OperatorPrincipalID != "site-human-operator-1" {
		t.Fatalf("resolved attribution=%+v", payload)
	}
}

func TestFactoryV1OperatorRoutesRequireBearer(t *testing.T) {
	handler, _, _ := newFactoryV1OperatorTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/hive/factory/v1/projection", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want=%d", response.Code, http.StatusUnauthorized)
	}
}

func validFactoryV1APIOrder(channel factoryv1.Channel, id, repository string) factoryv1.FactoryOrder {
	return factoryv1.FactoryOrder{
		DocID: id, Version: "1.0.0", Status: "approved", Title: "Bounded demo order", Channel: channel, TargetRepository: repository,
		SourceReferences:   []factoryv1.SourceReference{{Kind: "test", Identity: "test:" + id, SHA256: factoryv1.HashText(id)}},
		Requirements:       []factoryv1.Requirement{{ID: "R1", Statement: "Make one bounded repository change.", Rationale: "Demonstrate direct FactoryOrder intake."}},
		AcceptanceCriteria: []factoryv1.AcceptanceCriterion{{ID: "AC1", Statement: "The change is verified.", VerificationMethod: "Run repository tests.", RiskClass: "medium"}},
		TestPlan:           []string{"Run repository verification."}, Constraints: []string{"No merge"}, NonGoals: []string{"Unrelated refactors"}, ExpectedOutputs: []string{"Ready PR"},
		Authority: factoryv1.AuthorityScope{ActorID: "actor_00000000000000000000000000000077", AllowedActions: []string{"repo.branch.create", "repo.pull_request.create"}, TargetRepositories: []string{repository}, NonProductionOnly: true},
		Budget:    factoryv1.BudgetLimit{MaxAttempts: 20, MaxTokens: 100000, MaxCostMicros: 1000000},
	}
}
