package localapi

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx database/sql driver
)

// testDB opens a connection to the test Postgres, runs Migrate, and truncates
// all local tables so each test starts clean.
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hive:hive@localhost:5432/hive?sslmode=disable"
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
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
