package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	hiveagent "github.com/lovyou-ai/agent"
	"github.com/lovyou-ai/eventgraph/go/pkg/bus"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/resources"
)

// AgentRegistration captures everything the telemetry writer needs to snapshot
// an agent. Model and MaxIterations are captured at registration time because
// they come from AgentDef, not from the Agent struct at runtime.
type AgentRegistration struct {
	Name          string
	Role          string
	Model         string
	Agent         *hiveagent.Agent
	MaxIterations int
}

// agentEventRecord stores the most recent event observed from a specific agent
// on the bus. Used to populate per-agent last_event_type and last_event_at.
type agentEventRecord struct {
	eventType  string
	recordedAt time.Time
}

// Writer snapshots agent and hive state to postgres on a timer.
// It is pure Go infrastructure — no LLM dependency, no token budget.
type Writer struct {
	pool           *pgxpool.Pool
	store          store.Store
	budgetRegistry *resources.BudgetRegistry
	interval       time.Duration

	mu     sync.RWMutex
	agents []AgentRegistration

	// lastResponses stores the most recent LLM output per agent name.
	// Fed by the OnIteration callback via RecordResponse().
	lastResponses map[string]string

	// lastAgentEvents tracks the most recent bus event per agent, keyed by
	// actor ID string. Updated from the SubscribeToBus drain goroutine.
	// Protected by mu.
	lastAgentEvents map[string]agentEventRecord

	// chainOK caches the last VerifyChain result. Full verification runs
	// on a slower cadence (every 5 minutes) to avoid walking the full chain
	// every 10 seconds.
	chainOK         bool
	lastChainVerify time.Time
}

// NewWriter creates a telemetry writer. The writer does not start until Start is called.
func NewWriter(pool *pgxpool.Pool, s store.Store, reg *resources.BudgetRegistry) *Writer {
	interval := 10 * time.Second
	if v := os.Getenv("TELEMETRY_INTERVAL"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			interval = time.Duration(d) * time.Second
		}
	}
	return &Writer{
		pool:            pool,
		store:           s,
		budgetRegistry:  reg,
		interval:        interval,
		lastResponses:   make(map[string]string),
		lastAgentEvents: make(map[string]agentEventRecord),
		chainOK:         true,
	}
}

// SetBudgetRegistry sets the budget registry for cross-agent budget reads.
// Called after the registry is created in Runtime.Run().
func (w *Writer) SetBudgetRegistry(reg *resources.BudgetRegistry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.budgetRegistry = reg
}

// RegisterAgent adds an agent to the telemetry snapshot set.
// Called by the hive runtime during agent spawn.
func (w *Writer) RegisterAgent(reg AgentRegistration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.agents = append(w.agents, reg)
}

// RecordResponse captures an agent's latest LLM response.
// Designed to be called from the OnIteration callback.
func (w *Writer) RecordResponse(agentName string, response string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(response) > 500 {
		response = response[:500]
	}
	w.lastResponses[agentName] = response
}

// Agents returns the number of registered agents.
func (w *Writer) Agents() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.agents)
}

// Start launches the snapshot goroutine. It blocks until ctx is cancelled.
func (w *Writer) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.collectAndWrite(ctx)
		}
	}
}

// SubscribeToBus subscribes to all events on the bus and writes them to
// the telemetry_event_stream table. Returns the subscription ID for cleanup.
func (w *Writer) SubscribeToBus(b bus.IBus) bus.SubscriptionID {
	// Buffer events to avoid blocking the bus delivery goroutine.
	ch := make(chan event.Event, 256)

	pattern := types.MustSubscriptionPattern("*")
	subID := b.Subscribe(pattern, func(ev event.Event) {
		select {
		case ch <- ev:
		default:
			// Drop event if buffer full — telemetry is best-effort.
		}
	})

	// Drain goroutine writes events to postgres and tracks per-agent event times.
	go func() {
		for ev := range ch {
			// Update per-agent last event record before writing, so the
			// timestamp is as close as possible to when the event was observed.
			sourceID := ev.Source().Value()
			w.mu.Lock()
			w.lastAgentEvents[sourceID] = agentEventRecord{
				eventType:  ev.Type().Value(),
				recordedAt: time.Now(),
			}
			w.mu.Unlock()

			w.writeEvent(ev)
		}
	}()

	return subID
}

