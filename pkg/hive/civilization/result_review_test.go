package civilization

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

func preparedReviewFixture(t *testing.T) (*Engine, *fakeProvider, *fakeEffects, WorkProjection) {
	t.Helper()
	engine, provider, effects := newTestEngine(t, "Routine", false)
	ctx := context.Background()
	item, err := engine.SubmitText(ctx, tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "human:alice:original", Repository: "transpara-ai/hive"}, "Improve original docs")
	if err != nil {
		t.Fatal(err)
	}
	_, err = appendEvent(ctx, engine.store, EventStateChanged, item.WorkID, "prepared:fixture", []string{item.LatestEventID}, StateChange{From: item.State, To: StatePrepared, Artifact: &Artifact{Repository: item.Source.Repository, WorkspaceDigest: strings.Repeat("d", 64), Patch: "New file: original.txt\nOriginal delivered content\n"}})
	if err != nil {
		t.Fatal(err)
	}
	item, err = engine.Get(ctx, item.WorkID)
	if err != nil {
		t.Fatal(err)
	}
	return engine, provider, effects, item
}
func reviewInput(item WorkProjection, decision string) ResultReviewRequest {
	return ResultReviewRequest{ResultID: item.PreparedResultID, WorkspaceDigest: item.PreparedResultDigest, ReviewedBy: "alice", Decision: decision, Feedback: "Add the requested enhancement"}
}

func TestResultReviewApprovalAndRejectionPersistWithoutEffects(t *testing.T) {
	for _, decision := range []string{"approve", "reject"} {
		t.Run(decision, func(t *testing.T) {
			ctx := context.Background()
			e, p, effects, item := preparedReviewFixture(t)
			input := reviewInput(item, decision)
			reviewed, err := e.ReviewResult(ctx, item.WorkID, input)
			if err != nil || reviewed.ResultReview == nil || reviewed.ResultReview.Decision != decision {
				t.Fatalf("review=%+v %v", reviewed, err)
			}
			expected := StateApproved
			if decision == "reject" {
				expected = StateRejected
			}
			if reviewed.State != expected || reviewed.ResultReview.RecordedAt.IsZero() {
				t.Fatalf("state=%+v", reviewed)
			}
			events, _ := e.store.List(ctx)
			again, err := e.ReviewResult(ctx, item.WorkID, input)
			if err != nil || again.State != expected {
				t.Fatal(err)
			}
			repeated, _ := e.store.List(ctx)
			if len(events) != len(repeated) {
				t.Fatal("repeated review wrote twice")
			}
			restarted, _ := NewEngine(EngineConfig{Store: e.store, Provider: p, Effects: effects})
			loaded, err := restarted.Advance(ctx, item.WorkID)
			if err != nil || loaded.State != expected {
				t.Fatalf("restart=%+v %v", loaded, err)
			}
			artifact, err := restarted.Artifact(ctx, item.WorkID)
			if err != nil || artifact.Patch != item.Artifact.Patch {
				t.Fatalf("lost artifact: %+v %v", artifact, err)
			}
			if p.runs[OperationImplement] != 0 || len(effects.publishByID) != 0 || len(effects.mergeHeads) != 0 {
				t.Fatal("review executed or published work")
			}
			input.Decision = "request_changes"
			if _, err = e.ReviewResult(ctx, item.WorkID, input); !errors.Is(err, ErrResultReviewConflict) {
				t.Fatalf("conflicting decision accepted: %v", err)
			}
		})
	}
}

func TestResultReviewBindsArtifactAndValidatesFeedback(t *testing.T) {
	for _, mutate := range []func(*ResultReviewRequest){
		func(r *ResultReviewRequest) { r.ResultID = "stale" }, func(r *ResultReviewRequest) { r.WorkspaceDigest = "wrong" },
		func(r *ResultReviewRequest) { r.Feedback = "" }, func(r *ResultReviewRequest) { r.ReviewedBy = "" },
		func(r *ResultReviewRequest) { r.Decision = "merge" }, func(r *ResultReviewRequest) { r.Feedback = strings.Repeat("x", 12001) },
	} {
		e, _, _, item := preparedReviewFixture(t)
		input := reviewInput(item, "request_changes")
		mutate(&input)
		if _, err := e.ReviewResult(context.Background(), item.WorkID, input); err == nil {
			t.Fatal("invalid decision accepted")
		}
		unchanged, _ := e.Get(context.Background(), item.WorkID)
		if unchanged.State != StatePrepared || unchanged.ResultReview != nil {
			t.Fatal("invalid review changed work")
		}
	}
}

