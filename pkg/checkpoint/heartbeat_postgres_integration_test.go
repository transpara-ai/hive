//go:build integration

package checkpoint

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// TestHeartbeatEmitter_PostgresRestart verifies that heartbeat events emitted
// in one session are fully recoverable after a simulated process restart
// (pool close + reconnect).
//
// This exercises the persistence path: EventFactory → pgstore.Append → close →
// reopen → QueryLatestHeartbeat. It proves the hive can recover agent iteration
// state from the event chain after an unclean shutdown.
//
// Requires TEST_POSTGRES_DSN pointing at a disposable Postgres database. The
// test TRUNCATEs at start and end — never point at a production database.
func TestHeartbeatEmitter_PostgresRestart(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping. Point it at a disposable test database (e.g. docker compose up -d postgres).")
	}

	ctx := context.Background()

	// Wipe the test DB before and after so fixtures from prior runs never
	// accumulate in the append-only event chain.
	truncate := func(label string) {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatalf("%s: pool: %v", label, err)
		}
		defer pool.Close()
		s, err := pgstore.NewPostgresStoreFromPool(ctx, pool)
		if err != nil {
			t.Fatalf("%s: store: %v", label, err)
		}
		defer s.Close()
		if err := s.Truncate(ctx); err != nil {
			t.Fatalf("%s: truncate: %v", label, err)
		}
	}
	truncate("pre-test")
	t.Cleanup(func() { truncate("post-test") })

	// Register HeartbeatContent as the content unmarshaler so pgstore
	// deserializes heartbeat events into the proper type (not passthrough).
	// This must happen before any store reads that use QueryLatestHeartbeat.
	event.RegisterContentUnmarshaler("hive.agent.heartbeat", event.Unmarshal[HeartbeatContent])

	humanID := types.MustActorID("actor_00000000000000000000000000000099")
	signer := &testSigner{humanID: humanID}
	convID := types.MustConversationID("test-heartbeat-restart")

	// ── Session 1: bootstrap graph and emit 3 heartbeats ─────────────────

	t.Run("session1_emit", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatalf("pool: %v", err)
		}
		defer pool.Close()

		s, err := pgstore.NewPostgresStoreFromPool(ctx, pool)
		if err != nil {
			t.Fatalf("store: %v", err)
		}
		defer s.Close()

		bootstrapTestGraph(t, s, humanID, signer)

		// Register the heartbeat event type with the factory's registry
		// so EventFactory.Create accepts HeartbeatContent.
		registry := event.DefaultRegistry()
		registry.Register(EventTypeAgentHeartbeat, nil)
		factory := event.NewEventFactory(registry)

		head, err := s.Head()
		if err != nil {
			t.Fatalf("head: %v", err)
		}
		if !head.IsSome() {
			t.Fatal("no head after bootstrap")
		}
		prevID := head.Unwrap().ID()

		// Emit 3 heartbeats with increasing iteration numbers.
		for i := 1; i <= 3; i++ {
			hb := HeartbeatContent{
				Role:          "implementer",
				Iteration:     i,
				MaxIterations: 20,
				TokensUsed:    i * 500,
				CostUSD:       float64(i) * 0.10,
				Signal:        "ACTIVE",
			}

			ev, err := factory.Create(
				EventTypeAgentHeartbeat,
				humanID,
				hb,
				[]types.EventID{prevID},
				convID,
				s, // HeadProvider
				signer,
			)
			if err != nil {
				t.Fatalf("create heartbeat %d: %v", i, err)
			}
			if _, err := s.Append(ev); err != nil {
				t.Fatalf("append heartbeat %d: %v", i, err)
			}
			prevID = ev.ID()
		}

		t.Log("emitted 3 heartbeat events for role 'implementer'")

		// pool.Close() via defer simulates process death — all committed
		// transactions are already durable in Postgres.
	})

	// ── Session 2: reconnect and verify recovery ─────────────────────────

	t.Run("session2_query", func(t *testing.T) {
		pool, err := pgxpool.New(ctx, dsn)
		if err != nil {
			t.Fatalf("pool: %v", err)
		}
		defer pool.Close()

		s, err := pgstore.NewPostgresStoreFromPool(ctx, pool)
		if err != nil {
			t.Fatalf("store: %v", err)
		}
		defer s.Close()

		// Query the latest heartbeat for 'implementer' with no time cutoff.
		hb, err := QueryLatestHeartbeat(s, "implementer", time.Time{})
		if err != nil {
			t.Fatalf("QueryLatestHeartbeat: %v", err)
		}
		if hb == nil {
			t.Fatal("no heartbeat found after restart — persistence FAILED")
		}

		// Assert the latest heartbeat is iteration 3 (the last one emitted).
		if hb.Iteration != 3 {
			t.Errorf("Iteration = %d, want 3", hb.Iteration)
		}
		if hb.Role != "implementer" {
			t.Errorf("Role = %q, want %q", hb.Role, "implementer")
		}

		t.Logf("PASS: heartbeat recovered after restart: iteration=%d, role=%s, tokens=%d, cost=%.2f",
			hb.Iteration, hb.Role, hb.TokensUsed, hb.CostUSD)
	})
}
