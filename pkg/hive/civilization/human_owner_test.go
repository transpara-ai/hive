package civilization

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

func TestHumanResponsibilityReplaysWithoutAdvancingOrGrantingApproval(t *testing.T) {
	ctx := context.Background()
	engine, provider, effects := newTestEngine(t, "Routine", false)
	work, err := engine.AcceptSelectedText(ctx, tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "human:alice:intake-test", Repository: "transpara-ai/hive"}, "Improve operator guidance", ExecutionSelection{}, true)
	if err != nil {
		t.Fatal(err)
	}
	assigned, err := engine.AssignHumanOwner(ctx, work.WorkID, HumanOwnerAssignment{OwnerID: "alice", AssignedBy: "bob"})
	if err != nil || assigned.HumanOwnerID != "alice" || assigned.HumanOwnerAssignedBy != "bob" || assigned.HumanOwnerAssignmentID == "" {
		t.Fatalf("assignment=%+v error=%v", assigned, err)
	}
	if assigned.State != work.State || assigned.RequireConfirmation != work.RequireConfirmation || assigned.Bound != nil || len(assigned.ProviderRuns) != 0 {
		t.Fatal("assignment changed execution or confirmation")
	}
	restarted, err := NewEngine(EngineConfig{Store: engine.store, Provider: provider, Effects: effects})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := restarted.Get(ctx, work.WorkID)
	if err != nil || replayed.HumanOwnerAssignmentID != assigned.HumanOwnerAssignmentID || replayed.HumanOwnerID != "alice" {
		t.Fatal("assignment did not replay")
	}
	if _, err := restarted.AssignHumanOwner(ctx, work.WorkID, HumanOwnerAssignment{OwnerID: "bob", AssignedBy: "bob"}); !errors.Is(err, ErrHumanOwnerConflict) {
		t.Fatalf("stale assignment accepted: %v", err)
	}
	cleared, err := restarted.AssignHumanOwner(ctx, work.WorkID, HumanOwnerAssignment{AssignedBy: "bob", PreviousAssignmentID: assigned.HumanOwnerAssignmentID})
	if err != nil || cleared.HumanOwnerID != "" || cleared.HumanOwnerAssignmentID == assigned.HumanOwnerAssignmentID {
		t.Fatal("cannot return work to shared queue")
	}
}

func TestConcurrentHumanAssignmentsHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	engine, _, _ := newTestEngine(t, "Routine", false)
	work, err := engine.AcceptText(ctx, tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "human:alice:compete", Repository: "transpara-ai/hive"}, "Concurrent assignment")
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, owner := range []string{"alice", "bob"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			_, err := engine.AssignHumanOwner(ctx, work.WorkID, HumanOwnerAssignment{OwnerID: owner, AssignedBy: owner})
			results <- err
		}(owner)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrHumanOwnerConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d", successes, conflicts)
	}
}

func TestHumanAssignmentDoesNotWaitForExecutionLock(t *testing.T) {
	engine, _, _ := newTestEngine(t, "Routine", false)
	work, err := engine.AcceptText(context.Background(), tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "human:alice:active", Repository: "transpara-ai/hive"}, "Active work")
	if err != nil {
		t.Fatal(err)
	}
	release := engine.lockWork(work.WorkID)
	defer release()
	result := make(chan error, 1)
	go func() {
		_, err := engine.AssignHumanOwner(context.Background(), work.WorkID, HumanOwnerAssignment{OwnerID: "alice", AssignedBy: "alice"})
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("assignment waited for the running provider")
	}
}

func TestHumanOwnerHTTPUsesExistingAuthenticationAndConflictResponse(t *testing.T) {
	engine, _, _ := newTestEngine(t, "Routine", false)
	work, err := engine.AcceptText(context.Background(), tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "human:alice:http", Repository: "transpara-ai/hive"}, "Assign through HTTP")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHTTPHandler(HTTPConfig{Engine: engine, APIKey: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/civilization/v1/work/" + work.WorkID + "/human-owner"
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest("POST", path, nil))
	if unauthorized.Code != 401 {
		t.Fatalf("unauthenticated assignment status=%d", unauthorized.Code)
	}
	for _, tc := range []struct {
		payload string
		status  int
	}{
		{`{"owner_id":"alice","assigned_by":"bob","previous_assignment_id":""}`, 200},
		{`{"owner_id":"bob","assigned_by":"bob","previous_assignment_id":""}`, 409},
		{`{"owner_id":"bob","assigned_by":""}`, 400},
		{`{"owner_id":"bob","assigned_by":"bob","approval":true}`, 400},
	} {
		response := apiRequest(t, handler, "POST", path, []byte(tc.payload))
		if response.Code != tc.status {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if response.Code == 200 {
			var result WorkProjection
			json.Unmarshal(response.Body.Bytes(), &result)
			if result.HumanOwnerID != "alice" {
				t.Fatal("assignment not projected")
			}
		}
	}
}
