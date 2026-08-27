package hive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

const (
	factoryTLC51WorkArtifactLabel     = "factory_tlc51_event"
	factoryTLC51WorkArtifactMediaType = "application/vnd.transpara.factory-tlc51-event+json"
)

// LinkTLC51Event attaches the exact EventGraph payload to the corresponding
// FactoryOrder Work task. The EventGraph event is always committed first.
func (store *FactoryV1WorkStore) LinkTLC51Event(ctx context.Context, entry factoryv1.TLC51HistoryEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	link, err := store.factoryTLC51WorkLink(ctx, entry.Identity.FactoryOrderID)
	if err != nil {
		return err
	}
	taskID, err := types.NewEventID(link.TaskID)
	if err != nil {
		return err
	}
	wanted := factoryv1.TLC51WorkArtifactFromEntry(entry)
	body, err := json.Marshal(wanted)
	if err != nil {
		return err
	}
	artifacts, err := store.tasks.ListArtifacts(taskID)
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if artifact.Label != factoryTLC51WorkArtifactLabel {
			continue
		}
		if artifact.MediaType != factoryTLC51WorkArtifactMediaType {
			return errors.New("TLC 5.1 Work artifact has conflicting media type")
		}
		candidate, err := decodeFactoryTLC51WorkArtifact(artifact.Body)
		if err != nil {
			return err
		}
		if candidate.FactoryOrderID == wanted.FactoryOrderID && candidate.ChangeSeriesID == wanted.ChangeSeriesID && candidate.EventOrdinal == wanted.EventOrdinal {
			if factoryTLC51WorkArtifactsEqual(candidate, wanted) {
				return nil
			}
			return fmt.Errorf("%w: conflicting Work twin at ordinal %d", factoryv1.ErrTLC51HistoryConflict, wanted.EventOrdinal)
		}
	}
	eventID, err := types.NewEventID(entry.EventID)
	if err != nil {
		return fmt.Errorf("TLC 5.1 EventGraph event id: %w", err)
	}
	return store.tasks.AddArtifact(store.actor, taskID, factoryTLC51WorkArtifactLabel, factoryTLC51WorkArtifactMediaType, string(body), []types.EventID{eventID}, store.conv)
}

func (store *FactoryV1WorkStore) TLC51WorkArtifacts(ctx context.Context, factoryOrderID, changeSeriesID string) ([]factoryv1.TLC51WorkArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	link, err := store.factoryTLC51WorkLink(ctx, factoryOrderID)
	if err != nil {
		return nil, err
	}
	taskID, err := types.NewEventID(link.TaskID)
	if err != nil {
		return nil, err
	}
	artifacts, err := store.tasks.ListArtifacts(taskID)
	if err != nil {
		return nil, err
	}
	byOrdinal := map[uint64]factoryv1.TLC51WorkArtifact{}
	for _, artifact := range artifacts {
		if artifact.Label != factoryTLC51WorkArtifactLabel {
			continue
		}
		if artifact.MediaType != factoryTLC51WorkArtifactMediaType {
			return nil, errors.New("TLC 5.1 Work artifact has conflicting media type")
		}
		value, err := decodeFactoryTLC51WorkArtifact(artifact.Body)
		if err != nil {
			return nil, fmt.Errorf("decode TLC 5.1 Work artifact %s: %w", artifact.ID.Value(), err)
		}
		if value.FactoryOrderID != factoryOrderID || value.ChangeSeriesID != changeSeriesID {
			continue
		}
		if existing, exists := byOrdinal[value.EventOrdinal]; exists && !factoryTLC51WorkArtifactsEqual(existing, value) {
			return nil, fmt.Errorf("%w: conflicting Work twins at ordinal %d", factoryv1.ErrTLC51HistoryConflict, value.EventOrdinal)
		}
		byOrdinal[value.EventOrdinal] = value
	}
	ordinals := make([]int, 0, len(byOrdinal))
	for ordinal := range byOrdinal {
		ordinals = append(ordinals, int(ordinal))
	}
	sort.Ints(ordinals)
	result := make([]factoryv1.TLC51WorkArtifact, 0, len(ordinals))
	for _, ordinal := range ordinals {
		result = append(result, byOrdinal[uint64(ordinal)])
	}
	return result, nil
}

func (store *FactoryV1WorkStore) factoryTLC51WorkLink(ctx context.Context, factoryOrderID string) (factoryv1.WorkLink, error) {
	links, err := store.ListFactoryOrders(ctx)
	if err != nil {
		return factoryv1.WorkLink{}, err
	}
	for _, link := range links {
		if factoryv1.TLC51FactoryOrderID(link.OrderID, link.Version) == factoryOrderID {
			if link.Quarantined {
				return factoryv1.WorkLink{}, errors.New("FactoryOrder Work link is quarantined")
			}
			return link, nil
		}
	}
	return factoryv1.WorkLink{}, factoryv1.ErrWorkNotFound
}

