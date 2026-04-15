package telemetry

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestEnsureTablesIntegration verifies the full schema, migrations, and seed
// data against a real Postgres instance. Requires DATABASE_URL or defaults to
// the local docker-compose DSN. Skipped in short mode / CI.
func TestEnsureTablesIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hive:hive@localhost:5432/hive"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to postgres: %v", err)
	}
	defer pool.Close()

	// Ping to confirm connectivity.
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}

	// Run EnsureTables — should be idempotent.
	if err := EnsureTables(ctx, pool); err != nil {
		t.Fatalf("EnsureTables: %v", err)
	}

	// Run it again to verify idempotency.
	if err := EnsureTables(ctx, pool); err != nil {
		t.Fatalf("EnsureTables (second call): %v", err)
	}

	// --- Verify new tables exist ---

	t.Run("role_definitions_table", func(t *testing.T) {
		var count int
		err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM telemetry_role_definitions").Scan(&count)
		if err != nil {
			t.Fatalf("query role_definitions: %v", err)
		}
		if count < 25 {
			t.Errorf("role_definitions has %d rows, want >= 25 (seed data)", count)
		}
	})

	t.Run("layers_table", func(t *testing.T) {
		var count int
		err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM telemetry_layers").Scan(&count)
		if err != nil {
			t.Fatalf("query layers: %v", err)
		}
		if count != 14 {
			t.Errorf("layers has %d rows, want 14", count)
		}
	})

	t.Run("phase_agents_table", func(t *testing.T) {
		var count int
		err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM telemetry_phase_agents").Scan(&count)
		if err != nil {
			t.Fatalf("query phase_agents: %v", err)
		}
		if count < 24 {
			t.Errorf("phase_agents has %d rows, want >= 24", count)
		}
	})

	t.Run("exit_criteria_column", func(t *testing.T) {
		var criteria *string
		err := pool.QueryRow(ctx,
			"SELECT exit_criteria FROM telemetry_phases WHERE phase = 0",
		).Scan(&criteria)
		if err != nil {
			t.Fatalf("query exit_criteria: %v", err)
		}
		if criteria == nil || *criteria == "" {
			t.Error("phase 0 exit_criteria is empty, want non-empty")
		}
	})

	// --- Verify specific seed data ---

	t.Run("layer_0_is_foundation", func(t *testing.T) {
		var name string
		err := pool.QueryRow(ctx,
			"SELECT name FROM telemetry_layers WHERE layer = 0",
		).Scan(&name)
		if err != nil {
			t.Fatalf("query layer 0: %v", err)
		}
		if name != "Foundation" {
			t.Errorf("layer 0 name = %q, want %q", name, "Foundation")
		}
	})

	t.Run("guardian_in_phase_0", func(t *testing.T) {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM telemetry_phase_agents WHERE phase = 0 AND agent_role = 'guardian')",
		).Scan(&exists)
		if err != nil {
			t.Fatalf("query phase_agents: %v", err)
		}
		if !exists {
			t.Error("guardian not found in phase 0")
		}
	})

	t.Run("integrator_depends_on_reviewer_tester", func(t *testing.T) {
		var deps []string
		err := pool.QueryRow(ctx,
			"SELECT depends_on FROM telemetry_role_definitions WHERE role = 'integrator'",
		).Scan(&deps)
		if err != nil {
			t.Fatalf("query integrator deps: %v", err)
		}
		if len(deps) != 2 {
			t.Errorf("integrator depends_on has %d entries, want 2", len(deps))
		}
	})

	t.Run("phase4_reverted_to_in_progress", func(t *testing.T) {
		var status string
		var completedAt *string
		err := pool.QueryRow(ctx,
			"SELECT status, completed_at::text FROM telemetry_phases WHERE phase = 4",
		).Scan(&status, &completedAt)
		if err != nil {
			t.Fatalf("query phase 4: %v", err)
		}
		if status != PhaseInProgress {
			t.Errorf("phase 4 status = %q, want %q", status, PhaseInProgress)
		}
		if completedAt != nil {
			t.Errorf("phase 4 completed_at = %v, want NULL", *completedAt)
		}
	})
}
