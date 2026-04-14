package localapi

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Node mirrors api.Node — duplicated here to avoid an import cycle.
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

// Agent mirrors api.Agent — duplicated here to avoid an import cycle.
type Agent struct {
	Name        string `json:"name"`
	Display     string `json:"display"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Prompt      string `json:"prompt"`
	Model       string `json:"model"`
	Active      bool   `json:"active"`
	SessionID   string `json:"session_id"`
}

// Store provides CRUD operations backed by a local Postgres database.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store wrapping the given database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// ---------------------------------------------------------------------------
// Node operations
// ---------------------------------------------------------------------------

// CreateNode inserts a node. If n.ID is empty a UUID is generated.
// Returns the node with server-assigned timestamps.
func (s *Store) CreateNode(spaceID string, n Node) (*Node, error) {
	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	if spaceID != "" {
		n.SpaceID = spaceID
	}
	if n.SpaceID == "" {
		n.SpaceID = "hive"
	}
	if n.Kind == "" {
		n.Kind = "task"
	}
	if n.State == "" {
		n.State = "open"
	}
	if n.Priority == "" {
		n.Priority = "medium"
	}

	var createdAt, updatedAt time.Time
	err := s.db.QueryRow(`
		INSERT INTO local_nodes
			(id, space_id, parent_id, kind, title, body, state, priority,
			 assignee, assignee_id, author, author_id, author_kind,
			 due_date, pinned)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING created_at, updated_at`,
		n.ID, n.SpaceID, nilIfEmpty(n.ParentID), n.Kind, n.Title, n.Body,
		n.State, n.Priority,
		nilIfEmpty(n.Assignee), nilIfEmpty(n.AssigneeID),
		nilIfEmpty(n.Author), nilIfEmpty(n.AuthorID), nilIfEmpty(n.AuthorKind),
		nilIfEmpty(n.DueDate), n.Pinned,
	).Scan(&createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("CreateNode: %w", err)
	}
	n.CreatedAt = createdAt.Format(time.RFC3339)
	n.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &n, nil
}

// GetNode fetches a single node by ID.
func (s *Store) GetNode(id string) (*Node, error) {
	n, err := s.scanNode(s.db.QueryRow(`
		SELECT id, space_id, parent_id, kind, title, body, state, priority,
		       assignee, assignee_id, author, author_id, author_kind,
		       due_date, pinned, created_at, updated_at
		FROM local_nodes WHERE id = $1`, id))
	if err != nil {
		return nil, fmt.Errorf("GetNode: %w", err)
	}
	return n, nil
}

// ListNodes returns nodes filtered by spaceID, kind, and state (empty string
// skips that filter). Results are ordered newest-first.
func (s *Store) ListNodes(spaceID, kind, state string) ([]Node, error) {
	query := `SELECT id, space_id, parent_id, kind, title, body, state, priority,
	                 assignee, assignee_id, author, author_id, author_kind,
	                 due_date, pinned, created_at, updated_at
	          FROM local_nodes WHERE 1=1`
	args := []any{}
	idx := 1

	if spaceID != "" {
		query += fmt.Sprintf(" AND space_id = $%d", idx)
		args = append(args, spaceID)
		idx++
	}
	if kind != "" {
		query += fmt.Sprintf(" AND kind = $%d", idx)
		args = append(args, kind)
		idx++
	}
	if state != "" {
		query += fmt.Sprintf(" AND state = $%d", idx)
		args = append(args, state)
		idx++
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ListNodes: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		n, err := s.scanNodeRow(rows)
		if err != nil {
			return nil, fmt.Errorf("ListNodes scan: %w", err)
		}
		nodes = append(nodes, *n)
	}
	return nodes, rows.Err()
}

// UpdateNodeState sets the state of a node and bumps updated_at.
func (s *Store) UpdateNodeState(id, state string) error {
	_, err := s.db.Exec(`UPDATE local_nodes SET state = $1, updated_at = now() WHERE id = $2`, state, id)
	return err
}

// ClaimNode assigns a node to the given assignee.
func (s *Store) ClaimNode(id, assignee string) error {
	_, err := s.db.Exec(`UPDATE local_nodes SET assignee = $1, updated_at = now() WHERE id = $2`, assignee, id)
	return err
}

// CompleteNode sets a node's state to "done".
func (s *Store) CompleteNode(id string) error {
	return s.UpdateNodeState(id, "done")
}

// ChildCounts returns (total, done) child counts for a parent node.
func (s *Store) ChildCounts(parentID string) (int, int) {
	var total, done int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM local_nodes WHERE parent_id = $1`, parentID).Scan(&total)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM local_nodes WHERE parent_id = $1 AND state = 'done'`, parentID).Scan(&done)
	return total, done
}

// MaxLessonNumber scans node titles in a space for patterns like "Lesson N:"
// and returns the highest N found. Returns 0 if none match.
func (s *Store) MaxLessonNumber(spaceID string) int {
	re := regexp.MustCompile(`Lesson\s+(\d+):`)
	rows, err := s.db.Query(`SELECT title FROM local_nodes WHERE space_id = $1`, spaceID)
	if err != nil {
		return 0
	}
	defer rows.Close()

	max := 0
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			continue
		}
		if m := re.FindStringSubmatch(title); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil && n > max {
				max = n
			}
		}
	}
	return max
}

// NodeExists returns true if a node with the given ID exists.
func (s *Store) NodeExists(id string) bool {
	var exists bool
	_ = s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM local_nodes WHERE id = $1)`, id).Scan(&exists)
	return exists
}