func decodeFactoryTLC51WorkArtifact(body string) (factoryv1.TLC51WorkArtifact, error) {
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.DisallowUnknownFields()
	var value factoryv1.TLC51WorkArtifact
	if err := decoder.Decode(&value); err != nil {
		return factoryv1.TLC51WorkArtifact{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return factoryv1.TLC51WorkArtifact{}, errors.New("TLC 5.1 Work artifact must contain one JSON value")
	}
	if value.FactoryOrderID == "" || value.ChangeSeriesID == "" || value.EventOrdinal == 0 || !factoryv1.TLC51EventType(value.EventType).IsValid() || !json.Valid(value.Payload) || factoryv1.HashText(string(value.Payload)) != value.PayloadSHA256 {
		return factoryv1.TLC51WorkArtifact{}, errors.New("invalid TLC 5.1 Work artifact identity or digest")
	}
	return value, nil
}

func factoryTLC51WorkArtifactsEqual(left, right factoryv1.TLC51WorkArtifact) bool {
	return left.FactoryOrderID == right.FactoryOrderID && left.ChangeSeriesID == right.ChangeSeriesID &&
		left.EventOrdinal == right.EventOrdinal && left.EventType == right.EventType &&
		left.PayloadSHA256 == right.PayloadSHA256 && bytes.Equal(left.Payload, right.Payload)
}

type FactoryTLC51Reconciliation struct {
	RepairedOrdinals          []uint64 `json:"repaired_ordinals"`
	MatchedOrdinals           []uint64 `json:"matched_ordinals"`
	Quarantined               bool     `json:"quarantined"`
	HumanInterventionRequired bool     `json:"human_intervention_required"`
	Reason                    string   `json:"reason,omitempty"`
}

// ReconcileTLC51Work repairs only a missing Work twin from valid EventGraph
// history. A conflicting Work twin is quarantined and never overwrites source
// history.
func (store *FactoryV1WorkStore) ReconcileTLC51Work(ctx context.Context, entries []factoryv1.TLC51HistoryEntry) (FactoryTLC51Reconciliation, error) {
	result := FactoryTLC51Reconciliation{}
	if len(entries) == 0 {
		result.Quarantined = true
		result.HumanInterventionRequired = true
		result.Reason = "EventGraph TLC 5.1 history is missing"
		return result, nil
	}
	first := entries[0]
	existing, err := store.TLC51WorkArtifacts(ctx, first.Identity.FactoryOrderID, first.Identity.ChangeSeriesID)
	if err != nil && !errors.Is(err, factoryv1.ErrWorkNotFound) {
		if link, linkErr := store.factoryTLC51WorkLink(ctx, first.Identity.FactoryOrderID); linkErr == nil {
			_ = store.QuarantineFactoryOrder(ctx, link, "invalid or conflicting TLC 5.1 Work twins: "+err.Error())
		}
		result.Quarantined = true
		result.HumanInterventionRequired = true
		result.Reason = err.Error()
		return result, nil
	}
	byOrdinal := make(map[uint64]factoryv1.TLC51WorkArtifact, len(existing))
	for _, artifact := range existing {
		byOrdinal[artifact.EventOrdinal] = artifact
	}
	for _, entry := range factoryv1.SortTLC51History(entries) {
		wanted := factoryv1.TLC51WorkArtifactFromEntry(entry)
		if got, exists := byOrdinal[wanted.EventOrdinal]; exists {
			if !factoryTLC51WorkArtifactsEqual(got, wanted) {
				link, linkErr := store.factoryTLC51WorkLink(ctx, entry.Identity.FactoryOrderID)
				if linkErr == nil {
					_ = store.QuarantineFactoryOrder(ctx, link, "conflicting TLC 5.1 EventGraph/Work twins")
				}
				result.Quarantined = true
				result.HumanInterventionRequired = true
				result.Reason = fmt.Sprintf("conflicting twin at ordinal %d", wanted.EventOrdinal)
				return result, nil
			}
			result.MatchedOrdinals = append(result.MatchedOrdinals, wanted.EventOrdinal)
			continue
		}
		if err := store.LinkTLC51Event(ctx, entry); err != nil {
			return result, err
		}
		result.RepairedOrdinals = append(result.RepairedOrdinals, wanted.EventOrdinal)
	}
	return result, nil
}
