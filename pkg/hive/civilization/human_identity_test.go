package civilization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func namedAPI(t *testing.T, h http.Handler, method, path, actor, role string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer test-secret")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Civilization-Actor", actor)
	r.Header.Set("X-Civilization-Role", role)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

type failResumeStore struct {
	Store
	fail bool
}

func (s *failResumeStore) Append(ctx context.Context, n NewEvent) (Event, error) {
	if s.fail && strings.HasPrefix(n.IdempotencyKey, "state:resumed:") {
		s.fail = false
		return Event{}, errors.New("interrupted after durable answer")
	}
	return s.Store.Append(ctx, n)
}

func TestNamedInterventionRecoversAcrossInterruptedResumeAndRetry(t *testing.T) {
	ctx := context.WithValue(context.Background(), humanIdentityKey{}, "alice")
	e, p, effects, item := preparedReviewFixture(t)
	blocked, err := e.block(ctx, item.WorkID, "repair required", "Supply a repair.")
	if err != nil {
		t.Fatal(err)
	}
	id := blocked.Interventions[0].ID
	e.store = &failResumeStore{Store: e.store, fail: true}
	if _, err := e.ResolveIntervention(ctx, item.WorkID, id, "Repair applied"); err == nil {
		t.Fatal("expected interruption")
	}
	restarted, err := NewEngine(EngineConfig{Store: e.store, Provider: p, Effects: effects})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.Advance(context.Background(), item.WorkID)
	if err != nil || recovered.State != item.State || recovered.Blocker != "" {
		t.Fatalf("stranded work: %+v %v", recovered, err)
	}
	before, _ := e.store.List(ctx)
	bob := context.WithValue(context.Background(), humanIdentityKey{}, "bob")
	retried, err := restarted.ResolveIntervention(bob, item.WorkID, id, "Repair applied")
	if err != nil || retried.Interventions[0].ResolvedBy != "alice" || retried.Interventions[0].ResolvedAt.IsZero() {
		t.Fatalf("retry changed answer: %+v %v", retried, err)
	}
	if _, err := restarted.ResolveIntervention(bob, item.WorkID, id, "Different answer"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("conflicting answer accepted", err)
	}
	after, _ := e.store.List(ctx)
	if len(before) != len(after) {
		t.Fatal("retry duplicated events")
	}
}

func TestNamedHTTPPermissionsAndImmutableAttribution(t *testing.T) {
	e, _, _ := newTestEngine(t, "Routine", false)
	h, err := NewHTTPHandler(HTTPConfig{Engine: e, APIKey: "test-secret", RequireHumanIdentity: true})
	if err != nil {
		t.Fatal(err)
	}
	intake := map[string]any{"source_kind": "human", "source_identity": "human:alice:one", "repository": "transpara-ai/hive", "text": "Improve documentation"}
	for _, who := range []struct{ actor, role string }{{"", ""}, {"anonymous", "reviewer"}, {"alice", "admin"}, {"alice", "viewer"}, {"bob", "operator"}} {
		w := namedAPI(t, h, "POST", "/api/civilization/v1/intake", who.actor, who.role, intake)
		if w.Code != 403 {
			t.Fatalf("unauthorized intake: %d %s", w.Code, w.Body.String())
		}
	}
	w := namedAPI(t, h, "POST", "/api/civilization/v1/intake", "alice", "operator", intake)
	if w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	var item WorkProjection
	json.Unmarshal(w.Body.Bytes(), &item)
	if item.RequestedBy != "alice" {
		t.Fatal("intake actor lost")
	}
	item, err = e.Advance(context.Background(), item.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/civilization/v1/work/" + item.WorkID + "/confirm"
	confirm := map[string]string{"brief_id": item.Bound.IdempotencyKey}
	if w := namedAPI(t, h, "POST", path, "alice", "operator", confirm); w.Code != 403 {
		t.Fatal("operator confirmed")
	}
	if w := namedAPI(t, h, "POST", path, "bob", "reviewer", confirm); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	// Another reviewer's exact retry cannot replace the first confirmation.
	if w := namedAPI(t, h, "POST", path, "alice", "reviewer", confirm); w.Code != 202 {
		t.Fatal(w.Body.String())
	}
	restarted, _ := NewEngine(EngineConfig{Store: e.store, Provider: e.provider, Effects: e.effects})
	item, err = restarted.Get(context.Background(), item.WorkID)
	if err != nil || item.ConfirmedBy != "bob" || item.ConfirmedAt.IsZero() || item.RequestedBy != "alice" {
		t.Fatalf("replay attribution: %+v %v", item, err)
	}
	events, _ := e.store.List(context.Background())
	confirmations := 0
	for _, event := range events {
		if event.IdempotencyKey == "confirmation:"+item.Bound.IdempotencyKey {
			confirmations++
		}
	}
	if confirmations != 1 {
		t.Fatal("confirmation duplicated")
	}
	if w := namedAPI(t, h, "POST", path, "bob", "reviewer", map[string]string{"brief_id": "stale"}); w.Code != 409 {
		t.Fatal("stale brief accepted")
	}
}

func TestNamedReviewAndAssignmentRejectImpersonation(t *testing.T) {
	e, _, _, item := preparedReviewFixture(t)
	h, _ := NewHTTPHandler(HTTPConfig{Engine: e, APIKey: "test-secret", RequireHumanIdentity: true})
	root := "/api/civilization/v1/work/" + item.WorkID
	input := reviewInput(item, "approve")
	if w := namedAPI(t, h, "POST", root+"/result-review", "bob", "reviewer", input); w.Code != 403 {
		t.Fatal("spoofed reviewer accepted")
	}
	if w := namedAPI(t, h, "POST", root+"/result-review", "alice", "operator", input); w.Code != 403 {
		t.Fatal("operator review accepted")
	}
	if w := namedAPI(t, h, "POST", root+"/human-owner", "bob", "operator", HumanOwnerAssignment{OwnerID: "bob", AssignedBy: "alice"}); w.Code != 403 {
		t.Fatal("spoofed assigner accepted")
	}
	if w := namedAPI(t, h, "POST", root+"/human-owner", "bob", "operator", HumanOwnerAssignment{OwnerID: "bob"}); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	input.ReviewedBy = ""
	if w := namedAPI(t, h, "POST", root+"/result-review", "alice", "reviewer", input); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	item, _ = e.Get(context.Background(), item.WorkID)
	if item.ResultReview.ReviewedBy != "alice" || item.HumanOwnerAssignedBy != "bob" {
		t.Fatal("trusted actor missing")
	}
	r := httptest.NewRequest("GET", root+"/artifact", nil)
	r.Header.Set("X-Civilization-Actor", "alice")
	r.Header.Set("X-Civilization-Role", "reviewer")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("headers bypassed bearer authentication")
	}
}

func TestNamedInterventionPersistsActorAndTime(t *testing.T) {
	e, _, _, item := preparedReviewFixture(t)
	blocked, err := e.block(context.Background(), item.WorkID, "repair required", "Supply a repair.")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := NewHTTPHandler(HTTPConfig{Engine: e, APIKey: "test-secret", RequireHumanIdentity: true})
	path := "/api/civilization/v1/work/" + item.WorkID + "/interventions/" + blocked.Interventions[0].ID + "/resolve"
	w := namedAPI(t, h, "POST", path, "bob", "operator", map[string]string{"resolution": "Use the corrected input"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	item, _ = e.Get(context.Background(), item.WorkID)
	if item.Interventions[0].ResolvedBy != "bob" || item.Interventions[0].ResolvedAt.IsZero() {
		t.Fatal("intervention actor/time lost")
	}
}
