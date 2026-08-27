package hive

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

// FactoryTLC51Daemon is the dormant orchestration seam for factory-tlc51/v1.
// Constructing it or calling its pure plan/evaluation methods does not start a
// process and does not grant or invoke a protected effect.
type FactoryTLC51Daemon struct {
	client    factoryv1.TLC51GateClient
	scheduler *factoryv1.TLC51Scheduler
}

func NewFactoryTLC51Daemon(client factoryv1.TLC51GateClient, scheduler *factoryv1.TLC51Scheduler) (*FactoryTLC51Daemon, error) {
	if client == nil || scheduler == nil {
		return nil, errors.New("TLC 5.1 daemon requires a trusted gate client and durable scheduler")
	}
	return &FactoryTLC51Daemon{client: client, scheduler: scheduler}, nil
}

// Admit obtains and durably records the exact trusted plan before returning it
// to the scheduler. It never infers a plan or falls back to tlc-v1.
func (daemon *FactoryTLC51Daemon) Admit(ctx context.Context, binding factoryv1.TLC51OrderBinding, facts json.RawMessage) (factoryv1.TLC51GatePlan, error) {
	plan, err := daemon.client.Plan(ctx, facts)
	if err != nil {
		return factoryv1.TLC51GatePlan{}, err
	}
	if _, err := daemon.scheduler.RecordPlan(ctx, binding, plan); err != nil {
		return factoryv1.TLC51GatePlan{}, err
	}
	return plan, nil
}

// RunOnce executes one bounded wave from the exact plan DAG. Looping and
// activation remain an operator-owned concern outside this dormant seam.
func (daemon *FactoryTLC51Daemon) RunOnce(ctx context.Context, binding factoryv1.TLC51OrderBinding, plan factoryv1.TLC51GatePlan) error {
	return daemon.scheduler.RunOnce(ctx, binding, plan)
}

// Evaluate obtains and records the exact report-only receipt. The receipt is
// evidence, not authority, and this method exposes no mutation driver.
func (daemon *FactoryTLC51Daemon) Evaluate(ctx context.Context, binding factoryv1.TLC51OrderBinding, plan factoryv1.TLC51GatePlan, evaluation json.RawMessage) (factoryv1.TLC51GateReceipt, error) {
	receipt, err := daemon.client.Evaluate(ctx, evaluation)
	if err != nil {
		return factoryv1.TLC51GateReceipt{}, err
	}
	if _, err := daemon.scheduler.RecordDecision(ctx, binding, plan, receipt); err != nil {
		return factoryv1.TLC51GateReceipt{}, err
	}
	return receipt, nil
}