func TestRequestedRevisionRetainsOriginalAndRequiresNewConfirmation(t *testing.T) {
	ctx := context.Background()
	e, p, effects, item := preparedReviewFixture(t)
	input := reviewInput(item, "request_changes")
	reviewed, err := e.ReviewResult(ctx, item.WorkID, input)
	if err != nil || reviewed.State != StateChangesRequested || reviewed.ResultReview.RevisionWorkID == "" {
		t.Fatalf("review=%+v %v", reviewed, err)
	}
	child, err := e.Get(ctx, reviewed.ResultReview.RevisionWorkID)
	if err != nil || child.RevisionOf == nil || child.RevisionOf.ResultID != item.PreparedResultID || !child.RequireConfirmation || !strings.Contains(child.IntakeText, input.Feedback) {
		t.Fatalf("revision=%+v %v", child, err)
	}
	if child.WorkID == item.WorkID {
		t.Fatal("original work overwritten")
	}
	restarted, _ := NewEngine(EngineConfig{Store: e.store, Provider: p, Effects: effects})
	child, err = restarted.Advance(ctx, child.WorkID)
	if err != nil || child.State != StateAwaitingConfirmation || p.runs[OperationImplement] != 0 {
		t.Fatalf("revision bypassed confirmation: %+v %v", child, err)
	}
	if _, err = restarted.Confirm(ctx, child.WorkID, item.Bound.IdempotencyKey); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("original brief confirmed revision: %v", err)
	}
	_, err = restarted.Confirm(ctx, child.WorkID, child.Bound.IdempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Run(ctx, child.WorkID); err != nil {
		t.Fatal(err)
	}
	prompt := p.prompts[OperationImplement][0]
	if !strings.Contains(prompt, "Original delivered content") || !strings.Contains(p.prompts[OperationRoute][1], input.Feedback) {
		t.Fatal("revision lost prior artifact or feedback")
	}
	original, _ := restarted.Get(ctx, item.WorkID)
	if original.State != StateChangesRequested || original.Artifact.Patch != item.Artifact.Patch {
		t.Fatal("revision mutated original artifact")
	}
	if _, err = e.ReviewResult(ctx, item.WorkID, input); err != nil {
		t.Fatal(err)
	}
	all, _ := e.List(ctx)
	if len(all) != 2 {
		t.Fatalf("duplicate revision created: %d", len(all))
	}
}

type failRevisionStore struct {
	Store
	fail bool
}

func (s *failRevisionStore) Append(ctx context.Context, n NewEvent) (Event, error) {
	if s.fail && n.Type == EventIntakeAccepted {
		s.fail = false
		return Event{}, errors.New("interrupted revision creation")
	}
	return s.Store.Append(ctx, n)
}
func TestRevisionCreationRecoversAfterRecordedDecision(t *testing.T) {
	ctx := context.Background()
	e, p, effects, item := preparedReviewFixture(t)
	e.store = &failRevisionStore{Store: e.store, fail: true}
	if _, err := e.ReviewResult(ctx, item.WorkID, reviewInput(item, "request_changes")); err == nil {
		t.Fatal("expected interrupted creation")
	}
	saved, _ := e.Get(ctx, item.WorkID)
	if saved.State != StateChangesRequested {
		t.Fatal("decision lost")
	}
	restarted, _ := NewEngine(EngineConfig{Store: e.store, Provider: p, Effects: effects})
	child, err := restarted.Advance(ctx, item.WorkID)
	if err != nil || child.RevisionOf == nil || child.State != StateRouting {
		t.Fatalf("recovery=%+v %v", child, err)
	}
}

func TestConcurrentResultReviewsHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	e, p, effects, item := preparedReviewFixture(t)
	other, _ := NewEngine(EngineConfig{Store: e.store, Provider: p, Effects: effects})
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for index, engine := range []*Engine{e, other} {
		wait.Add(1)
		go func(index int, engine *Engine) {
			defer wait.Done()
			decision := "approve"
			if index == 1 {
				decision = "reject"
			}
			_, err := engine.ReviewResult(ctx, item.WorkID, reviewInput(item, decision))
			results <- err
		}(index, engine)
	}
	wait.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrResultReviewConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d", success, conflicts)
	}
}

func TestResultReviewHTTPUsesExistingAuthenticationAndConflicts(t *testing.T) {
	e, _, _, item := preparedReviewFixture(t)
	h, err := NewHTTPHandler(HTTPConfig{Engine: e, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/civilization/v1/work/" + item.WorkID + "/result-review"
	body := `{"result_id":"stale","workspace_digest":"digest","decision":"approve","reviewed_by":"alice"}`
	for _, auth := range []bool{false, true} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		if auth {
			r.Header.Set("Authorization", "Bearer test-key")
		}
		h.ServeHTTP(w, r)
		expected := 401
		if auth {
			expected = 409
		}
		if w.Code != expected {
			t.Fatalf("status=%d want %d: %s", w.Code, expected, w.Body.String())
		}
	}
}
