package hive

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
	workpkg "github.com/transpara-ai/work"
)

func testFactoryTLC51Append(t *testing.T, orderID, seriesID string, kind factoryv1.TLC51EventType, ordinal uint64, attempt uint32, fields map[string]any) factoryv1.TLC51Append {
	t.Helper()
	identity := factoryv1.TLC51EventIdentity{
		ProtocolVersion: factoryv1.TLC51ProtocolVersion, FactoryOrderID: orderID, ChangeSeriesID: seriesID,
		PlanDigest: strings.Repeat("a", 64), SubjectDigest: strings.Repeat("b", 64), EventOrdinal: ordinal, AttemptOrdinal: attempt,
	}
	payload, err := factoryv1.NewTLC51EventPayload(identity, fields)
	if err != nil {
		t.Fatal(err)
	}
	return factoryv1.TLC51Append{Type: kind, Identity: identity, Payload: payload, OccurredAt: time.Date(2026, 8, 27, 12, 0, int(ordinal), 0, time.UTC)}
}

func TestFactoryTLC51EventGraphJournalExactReplayGapAndConflict(t *testing.T) {
	ctx := context.Background()
	s, factory, signer, actor, conv := newDecisionTestStore(t)
	journal, err := NewFactoryTLC51EventGraphJournal(s, factory, signer, actor, conv)
	if err != nil {
		t.Fatal(err)
	}
	firstInput := testFactoryTLC51Append(t, "fo_tlc51_adapter", "series-1", factoryv1.TLC51PlanRecorded, 1, 0, map[string]any{
		"plan":        map[string]any{"schema_version": factoryv1.TLC51PlanSchema, "canonical_json": "{}\n", "sha256": strings.Repeat("c", 64)},
		"recorded_at": time.Date(2026, 8, 27, 12, 0, 1, 0, time.UTC),
	})
	first, err := journal.AppendTLC51(ctx, firstInput)
	if err != nil {
		t.Fatalf("AppendTLC51: %v", err)
	}
	repeated, err := journal.AppendTLC51(ctx, firstInput)
	if err != nil || repeated.EventID != first.EventID {
		t.Fatalf("exact replay = %+v err=%v", repeated, err)
	}
	conflict := testFactoryTLC51Append(t, "fo_tlc51_adapter", "series-1", factoryv1.TLC51HumanRequested, 1, 0, map[string]any{
		"request_id": "H001", "boundary": "test", "reason": "conflict", "requested_at": time.Date(2026, 8, 27, 12, 0, 1, 0, time.UTC),
	})
	if _, err := journal.AppendTLC51(ctx, conflict); !errors.Is(err, factoryv1.ErrTLC51HistoryConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	gap := testFactoryTLC51Append(t, "fo_tlc51_adapter", "series-1", factoryv1.TLC51HumanRequested, 3, 0, map[string]any{
		"request_id": "H002", "boundary": "test", "reason": "gap", "requested_at": time.Date(2026, 8, 27, 12, 0, 3, 0, time.UTC),
	})
	if _, err := journal.AppendTLC51(ctx, gap); !errors.Is(err, factoryv1.ErrTLC51HistoryGap) {
		t.Fatalf("gap error = %v", err)
	}
	history, err := journal.TLC51History(ctx, "fo_tlc51_adapter", "series-1")
	if err != nil || len(history) != 1 || string(history[0].Payload) != string(firstInput.Payload) {
		t.Fatalf("history = %+v err=%v", history, err)
	}
	page, err := s.ByType(types.MustEventType(string(factoryv1.TLC51PlanRecorded)), 10, types.None[types.Cursor]())
	if err != nil || len(page.Items()) != 1 {
		t.Fatalf("EventGraph page = %+v err=%v", page.Items(), err)
	}
	raw, err := json.Marshal(page.Items()[0].Content())
	if err != nil || string(raw) != string(firstInput.Payload) {
		t.Fatalf("stored content bytes = %s want %s err=%v", raw, firstInput.Payload, err)
	}
}

func TestFactoryTLC51WorkReconciliationRepairsMissingAndQuarantinesConflict(t *testing.T) {
	ctx := context.Background()
	s, factory, signer, actor, conv := newDecisionTestStore(t)
	workpkg.RegisterWithRegistry(factory.Registry)
	workStore, err := NewFactoryV1WorkStore(s, factory, signer, actor, conv)
	if err != nil {
		t.Fatal(err)
	}
	head, err := s.Head()
	if err != nil || head.IsNone() {
		t.Fatalf("bootstrap head: %v", err)
	}
	link, err := workStore.SeedFactoryOrder(ctx, factoryv1.WorkSeed{
		OrderID: "FO-TLC51-WORK", Version: "1.0.0", DocumentSHA256: strings.Repeat("d", 64),
		Markdown: "# TLC 5.1 Work adapter\n", SourceSHA256: strings.Repeat("e", 64),
		AcceptedEventID: head.Unwrap().ID().Value(), IdempotencyKey: "tlc51-work-seed",
	})
	if err != nil {
		t.Fatalf("SeedFactoryOrder: %v", err)
	}
	factoryOrderID := factoryv1.TLC51FactoryOrderID(link.OrderID, link.Version)
	journal, err := NewFactoryTLC51EventGraphJournal(s, factory, signer, actor, conv)
	if err != nil {
		t.Fatal(err)
	}
	firstInput := testFactoryTLC51Append(t, factoryOrderID, "series-1", factoryv1.TLC51PlanRecorded, 1, 0, map[string]any{
		"plan":        map[string]any{"schema_version": factoryv1.TLC51PlanSchema, "canonical_json": "{}\n", "sha256": strings.Repeat("c", 64)},
		"recorded_at": time.Date(2026, 8, 27, 12, 0, 1, 0, time.UTC),
	})
	firstInput.Causes = []string{head.Unwrap().ID().Value()}
	first, err := journal.AppendTLC51(ctx, firstInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := workStore.LinkTLC51Event(ctx, first); err != nil {
		t.Fatalf("LinkTLC51Event: %v", err)
	}
	secondInput := testFactoryTLC51Append(t, factoryOrderID, "series-1", factoryv1.TLC51HumanRequested, 2, 0, map[string]any{
		"request_id": "H001", "boundary": "obligation:O001", "reason": "Human wait", "requested_at": time.Date(2026, 8, 27, 12, 0, 2, 0, time.UTC),
	})
	second, err := journal.AppendTLC51(ctx, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	reconciliation, err := workStore.ReconcileTLC51Work(ctx, []factoryv1.TLC51HistoryEntry{first, second})
	if err != nil {
		t.Fatalf("ReconcileTLC51Work: %v", err)
	}
	if len(reconciliation.MatchedOrdinals) != 1 || len(reconciliation.RepairedOrdinals) != 1 || reconciliation.RepairedOrdinals[0] != 2 || reconciliation.Quarantined {
		t.Fatalf("reconciliation = %+v", reconciliation)
	}

	// A raw conflicting Work twin cannot overwrite the EventGraph source.
	conflictingInput := testFactoryTLC51Append(t, factoryOrderID, "series-1", factoryv1.TLC51HumanResolved, 2, 0, map[string]any{
		"request_id": "H001", "resolution": "forged", "resolved_at": time.Date(2026, 8, 27, 12, 1, 0, 0, time.UTC),
	})
	conflicting := factoryv1.TLC51WorkArtifact{
		FactoryOrderID: factoryOrderID, ChangeSeriesID: "series-1", EventOrdinal: 2,
		EventType: string(conflictingInput.Type), Payload: conflictingInput.Payload,
		PayloadSHA256: factoryv1.HashText(string(conflictingInput.Payload)),
	}
	body, _ := json.Marshal(conflicting)
	taskID := types.MustEventID(link.TaskID)
	latestHead, _ := s.Head()
	if err := workStore.tasks.AddArtifact(actor, taskID, factoryTLC51WorkArtifactLabel, factoryTLC51WorkArtifactMediaType, string(body), []types.EventID{latestHead.Unwrap().ID()}, conv); err != nil {
		t.Fatalf("inject conflicting Work twin: %v", err)
	}
	quarantined, err := workStore.ReconcileTLC51Work(ctx, []factoryv1.TLC51HistoryEntry{first, second})
	if err != nil {
		t.Fatalf("conflict reconciliation: %v", err)
	}
	if !quarantined.Quarantined || !quarantined.HumanInterventionRequired {
		t.Fatalf("conflict did not quarantine: %+v", quarantined)
	}
	updated, err := workStore.GetFactoryOrder(ctx, link.OrderID, link.Version)
	if err != nil || !updated.Quarantined {
		t.Fatalf("FactoryOrder quarantine = %+v err=%v", updated, err)
	}
}
