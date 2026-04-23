# Offline Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `hive --pipeline` work with zero dependency on lovyou.ai — fully local using postgres + a new local API shim server.

**Architecture:** Build a lightweight HTTP server (`cmd/localapi/`) that implements the same REST endpoints the pipeline expects from lovyou.ai, backed by local postgres tables. The `--api` flag (already exists, defaults to `https://lovyou.ai`) gets pointed at `http://localhost:8082`. Embedded curl commands in agent prompts already use `fmt.Sprintf` with the API key and slug — we add the base URL as a third template parameter so they target the local server instead of the hardcoded `https://lovyou.ai`.

**Tech Stack:** Go, `net/http`, `database/sql`, `lib/pq`, PostgreSQL 16

---

## File Structure

| File | Responsibility |
|------|---------------|
| **Create:** `cmd/localapi/main.go` | Local API server entry point — registers routes, starts HTTP on `:8082` |
| **Create:** `pkg/localapi/server.go` | Route handlers: board, op, documents, knowledge, agents, diagnostics, nodes |
| **Create:** `pkg/localapi/store.go` | Postgres-backed storage: CRUD for nodes, agents, diagnostics |
| **Create:** `pkg/localapi/schema.go` | `CREATE TABLE IF NOT EXISTS` DDL for `local_nodes`, `local_agents`, `local_diagnostics` |
| **Create:** `pkg/localapi/store_test.go` | Integration tests for store operations |
| **Create:** `pkg/localapi/server_test.go` | HTTP handler tests (round-trip: POST op → GET board) |
| **Modify:** `pkg/runner/runner.go:47-66` | Add `APIBase string` to `Config` struct |
| **Modify:** `pkg/runner/scout.go:71` | Template base URL into curl instead of hardcoded `https://lovyou.ai` |
| **Modify:** `pkg/runner/architect.go:236,245` | Same — template base URL |
| **Modify:** `pkg/runner/pm.go:57,64` | Same — template base URL |
| **Modify:** `pkg/runner/critic.go:145` | Same — template base URL |
| **Modify:** `pkg/runner/reflector.go:285,287` | Same — template base URL |
| **Modify:** `pkg/runner/observer.go:263,267,297,300,307` | Same — template base URL |
| **Modify:** `pkg/runner/spawner.go:107` | Same — template base URL |
| **Modify:** `pkg/runner/scribe.go:59` | Same — template base URL |
| **Modify:** `pkg/runner/council.go:308` | Same — template base URL |
| **Modify:** `cmd/hive/main.go:258-260` | Make `LOVYOU_API_KEY` optional when `--api` points to localhost |
| **Modify:** `cmd/hive/main.go:350-366` | Pass `APIBase` through to runner `Config` |

---

### Task 1: Database Schema + Store Layer

**Files:**
- Create: `pkg/localapi/schema.go`
- Create: `pkg/localapi/store.go`
- Create: `pkg/localapi/store_test.go`

The store needs three tables: `local_nodes` (tasks, documents, claims, posts — everything is a node), `local_agents` (agent personas + session IDs), and `local_diagnostics` (phase events). The node table mirrors `api.Node` from `pkg/api/client.go:17-38`.

- [ ] **Step 1: Write the failing test for node creation and retrieval**

```go
// pkg/localapi/store_test.go
package localapi

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hive:hive@localhost:5432/hive?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Clean slate for each test.
	db.Exec("DELETE FROM local_nodes")
	db.Exec("DELETE FROM local_agents")
	db.Exec("DELETE FROM local_diagnostics")
	return db
}

func TestCreateAndGetNode(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)

	node, err := s.CreateNode("hive", Node{
		Kind:     "task",
		Title:    "Build login page",
		Body:     "Implement OAuth login",
		State:    "open",
		Priority: "high",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if node.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if node.Kind != "task" {
		t.Fatalf("kind = %q, want task", node.Kind)
	}

	got, err := s.GetNode(node.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Build login page" {
		t.Fatalf("title = %q, want 'Build login page'", got.Title)
	}
}

func TestListNodes_FilterByKindAndState(t *testing.T) {
	db := testDB(t)
	s := NewStore(db)

	s.CreateNode("hive", Node{Kind: "task", Title: "Task 1", State: "open"})
	s.CreateNode("hive", Node{Kind: "task", Title: "Task 2", State: "done"})
	s.CreateNode("hive", Node{Kind: "document", Title: "Doc 1", State: "open"})

	tasks, err := s.ListNodes("hive", "task", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}

	openTasks, err := s.ListNodes("hive", "task", "open")
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(openTasks) != 1 {
		t.Fatalf("got %d open tasks, want 1", len(openTasks))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ~/transpara-ai/repos/hive && go test ./pkg/localapi/ -run TestCreate -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the schema migration**

```go
// pkg/localapi/schema.go
package localapi

import "database/sql"

