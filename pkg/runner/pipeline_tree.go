package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lovyou-ai/hive/pkg/api"
)

// Phase is a single pipeline phase that can succeed or fail.
type Phase struct {
	Name string
	Run  func(ctx context.Context) error
}

// FixTasker creates a remediation task when a phase failure is detected.
type FixTasker interface {
	CreateTask(ctx context.Context, title string) error
}

// PipelineTree orchestrates a sequence of phases, emitting diagnostics on failure.
type PipelineTree struct {
	cfg        Config
	phases     []Phase
	fixTasker  FixTasker
}

// clientFixTasker adapts an api.Client to the FixTasker interface.
type clientFixTasker struct {
	client *api.Client
	slug   string
}

func (f *clientFixTasker) CreateTask(_ context.Context, title string) error {
	_, err := f.client.CreateTask(f.slug, title, "", "high")
	return err
}

// NewPipelineTree creates a PipelineTree wired to r's phase implementations.
// Each phase delegates to the corresponding Runner method. Execute detects
// failures via direct error returns and by monitoring the diagnostics count.
func NewPipelineTree(r *Runner) *PipelineTree {
	var ft FixTasker
	if r.cfg.APIClient != nil {
		ft = &clientFixTasker{client: r.cfg.APIClient, slug: r.cfg.SpaceSlug}
	}
	return &PipelineTree{
		cfg: r.cfg,
		phases: []Phase{
			{Name: "scout", Run: func(ctx context.Context) error { r.runScout(ctx); return nil }},
			{Name: "architect", Run: func(ctx context.Context) error { r.runArchitect(ctx); return nil }},
			{Name: "builder", Run: func(ctx context.Context) error { r.runBuilder(ctx); return nil }},
			{Name: "critic", Run: func(ctx context.Context) error { r.runCritic(ctx); return nil }},
		},
		fixTasker: ft,
	}
}

// Execute runs each phase in order. On the first failure it emits a PhaseEvent
// diagnostic and returns the error; subsequent phases are skipped. A phase that
// writes new diagnostics internally but returns nil is also treated as a failure.
func (pt *PipelineTree) Execute(ctx context.Context) error {
	for _, phase := range pt.phases {
		prevCount := pt.diagnosticCount()
		err := phase.Run(ctx)
		if err != nil {
			_ = appendDiagnostic(pt.cfg.HiveDir, PhaseEvent{
				Phase:   phase.Name,
				Outcome: "failure",
				Error:   err.Error(),
			})
			pt.callFixTasker(ctx, phase.Name)
			return fmt.Errorf("phase %s failed: %w", phase.Name, err)
		}
		if pt.diagnosticCount() > prevCount {
			pt.callFixTasker(ctx, phase.Name)
			return fmt.Errorf("phase %s failed: diagnostics written without error return", phase.Name)
		}
	}
	return nil
}

// diagnosticCount returns the number of newline-terminated lines in diagnostics.jsonl.
func (pt *PipelineTree) diagnosticCount() int {
	if pt.cfg.HiveDir == "" {
		return 0
	}
	data, err := os.ReadFile(filepath.Join(pt.cfg.HiveDir, "loop", "diagnostics.jsonl"))
	if err != nil {
		return 0
	}
	return bytes.Count(data, []byte("\n"))
}

// callFixTasker calls fixTasker.CreateTask if a tasker is configured.
func (pt *PipelineTree) callFixTasker(ctx context.Context, phaseName string) {
	if pt.fixTasker == nil {
		return
	}
	_ = pt.fixTasker.CreateTask(ctx, fmt.Sprintf("Fix: %s phase failed", phaseName))
}