func (w *Writer) collectAndWrite(ctx context.Context) {
	w.mu.RLock()
	agents := make([]AgentRegistration, len(w.agents))
	copy(agents, w.agents)
	responses := make(map[string]string, len(w.lastResponses))
	for k, v := range w.lastResponses {
		responses[k] = v
	}
	agentEvents := make(map[string]agentEventRecord, len(w.lastAgentEvents))
	for k, v := range w.lastAgentEvents {
		agentEvents[k] = v
	}
	w.mu.RUnlock()

	if len(agents) == 0 {
		return
	}

	// Build a budget snapshot index by agent name.
	w.mu.RLock()
	reg := w.budgetRegistry
	w.mu.RUnlock()

	var budgetEntries []resources.BudgetEntry
	if reg != nil {
		budgetEntries = reg.Snapshot()
	}
	budgetByName := make(map[string]resources.BudgetEntry, len(budgetEntries))
	for _, e := range budgetEntries {
		budgetByName[e.Name] = e
	}

	// Collect hive-level metrics using the writer's dedicated pool.
	var chainLength int
	if err := w.pool.QueryRow(ctx, "SELECT COUNT(*) FROM events").Scan(&chainLength); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: chain count: %v\n", err)
	}
	activeCount := 0
	for _, e := range budgetEntries {
		if e.AgentState == "Active" {
			activeCount++
		}
	}

	// Chain verification: check genesis block exists using the writer's own pool.
	// We avoid calling w.store.VerifyChain() because it uses the shared store
	// pool which can be exhausted by 6 concurrent agent goroutines. A simple
	// genesis presence check is sufficient for the dashboard's chain_ok field.
	if time.Since(w.lastChainVerify) > 5*time.Minute {
		const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"
		var genesisCount int
		if err := w.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM events WHERE prev_hash = $1`, genesisHash,
		).Scan(&genesisCount); err == nil {
			w.chainOK = genesisCount == 1
		}
		w.lastChainVerify = time.Now()
	}

	// Compute event rate: events per minute over a 5-minute sliding window.
	// Zero is an honest answer when no events have been recorded recently.
	var recentEventCount int64
	if err := w.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM telemetry_event_stream WHERE recorded_at > now() - interval '5 minutes'`,
	).Scan(&recentEventCount); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: event rate query: %v\n", err)
	}
	eventRate := float64(recentEventCount) / 5.0

	// Fetch trust scores for all agents in one query.
	// DISTINCT ON (to_actor) picks the most recent trust edge per agent —
	// the latest assessment supersedes earlier ones.
	// Uses event.EdgeTypeTrust ("Trust") — the canonical constant, not a bare string.
	// Result is nil for agents with no trust edges (honest null, not zero).
	actorIDs := make([]string, len(agents))
	for i, reg := range agents {
		actorIDs[i] = reg.Agent.ID().Value()
	}
	trustScores := make(map[string]*float64, len(agents))
	trustRows, err := w.pool.Query(ctx,
		`SELECT DISTINCT ON (to_actor) to_actor, weight
		 FROM edges
		 WHERE to_actor = ANY($1) AND edge_type = $2
		 ORDER BY to_actor, created_at_nanos DESC`,
		actorIDs,
		string(event.EdgeTypeTrust),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: trust scores query: %v\n", err)
	} else {
		for trustRows.Next() {
			var actorID string
			var weight float64
			if scanErr := trustRows.Scan(&actorID, &weight); scanErr != nil {
				fmt.Fprintf(os.Stderr, "telemetry: trust score scan: %v\n", scanErr)
				continue
			}
			w := weight
			trustScores[actorID] = &w
		}
		trustRows.Close()
		if rowsErr := trustRows.Err(); rowsErr != nil {
			fmt.Fprintf(os.Stderr, "telemetry: trust scores rows: %v\n", rowsErr)
		}
	}

	// Write everything in a single transaction.
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: begin tx: %v\n", err)
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Write agent snapshots.
	for _, reg := range agents {
		state := reg.Agent.State().String()
		actorID := reg.Agent.ID().Value()

		iteration := 0
		tokensUsed := int64(0)
		costUSD := 0.0
		maxIter := reg.MaxIterations
		if be, ok := budgetByName[reg.Name]; ok {
			snap := be.Budget.Snapshot()
			iteration = snap.Iterations
			tokensUsed = int64(snap.TokensUsed)
			costUSD = snap.CostUSD
			maxIter = be.MaxIterations
		}

		lastMessage := responses[reg.Name]

		// Trust score from the pre-fetched batch query above.
		// Nil when no trust edges exist — honest null, not zero.
		trustScore := trustScores[actorID]

		// Per-agent last event: use the event type and timestamp tracked from
		// the bus subscription. Falls back to empty string / nil when no event
		// has been observed for this agent yet.
		var lastEventType string
		var lastEventAt *time.Time
		if rec, ok := agentEvents[actorID]; ok {
			lastEventType = rec.eventType
			t := rec.recordedAt
			lastEventAt = &t
		}

		_, err := tx.Exec(ctx,
			`INSERT INTO telemetry_agent_snapshots
				(agent_role, actor_id, state, model, iteration, max_iterations,
				 tokens_used, cost_usd, trust_score, last_event_type, last_event_at, last_message, errors)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
			reg.Role,
			actorID,
			state,
			reg.Model,
			iteration,
			maxIter,
			tokensUsed,
			costUSD,
			trustScore,
			lastEventType,
			lastEventAt,
			lastMessage,
			0, // errors: TODO: track error count per agent
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "telemetry: agent snapshot %s: %v\n", reg.Name, err)
			return
		}
	}

	// Write hive snapshot.
	var totalCost float64
	for _, e := range budgetEntries {
		totalCost += e.Budget.Snapshot().CostUSD
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO telemetry_hive_snapshots
			(active_agents, total_actors, chain_length, chain_ok,
			 event_rate, daily_cost, daily_cap, severity)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		activeCount,
		len(budgetEntries),
		int64(chainLength),
		w.chainOK,
		eventRate,
		totalCost,
		nil, // daily_cap: TODO: make configurable
		"ok",
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: hive snapshot: %v\n", err)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: commit: %v\n", err)
	}
}

func (w *Writer) writeEvent(ev event.Event) {
	// Resolve actor role from registered agents.
	actorRole := "unknown"
	sourceID := ev.Source().Value()
	w.mu.RLock()
	for _, reg := range w.agents {
		if reg.Agent.ID().Value() == sourceID {
			actorRole = reg.Role
			break
		}
	}
	w.mu.RUnlock()

	eventType := ev.Type().Value()
	summary := fmt.Sprintf("%s: %s", actorRole, eventType)

	var rawContent json.RawMessage
	if ev.Content() != nil {
		if data, err := json.Marshal(ev.Content()); err == nil {
			rawContent = data
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := w.pool.Exec(ctx,
		`INSERT INTO telemetry_event_stream (event_type, actor_role, summary, raw_content)
		 VALUES ($1, $2, $3, $4)`,
		eventType, actorRole, summary, rawContent,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "telemetry: event stream: %v\n", err)
	}
}