// Migrate creates tables if they don't exist. Safe to call on every startup.
func Migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS local_nodes (
			id          TEXT PRIMARY KEY,
			space_id    TEXT NOT NULL DEFAULT 'hive',
			parent_id   TEXT,
			kind        TEXT NOT NULL DEFAULT 'task',
			title       TEXT NOT NULL DEFAULT '',
			body        TEXT NOT NULL DEFAULT '',
			state       TEXT NOT NULL DEFAULT 'open',
			priority    TEXT NOT NULL DEFAULT 'medium',
			assignee    TEXT,
			assignee_id TEXT,
			author      TEXT,
			author_id   TEXT,
			author_kind TEXT,
			due_date    TEXT,
			pinned      BOOLEAN NOT NULL DEFAULT false,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		);

		CREATE INDEX IF NOT EXISTS idx_local_nodes_space_kind
			ON local_nodes(space_id, kind);
		CREATE INDEX IF NOT EXISTS idx_local_nodes_state
			ON local_nodes(state);
		CREATE INDEX IF NOT EXISTS idx_local_nodes_parent
			ON local_nodes(parent_id);

		CREATE TABLE IF NOT EXISTS local_agents (
			name        TEXT PRIMARY KEY,
			display     TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			category    TEXT NOT NULL DEFAULT '',
			prompt      TEXT NOT NULL DEFAULT '',
			model       TEXT NOT NULL DEFAULT 'sonnet',
			active      BOOLEAN NOT NULL DEFAULT true,
			session_id  TEXT NOT NULL DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS local_diagnostics (
			id          SERIAL PRIMARY KEY,
			payload     JSONB NOT NULL,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	return err
}
```

- [ ] **Step 4: Write the store implementation**

```go
// pkg/localapi/store.go
package localapi

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Node mirrors api.Node from pkg/api/client.go. Duplicated here to avoid
// an import cycle — localapi should not depend on the api package.
type Node struct {
	ID           string `json:"id"`
	SpaceID      string `json:"space_id"`
	ParentID     string `json:"parent_id,omitempty"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	State        string `json:"state"`
	Priority     string `json:"priority"`
	Assignee     string `json:"assignee,omitempty"`
	AssigneeID   string `json:"assignee_id,omitempty"`
	Author       string `json:"author,omitempty"`
	AuthorID     string `json:"author_id,omitempty"`
	AuthorKind   string `json:"author_kind,omitempty"`
	DueDate      string `json:"due_date,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	ChildCount   int    `json:"child_count"`
	ChildDone    int    `json:"child_done"`
	BlockerCount int    `json:"blocker_count"`
	Pinned       bool   `json:"pinned"`
}

// Agent mirrors api.Agent from pkg/api/client.go.
type Agent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Display     string `json:"display"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Prompt      string `json:"prompt"`
	Model       string `json:"model"`
	Active      bool   `json:"active"`
	SessionID   string `json:"session_id"`
}

// Store provides CRUD operations backed by postgres.
type Store struct {
	db *sql.DB
}

// NewStore creates a store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateNode inserts a node and returns it with generated ID and timestamps.
func (s *Store) CreateNode(spaceID string, n Node) (*Node, error) {
	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	if n.Kind == "" {
		n.Kind = "task"
	}
	if n.State == "" {
		n.State = "open"
	}
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		INSERT INTO local_nodes (id, space_id, parent_id, kind, title, body, state, priority,
			assignee, assignee_id, author, author_id, author_kind, due_date, pinned)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		n.ID, spaceID, nilIfEmpty(n.ParentID), n.Kind, n.Title, n.Body, n.State, n.Priority,
		nilIfEmpty(n.Assignee), nilIfEmpty(n.AssigneeID),
		nilIfEmpty(n.Author), nilIfEmpty(n.AuthorID), nilIfEmpty(n.AuthorKind),
		nilIfEmpty(n.DueDate), n.Pinned,
	)
	if err != nil {
		return nil, fmt.Errorf("insert node: %w", err)
	}

	n.SpaceID = spaceID
	n.CreatedAt = now
	n.UpdatedAt = now
	return &n, nil
}

// GetNode fetches a single node by ID.
func (s *Store) GetNode(id string) (*Node, error) {
	n := &Node{}
	err := s.db.QueryRow(`
		SELECT id, space_id, COALESCE(parent_id,''), kind, title, body, state, priority,
			COALESCE(assignee,''), COALESCE(assignee_id,''),
			COALESCE(author,''), COALESCE(author_id,''), COALESCE(author_kind,''),
			COALESCE(due_date,''), pinned, created_at, updated_at
		FROM local_nodes WHERE id = $1`, id).Scan(
		&n.ID, &n.SpaceID, &n.ParentID, &n.Kind, &n.Title, &n.Body, &n.State, &n.Priority,
		&n.Assignee, &n.AssigneeID, &n.Author, &n.AuthorID, &n.AuthorKind,
		&n.DueDate, &n.Pinned, &n.CreatedAt, &n.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("node %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	return n, nil
}

// ListNodes lists nodes filtered by space, kind, and optionally state.
// Pass empty strings to skip filters.
func (s *Store) ListNodes(spaceID, kind, state string) ([]Node, error) {
	q := "SELECT id, space_id, COALESCE(parent_id,''), kind, title, body, state, priority, " +
		"COALESCE(assignee,''), COALESCE(assignee_id,''), " +
		"COALESCE(author,''), COALESCE(author_id,''), COALESCE(author_kind,''), " +
		"COALESCE(due_date,''), pinned, created_at, updated_at " +
		"FROM local_nodes WHERE 1=1"
	var args []any
	idx := 1

	if spaceID != "" {
		q += fmt.Sprintf(" AND space_id = $%d", idx)
		args = append(args, spaceID)
		idx++
	}
	if kind != "" {
		q += fmt.Sprintf(" AND kind = $%d", idx)
		args = append(args, kind)
		idx++
	}
	if state != "" {
		q += fmt.Sprintf(" AND state = $%d", idx)
		args = append(args, state)
		idx++
	}
	q += " ORDER BY created_at DESC"

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.ID, &n.SpaceID, &n.ParentID, &n.Kind, &n.Title, &n.Body, &n.State, &n.Priority,
			&n.Assignee, &n.AssigneeID, &n.Author, &n.AuthorID, &n.AuthorKind,
			&n.DueDate, &n.Pinned, &n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// UpdateNodeState sets the state and updated_at of a node.
func (s *Store) UpdateNodeState(id, state string) error {
	res, err := s.db.Exec(`UPDATE local_nodes SET state = $1, updated_at = now() WHERE id = $2`, state, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node %q not found", id)
	}
	return nil
}

// ClaimNode sets the assignee on an unassigned node.
func (s *Store) ClaimNode(id, assignee string) error {
	res, err := s.db.Exec(`UPDATE local_nodes SET assignee = $1, updated_at = now() WHERE id = $2`, assignee, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("node %q not found", id)
	}
	return nil
}

// CompleteNode marks a node as done.
func (s *Store) CompleteNode(id string) error {
	return s.UpdateNodeState(id, "done")
}

// ChildCounts returns (child_count, child_done) for a parent node by
// counting rows with parent_id = id.
func (s *Store) ChildCounts(parentID string) (int, int) {
	var total, done int
	s.db.QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE state = 'done') FROM local_nodes WHERE parent_id = $1`, parentID).Scan(&total, &done)
	return total, done
}

