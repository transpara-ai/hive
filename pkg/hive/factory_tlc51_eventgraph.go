package hive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

func factoryTLC51EventTypes() []types.EventType {
	result := make([]types.EventType, 0, len(factoryv1.AllTLC51EventTypes()))
	for _, value := range factoryv1.AllTLC51EventTypes() {
		result = append(result, types.MustEventType(string(value)))
	}
	return result
}

// FactoryTLC51EventContent preserves the exact typed factory.tlc51.* payload
// while allowing Hive to consume it without copying EventGraph's Go structs.
// Its JSON representation is exactly Raw, with no wrapper object.
type FactoryTLC51EventContent struct {
	hiveContent
	Type factoryv1.TLC51EventType `json:"-"`
	Raw  json.RawMessage          `json:"-"`
}

func (content FactoryTLC51EventContent) EventTypeName() string { return string(content.Type) }

func (content FactoryTLC51EventContent) MarshalJSON() ([]byte, error) {
	if len(content.Raw) == 0 || !json.Valid(content.Raw) {
		return nil, errors.New("factory-tlc51/v1 content is not valid JSON")
	}
	return append([]byte(nil), content.Raw...), nil
}

func (content *FactoryTLC51EventContent) UnmarshalJSON(raw []byte) error {
	if !json.Valid(raw) {
		return errors.New("factory-tlc51/v1 content is not valid JSON")
	}
	content.Raw = append(json.RawMessage(nil), raw...)
	return nil
}

func validateFactoryTLC51EventContent(content event.EventContent) error {
	value, ok := content.(FactoryTLC51EventContent)
	if !ok {
		if pointer, pointerOK := content.(*FactoryTLC51EventContent); pointerOK && pointer != nil {
			value = *pointer
			ok = true
		}
	}
	if !ok || !value.Type.IsValid() {
		return fmt.Errorf("unexpected factory-tlc51/v1 content %T", content)
	}
	var identity factoryv1.TLC51EventIdentity
	if err := json.Unmarshal(value.Raw, &identity); err != nil {
		return err
	}
	return factoryv1.ValidateTLC51Append(factoryv1.TLC51Append{
		Type: value.Type, Identity: identity, Payload: value.Raw, OccurredAt: time.Unix(1, 0).UTC(),
	})
}

func registerFactoryTLC51ContentUnmarshalers() {
	for _, eventType := range factoryv1.AllTLC51EventTypes() {
		kind := eventType
		event.RegisterContentUnmarshaler(string(kind), func(raw []byte) (event.EventContent, error) {
			content := &FactoryTLC51EventContent{Type: kind}
			if err := content.UnmarshalJSON(raw); err != nil {
				return nil, err
			}
			if err := validateFactoryTLC51EventContent(content); err != nil {
				return nil, err
			}
			return content, nil
		})
	}
}

func registerFactoryTLC51WithRegistry(registry *event.EventTypeRegistry) {
	for _, eventType := range factoryTLC51EventTypes() {
		registry.Register(eventType, validateFactoryTLC51EventContent)
	}
}

// FactoryTLC51EventGraphJournal persists exact factory.tlc51.* payloads as
// signed EventGraph events. It enforces one contiguous ordinal per
// FactoryOrder/change series and idempotent exact replay.
type FactoryTLC51EventGraphJournal struct {
	store   store.Store
	factory *event.EventFactory
	signer  event.Signer
	actor   types.ActorID
	conv    types.ConversationID
}

func NewFactoryTLC51EventGraphJournal(s store.Store, factory *event.EventFactory, signer event.Signer, actor types.ActorID, conv types.ConversationID) (*FactoryTLC51EventGraphJournal, error) {
	if s == nil || factory == nil || signer == nil || actor.Value() == "" || conv.Value() == "" {
		return nil, errors.New("factory-tlc51/v1 EventGraph journal requires store, factory, signer, actor, and conversation")
	}
	return &FactoryTLC51EventGraphJournal{store: s, factory: factory, signer: signer, actor: actor, conv: conv}, nil
}

