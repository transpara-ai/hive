package membrane

import (
	"context"
	"log"
	"time"
)

// LoopStopReason explains why the membrane loop stopped.
type LoopStopReason string

const (
	LoopStopCancelled LoopStopReason = "cancelled"
	LoopStopBudget    LoopStopReason = "budget"
	LoopStopHalt      LoopStopReason = "halt"
	LoopStopError     LoopStopReason = "error"
)

// LoopResult captures the outcome of a membrane loop run.
type LoopResult struct {
	Reason     LoopStopReason
	Iterations int
	Detail     string
}

// LoopConfig configures a membrane loop run.
type LoopConfig struct {
	Service       ServiceClient
	PollInterval  time.Duration
	PollPath      string
	MaxIterations int           // 0 = unlimited
	MaxDuration   time.Duration // 0 = unlimited
	OnPoll        func(iteration int, data []byte) // callback after each poll
}

// RunMembraneLoop runs the poll-translate-gate-dispatch cycle.
func RunMembraneLoop(ctx context.Context, cfg LoopConfig) LoopResult {
	iteration := 0
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	// nil channel blocks forever in select, which is correct for unlimited duration
	var deadline <-chan time.Time
	if cfg.MaxDuration > 0 {
		timer := time.NewTimer(cfg.MaxDuration)
		defer timer.Stop()
		deadline = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			return LoopResult{Reason: LoopStopCancelled, Iterations: iteration}
		case <-deadline:
			return LoopResult{Reason: LoopStopBudget, Iterations: iteration, Detail: "max duration"}
		case <-ticker.C:
			iteration++

			data, err := cfg.Service.Get(ctx, cfg.PollPath)
			if err != nil {
				log.Printf("membrane poll %d: %v", iteration, err)
				if cfg.MaxIterations > 0 && iteration >= cfg.MaxIterations {
					return LoopResult{Reason: LoopStopBudget, Iterations: iteration}
				}
				continue
			}

			if cfg.OnPoll != nil {
				cfg.OnPoll(iteration, data)
			}

			if cfg.MaxIterations > 0 && iteration >= cfg.MaxIterations {
				return LoopResult{Reason: LoopStopBudget, Iterations: iteration}
			}
		}
	}
}
