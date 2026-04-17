# Pipeline Visibility Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the pipeline's work visible — board task status updates, telemetry agent snapshots, and daemon git recovery.

**Architecture:** Three independent fixes in dependency order. (1) Force-checkout in daemon so stale branches don't jam the loop. (2) Point localapi at the site database so the pipeline reads/writes the same tasks the board shows. (3) Emit telemetry from pipeline phases so the dashboard shows agent activity.

**Tech Stack:** Go, PostgreSQL, git

---

## File Map

| File | Action | Responsibility |
|------|--------|---------------|
| `cmd/hive/main.go:909` | Modify | Force-checkout in daemonResetToMain |
| `cmd/hive/main.go:~50` | Modify | Add localapi DSN flag / env var |
| `cmd/localapi/main.go` | Modify | Accept `--db` flag or `LOCALAPI_DSN` for site database |
| `pkg/localapi/schema.go` | Modify | Configurable table name (site uses `nodes`, local uses `local_nodes`) |
| `pkg/localapi/store.go` | Modify | Use configurable table name in all SQL |
| `pkg/localapi/store_test.go` | Modify | Test with configurable table |
| `pkg/localapi/server_test.go` | Verify | Existing tests still pass |
| `pkg/runner/pipeline_state.go` | Modify | Emit telemetry in PostPhaseFunc |
| `cmd/hive/main.go:577` | Modify | Create telemetry.Writer in runPipeline |
| `pkg/telemetry/writer.go` | Modify | Add WritePipelineSnapshot for direct writes |

---

### Task 1: Force-checkout in daemon reset

**Files:**
- Modify: `cmd/hive/main.go:906-917`

This is the highest-priority fix. The daemon gets stuck on a stale feature branch when `loop/` artifacts (committed on the feature branch) conflict with main. Every subsequent cycle is a 3-second no-op.

- [ ] **Step 1: Write the failing test**

Create `cmd/hive/daemon_test.go`:

```go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	os.WriteFile(filepath.Join(dir, "README"), []byte("init"), 0644)
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestDaemonResetToMain_DirtyFiles(t *testing.T) {
	dir := initTestRepo(t)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.CombinedOutput()
	}

	// Create a feature branch with a tracked file, then modify it.
	run("checkout", "-b", "feat/test")
	os.WriteFile(filepath.Join(dir, "loop-status"), []byte("v1"), 0644)
	run("add", ".")
	run("commit", "-m", "add loop status")

	// Modify the tracked file (dirty working tree).
	os.WriteFile(filepath.Join(dir, "loop-status"), []byte("v2-dirty"), 0644)

	// daemonResetToMain should succeed despite dirty files.
	daemonResetToMain(dir)

	// Verify we're on main.
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = dir
	out, _ := branchCmd.Output()
	branch := string(out)
	if branch != "main\n" {
		t.Errorf("expected main, got %q", branch)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/hive/ -run TestDaemonResetToMain_DirtyFiles -v`
Expected: FAIL — `daemonResetToMain` is unexported. Export it or use the test in the same package.

Note: `daemonResetToMain` is already in `package main` and the test is in `package main`, so it should be callable. If it fails with the checkout error, that confirms the bug.

- [ ] **Step 3: Fix daemonResetToMain**

In `cmd/hive/main.go`, change line 909:

```go
// Before:
if err := gitCmd("checkout", "main"); err != nil {
    return
}

// After:
if err := gitCmd("checkout", "-f", "main"); err != nil {
    return
}
```

