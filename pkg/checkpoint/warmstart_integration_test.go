package checkpoint

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lovyou-ai/eventgraph/go/pkg/actor/pgactor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/work"
)

// TestWarmStartIntegration_TaskRecovery verifies that tasks and events created
// in one "session" survive a simulated SIGKILL (pool close) and are fully
// recoverable when a new session connects to the same Postgres database.
//
// This is the hive's fundamental persistence guarantee: if the process dies,
// all committed events are durable and the causal chain is intact.
//
// Requires: docker compose up -d postgres (DSN: postgres://hive:hive@localhost:5432/hive)
// Skip: automatically skipped when HIVE_TEST_DSN is unset.
func TestWarmStartIntegration_TaskRecovery(t *testing.T) {
	dsn := os.Getenv("HIVE_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://hive:hive@localhost:5432/hive"
		// Try to connect; skip if unavailable.
		pool, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			t.Skipf("Postgres unavailable (set HIVE_TEST_DSN): %v", err)
		}
		if err := pool.Ping(context.Background()); err != nil {
			pool.Close()
			t.Skipf("Postgres unavailable (set HIVE_TEST_DSN): %v", err)
		}
		pool.Close()
	}

	ctx := context.Background()

	// Register content unmarshalers so Postgres can deserialize all event types.
	// We register work types (needed for task ops) and hive types as passthrough
	// (avoids import cycle: hive → checkpoint → hive).
	work.RegisterEventTypes()
	registerHiveEventTypesForTest()

	humanID := types.MustActorID("actor_00000000000000000000000000000099")
	signer := &testSigner{humanID: humanID}

	// Use a unique conversation ID so concurrent test runs don't interfere.
	convID := types.MustConversationID("test-warmstart")

	// ── Session 1: create events, seed tasks ──────────────────────────────

	var (
		taskID    types.EventID
		assignEvt types.EventID
		headHash  types.Hash
	)

	t.Run("session1_seed", func(t *testing.T) {
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

		actors, err := pgactor.NewPostgresActorStoreFromPool(ctx, pool)
		if err != nil {
			t.Fatalf("actor store: %v", err)
		}

		// Register human actor. Derive a deterministic key from the actor ID
		// (same seed as testSigner) so the store can link them.
		seed := sha256.Sum256([]byte(fmt.Sprintf("signer:%s", humanID.Value())))
		priv := ed25519.NewKeyFromSeed(seed[:])
		pub, _ := types.NewPublicKey(priv.Public().(ed25519.PublicKey))
		_, _ = actors.Register(pub, "test-human", event.ActorTypeHuman)

		// Bootstrap graph if needed (idempotent).
		bootstrapTestGraph(t, s, humanID, signer)

		registry := event.DefaultRegistry()
		work.RegisterWithRegistry(registry)
		factory := event.NewEventFactory(registry)

		tasks := work.NewTaskStore(s, factory, signer)

		// Get head for causal chain.
		head, err := s.Head()
		if err != nil {
			t.Fatalf("head: %v", err)
		}
		if !head.IsSome() {
			t.Fatal("no head after bootstrap")
		}
		genesisID := head.Unwrap().ID()

		// Create a task.
		task, err := tasks.Create(
			humanID,
			"warm-start integration test task",
			"verify persistence across process restart",
			[]types.EventID{genesisID},
			convID,
			work.PriorityHigh,
		)
		if err != nil {
			t.Fatalf("create task: %v", err)
		}
		taskID = task.ID
		t.Logf("created task: %s", taskID)

		// Assign the task.
		err = tasks.Assign(
			humanID,
			taskID,
			humanID, // self-assign
			[]types.EventID{taskID},
			convID,
		)
		if err != nil {
			t.Fatalf("assign task: %v", err)
		}

		// Query the assign event for later verification.
		page, err := s.ByType(work.EventTypeTaskAssigned, 10, types.None[types.Cursor]())
		if err != nil {
			t.Fatalf("query assigned: %v", err)
		}
		items := page.Items()
		// Find our assignment (there may be others from previous runs).
		found := false
		for _, ev := range items {
			c, ok := ev.Content().(work.TaskAssignedContent)
			if ok && c.TaskID == taskID {
				assignEvt = ev.ID()
				found = true
				break
			}
		}
		if !found {
			t.Fatal("assigned event not found in store")
		}

		// Record the head hash for chain integrity check.
		head2, err := s.Head()
		if err != nil {
			t.Fatalf("head after assign: %v", err)
		}
		headHash = head2.Unwrap().Hash()
		t.Logf("session 1 head hash: %s", headHash)

		// ── SIMULATE SIGKILL: close pool abruptly ────────────────────────
		// pool.Close() is called by defer — equivalent to process death.
		// All committed transactions are already durable in Postgres.
	})

	// ── Session 2: reconnect, verify recovery ─────────────────────────────

	t.Run("session2_recover", func(t *testing.T) {
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

		registry := event.DefaultRegistry()
		work.RegisterWithRegistry(registry)
		factory := event.NewEventFactory(registry)

		tasks := work.NewTaskStore(s, factory, signer)

		// ── Check 1: pending tasks still present ──────────────────────────

		allTasks, err := tasks.List(100)
		if err != nil {
			t.Fatalf("list tasks: %v", err)
		}

		found := false
		for _, task := range allTasks {
			if task.ID == taskID {
				found = true
				if task.Title != "warm-start integration test task" {
					t.Errorf("title = %q, want %q", task.Title, "warm-start integration test task")
				}
				if task.Priority != work.PriorityHigh {
					t.Errorf("priority = %q, want %q", task.Priority, work.PriorityHigh)
				}
				break
			}
		}
		if !found {
			t.Errorf("task %s not found after restart — task recovery FAILED", taskID)
		} else {
			t.Log("PASS: task.created event survived restart")
		}

		// ── Check 2: assigned tasks still assigned ────────────────────────

		page, err := s.ByType(work.EventTypeTaskAssigned, 100, types.None[types.Cursor]())
		if err != nil {
			t.Fatalf("query assigned: %v", err)
		}

		assignFound := false
		for _, ev := range page.Items() {
			c, ok := ev.Content().(work.TaskAssignedContent)
			if ok && c.TaskID == taskID {
				assignFound = true
				if c.AssignedTo != humanID {
					t.Errorf("assignee = %s, want %s", c.AssignedTo, humanID)
				}
				if ev.ID() != assignEvt {
					t.Errorf("assign event ID = %s, want %s", ev.ID(), assignEvt)
				}
				break
			}
		}
		if !assignFound {
			t.Errorf("assign event for task %s not found after restart — assignment recovery FAILED", taskID)
		} else {
			t.Log("PASS: task.assigned event survived restart")
		}

		// ── Check 3: event chain integrity (hash chain intact) ────────────

		head, err := s.Head()
		if err != nil {
			t.Fatalf("head: %v", err)
		}
		if !head.IsSome() {
			t.Fatal("no head after restart — event chain LOST")
		}

		recoveredHash := head.Unwrap().Hash()
		if recoveredHash != headHash {
			t.Errorf("head hash mismatch: got %s, want %s — chain integrity FAILED", recoveredHash, headHash)
		} else {
			t.Log("PASS: event chain hash intact after restart")
		}

		// ── Check 4: chain replay functions work ──────────────────────────

		// ReplayReviewerFromStore should not error on a store with work events.
		reviewerState, err := ReplayReviewerFromStore(s)
		if err != nil {
			t.Errorf("ReplayReviewerFromStore: %v", err)
		} else {
			t.Logf("PASS: ReplayReviewerFromStore succeeded (completed tasks: %d)",
				len(reviewerState.CompletedTasks))
		}

		// ReplayIterationFromStore should also work.
		iterations, err := ReplayIterationFromStore(s)
		if err != nil {
			t.Errorf("ReplayIterationFromStore: %v", err)
		} else {
			t.Logf("PASS: ReplayIterationFromStore succeeded (agents: %d)", len(iterations))
		}

		// ReplayBudgetFromStore should also work.
		budgets, err := ReplayBudgetFromStore(s)
		if err != nil {
			t.Errorf("ReplayBudgetFromStore: %v", err)
		} else {
			t.Logf("PASS: ReplayBudgetFromStore succeeded (agents: %d)", len(budgets))
		}

		// ── Cleanup: mark test task as done so it doesn't pollute the board ──
		cleanupCauses := []types.EventID{taskID}
		if err := tasks.WaiveArtifact(humanID, taskID, "test cleanup — no deliverable", cleanupCauses, convID); err != nil {
			t.Logf("cleanup: waive artifact: %v (may already be waived)", err)
		}
		if err := tasks.Complete(humanID, taskID, "test cleanup", cleanupCauses, convID); err != nil {
			t.Logf("cleanup: complete task: %v (may already be completed)", err)
		} else {
			t.Logf("cleanup: marked task %s as completed", taskID)
		}
	})
}