// MaxLessonNumber scans claim titles matching "Lesson N:" and returns the max N.
func (s *Store) MaxLessonNumber(spaceID string) int {
	var max int
	s.db.QueryRow(`
		SELECT COALESCE(MAX(
			CAST(regexp_replace(title, '^Lesson (\d+):.*$', '\1') AS INTEGER)
		), 0)
		FROM local_nodes
		WHERE space_id = $1 AND kind = 'claim' AND title ~ '^Lesson \d+:'`, spaceID).Scan(&max)
	return max
}

// UpsertAgent inserts or updates an agent definition.
func (s *Store) UpsertAgent(a Agent) error {
	_, err := s.db.Exec(`
		INSERT INTO local_agents (name, display, description, category, prompt, model, active, session_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (name) DO UPDATE SET
			display=$2, description=$3, category=$4, prompt=$5, model=$6, active=$7, session_id=$8`,
		a.Name, a.Display, a.Description, a.Category, a.Prompt, a.Model, a.Active, a.SessionID,
	)
	return err
}

// ListAgents returns all active agents.
func (s *Store) ListAgents() ([]Agent, error) {
	rows, err := s.db.Query(`SELECT name, display, description, category, prompt, model, active, session_id FROM local_agents WHERE active = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.Name, &a.Display, &a.Description, &a.Category, &a.Prompt, &a.Model, &a.Active, &a.SessionID); err != nil {
			return nil, err
		}
		a.ID = a.Name // local agents use name as ID
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// UpdateAgentSession sets the session ID for an agent.
func (s *Store) UpdateAgentSession(name, sessionID string) error {
	_, err := s.db.Exec(`UPDATE local_agents SET session_id = $1 WHERE name = $2`, sessionID, name)
	return err
}

// InsertDiagnostic stores a raw JSON diagnostic event.
func (s *Store) InsertDiagnostic(payload []byte) error {
	_, err := s.db.Exec(`INSERT INTO local_diagnostics (payload) VALUES ($1)`, payload)
	return err
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// NodeExists checks if a node with the given ID exists.
func (s *Store) NodeExists(id string) bool {
	var exists bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM local_nodes WHERE id = $1)`, id).Scan(&exists)
	return exists
}

