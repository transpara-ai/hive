package runner

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"
)

// runTester executes go test ./... in r.cfg.RepoPath and emits a diagnostic on failure.
func (r *Runner) runTester(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "./...")
	cmd.Dir = r.cfg.RepoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[tester] tests FAILED:\n%s", truncateLog(string(out), 800))
		_ = appendDiagnostic(r.cfg.HiveDir, PhaseEvent{
			Phase:     "tester",
			Outcome:   "test_failure",
			Error:     truncateLog(string(out), 1000),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		return fmt.Errorf("tests failed: %w", err)
	}
	log.Printf("[tester] all tests passed")
	return nil
}