// ── Test helpers ──────────────────────────────────────────────────────────

// bootstrapTestGraph creates the genesis event if the store is empty.
func bootstrapTestGraph(t *testing.T, s store.Store, humanID types.ActorID, signer event.Signer) {
	t.Helper()

	head, err := s.Head()
	if err != nil {
		t.Fatalf("head check: %v", err)
	}
	if head.IsSome() {
		return // already bootstrapped
	}

	registry := event.DefaultRegistry()
	bsFactory := event.NewBootstrapFactory(registry)
	genesis, err := bsFactory.Init(humanID, signer)
	if err != nil {
		t.Fatalf("create genesis: %v", err)
	}
	if _, err := s.Append(genesis); err != nil {
		t.Fatalf("append genesis: %v", err)
	}
}

// testSigner is a deterministic signer for test reproducibility.
// Mirrors the bootstrapSigner in cmd/hive/main.go.
type testSigner struct {
	humanID types.ActorID
}

func (s *testSigner) Sign(data []byte) (types.Signature, error) {
	h := sha256.Sum256([]byte(fmt.Sprintf("signer:%s", s.humanID.Value())))
	priv := ed25519.NewKeyFromSeed(h[:])
	sig := ed25519.Sign(priv, data)
	return types.NewSignature(sig)
}

// registerHiveEventTypesForTest registers hive event types as passthrough
// so the Postgres store can deserialize them without importing pkg/hive
// (which would cause an import cycle: hive → checkpoint → hive).
func registerHiveEventTypesForTest() {
	hiveTypes := []string{
		"hive.run.started",
		"hive.run.completed",
		"hive.agent.spawned",
		"hive.agent.stopped",
		"hive.progress",
		"hive.agent.heartbeat",
		"hive.role.proposed",
		"hive.role.approved",
		"hive.site.respond",
		"hive.site.express",
		"hive.site.assert",
		"hive.site.progress",
		"membrane.sdr.started",
		"membrane.sdr.completed",
		"membrane.sdr.failed",
	}
	for _, et := range hiveTypes {
		event.RegisterContentUnmarshaler(et, event.Unmarshal[passthroughContent])
	}
}

// passthroughContent is a generic content type that accepts any JSON.
// Used in tests to avoid import cycles when deserializing event types
// from packages that import this one.
type passthroughContent struct {
	EventType string `json:"EventType"`
}

func (c passthroughContent) EventTypeName() string        { return c.EventType }
func (c passthroughContent) Validate() error               { return nil }
func (c passthroughContent) Accept(event.EventContentVisitor) {}