// SearchNodesByTitle finds nodes whose title starts with a prefix, newest first.
func (s *Store) SearchNodesByTitle(spaceID, prefix string, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id, space_id, COALESCE(parent_id,''), kind, title, body, state, priority,
		COALESCE(assignee,''), COALESCE(assignee_id,''),
		COALESCE(author,''), COALESCE(author_id,''), COALESCE(author_kind,''),
		COALESCE(due_date,''), pinned, created_at, updated_at
		FROM local_nodes
		WHERE space_id = $1 AND title LIKE $2
		ORDER BY created_at DESC
		LIMIT $3`
	rows, err := s.db.Query(q, spaceID, prefix+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.ID, &n.SpaceID, &n.ParentID, &n.Kind, &n.Title, &n.Body, &n.State, &n.Priority,
			&n.Assignee, &n.AssigneeID, &n.Author, &n.AuthorID, &n.AuthorKind,
			&n.DueDate, &n.Pinned, &n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// enrichChildCounts fills in ChildCount and ChildDone for each node
// that has children.
func (s *Store) enrichChildCounts(nodes []Node) {
	for i := range nodes {
		nodes[i].ChildCount, nodes[i].ChildDone = s.ChildCounts(nodes[i].ID)
	}
}

// filterExcludeDone returns nodes that are not done/closed.
func filterExcludeDone(nodes []Node) []Node {
	var out []Node
	for _, n := range nodes {
		if n.State != "done" && n.State != "closed" {
			out = append(out, n)
		}
	}
	return out
}

// filterByAssignee returns nodes assigned to a specific ID (or all if empty).
func filterByAssignee(nodes []Node, id string) []Node {
	if id == "" {
		return nodes
	}
	var out []Node
	for _, n := range nodes {
		if n.AssigneeID == id || strings.EqualFold(n.Assignee, id) {
			out = append(out, n)
		}
	}
	return out
}
```

- [ ] **Step 5: Run the tests**

Run: `cd ~/transpara-ai/repos/hive && go test ./pkg/localapi/ -v -count=1`
Expected: PASS — both tests green. (Requires local postgres running.)

- [ ] **Step 6: Commit**

```bash
git add pkg/localapi/schema.go pkg/localapi/store.go pkg/localapi/store_test.go
git commit -m "feat(localapi): add store layer for offline pipeline

Tables: local_nodes, local_agents, local_diagnostics.
Store provides CRUD that mirrors the lovyou.ai API surface."
```

---

### Task 2: HTTP Server — Route Handlers

**Files:**
- Create: `pkg/localapi/server.go`
- Create: `pkg/localapi/server_test.go`

The server implements the exact HTTP API that `pkg/api/client.go` calls. Every method in `Client` maps to a route here. The contract is: if `api.New("http://localhost:8082", "dev")` works against lovyou.ai, it works identically against this server.

- [ ] **Step 1: Write the failing test — round-trip through the HTTP API**

```go
// pkg/localapi/server_test.go
package localapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	db := testDB(t) // reuse from store_test.go
	s := NewStore(db)
	handler := NewServer(s, "dev")
	return httptest.NewServer(handler)
}

func TestRoundTrip_CreateAndListTasks(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// POST /app/hive/op — create a task via "intend" op
	body := `{"op":"intend","kind":"task","title":"Build login","description":"OAuth flow","priority":"high"}`
	resp, err := http.Post(ts.URL+"/app/hive/op", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST op: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("POST status = %d", resp.StatusCode)
	}
	var opResp struct {
		Node struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"node"`
	}
	json.NewDecoder(resp.Body).Decode(&opResp)
	resp.Body.Close()
	if opResp.Node.ID == "" {
		t.Fatal("expected node ID in response")
	}

	// GET /app/hive/board?format=json — verify task appears
	resp, err = http.Get(ts.URL + "/app/hive/board?format=json")
	if err != nil {
		t.Fatalf("GET board: %v", err)
	}
	var board struct {
		Nodes []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			State string `json:"state"`
		} `json:"nodes"`
	}
	json.NewDecoder(resp.Body).Decode(&board)
	resp.Body.Close()
	if len(board.Nodes) != 1 {
		t.Fatalf("board has %d nodes, want 1", len(board.Nodes))
	}
	if board.Nodes[0].Title != "Build login" {
		t.Fatalf("title = %q, want 'Build login'", board.Nodes[0].Title)
	}
}

func TestRoundTrip_CompleteTask(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create
	body := `{"op":"intend","kind":"task","title":"Fix bug"}`
	resp, _ := http.Post(ts.URL+"/app/hive/op", "application/json", strings.NewReader(body))
	var created struct{ Node struct{ ID string } }
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// Complete
	body = `{"op":"complete","node_id":"` + created.Node.ID + `"}`
	resp, _ = http.Post(ts.URL+"/app/hive/op", "application/json", strings.NewReader(body))
	resp.Body.Close()

	// Board should be empty (completed tasks excluded from board)
	resp, _ = http.Get(ts.URL + "/app/hive/board?format=json")
	var board struct{ Nodes []struct{ ID string } }
	json.NewDecoder(resp.Body).Decode(&board)
	resp.Body.Close()
	if len(board.Nodes) != 0 {
		t.Fatalf("board has %d nodes after complete, want 0", len(board.Nodes))
	}
}

