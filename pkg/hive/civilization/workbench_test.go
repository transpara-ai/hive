package civilization

import (
	"context"
	"errors"
	"testing"

	"github.com/transpara-ai/hive/pkg/hive/tlcbridge"
)

func TestConfirmationSurvivesReplayAndCannotBeSkipped(t *testing.T) {
	ctx := context.Background()
	engine, provider, _ := newTestEngine(t, "Routine", false)
	source := tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "human:preview", Repository: "transpara-ai/hive"}
	selection := ExecutionSelection{Provider: "claude", Model: "explicit", ReasoningEffort: "high"}
	item, err := engine.AcceptSelectedText(ctx, source, "Improve docs", selection, true)
	if err != nil {
		t.Fatal(err)
	}
	item, err = engine.Advance(ctx, item.WorkID)
	if err != nil || item.State != StateAwaitingConfirmation {
		t.Fatalf("preview=%+v %v", item, err)
	}
	if provider.runs[OperationImplement] != 0 {
		t.Fatal("preview implemented")
	}
	restarted, err := NewEngine(EngineConfig{Store: engine.store, Provider: engine.provider, Effects: engine.effects})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Advance(ctx, item.WorkID); err == nil {
		t.Fatal("unconfirmed work advanced")
	}
	if _, err := restarted.Confirm(ctx, item.WorkID, "stale"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("stale confirmation=%v", err)
	}
	confirmed, err := restarted.Confirm(ctx, item.WorkID, item.Bound.IdempotencyKey)
	if err != nil || confirmed.State != StateQueued || confirmed.Selection != selection {
		t.Fatalf("confirm=%+v %v", confirmed, err)
	}
	if _, err := restarted.Confirm(ctx, item.WorkID, item.Bound.IdempotencyKey); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.AcceptSelectedText(ctx, source, "Improve docs", ExecutionSelection{Provider: "codex"}, true); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("selection reuse=%v", err)
	}
	if _, err := restarted.Run(ctx, item.WorkID); err != nil {
		t.Fatal(err)
	}
	if provider.runs[OperationRoute] != 1 || provider.runs[OperationImplement] != 1 {
		t.Fatalf("runs=%v", provider.runs)
	}
}

func TestInvalidIntakeDoesNotPoisonPersistedWork(t *testing.T) {
	engine, _, _ := newTestEngine(t, "Routine", false)
	if _, err := engine.AcceptText(context.Background(), tlcbridge.Source{}, "bad source"); err == nil {
		t.Fatal("invalid source accepted")
	}
	items, err := engine.List(context.Background())
	if err != nil || len(items) != 0 {
		t.Fatalf("items=%v %v", items, err)
	}
}

func TestFailedInvocationKeepsSelectionAndIntakeReplay(t *testing.T) {
	ctx := context.Background()
	engine, _, _ := newTestEngine(t, "Routine", false)
	probe := &selectionProbe{err: errors.New("model unavailable")}
	router := &ProviderRouter{DefaultProvider: "claude", Hosts: map[string]ProviderHost{"claude": {Provider: probe, DefaultModel: "configured-model"}}}
	engine.provider = router
	source := tlcbridge.Source{Kind: tlcbridge.SourceHuman, Identity: "human:failed-model", Repository: "transpara-ai/hive"}
	item, err := engine.AcceptSelectedText(ctx, source, "Improve docs", ExecutionSelection{}, true)
	if err != nil {
		t.Fatal(err)
	}
	item, err = engine.Advance(ctx, item.WorkID)
	if err != nil || item.State != StateBlocked {
		t.Fatalf("failure=%+v %v", item, err)
	}
	if len(item.ProviderRuns) != 1 || item.ProviderRuns[0].Result.Execution.Effective.Model != "configured-model" {
		t.Fatalf("missing failure evidence: %+v", item.ProviderRuns)
	}
	delete(router.Hosts, "claude")
	replayed, err := engine.AcceptSelectedText(ctx, source, "Improve docs", ExecutionSelection{}, true)
	if err != nil || replayed.WorkID != item.WorkID {
		t.Fatalf("durable intake depends on live provider: %+v %v", replayed, err)
	}
}