// SearchNodesByTitle finds nodes whose title starts with prefix (case-sensitive).
// Results are ordered newest-first, limited to limit rows.
func (s *Store) SearchNodesByTitle(spaceID, prefix string, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, space_id, parent_id, kind, title, body, state, priority,
		       assignee, assignee_id, author, author_id, author_kind,
		       due_date, pinned, created_at, updated_at
		FROM local_nodes
		WHERE space_id = $1 AND title LIKE $2
		ORDER BY created_at DESC
		LIMIT $3`, spaceID, prefix+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("SearchNodesByTitle: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		n, err := s.scanNodeRow(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, *n)
	}
	return nodes, rows.Err()
}

// enrichChildCounts fills ChildCount and ChildDone on each node.
func (s *Store) enrichChildCounts(nodes []Node) {
	for i := range nodes {
		nodes[i].ChildCount, nodes[i].ChildDone = s.ChildCounts(nodes[i].ID)
	}
}

// filterExcludeDone returns nodes whose state is neither "done" nor "closed".
func filterExcludeDone(nodes []Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if n.State != "done" && n.State != "closed" {
			out = append(out, n)
		}
	}
	return out
}

// filterByAssignee returns nodes where Assignee or AssigneeID matches id.
func filterByAssignee(nodes []Node, id string) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Assignee == id || n.AssigneeID == id {
			out = append(out, n)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Agent operations
// ---------------------------------------------------------------------------

// UpsertAgent inserts or updates an agent record.
func (s *Store) UpsertAgent(a Agent) error {
	_, err := s.db.Exec(`
		INSERT INTO local_agents (name, display, description, category, prompt, model, active, session_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (name) DO UPDATE SET
			display     = EXCLUDED.display,
			description = EXCLUDED.description,
			category    = EXCLUDED.category,
			prompt      = EXCLUDED.prompt,
			model       = EXCLUDED.model,
			active      = EXCLUDED.active,
			session_id  = EXCLUDED.session_id`,
		a.Name, a.Display, a.Description, a.Category, a.Prompt, a.Model, a.Active, a.SessionID)
	return err
}

// ListAgents returns all active agents.
func (s *Store) ListAgents() ([]Agent, error) {
	rows, err := s.db.Query(`SELECT name, display, description, category, prompt, model, active, session_id
		FROM local_agents WHERE active = true ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.Name, &a.Display, &a.Description, &a.Category,
			&a.Prompt, &a.Model, &a.Active, &a.SessionID); err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// UpdateAgentSession sets the session_id for an agent.
func (s *Store) UpdateAgentSession(name, sessionID string) error {
	_, err := s.db.Exec(`UPDATE local_agents SET session_id = $1 WHERE name = $2`, sessionID, name)
	return err
}

// ---------------------------------------------------------------------------
// Diagnostics
// ---------------------------------------------------------------------------

// InsertDiagnostic stores a JSON payload for later inspection.
func (s *Store) InsertDiagnostic(payload []byte) error {
	_, err := s.db.Exec(`INSERT INTO local_diagnostics (payload) VALUES ($1)`, payload)
	return err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nilIfEmpty returns nil for empty strings, otherwise a pointer to s.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func (s *Store) scanNode(row scanner) (*Node, error) {
	var n Node
	var parentID, assignee, assigneeID, author, authorID, authorKind, dueDate sql.NullString
	var createdAt, updatedAt time.Time

	err := row.Scan(
		&n.ID, &n.SpaceID, &parentID, &n.Kind, &n.Title, &n.Body,
		&n.State, &n.Priority,
		&assignee, &assigneeID, &author, &authorID, &authorKind,
		&dueDate, &n.Pinned, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	n.ParentID = parentID.String
	n.Assignee = assignee.String
	n.AssigneeID = assigneeID.String
	n.Author = author.String
	n.AuthorID = authorID.String
	n.AuthorKind = authorKind.String
	n.DueDate = dueDate.String
	n.CreatedAt = createdAt.Format(time.RFC3339)
	n.UpdatedAt = updatedAt.Format(time.RFC3339)
	return &n, nil
}

func (s *Store) scanNodeRow(rows *sql.Rows) (*Node, error) {
	return s.scanNode(rows)
}
