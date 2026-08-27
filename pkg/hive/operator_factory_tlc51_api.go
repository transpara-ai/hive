package hive

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

const FactoryTLC51MissionControlPath = "/api/hive/factory-tlc51/mission-control"

type FactoryTLC51ProjectionTarget struct {
	Binding factoryv1.TLC51OrderBinding
	Plan    factoryv1.TLC51GatePlan
}

type FactoryTLC51ProjectionCatalog interface {
	TLC51ProjectionTargets(context.Context) ([]FactoryTLC51ProjectionTarget, error)
}

type FactoryTLC51MissionControlEnvelope struct {
	SchemaVersion    string                               `json:"schema_version"`
	GeneratedAt      time.Time                            `json:"generated_at"`
	Orders           []factoryv1.TLC51MissionControlOrder `json:"orders"`
	Errors           []string                             `json:"errors"`
	AuthorityGranted bool                                 `json:"authority_granted"`
}

type FactoryTLC51MissionControlSource interface {
	BuildFactoryTLC51MissionControl(context.Context) FactoryTLC51MissionControlEnvelope
}

// FactoryTLC51MissionControlProjector joins exact EventGraph history with Work
// twins. It only projects durable state and never evaluates TLC policy.
type FactoryTLC51MissionControlProjector struct {
	catalog FactoryTLC51ProjectionCatalog
	journal factoryv1.TLC51Journal
	work    factoryv1.TLC51WorkLinker
	clock   factoryv1.Clock
}

func NewFactoryTLC51MissionControlProjector(catalog FactoryTLC51ProjectionCatalog, journal factoryv1.TLC51Journal, work factoryv1.TLC51WorkLinker, clock factoryv1.Clock) (*FactoryTLC51MissionControlProjector, error) {
	if catalog == nil || journal == nil || work == nil {
		return nil, errors.New("TLC 5.1 Mission Control requires catalog, EventGraph journal, and Work linker")
	}
	if clock == nil {
		clock = factoryv1.WallClock{}
	}
	return &FactoryTLC51MissionControlProjector{catalog: catalog, journal: journal, work: work, clock: clock}, nil
}

func (projector *FactoryTLC51MissionControlProjector) BuildFactoryTLC51MissionControl(ctx context.Context) FactoryTLC51MissionControlEnvelope {
	now := projector.clock.Now().UTC()
	result := FactoryTLC51MissionControlEnvelope{
		SchemaVersion: "factory-tlc51-mission-control-envelope/v1", GeneratedAt: now,
		Orders: []factoryv1.TLC51MissionControlOrder{}, Errors: []string{}, AuthorityGranted: false,
	}
	targets, err := projector.catalog.TLC51ProjectionTargets(ctx)
	if err != nil {
		result.Errors = append(result.Errors, "catalog: "+err.Error())
		return result
	}
	seen := map[string]struct{}{}
	for _, target := range targets {
		key := target.Binding.FactoryOrderID + "\x00" + target.Plan.ChangeSeriesID
		if _, duplicate := seen[key]; duplicate {
			result.Errors = append(result.Errors, fmt.Sprintf("duplicate FactoryOrder/change series %q", key))
			continue
		}
		seen[key] = struct{}{}
		history, historyErr := projector.journal.TLC51History(ctx, target.Binding.FactoryOrderID, target.Plan.ChangeSeriesID)
		if historyErr != nil {
			result.Errors = append(result.Errors, target.Binding.FactoryOrderID+": EventGraph: "+historyErr.Error())
			continue
		}
		artifacts, workErr := projector.work.TLC51WorkArtifacts(ctx, target.Binding.FactoryOrderID, target.Plan.ChangeSeriesID)
		if workErr != nil {
			result.Errors = append(result.Errors, target.Binding.FactoryOrderID+": Work: "+workErr.Error())
			continue
		}
		row, projectErr := factoryv1.ProjectTLC51MissionControl(target.Binding, target.Plan, history, artifacts, now)
		if projectErr != nil {
			result.Errors = append(result.Errors, target.Binding.FactoryOrderID+": projection: "+projectErr.Error())
			continue
		}
		result.Orders = append(result.Orders, row)
	}
	sort.Slice(result.Orders, func(i, j int) bool {
		return result.Orders[i].FactoryOrderID+"\x00"+result.Orders[i].ChangeSeriesID < result.Orders[j].FactoryOrderID+"\x00"+result.Orders[j].ChangeSeriesID
	})
	sort.Strings(result.Errors)
	return result
}
