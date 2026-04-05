// Package telemetry provides operational snapshot storage for the hive dashboard.
// Tables are ephemeral — pruned on schedule, not part of the auditable event chain.
package telemetry

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const schema = `
CREATE TABLE IF NOT EXISTS telemetry_agent_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    agent_role      TEXT NOT NULL,
    actor_id        TEXT NOT NULL,
    state           TEXT NOT NULL,
    model           TEXT NOT NULL,
    iteration       INT NOT NULL,
    max_iterations  INT NOT NULL,
    tokens_used     BIGINT NOT NULL DEFAULT 0,
    cost_usd        NUMERIC(10,6) NOT NULL DEFAULT 0,
    trust_score     NUMERIC(4,3),
    last_event_type TEXT,
    last_message    TEXT,
    errors          INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_telemetry_agent_latest
    ON telemetry_agent_snapshots (agent_role, recorded_at DESC);

CREATE TABLE IF NOT EXISTS telemetry_hive_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    active_agents   INT NOT NULL,
    total_actors    INT NOT NULL,
    chain_length    BIGINT NOT NULL,
    chain_ok        BOOLEAN NOT NULL,
    event_rate      NUMERIC(8,2),
    daily_cost      NUMERIC(10,4),
    daily_cap       NUMERIC(10,4),
    severity        TEXT NOT NULL DEFAULT 'ok'
);

CREATE TABLE IF NOT EXISTS telemetry_phases (
    phase           INT PRIMARY KEY,
    label           TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'blocked',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    notes           TEXT
);

CREATE TABLE IF NOT EXISTS telemetry_event_stream (
    id              BIGSERIAL PRIMARY KEY,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    event_type      TEXT NOT NULL,
    actor_role      TEXT NOT NULL,
    summary         TEXT,
    raw_content     JSONB
);

CREATE INDEX IF NOT EXISTS idx_telemetry_stream_recent
    ON telemetry_event_stream (recorded_at DESC);
`

const seedPhases = `
INSERT INTO telemetry_phases (phase, label, status, started_at, completed_at, notes) VALUES
    (0, 'Foundation',                   'complete',    '2026-03-01', '2026-03-15', 'Strategist, Planner, Implementer, Guardian running. 6 agents functional, 8 hive runs.'),
    (1, 'Operational infrastructure',   'complete',    '2026-03-20', '2026-04-04', 'SysMon + Allocator graduated and running. 40 health.report events confirm SysMon active.'),
    (2, 'Technical leadership',         'blocked',     NULL, NULL, 'CTO + Reviewer — no AgentDefs, no code, only ROLES.md specs.'),
    (3, 'The growth loop',              'blocked',     NULL, NULL, 'Spawner — needs Phase 2. THE UNLOCK.'),
    (4, 'Tier B emergence',             'blocked',     NULL, NULL, 'Organic via growth loop'),
    (5, 'Production deployment',        'blocked',     NULL, NULL, 'Integrator — trust-gated (>0.7)'),
    (6, 'Business operations (Tier C)', 'blocked',     NULL, NULL, 'PM, Finance, CustomerService, SRE, DevOps, Legal'),
    (7, 'Self-governance (Tier D)',     'blocked',     NULL, NULL, 'Philosopher, RoleArchitect, Harmony, Politician'),
    (8, 'Emergent civilization',        'blocked',     NULL, NULL, 'Formalize 31 emergent roles')
ON CONFLICT (phase) DO NOTHING;
`

// phaseUpdates advances phases whose real-world status has changed since
// initial seeding. Each entry is idempotent: it only fires when the current
// DB status matches the expected "before" value, so restarts are safe.
var phaseUpdates = []struct {
	phase  int
	from   string
	status string
	startedAt string
	completedAt string
	notes  string
}{
	{
		phase:       2,
		from:        "blocked",
		status:      "complete",
		startedAt:   "2026-04-04",
		completedAt: "2026-04-05",
		notes:       "CTO graduated: running on Opus with /gap + /directive commands, leadership briefing, 15-iteration stabilization window, Guardian awareness. Reviewer deferred to Phase 4.",
	},
	{
		phase:     3,
		from:      "blocked",
		status:    "in_progress",
		startedAt: "2026-04-05",
		notes:     "Spawner unblocked by CTO graduation. CTO gap detection feeds role proposals.",
	},
}

// EnsureTables creates the telemetry tables, seeds phase data, and applies
// any phase status updates that reflect real-world progress.
// Safe to call on every startup — all operations are idempotent.
func EnsureTables(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("telemetry schema: %w", err)
	}
	if _, err := pool.Exec(ctx, seedPhases); err != nil {
		return fmt.Errorf("telemetry seed phases: %w", err)
	}
	for _, u := range phaseUpdates {
		var q string
		if u.completedAt != "" {
			q = fmt.Sprintf(
				`UPDATE telemetry_phases SET status = '%s', started_at = '%s', completed_at = '%s', notes = '%s' WHERE phase = %d AND status = '%s'`,
				u.status, u.startedAt, u.completedAt, u.notes, u.phase, u.from,
			)
		} else {
			q = fmt.Sprintf(
				`UPDATE telemetry_phases SET status = '%s', started_at = '%s', notes = '%s' WHERE phase = %d AND status = '%s'`,
				u.status, u.startedAt, u.notes, u.phase, u.from,
			)
		}
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("telemetry phase update (phase %d): %w", u.phase, err)
		}
	}
	return nil
}