The `-f` flag discards local modifications. This is safe because `loop/` files are regenerated every cycle — there is nothing to preserve.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/hive/ -run TestDaemonResetToMain_DirtyFiles -v`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./cmd/hive/ -v`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add cmd/hive/main.go cmd/hive/daemon_test.go
git commit -m "fix: force-checkout in daemon reset to survive dirty loop/ artifacts"
```

---

### Task 2: Point localapi at site database

**Files:**
- Modify: `pkg/localapi/store.go` (configurable table name)
- Modify: `pkg/localapi/schema.go` (skip migration when using external table)
- Modify: `cmd/localapi/main.go` (accept `--site-db` flag)
- Modify: `pkg/localapi/store_test.go` (test with configurable table)

The localapi currently reads from `hive.local_nodes` while the board shows tasks from `site.nodes`. The pipeline reads through localapi, so it sees an empty board. Fix: let localapi connect to the `site` database and read the `nodes` table directly.

- [ ] **Step 1: Make Store accept a configurable table name**

Add a `TableName` field to `Store` in `pkg/localapi/store.go`:

```go
type Store struct {
	db        *sql.DB
	tableName string // "local_nodes" (default) or "nodes" (site DB)
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, tableName: "local_nodes"}
}

func NewSiteStore(db *sql.DB) *Store {
	return &Store{db: db, tableName: "nodes"}
}
```

- [ ] **Step 2: Replace hardcoded `local_nodes` with `s.tableName`**

In `pkg/localapi/store.go`, replace every occurrence of `local_nodes` in SQL strings with `"+s.tableName+"`. There are approximately 12 occurrences across these methods: `CreateNode`, `GetNode`, `ListNodes`, `UpdateNodeState`, `ClaimNode`, `ChildCounts`, `MaxLessonNumber`, `NodeExists`, `SearchNodesByTitle`, `Search`.

For `CreateNode`, the site's `nodes` table has NOT NULL columns with '' defaults where localapi uses NULL. Change `nilIfEmpty` calls to `emptyIfNil` for the site table:

```go
func (s *Store) CreateNode(spaceID string, n Node) (*Node, error) {
	// ... existing validation ...

	assignee := nilIfEmpty(n.Assignee)
	assigneeID := nilIfEmpty(n.AssigneeID)
	author := nilIfEmpty(n.Author)
	authorID := nilIfEmpty(n.AuthorID)
	authorKind := nilIfEmpty(n.AuthorKind)

	// Site table uses NOT NULL with '' defaults — send '' not NULL.
	if s.tableName == "nodes" {
		assignee = strPtr(n.Assignee)
		assigneeID = strPtr(n.AssigneeID)
		author = strPtr(n.Author)
		authorID = strPtr(n.AuthorID)
		authorKind = strPtr(n.AuthorKind)
	}
	// ... rest of INSERT ...
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 3: Handle the schema difference in scanNode**

The site's `nodes` table has `assignee`, `assignee_id`, `author`, `author_id`, `author_kind` as NOT NULL TEXT (never sql.NullString). But since the scan uses `sql.NullString` and NOT NULL text columns scan fine into NullString (they just always have Valid=true), no change is needed here.

- [ ] **Step 4: Skip migration when using site database**

In `pkg/localapi/schema.go`, the `Migrate` function creates `local_nodes`. When connecting to the site database, skip this (the `nodes` table already exists):

```go
func Migrate(db *sql.DB) error { ... } // unchanged

func MigrateIfLocal(db *sql.DB, tableName string) error {
	if tableName == "nodes" {
		// Site database — tables already exist. Only ensure local_agents and local_diagnostics.
		stmts := []string{
			`CREATE TABLE IF NOT EXISTS local_agents (...)`,
			`CREATE TABLE IF NOT EXISTS local_diagnostics (...)`,
		}
		for _, s := range stmts {
			if _, err := db.Exec(s); err != nil {
				return err
			}
		}
		return nil
	}
	return Migrate(db)
}
```

- [ ] **Step 5: Update cmd/localapi/main.go to accept site DB flag**

```go
func main() {
	addr := flag.String("addr", ":8082", "listen address")
	apiKey := flag.String("api-key", "dev", "API key for authentication")
	siteDB := flag.Bool("site-db", false, "connect to site database (nodes table) instead of local (local_nodes)")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		if *siteDB {
			dsn = "postgres://hive:hive@localhost:5432/site?sslmode=disable"
		} else {
			dsn = "postgres://hive:hive@localhost:5432/hive?sslmode=disable"
		}
	}

	db, err := sql.Open("pgx", dsn)
	// ... existing error handling ...

	var store *localapi.Store
	if *siteDB {
		if err := localapi.MigrateIfLocal(db, "nodes"); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		store = localapi.NewSiteStore(db)
	} else {
		if err := localapi.Migrate(db); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		store = localapi.NewStore(db)
	}

	handler := localapi.NewServer(store, *apiKey)
	// ... rest unchanged ...
}
```

- [ ] **Step 6: Write test for site store**

In `pkg/localapi/store_test.go`, add a test that verifies `NewSiteStore` uses the `nodes` table name:

```go
func TestNewSiteStore_TableName(t *testing.T) {
	s := NewSiteStore(nil)
	if s.tableName != "nodes" {
		t.Errorf("tableName = %q, want %q", s.tableName, "nodes")
	}
}

func TestNewStore_TableName(t *testing.T) {
	s := NewStore(nil)
	if s.tableName != "local_nodes" {
		t.Errorf("tableName = %q, want %q", s.tableName, "local_nodes")
	}
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./pkg/localapi/ -v`
Expected: all pass (existing tests use `NewStore` which still defaults to `local_nodes`)

- [ ] **Step 8: Run full test suite**

Run: `go test ./... -count=1`
Expected: all pass

- [ ] **Step 9: Commit**

```bash
git add pkg/localapi/store.go pkg/localapi/schema.go pkg/localapi/store_test.go cmd/localapi/main.go
git commit -m "feat: let localapi connect to site database with --site-db flag"
```

---

### Task 3: Emit telemetry from pipeline phases

**Files:**
- Modify: `pkg/telemetry/writer.go` (add `WritePipelineSnapshot` method)
- Modify: `cmd/hive/main.go:577-788` (create Writer in `runPipeline`, wire PostPhase)

The pipeline runs agents but never writes telemetry snapshots. The legacy runtime uses `telemetry.Writer` with a bus subscription, but the pipeline has no bus. Add a direct-write method for pipeline use.

- [ ] **Step 1: Add WritePipelineSnapshot to telemetry.Writer**

In `pkg/telemetry/writer.go`, add a method that inserts a snapshot directly without needing the bus or BudgetRegistry:

```go
// PipelineAgentSnapshot holds the data needed for a single pipeline phase snapshot.
type PipelineAgentSnapshot struct {
	Role       string
	Model      string
	State      string  // "idle", "processing", "done"
	Iteration  int
	CostUSD    float64
	DurationS  float64
}

// WritePipelineSnapshot inserts a telemetry_agent_snapshots row directly.
// Used by the pipeline runner which has no bus or BudgetRegistry.
func (w *Writer) WritePipelineSnapshot(snap PipelineAgentSnapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := w.pool.Exec(ctx, `
		INSERT INTO telemetry_agent_snapshots
			(agent_role, model, state, iteration, cost_usd, recorded_at)
		VALUES ($1, $2, $3, $4, $5, now())`,
		snap.Role, snap.Model, snap.State, snap.Iteration, snap.CostUSD,
	)
	if err != nil {
		log.Printf("[telemetry] pipeline snapshot write error: %v", err)
	}
}
```

Note: verify the exact column names in `telemetry_agent_snapshots` before implementing. Run:
```sql
SELECT column_name FROM information_schema.columns
WHERE table_name = 'telemetry_agent_snapshots' ORDER BY ordinal_position;
```

- [ ] **Step 2: Write test for WritePipelineSnapshot**

In `pkg/telemetry/writer_test.go` (create if needed), test that the method doesn't panic with a nil pool (graceful degradation):

```go
func TestWritePipelineSnapshot_NilPool(t *testing.T) {
	// WritePipelineSnapshot with nil pool should log error, not panic.
	w := &Writer{} // zero value, pool is nil
	// This should not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("WritePipelineSnapshot panicked: %v", r)
		}
	}()
	w.WritePipelineSnapshot(PipelineAgentSnapshot{
		Role: "test", State: "done",
	})
}
```

- [ ] **Step 3: Wire telemetry into runPipeline**

In `cmd/hive/main.go`, inside `runPipeline` (around line 577), create a `telemetry.Writer` if postgres is available:

```go
// After the APIClient is created but before the pipeline loop:
var tw *telemetry.Writer
if storeDSN != "" {
	cfg, err := pgxpool.ParseConfig(storeDSN)
	if err == nil {
		cfg.MaxConns = 3
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			tw = telemetry.NewWriter(pool, nil, nil)
		}
	}
}
```

Then wire it into the PostPhase callback:

```go
sm.SetPostPhase(func(role string, provider interface{}) {
	// ... existing session persistence logic ...

	// Emit telemetry snapshot.
	if tw != nil {
		tw.WritePipelineSnapshot(telemetry.PipelineAgentSnapshot{
			Role:      role,
			Model:     "claude-sonnet-4-6", // or read from runner config
			State:     "done",
			Iteration: cycle,
			CostUSD:   r.cost.TotalCostUSD,
		})
	}
})
```

- [ ] **Step 4: Register pipeline agents at startup**

Before the daemon loop, register the pipeline roles so the dashboard knows about them:

```go
if tw != nil {
	for _, role := range []string{"pm", "scout", "architect", "builder", "tester", "critic", "reflector", "observer"} {
		tw.RegisterAgent(telemetry.AgentRegistration{
			Name:  role,
			Role:  role,
			Model: "claude-sonnet-4-6",
		})
	}
	go tw.Start(ctx)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./pkg/telemetry/ -v`
Run: `go test ./cmd/hive/ -v`
Expected: all pass

- [ ] **Step 6: Run full test suite**

Run: `go test ./... -count=1`
Expected: all pass

- [ ] **Step 7: Commit**

```bash
git add pkg/telemetry/writer.go cmd/hive/main.go
git commit -m "feat: emit telemetry snapshots from pipeline phases"
```

---

### Task 4: Integration test — restart pipeline with fixes

**Files:** None (operational verification)

- [ ] **Step 1: Restart localapi with --site-db**

```bash
pkill -9 -f "cmd/localapi" 2>/dev/null
sleep 2
cd /home/transpara/transpara-ai/repos/lovyou-ai-hive
nohup go run ./cmd/localapi/ --site-db > /tmp/localapi.log 2>&1 &
sleep 4
curl -s http://localhost:8082/health
```

- [ ] **Step 2: Verify localapi sees site tasks**

```bash
curl -s -H "Authorization: Bearer dev" http://localhost:8082/app/demo/board | jq '.nodes[] | select(.kind == "task") | {title, state}'
```

Expected: returns the same tasks visible on the board UI (Mutex test task, etc.)

- [ ] **Step 3: Restart pipeline pointing at localapi**

```bash
pkill -9 -f "cmd/hive.*pipeline" 2>/dev/null
sleep 2
cd /home/transpara/transpara-ai/repos/lovyou-ai-hive
nohup go run ./cmd/hive \
    --pipeline \
    --api http://localhost:8082 \
    --space demo \
    --store postgres://hive:hive@localhost:5432/hive \
    --human Michael \
    --pr \
    --budget 20 \
    > /tmp/hive-demo.log 2>&1 &
sleep 10
tail -20 /tmp/hive-demo.log
```

Expected: pipeline detects work (`board has work — starting at building`) and the builder claims a task.

- [ ] **Step 4: Verify board updates**

After the builder completes a task, check the board:

```bash
curl -s -H "Authorization: Bearer dev" http://localhost:8082/app/demo/board | jq '.nodes[] | select(.state == "done" or .state == "active") | {title, state}'
```

Expected: task states reflect pipeline activity.

- [ ] **Step 5: Verify telemetry**

```bash
docker exec lovyou-ai-hive-postgres-1 psql -U hive -d hive -c \
    "SELECT agent_role, state, recorded_at FROM telemetry_agent_snapshots ORDER BY recorded_at DESC LIMIT 5;"
```

Expected: recent snapshots from pipeline phases (builder, critic, etc.)

- [ ] **Step 6: Verify daemon git recovery**

Wait for a cycle to complete, then check the log:

```bash
grep "branch after reset" /tmp/hive-demo.log | tail -3
```

Expected: `branch after reset: main (was: feat/...)` — no more `git checkout failed` errors.
