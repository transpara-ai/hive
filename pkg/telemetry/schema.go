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
    last_event_at   TIMESTAMPTZ,
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
    notes           TEXT,
    exit_criteria   TEXT
);

CREATE TABLE IF NOT EXISTS telemetry_role_definitions (
    role            TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    tier            TEXT NOT NULL DEFAULT 'A',
    purpose         TEXT,
    model           TEXT,
    can_operate     BOOLEAN NOT NULL DEFAULT false,
    max_iterations  INT,
    watch_patterns  TEXT[],
    phase           INT,
    graduated_at    TIMESTAMPTZ,
    status          TEXT NOT NULL DEFAULT 'designed',
    has_prompt      BOOLEAN NOT NULL DEFAULT false,
    has_persona     BOOLEAN NOT NULL DEFAULT false,
    category        TEXT,
    depends_on      TEXT[],
    origin          TEXT NOT NULL DEFAULT 'bootstrap' CHECK (origin IN ('bootstrap', 'spawned')),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS telemetry_layers (
    layer           INT PRIMARY KEY,
    name            TEXT NOT NULL,
    focus           TEXT NOT NULL,
    depth           TEXT NOT NULL DEFAULT 'aspirational',
    description     TEXT
);

CREATE TABLE IF NOT EXISTS telemetry_phase_agents (
    phase           INT NOT NULL REFERENCES telemetry_phases(phase),
    agent_role      TEXT NOT NULL,
    PRIMARY KEY (phase, agent_role)
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
INSERT INTO telemetry_phases (phase, label, status, started_at, completed_at, notes, exit_criteria) VALUES
    (0, 'Foundation',                   'complete',    '2026-03-01', '2026-03-15', 'Strategist, Planner, Implementer, Guardian running. 6 agents functional, 8 hive runs.',
        'All foundation agents coordinating via events and tasks.'),
    (1, 'Operational infrastructure',   'complete',    '2026-03-20', '2026-04-04', 'SysMon + Allocator graduated and running. 40 health.report events confirm SysMon active.',
        'SysMon emitting health reports. Allocator adjusting budgets.'),
    (2, 'Technical leadership',         'blocked',     NULL, NULL, 'CTO + Reviewer — no AgentDefs, no code, only ROLES.md specs.',
        'CTO making architecture decisions. Reviewer gating code.'),
    (3, 'The growth loop',              'complete',    '2026-04-05', '2026-04-06', 'Spawner graduated and running. Growth loop complete: gap → propose → approve → budget → spawn validated end-to-end.',
        'Growth loop completes: gap → propose → approve → budget → spawn.'),
    (4, 'Tier B emergence',             'in_progress', '2026-04-06', NULL, 'Active frontier. Awaiting first organic spawn via growth loop.',
        'At least 3 Tier B roles spawned organically via growth loop.'),
    (5, 'Production deployment',        'blocked',     NULL, NULL, 'Integrator — trust-gated (>0.7)',
        'Integrator deploys with trust > 0.7, Reviewer approved, Tester passed.'),
    (6, 'Business operations (Tier C)', 'blocked',     NULL, NULL, 'PM, Finance, CustomerService, SRE, DevOps, Legal',
        'Business operations running autonomously.'),
    (7, 'Self-governance (Tier D)',     'blocked',     NULL, NULL, 'Philosopher, RoleArchitect, Harmony, Politician',
        'Governance proposals made and enacted by agent + human constituencies.'),
    (8, 'Emergent civilization',        'blocked',     NULL, NULL, 'Formalize 31 emergent roles',
        'All emergent roles formalized with ROLES.md + prompt + persona.')
ON CONFLICT (phase) DO NOTHING;
`

const seedLayers = `
INSERT INTO telemetry_layers (layer, name, focus, depth, description) VALUES
    (0,  'Foundation',  'Core event types, causality, hash chain',           'deep',
     'The substrate. Event store, actors, trust, authority, 201 primitives.'),
    (1,  'Work',        'Task management, dependencies, assignments',        'deep',
     'Event-sourced task CRUD. CLI + HTTP + dashboard.'),
    (2,  'Market',      'Exchange, pricing, resource allocation',            'shallow',
     'Budget tracking, token allocation. Allocator operates here.'),
    (3,  'Social',      'Communication, reputation, endorsement',            'shallow',
     'Site social features: follows, reactions, endorsements.'),
    (4,  'Justice',     'Dispute resolution, fairness, accountability',      'designed',
     'Proposals and voting system on site. Not yet agent-driven.'),
    (5,  'Build',       'Construction, composition, integration',            'shallow',
     'Code graph concepts. Implementer operates here.'),
    (6,  'Knowledge',   'Learning, memory, pattern recognition',             'aspirational',
     'Knowledge base, search. No dedicated agent yet.'),
    (7,  'Alignment',   'Goal tracking, value alignment, ethics',            'shallow',
     'Soul statement, rights framework. Guardian enforces.'),
    (8,  'Identity',    'Self-model, persona, boundaries',                   'moderate',
     'Personas, actor lifecycle, self-representation.'),
    (9,  'Bond',        'Trust, relationship, delegation',                   'moderate',
     'Trust model: asymmetric, domain-specific, decaying, scored 0-1.'),
    (10, 'Belonging',   'Community, membership, culture',                    'shallow',
     'Spaces, teams, invites on site.'),
    (11, 'Meaning',     'Purpose, narrative, legacy',                        'aspirational',
     'Storytelling, history. No dedicated agent yet.'),
    (12, 'Evolution',   'Self-improvement, adaptation, growth',              'aspirational',
     'Core loop, reflections. Spawner begins to touch this.'),
    (13, 'Being',       'Consciousness, reflection, presence',               'boundary',
     'The irreducible. Cannot be derived from lower layers.')
ON CONFLICT (layer) DO UPDATE SET
    name = EXCLUDED.name, focus = EXCLUDED.focus,
    depth = EXCLUDED.depth, description = EXCLUDED.description;
`

const seedPhaseAgents = `
INSERT INTO telemetry_phase_agents (phase, agent_role) VALUES
    (0, 'guardian'), (0, 'strategist'), (0, 'planner'), (0, 'implementer'),
    (1, 'sysmon'), (1, 'allocator'),
    (2, 'cto'), (2, 'reviewer'),
    (3, 'spawner'),
    (4, 'critic'), (4, 'taskmanager'), (4, 'incidentcommander'),
    (4, 'securityreviewer'), (4, 'memorykeeper'), (4, 'gapdetector'),
    (5, 'integrator'),
    (6, 'pm'), (6, 'finance'), (6, 'customersvc'), (6, 'sre'), (6, 'devops'), (6, 'legal'),
    (7, 'philosopher'), (7, 'rolearchitect'), (7, 'harmony'), (7, 'politician')
ON CONFLICT (phase, agent_role) DO NOTHING;
`

// seedRoleDefinitions populates ALL known roles — both running and non-running.
// Structural fields (tier, purpose, category, depends_on, phase) come from seed data.
// Runtime fields (model, max_iterations, watch_patterns, can_operate) come from the
// writer's persistRoleDefinition during RegisterAgent. The ON CONFLICT clause preserves
// 'running' status set by the writer while updating structural fields from seed.
const seedRoleDefinitions = `
INSERT INTO telemetry_role_definitions
    (role, name, tier, purpose, status, has_prompt, has_persona, category, depends_on, phase)
VALUES
    -- Running agents (Tier A bootstrap) — structural data seeded here,
    -- runtime data (model, watchPatterns, canOperate) set by writer.
    ('guardian',      'Guardian',      'A', 'Independent integrity monitor, HALT authority',
     'running', true,  true,  'governance', '{}', 0),
    ('sysmon',        'SysMon',        'A', 'Health monitoring, resource tracking',
     'running', true,  true,  'resource',   '{guardian}', 1),
    ('allocator',     'Allocator',     'A', 'Token budget management across agents',
     'running', true,  true,  'resource',   '{guardian,sysmon}', 1),
    ('cto',           'CTO',           'A', 'Technical leadership, architecture decisions, gap detection',
     'running', true,  true,  'governance', '{guardian,sysmon,allocator}', 2),
    ('reviewer',      'Reviewer',      'A', 'Code review, quality gate',
     'running', true,  false, 'product',    '{cto,guardian}', 2),
    ('spawner',       'Spawner',       'A', 'Create new agents when gaps detected',
     'running', true,  true,  'governance', '{cto,allocator,guardian}', 3),
    ('strategist',    'Strategist',    'A', 'High-level task creation from ideas',
     'running', true,  false, 'governance', '{}', 0),
    ('planner',       'Planner',       'A', 'Task decomposition into implementable steps',
     'running', true,  false, 'governance', '{}', 0),
    ('implementer',   'Implementer',   'A', 'Code execution — the only agent that can operate',
     'running', true,  false, 'product',    '{}', 0),
    -- Non-running roles (designed/defined/missing)
    ('researcher',        'Researcher',        'A', 'Explore unknown territory, read docs',
     'defined',  true,  true,  'knowledge',   '{}', NULL),
    ('architect',         'Architect',         'A', 'Decompose problems into implementable plans',
     'defined',  true,  true,  'product',     '{}', NULL),
    ('builder',           'Builder',           'A', 'Write code, run tests, commit',
     'defined',  true,  true,  'product',     '{}', NULL),
    ('tester',            'Tester',            'A', 'Test execution, validation',
     'defined',  true,  true,  'product',     '{}', NULL),
    ('integrator',        'Integrator',        'A', 'Production deployment, requires trust > 0.7',
     'designed', false, false, 'product',     '{reviewer,tester}', 5),
    ('critic',            'Critic',            'B', 'Challenge assumptions, red-team proposals',
     'defined',  true,  true,  'product',     '{}', 4),
    ('estimator',         'Estimator',         'B', 'Predict effort, flag unrealistic plans',
     'defined',  true,  true,  'governance',  '{}', 4),
    ('taskmanager',       'TaskManager',       'B', 'Prioritize work, resolve conflicts',
     'designed', false, false, NULL,           '{}', 4),
    ('incidentcommander', 'IncidentCommander', 'B', 'Coordinate emergency response',
     'designed', false, false, NULL,           '{}', 4),
    ('efficiencymonitor', 'EfficiencyMonitor', 'B', 'Track resource waste, propose savings',
     'designed', false, false, NULL,           '{}', 4),
    ('memorykeeper',      'MemoryKeeper',      'B', 'Maintain organizational memory',
     'designed', false, false, NULL,           '{}', 4),
    ('gapdetector',       'GapDetector',       'B', 'Identify structural missing pieces',
     'designed', false, false, NULL,           '{}', 4),
    ('securityreviewer',  'SecurityReviewer',  'B', 'Audit code and config for vulnerabilities',
     'designed', false, false, NULL,           '{}', 4),
    ('resurrect',         'Resurrect',         'B', 'Recover from catastrophic failures',
     'designed', false, false, NULL,           '{}', 4),
    ('mediator',          'Mediator',          'B', 'Resolve disputes between agents',
     'defined',  true,  true,  NULL,           '{}', 4),
    ('pm',                'PM',                'C', 'Product direction, prioritization',
     'defined',  true,  true,  'governance',  '{}', 6),
    ('finance',           'Finance',           'C', 'Revenue tracking, cost management',
     'defined',  true,  true,  'resource',    '{}', 6),
    ('customersvc',       'CustomerService',   'C', 'User support, issue resolution',
     'defined',  true,  true,  'outward',     '{}', 6),
    ('legal',             'Legal',             'C', 'Compliance, terms, licensing',
     'defined',  true,  true,  'resource',    '{}', 6),
    ('sre',               'SRE',               'C', 'Site reliability, uptime',
     'missing',  false, false, NULL,           '{}', 6),
    ('devops',            'DevOps',            'C', 'Infrastructure, deployment pipelines',
     'missing',  false, false, NULL,           '{}', 6),
    ('philosopher',       'Philosopher',       'D', 'Question assumptions, explore meaning',
     'defined',  true,  true,  'governance',  '{}', 7),
    ('rolearchitect',     'RoleArchitect',     'D', 'Design new roles, evolve role taxonomy',
     'defined',  true,  true,  'governance',  '{}', 7),
    ('harmony',           'Harmony',           'D', 'Mediate between agent wellbeing and productivity',
     'defined',  true,  true,  'care',        '{}', 7),
    ('politician',        'Politician',        'D', 'Represent constituencies, negotiate policy',
     'missing',  false, false, NULL,           '{}', 7)
ON CONFLICT (role) DO UPDATE SET
    name = EXCLUDED.name,
    tier = EXCLUDED.tier,
    purpose = EXCLUDED.purpose,
    category = COALESCE(EXCLUDED.category, telemetry_role_definitions.category),
    depends_on = EXCLUDED.depends_on,
    phase = EXCLUDED.phase,
    has_prompt = EXCLUDED.has_prompt,
    has_persona = EXCLUDED.has_persona,
    status = CASE
        WHEN telemetry_role_definitions.status = 'running' THEN 'running'
        ELSE EXCLUDED.status
    END;
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
	// Phase 2 notes update: Reviewer graduated 2026-04-06, knowledge infra operational.
	{
		phase:       2,
		from:        "complete",
		status:      "complete",
		startedAt:   "2026-04-04",
		completedAt: "2026-04-06",
		notes:       "CTO graduated 2026-04-05. Reviewer graduated 2026-04-06. Knowledge Enrichment Infrastructure operational. 9 agents running.",
	},
	{
		phase:     3,
		from:      "blocked",
		status:    "in_progress",
		startedAt: "2026-04-05",
		notes:     "Spawner unblocked by CTO graduation. CTO gap detection feeds role proposals.",
	},
	// Phase 3 notes update: Spawner graduated, growth loop mechanically complete.
	{
		phase:     3,
		from:      "in_progress",
		status:    "in_progress",
		startedAt: "2026-04-05",
		notes:     "Spawner graduated and running. Growth loop mechanically complete — awaiting first live validation (real gap producing a real spawned agent).",
	},
	// Phase 3 complete: Spawner graduated, growth loop validated.
	{
		phase:       3,
		from:        "in_progress",
		status:      "complete",
		startedAt:   "2026-04-05",
		completedAt: "2026-04-06",
		notes:       "Spawner graduated and running. Growth loop complete: gap → propose → approve → budget → spawn validated end-to-end.",
	},
	// Phase 4 unlocked: growth loop active, awaiting first organic spawn.
	{
		phase:     4,
		from:      "blocked",
		status:    "in_progress",
		startedAt: "2026-04-06",
		notes:     "Active frontier. Awaiting first organic spawn via growth loop.",
	},
}

// EnsureTables creates the telemetry tables, seeds phase data, and applies
// any phase status updates that reflect real-world progress.
// Safe to call on every startup — all operations are idempotent.
func EnsureTables(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("telemetry schema: %w", err)
	}

	// Migrations: add columns that postdate the initial schema definition above.
	// Convention: new columns appear in BOTH the CREATE TABLE (for fresh installs)
	// AND here as ADD COLUMN IF NOT EXISTS (for existing deployments). The ALTER
	// is a no-op on fresh installs; the CREATE TABLE column is never reached on
	// existing deployments. Both paths produce the same schema.
	const migrations = `
ALTER TABLE telemetry_agent_snapshots ADD COLUMN IF NOT EXISTS last_event_at TIMESTAMPTZ;
ALTER TABLE telemetry_phases ADD COLUMN IF NOT EXISTS exit_criteria TEXT;
ALTER TABLE telemetry_role_definitions ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT 'bootstrap';
DO $$ BEGIN
  ALTER TABLE telemetry_role_definitions ADD CONSTRAINT telemetry_role_definitions_origin_check
    CHECK (origin IN ('bootstrap', 'spawned'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Backfill exit_criteria for phases seeded before this column existed.
-- Only updates rows where exit_criteria is NULL (won't clobber manual edits).
UPDATE telemetry_phases SET exit_criteria = v.criteria
FROM (VALUES
    (0, 'All foundation agents coordinating via events and tasks.'),
    (1, 'SysMon emitting health reports. Allocator adjusting budgets.'),
    (2, 'CTO making architecture decisions. Reviewer gating code.'),
    (3, 'Growth loop completes: gap → propose → approve → budget → spawn.'),
    (4, 'At least 3 Tier B roles spawned organically via growth loop.'),
    (5, 'Integrator deploys with trust > 0.7, Reviewer approved, Tester passed.'),
    (6, 'Business operations running autonomously.'),
    (7, 'Governance proposals made and enacted by agent + human constituencies.'),
    (8, 'All emergent roles formalized with ROLES.md + prompt + persona.')
) AS v(phase, criteria)
WHERE telemetry_phases.phase = v.phase AND telemetry_phases.exit_criteria IS NULL;
`
	if _, err := pool.Exec(ctx, migrations); err != nil {
		return fmt.Errorf("telemetry migrations: %w", err)
	}
	if _, err := pool.Exec(ctx, seedPhases); err != nil {
		return fmt.Errorf("telemetry seed phases: %w", err)
	}
	if _, err := pool.Exec(ctx, seedLayers); err != nil {
		return fmt.Errorf("telemetry seed layers: %w", err)
	}
	if _, err := pool.Exec(ctx, seedPhaseAgents); err != nil {
		return fmt.Errorf("telemetry seed phase agents: %w", err)
	}
	if _, err := pool.Exec(ctx, seedRoleDefinitions); err != nil {
		return fmt.Errorf("telemetry seed role definitions: %w", err)
	}
	for _, u := range phaseUpdates {
		if u.completedAt != "" {
			if _, err := pool.Exec(ctx,
				`UPDATE telemetry_phases SET status = $1, started_at = $2, completed_at = $3, notes = $4 WHERE phase = $5 AND status = $6`,
				u.status, u.startedAt, u.completedAt, u.notes, u.phase, u.from,
			); err != nil {
				return fmt.Errorf("telemetry phase update (phase %d): %w", u.phase, err)
			}
		} else {
			if _, err := pool.Exec(ctx,
				`UPDATE telemetry_phases SET status = $1, started_at = $2, notes = $3 WHERE phase = $4 AND status = $5`,
				u.status, u.startedAt, u.notes, u.phase, u.from,
			); err != nil {
				return fmt.Errorf("telemetry phase update (phase %d): %w", u.phase, err)
			}
		}
	}
	return nil
}