func (journal *FactoryTLC51EventGraphJournal) AppendTLC51(ctx context.Context, input factoryv1.TLC51Append) (factoryv1.TLC51HistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return factoryv1.TLC51HistoryEntry{}, err
	}
	if err := factoryv1.ValidateTLC51Append(input); err != nil {
		return factoryv1.TLC51HistoryEntry{}, err
	}
	history, err := journal.TLC51History(ctx, input.Identity.FactoryOrderID, input.Identity.ChangeSeriesID)
	if err != nil {
		return factoryv1.TLC51HistoryEntry{}, err
	}
	if input.Identity.EventOrdinal <= uint64(len(history)) {
		existing := history[input.Identity.EventOrdinal-1]
		if existing.Type == input.Type && bytes.Equal(existing.Payload, input.Payload) {
			return existing, nil
		}
		return factoryv1.TLC51HistoryEntry{}, fmt.Errorf("%w: ordinal %d", factoryv1.ErrTLC51HistoryConflict, input.Identity.EventOrdinal)
	}
	if input.Identity.EventOrdinal != uint64(len(history)+1) {
		return factoryv1.TLC51HistoryEntry{}, fmt.Errorf("%w: got %d want %d", factoryv1.ErrTLC51HistoryGap, input.Identity.EventOrdinal, len(history)+1)
	}
	causes, err := journal.causes(input.Causes, history)
	if err != nil {
		return factoryv1.TLC51HistoryEntry{}, err
	}
	content := &FactoryTLC51EventContent{Type: input.Type, Raw: append(json.RawMessage(nil), input.Payload...)}
	eventType := types.MustEventType(string(input.Type))
	const maxChainAttempts = 8
	for attempt := 0; attempt < maxChainAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return factoryv1.TLC51HistoryEntry{}, err
		}
		ev, createErr := journal.factory.Create(eventType, journal.actor, content, causes, journal.conv, journal.store, journal.signer)
		if createErr != nil {
			return factoryv1.TLC51HistoryEntry{}, fmt.Errorf("create %s: %w", input.Type, createErr)
		}
		appended, appendErr := journal.store.Append(ev)
		if appendErr == nil {
			return factoryTLC51HistoryEntry(appended, *content), nil
		}
		if !strings.Contains(appendErr.Error(), "chain integrity violation") || attempt == maxChainAttempts-1 {
			return factoryv1.TLC51HistoryEntry{}, fmt.Errorf("append %s: %w", input.Type, appendErr)
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return factoryv1.TLC51HistoryEntry{}, errors.New("append factory-tlc51/v1 event exhausted retries")
}

func (journal *FactoryTLC51EventGraphJournal) TLC51History(ctx context.Context, factoryOrderID, changeSeriesID string) ([]factoryv1.TLC51HistoryEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var result []factoryv1.TLC51HistoryEntry
	for _, eventType := range factoryTLC51EventTypes() {
		cursor := types.None[types.Cursor]()
		for {
			page, err := journal.store.ByType(eventType, 200, cursor)
			if err != nil {
				return nil, fmt.Errorf("list %s: %w", eventType.Value(), err)
			}
			for _, stored := range page.Items() {
				content, ok := factoryTLC51Content(stored.Content())
				if !ok {
					return nil, fmt.Errorf("event %s content is %T, want FactoryTLC51EventContent", stored.ID().Value(), stored.Content())
				}
				entry := factoryTLC51HistoryEntry(stored, content)
				if entry.Identity.FactoryOrderID == factoryOrderID && entry.Identity.ChangeSeriesID == changeSeriesID {
					result = append(result, entry)
				}
			}
			if !page.HasMore() {
				break
			}
			cursor = page.Cursor()
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Identity.EventOrdinal < result[j].Identity.EventOrdinal })
	for index, entry := range result {
		if entry.Identity.EventOrdinal != uint64(index+1) {
			return nil, fmt.Errorf("%w: projected ordinal %d at index %d", factoryv1.ErrTLC51HistoryConflict, entry.Identity.EventOrdinal, index)
		}
	}
	return result, nil
}

func factoryTLC51Content(content event.EventContent) (FactoryTLC51EventContent, bool) {
	switch value := content.(type) {
	case FactoryTLC51EventContent:
		return value, true
	case *FactoryTLC51EventContent:
		if value != nil {
			return *value, true
		}
	}
	return FactoryTLC51EventContent{}, false
}

func factoryTLC51HistoryEntry(stored event.Event, content FactoryTLC51EventContent) factoryv1.TLC51HistoryEntry {
	var identity factoryv1.TLC51EventIdentity
	_ = json.Unmarshal(content.Raw, &identity)
	causes := make([]string, 0, len(stored.Causes()))
	for _, cause := range stored.Causes() {
		causes = append(causes, cause.Value())
	}
	return factoryv1.TLC51HistoryEntry{
		EventID: stored.ID().Value(), Type: content.Type, Identity: identity,
		Payload:       append(json.RawMessage(nil), content.Raw...),
		PayloadSHA256: factoryv1.HashText(string(content.Raw)), OccurredAt: stored.Timestamp().Value(), Causes: causes,
	}
}

func (journal *FactoryTLC51EventGraphJournal) causes(refs []string, history []factoryv1.TLC51HistoryEntry) ([]types.EventID, error) {
	if len(history) > 0 {
		id, err := types.NewEventID(history[len(history)-1].EventID)
		if err != nil {
			return nil, err
		}
		return []types.EventID{id}, nil
	}
	causes := make([]types.EventID, 0, len(refs))
	for _, ref := range refs {
		id, err := types.NewEventID(ref)
		if err != nil {
			return nil, fmt.Errorf("factory-tlc51/v1 cause %q: %w", ref, err)
		}
		causes = append(causes, id)
	}
	if len(causes) > 0 {
		return causes, nil
	}
	head, err := journal.store.Head()
	if err != nil {
		return nil, err
	}
	if head.IsNone() {
		return nil, errors.New("factory-tlc51/v1 EventGraph journal requires a bootstrap cause")
	}
	return []types.EventID{head.Unwrap().ID()}, nil
}