func TestRoundTrip_NodeExists(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a node
	body := `{"op":"intend","kind":"task","title":"Exists test"}`
	resp, _ := http.Post(ts.URL+"/app/hive/op", "application/json", strings.NewReader(body))
	var created struct{ Node struct{ ID string } }
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()

	// Exists → 200
	resp, _ = http.Get(ts.URL + "/app/hive/node/" + created.Node.ID + "?format=json")
	if resp.StatusCode != 200 {
		t.Fatalf("existing node status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// Not exists → 404
	resp, _ = http.Get(ts.URL + "/app/hive/node/nonexistent?format=json")
	if resp.StatusCode != 404 {
		t.Fatalf("missing node status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ~/transpara-ai/repos/hive && go test ./pkg/localapi/ -run TestRoundTrip -v`
Expected: FAIL — `NewServer` not defined.

- [ ] **Step 3: Write the server implementation**

```go
// pkg/localapi/server.go
package localapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// NewServer returns an http.Handler implementing the lovyou.ai API surface.
// apiKey is the expected Bearer token (use "dev" for local).
func NewServer(store *Store, apiKey string) http.Handler {
	s := &server{store: store, apiKey: apiKey}
	mux := http.NewServeMux()

	// Board — GET /app/{slug}/board
	mux.HandleFunc("GET /app/{slug}/board", s.handleBoard)

	// Grammar operations — POST /app/{slug}/op
	mux.HandleFunc("POST /app/{slug}/op", s.handleOp)

	// Single node — GET /app/{slug}/node/{id}
	mux.HandleFunc("GET /app/{slug}/node/{id}", s.handleGetNode)

	// Documents — GET /app/{slug}/documents
	mux.HandleFunc("GET /app/{slug}/documents", s.handleDocuments)

	// Knowledge (claims + max_lesson) — GET /app/{slug}/knowledge
	mux.HandleFunc("GET /app/{slug}/knowledge", s.handleKnowledge)

	// Feed — GET /app/{slug}/feed
	mux.HandleFunc("GET /app/{slug}/feed", s.handleFeed)

	// Agents — GET /app/{slug}/people
	mux.HandleFunc("GET /app/{slug}/people", s.handleListAgents)

	// Agent session — PATCH /app/{slug}/agents/{name}/session
	mux.HandleFunc("PATCH /app/{slug}/agents/{name}/session", s.handleUpdateSession)

	// Escalation — POST /api/hive/escalation
	mux.HandleFunc("POST /api/hive/escalation", s.handleEscalation)

	// Diagnostic — POST /api/hive/diagnostic
	mux.HandleFunc("POST /api/hive/diagnostic", s.handleDiagnostic)

	// Health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	return mux
}

type server struct {
	store  *Store
	apiKey string
}

// handleBoard returns open tasks for the board (excludes done/closed).
func (s *server) handleBoard(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	assignee := r.URL.Query().Get("assignee")

	nodes, err := s.store.ListNodes(slug, "", "")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	nodes = filterExcludeDone(nodes)
	nodes = filterByAssignee(nodes, assignee)
	s.store.enrichChildCounts(nodes)

	writeJSON(w, map[string]any{"nodes": nodes})
}

// handleOp dispatches grammar operations: intend, complete, edit, claim,
// respond, express, assert, assign, discuss.
func (s *server) handleOp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	var fields map[string]any
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		http.Error(w, "bad json: "+err.Error(), 400)
		return
	}

	op, _ := fields["op"].(string)
	switch op {
	case "intend":
		node := Node{
			Kind:     stringField(fields, "kind", "task"),
			Title:    stringField(fields, "title", ""),
			Body:     stringField(fields, "description", ""),
			State:    "open",
			Priority: stringField(fields, "priority", "medium"),
		}
		created, err := s.store.CreateNode(slug, node)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"op": op, "node": created})

	case "complete":
		nodeID := stringField(fields, "node_id", "")
		if err := s.store.CompleteNode(nodeID); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		writeJSON(w, map[string]any{"op": op, "status": "ok"})

	case "edit":
		nodeID := stringField(fields, "node_id", "")
		state := stringField(fields, "state", "")
		if state != "" {
			if err := s.store.UpdateNodeState(nodeID, state); err != nil {
				http.Error(w, err.Error(), 404)
				return
			}
		}
		writeJSON(w, map[string]any{"op": op, "status": "ok"})

	case "claim":
		nodeID := stringField(fields, "node_id", "")
		// In local mode, "claim" sets the assignee to the agent making the call.
		// The actual agent name isn't in the request — use a placeholder.
		if err := s.store.ClaimNode(nodeID, "agent"); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		writeJSON(w, map[string]any{"op": op, "status": "ok"})

	case "assign":
		nodeID := stringField(fields, "node_id", "")
		assignee := stringField(fields, "assignee", "")
		if err := s.store.ClaimNode(nodeID, assignee); err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		writeJSON(w, map[string]any{"op": op, "status": "ok"})

	case "respond":
		// Comment on a node — create a child node of kind "comment".
		parentID := stringField(fields, "parent_id", "")
		comment := Node{
			Kind:     "comment",
			ParentID: parentID,
			Body:     stringField(fields, "body", ""),
			State:    "open",
		}
		created, err := s.store.CreateNode(slug, comment)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"op": op, "node": created})

	case "express":
		// Feed post.
		post := Node{
			Kind:  "post",
			Title: stringField(fields, "title", ""),
			Body:  stringField(fields, "body", ""),
			State: "open",
		}
		created, err := s.store.CreateNode(slug, post)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"op": op, "node": created})

	case "assert":
		// Knowledge claim.
		claim := Node{
			Kind:  "claim",
			Title: stringField(fields, "title", ""),
			Body:  stringField(fields, "body", ""),
			State: "open",
		}
		created, err := s.store.CreateNode(slug, claim)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"op": op, "node": created})

	case "discuss":
		// Discussion thread.
		thread := Node{
			Kind:  "thread",
			Title: stringField(fields, "title", ""),
			Body:  stringField(fields, "body", ""),
			State: "open",
		}
		created, err := s.store.CreateNode(slug, thread)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"op": op, "node": created})

	default:
		http.Error(w, fmt.Sprintf("unknown op: %q", op), 400)
	}
}

func (s *server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	node, err := s.store.GetNode(id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, node)
}

func (s *server) handleDocuments(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	docs, err := s.store.ListNodes(slug, "document", "")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"documents": docs})
}

func (s *server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	if r.URL.Query().Get("op") == "max_lesson" {
		max := s.store.MaxLessonNumber(slug)
		writeJSON(w, map[string]any{"max_lesson": max})
		return
	}

	claims, err := s.store.ListNodes(slug, "claim", "")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"claims": claims})
}

func (s *server) handleFeed(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	posts, err := s.store.ListNodes(slug, "post", "")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"posts": posts})
}

func (s *server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.ListAgents()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"agents": agents})
}

func (s *server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.store.UpdateAgentSession(name, body.SessionID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func (s *server) handleEscalation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TaskID string `json:"task_id"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// Mark the task as escalated.
	if err := s.store.UpdateNodeState(body.TaskID, "escalated"); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	writeJSON(w, map[string]any{"status": "escalated"})
}

func (s *server) handleDiagnostic(w http.ResponseWriter, r *http.Request) {
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.store.InsertDiagnostic(raw); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func stringField(m map[string]any, key, fallback string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

// slugFromPath extracts the slug from paths like /app/{slug}/...
// Only needed if PathValue isn't available (pre-Go 1.22 compat).
func slugFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[1] == "app" {
		return parts[2]
	}
	return ""
}
```

- [ ] **Step 4: Run the tests**

Run: `cd ~/transpara-ai/repos/hive && go test ./pkg/localapi/ -run TestRoundTrip -v -count=1`
Expected: PASS — all three round-trip tests green.

- [ ] **Step 5: Commit**

```bash
git add pkg/localapi/server.go pkg/localapi/server_test.go
git commit -m "feat(localapi): add HTTP server implementing lovyou.ai API surface

Routes: board, op, node, documents, knowledge, feed, agents, session,
escalation, diagnostic. All backed by local postgres."
```

---

### Task 3: CLI Entry Point — `cmd/localapi/main.go`

**Files:**
- Create: `cmd/localapi/main.go`

Thin main that connects to postgres, runs the migration, and starts the HTTP server.

- [ ] **Step 1: Write the entry point**

```go
// cmd/localapi/main.go
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
	"github.com/lovyou-ai/hive/pkg/localapi"
)

func main() {
	addr := flag.String("addr", ":8082", "Listen address")
	apiKey := flag.String("api-key", "dev", "Expected Bearer token (use 'dev' for local)")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://hive:hive@localhost:5432/hive?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	if err := localapi.Migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	store := localapi.NewStore(db)
	handler := localapi.NewServer(store, *apiKey)

	fmt.Printf("localapi listening on %s (key=%s)\n", *addr, *apiKey)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
```

- [ ] **Step 2: Verify it compiles and starts**

Run: `cd ~/transpara-ai/repos/hive && go build ./cmd/localapi/`
Expected: Binary builds without error.

Run: `cd ~/transpara-ai/repos/hive && timeout 3 go run ./cmd/localapi/ 2>&1 || true`
Expected: Output includes `localapi listening on :8082`.

- [ ] **Step 3: Smoke test against the running server**

```bash
# In one terminal:
go run ./cmd/localapi/ &
sleep 2

# Create a task
curl -s -X POST http://localhost:8082/app/hive/op \
  -H "Content-Type: application/json" \
  -d '{"op":"intend","kind":"task","title":"Test task","priority":"high"}' | jq .

# Read the board
curl -s http://localhost:8082/app/hive/board?format=json | jq .

# Health
curl -s http://localhost:8082/health

kill %1
```

- [ ] **Step 4: Commit**

```bash
git add cmd/localapi/main.go
git commit -m "feat(localapi): add CLI entry point for local API server

Starts on :8082 by default, auto-migrates tables on startup."
```

---

### Task 4: Template Base URL Into Agent Curl Commands

**Files:**
- Modify: `pkg/runner/runner.go:47-66` — add `APIBase` to Config
- Modify: `pkg/runner/scout.go:71`
- Modify: `pkg/runner/architect.go:236,245`
- Modify: `pkg/runner/pm.go:57,64`
- Modify: `pkg/runner/critic.go:145`
- Modify: `pkg/runner/reflector.go:285,287`
- Modify: `pkg/runner/observer.go:263,267,297,300,307`
- Modify: `pkg/runner/spawner.go:107`
- Modify: `pkg/runner/scribe.go:59`
- Modify: `pkg/runner/council.go:308`

Every embedded curl command currently hardcodes `https://lovyou.ai`. The `--api` flag value is already available via `api.Client.base`, but it's not exposed. We add `APIBase` to `runner.Config` and thread it through `fmt.Sprintf` in every agent instruction.

- [ ] **Step 1: Add `APIBase` to runner.Config**

In `pkg/runner/runner.go`, add the field to the `Config` struct:

```go
// In the Config struct, after APIClient:
APIBase  string      // Base URL for agent curl commands (e.g. "http://localhost:8082")
```

- [ ] **Step 2: Replace hardcoded URLs in scout.go**

In `pkg/runner/scout.go`, find the curl command at line ~71:

```
"https://lovyou.ai/app/%s/op"
```

Replace with:

```
"%s/app/%s/op"
```

And update the `fmt.Sprintf` arguments to include `r.cfg.APIBase` before `r.cfg.SpaceSlug` in the curl line. Repeat for every `https://lovyou.ai` occurrence in the file.

- [ ] **Step 3: Replace hardcoded URLs in architect.go**

At lines ~236 and ~245, replace `"https://lovyou.ai/app/%s/op"` and `"https://lovyou.ai/app/%s/board"` with `"%s/app/%s/op"` and `"%s/app/%s/board"`. Update the `fmt.Sprintf` arguments to prepend `r.cfg.APIBase` (or the local variable carrying it).

- [ ] **Step 4: Replace hardcoded URLs in pm.go**

At lines ~57 and ~64, same pattern: `"https://lovyou.ai/..."` → `"%s/..."` with APIBase.

- [ ] **Step 5: Replace hardcoded URLs in critic.go**

At line ~145, same pattern.

- [ ] **Step 6: Replace hardcoded URLs in reflector.go**

At lines ~285 and ~287, same pattern.

- [ ] **Step 7: Replace hardcoded URLs in observer.go**

At lines ~263, ~267, ~297, ~300, ~307, same pattern. Also check line ~230 for the plain text reference to `https://lovyou.ai/` and update that too.

- [ ] **Step 8: Replace hardcoded URLs in spawner.go, scribe.go, council.go**

Same pattern in each file.

- [ ] **Step 9: Verify no remaining hardcoded lovyou.ai URLs in agent prompts**

Run: `grep -rn 'https://lovyou.ai' pkg/runner/ | grep -v '_test.go'`
Expected: Zero matches.

- [ ] **Step 10: Verify compilation**

Run: `cd ~/transpara-ai/repos/hive && go build ./...`
Expected: No errors.

- [ ] **Step 11: Commit**

```bash
git add pkg/runner/runner.go pkg/runner/scout.go pkg/runner/architect.go \
    pkg/runner/pm.go pkg/runner/critic.go pkg/runner/reflector.go \
    pkg/runner/observer.go pkg/runner/spawner.go pkg/runner/scribe.go \
    pkg/runner/council.go
git commit -m "refactor(runner): template base URL into agent curl commands

Replace all hardcoded https://lovyou.ai references with the
configurable APIBase from runner.Config. This is the key change
that allows pipeline mode to work against a local API server."
```

---

### Task 5: Wire APIBase Through the Pipeline Entry Point

**Files:**
- Modify: `cmd/hive/main.go:254-366`

Thread the `--api` flag value into `runner.Config.APIBase` and make `LOVYOU_API_KEY` optional when targeting localhost.

- [ ] **Step 1: Make LOVYOU_API_KEY optional for localhost**

In `cmd/hive/main.go`, `runPipeline()` at lines 258-261, replace:

```go
apiKey := os.Getenv("LOVYOU_API_KEY")
if apiKey == "" {
    return fmt.Errorf("LOVYOU_API_KEY required")
}
```

With:

```go
apiKey := os.Getenv("LOVYOU_API_KEY")
if apiKey == "" {
    if strings.Contains(apiBase, "localhost") || strings.Contains(apiBase, "127.0.0.1") {
        apiKey = "dev"
        log.Printf("[pipeline] no LOVYOU_API_KEY — using 'dev' for local API")
    } else {
        return fmt.Errorf("LOVYOU_API_KEY required (set env or use --api http://localhost:8082 for local mode)")
    }
}
```

- [ ] **Step 2: Pass APIBase into runner.Config**

In `cmd/hive/main.go`, in the `makeRunner` closure at line ~350, add `APIBase` to the `runner.Config`:

```go
return runner.New(runner.Config{
    // ... existing fields ...
    APIBase:  apiBase,   // ← add this line
}), nil
```

- [ ] **Step 3: Add `strings` import if not already present**

Check the import block in `cmd/hive/main.go` — add `"strings"` if missing.

- [ ] **Step 4: Verify compilation and that --help still works**

Run: `cd ~/transpara-ai/repos/hive && go build ./cmd/hive/ && ./hive --help`
Expected: Builds and shows help with `--api` flag.

- [ ] **Step 5: Commit**

```bash
git add cmd/hive/main.go
git commit -m "feat(pipeline): wire APIBase through to runners, optional API key for localhost

LOVYOU_API_KEY defaults to 'dev' when --api targets localhost.
APIBase is passed to runner.Config so agent curl commands use it."
```

---

### Task 6: Update Hive Lifecycle Skill

**Files:**
- Modify: `.claude/skills/hive-lifecycle/skill.md`

Add `hive run --pipeline` instructions for local mode so future sessions know how to start the offline pipeline.

- [ ] **Step 1: Add Local Pipeline section to the skill**

After the existing "Hive Run" section, add:

```markdown
### Local Pipeline (no lovyou.ai dependency)

Requires the local API server (`cmd/localapi`) running alongside postgres.

```bash
# 1. Start localapi (if not already running)
cd $HIVE_REPO
nohup go run ./cmd/localapi/ > /tmp/localapi.log 2>&1 &
echo $! > /tmp/localapi.pid
echo "localapi started (PID: $(cat /tmp/localapi.pid))"

# Verify
curl -s http://localhost:8082/health  # → "ok"

# 2. Seed a task for the pipeline to work on
curl -s -X POST http://localhost:8082/app/hive/op \
  -H "Content-Type: application/json" \
  -d '{"op":"intend","kind":"task","title":"Build a habit tracker CLI","description":"Go CLI: habit add, habit check, habit streak. SQLite storage. Include tests.","priority":"high"}'

# 3. Run the pipeline against local API
go run ./cmd/hive \
    --pipeline \
    --api http://localhost:8082 \
    --space hive \
    --store postgres://hive:hive@localhost:5432/hive \
    --human Michael \
    --pr
```

No `LOVYOU_API_KEY` needed — defaults to "dev" for localhost.
```

- [ ] **Step 2: Add localapi to the Stack Components table**

Add row:
```
| **localapi** | Go binary (background) | hive | `go run ./cmd/localapi` |
```

- [ ] **Step 3: Add localapi to Hive Status check**

Add:
```bash
echo ""
echo "=== Local API ==="
curl -s -o /dev/null -w "HTTP %{http_code}" http://localhost:8082/health 2>/dev/null || echo "Not running"
```

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/hive-lifecycle/skill.md
git commit -m "docs(skill): add local pipeline instructions to hive-lifecycle

Documents how to start localapi and run the pipeline offline."
```

---

### Task 7: End-to-End Smoke Test

No new files — this is a manual verification task.

- [ ] **Step 1: Start postgres (should already be running)**

```bash
docker compose up -d postgres
```

- [ ] **Step 2: Start localapi**

```bash
cd ~/transpara-ai/repos/hive
go run ./cmd/localapi/ &
sleep 2
curl -s http://localhost:8082/health  # → "ok"
```

- [ ] **Step 3: Seed a task**

```bash
curl -s -X POST http://localhost:8082/app/hive/op \
  -H "Content-Type: application/json" \
  -d '{"op":"intend","kind":"task","title":"Add --version flag to hive CLI","description":"Print version and exit.","priority":"high"}' | jq .
```

- [ ] **Step 4: Run the pipeline**

```bash
go run ./cmd/hive \
    --pipeline \
    --api http://localhost:8082 \
    --space hive \
    --store postgres://hive:hive@localhost:5432/hive \
    --human Michael \
    --pr \
    --budget 5.0
```

Expected: Pipeline starts, Scout runs, Architect decomposes, Builder works, Critic reviews. No "connection refused" or "LOVYOU_API_KEY required" errors.

- [ ] **Step 5: Verify board state after run**

```bash
curl -s http://localhost:8082/app/hive/board?format=json | jq '.nodes | length'
```

- [ ] **Step 6: Check diagnostics were recorded**

```bash
docker exec hive-postgres-1 psql -U hive -d hive -t -c "SELECT COUNT(*) FROM local_diagnostics"
```

- [ ] **Step 7: Kill localapi**

```bash
kill $(cat /tmp/localapi.pid)
```
