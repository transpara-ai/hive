package localapi

import (
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx database/sql driver
)

// testDB opens a connection to the test Postgres, runs Migrate, and truncates
// all local tables so each test starts clean.
//
// Skipped in -short mode (used by CI without a Postgres service) and when
// the DB is unreachable, matching the convention in pkg/telemetry. To run
// these tests locally start the docker-compose stack
// (`docker compose up -d postgres`) and rerun without -short.
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping localapi integration test in -short mode (no Postgres in CI)")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hive:hive@localhost:5432/hive?sslmode=disable"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("cannot open db (%s): %v", dsn, err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("cannot reach Postgres at %s: %v", dsn, err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Clean slate for each test.
	for _, tbl := range []string{"local_nodes", "local_agents", "local_diagnostics"} {
		if _, err := db.Exec("DELETE FROM " + tbl); err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}

	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndGetNode(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	created, err := store.CreateNode("hive", Node{
		Title: "test task",
		Body:  "do the thing",
	})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated ID, got empty string")
	}
	if created.CreatedAt == "" {
		t.Fatal("expected created_at timestamp")
	}

	got, err := store.GetNode(created.ID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Title != "test task" {
		t.Fatalf("title mismatch: got %q, want %q", got.Title, "test task")
	}
	if got.State != "open" {
		t.Fatalf("state mismatch: got %q, want %q", got.State, "open")
	}
}

func TestListNodes_FilterByKindAndState(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	// Create a mix of nodes.
	mustCreate := func(n Node) {
		t.Helper()
		if _, err := store.CreateNode("hive", n); err != nil {
			t.Fatalf("CreateNode: %v", err)
		}
	}

	mustCreate(Node{Title: "task open", Kind: "task", State: "open"})
	mustCreate(Node{Title: "task done", Kind: "task", State: "done"})
	mustCreate(Node{Title: "doc open", Kind: "document", State: "open"})

	// Filter by kind=task — should get 2.
	tasks, err := store.ListNodes("hive", "task", "")
	if err != nil {
		t.Fatalf("ListNodes kind=task: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}

	// Filter by state=open — should get 2 (task open + doc open).
	open, err := store.ListNodes("hive", "", "open")
	if err != nil {
		t.Fatalf("ListNodes state=open: %v", err)
	}
	if len(open) != 2 {
		t.Fatalf("expected 2 open nodes, got %d", len(open))
	}

	// Filter by kind=task AND state=done — should get 1.
	done, err := store.ListNodes("hive", "task", "done")
	if err != nil {
		t.Fatalf("ListNodes kind=task state=done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("expected 1 done task, got %d", len(done))
	}
	if done[0].Title != "task done" {
		t.Fatalf("expected 'task done', got %q", done[0].Title)
	}
}

func TestNewStoreTableName(t *testing.T) {
	store := NewStore(nil)
	if store.tableName != "local_nodes" {
		t.Errorf("NewStore: tableName = %q, want %q", store.tableName, "local_nodes")
	}
}

func TestNewSiteStoreTableName(t *testing.T) {
	store := NewSiteStore(nil)
	if store.tableName != "nodes" {
		t.Errorf("NewSiteStore: tableName = %q, want %q", store.tableName, "nodes")
	}
}

func TestResolveSpaceID_LocalPassthrough(t *testing.T) {
	store := NewStore(nil)
	slug := "demo"
	got := store.ResolveSpaceID(slug)
	if got != slug {
		t.Errorf("ResolveSpaceID (local): got %q, want %q", got, slug)
	}
}

func TestStrPtr(t *testing.T) {
	empty := strPtr("")
	if empty == nil {
		t.Fatal("strPtr(\"\") returned nil")
	}
	if *empty != "" {
		t.Errorf("strPtr(\"\"): got %q, want %q", *empty, "")
	}

	val := strPtr("hello")
	if val == nil {
		t.Fatal("strPtr(\"hello\") returned nil")
	}
	if *val != "hello" {
		t.Errorf("strPtr(\"hello\"): got %q, want %q", *val, "hello")
	}
}

func TestNilIfEmpty(t *testing.T) {
	if nilIfEmpty("") != nil {
		t.Error("nilIfEmpty(\"\") should return nil")
	}
	got := nilIfEmpty("x")
	if got == nil || *got != "x" {
		t.Errorf("nilIfEmpty(\"x\"): got %v, want pointer to \"x\"", got)
	}
}

func TestOpenNode_RoundTrip(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	created, err := store.CreateNode("hive", Node{Title: "test task"})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}
	nodeID := created.ID

	if err := store.CompleteNode("hive", nodeID); err != nil {
		t.Fatalf("CompleteNode: %v", err)
	}
	done, err := store.GetNode(nodeID)
	if err != nil {
		t.Fatalf("GetNode after complete: %v", err)
	}
	if done.State != NodeStateDone {
		t.Fatalf("state after complete: got %q, want %q", done.State, NodeStateDone)
	}

	if err := store.OpenNode("hive", nodeID); err != nil {
		t.Fatalf("OpenNode: %v", err)
	}
	reopened, err := store.GetNode(nodeID)
	if err != nil {
		t.Fatalf("GetNode after open: %v", err)
	}
	if reopened.State != NodeStateOpen {
		t.Fatalf("state after open: got %q, want %q", reopened.State, NodeStateOpen)
	}
}

func TestMutations_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	missing := "00000000-0000-0000-0000-000000000000"

	tests := []struct {
		name string
		fn   func() error
	}{
		{"CompleteNode", func() error { return store.CompleteNode("hive", missing) }},
		{"OpenNode", func() error { return store.OpenNode("hive", missing) }},
		{"ClaimNode", func() error { return store.ClaimNode("hive", missing, "alice") }},
		{"UpdateNodeStateInSpace", func() error { return store.UpdateNodeStateInSpace("hive", missing, NodeStateEscalated) }},
		{"UpdateNodeState", func() error { return store.UpdateNodeState(missing, NodeStateEscalated) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("got %v, want ErrNotFound", err)
			}
		})
	}
}

func TestMutations_CrossSpaceMismatch(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	created, err := store.CreateNode("hive", Node{Title: "owned by hive"})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	// Same node id, different space → ErrNotFound (existence is hidden across spaces).
	if err := store.CompleteNode("other", created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("CompleteNode cross-space: got %v, want ErrNotFound", err)
	}
	if err := store.ClaimNode("other", created.ID, "alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ClaimNode cross-space: got %v, want ErrNotFound", err)
	}
	if err := store.UpdateNodeStateInSpace("other", created.ID, NodeStateDone); !errors.Is(err, ErrNotFound) {
		t.Fatalf("UpdateNodeStateInSpace cross-space: got %v, want ErrNotFound", err)
	}

	got, err := store.GetNode(created.ID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.State != NodeStateOpen {
		t.Fatalf("state mutated by cross-space call: got %q, want %q", got.State, NodeStateOpen)
	}
	if got.Assignee != "" {
		t.Fatalf("assignee mutated by cross-space call: got %q, want empty", got.Assignee)
	}
}

func TestOpenNode_AlreadyOpen(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	created, err := store.CreateNode("hive", Node{Title: "born open"})
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	err = store.OpenNode("hive", created.ID)
	if !errors.Is(err, ErrInvalidState) {
		t.Fatalf("OpenNode on already-open node: got %v, want ErrInvalidState", err)
	}
}
