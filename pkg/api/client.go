// Package api provides an HTTP client for the lovyou.ai JSON API.
// Agents use this to poll tasks, post updates, and close work items.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Node is a task, post, comment, or any other graph node returned by the API.
type Node struct {
	ID         string `json:"id"`
	SpaceID    string `json:"space_id"`
	ParentID   string `json:"parent_id,omitempty"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	State      string `json:"state"`
	Priority   string `json:"priority"`
	Assignee   string `json:"assignee,omitempty"`
	AssigneeID string `json:"assignee_id,omitempty"`
	Author     string `json:"author,omitempty"`
	AuthorID   string `json:"author_id,omitempty"`
	AuthorKind string `json:"author_kind,omitempty"`
	DueDate      string `json:"due_date,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	ChildCount   int    `json:"child_count"`
	ChildDone    int    `json:"child_done"`
	BlockerCount int    `json:"blocker_count"`
	Pinned       bool   `json:"pinned"`
}

// BoardResponse is the JSON returned by GET /app/{slug}/board?format=json.
type BoardResponse struct {
	Nodes []Node `json:"nodes"`
}

// OpResponse is the JSON returned by POST /app/{slug}/op.
type OpResponse struct {
	Node   *Node  `json:"node,omitempty"`
	Op     string `json:"op"`
	Status string `json:"status,omitempty"`
}

// Client talks to the lovyou.ai JSON API.
type Client struct {
	base   string // e.g. "https://lovyou.ai"
	apiKey string // LOVYOU_API_KEY (sent as Bearer token)
	http   *http.Client
}

// New creates an API client. base is the origin (e.g. "https://lovyou.ai").
func New(base, apiKey string) *Client {
	return &Client{
		base:   base,
		apiKey: apiKey,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetTasks fetches open tasks from the board, optionally filtered by assignee.
func (c *Client) GetTasks(slug string, assigneeID string) ([]Node, error) {
	u := fmt.Sprintf("%s/app/%s/board?format=json", c.base, slug)
	if assigneeID != "" {
		u += "&assignee=" + url.QueryEscape(assigneeID)
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	var resp BoardResponse
	if err := c.do(req, &resp); err != nil {
		return nil, fmt.Errorf("GetTasks: %w", err)
	}
	return resp.Nodes, nil
}

// postOpAny sends a grammar operation with arbitrary field types.
// Supports array fields like "causes" that map[string]string cannot represent.
func (c *Client) postOpAny(slug string, fields map[string]any) (*OpResponse, error) {
	u := fmt.Sprintf("%s/app/%s/op", c.base, slug)

	body, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	var resp OpResponse
	if err := c.do(req, &resp); err != nil {
		op, _ := fields["op"].(string)
		return nil, fmt.Errorf("PostOp(%s): %w", op, err)
	}
	return &resp, nil
}

// PostOp sends a grammar operation to the space.
// fields is a flat map sent as JSON body (must include "op").
func (c *Client) PostOp(slug string, fields map[string]string) (*OpResponse, error) {
	any := make(map[string]any, len(fields))
	for k, v := range fields {
		any[k] = v
	}
	return c.postOpAny(slug, any)
}

// ClaimTask claims an unassigned task for the current agent.
func (c *Client) ClaimTask(slug, nodeID string) error {
	_, err := c.PostOp(slug, map[string]string{
		"op":      "claim",
		"node_id": nodeID,
	})
	return err
}

// UpdateTaskStatus sets a task's state (e.g. "active" for in_progress).
func (c *Client) UpdateTaskStatus(slug, nodeID, state string) error {
	_, err := c.PostOp(slug, map[string]string{
		"op":      "edit",
		"node_id": nodeID,
		"state":   state,
	})
	return err
}

// CompleteTask marks a task as done.
func (c *Client) CompleteTask(slug, nodeID string) error {
	_, err := c.PostOp(slug, map[string]string{
		"op":      "complete",
		"node_id": nodeID,
	})
	return err
}

// CommentTask adds a comment to a task.
func (c *Client) CommentTask(slug, nodeID, body string) error {
	_, err := c.PostOp(slug, map[string]string{
		"op":        "respond",
		"parent_id": nodeID,
		"body":      body,
	})
	return err
}

// CreateTask creates a new task on the board.
// causes is the list of node IDs that triggered this task (Invariant 2 — CAUSALITY).
// Pass nil when there is no triggering node.
func (c *Client) CreateTask(slug, title, description, priority string, causes []string) (*Node, error) {
	fields := map[string]any{
		"op":    "intend",
		"title": title,
		"kind":  "task",
	}
	if description != "" {
		fields["description"] = description
	}
	if priority != "" {
		fields["priority"] = priority
	}
	if len(causes) > 0 {
		fields["causes"] = causes
	}
	resp, err := c.postOpAny(slug, fields)
	if err != nil {
		return nil, err
	}
	return resp.Node, nil
}

// GetDocuments fetches document nodes from a space's knowledge layer.
func (c *Client) GetDocuments(slug string, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 20
	}
	u := fmt.Sprintf("%s/app/%s/documents?limit=%d", c.base, slug, limit)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	var resp struct {
		Documents []Node `json:"documents"`
	}
	if err := c.do(req, &resp); err != nil {
		return nil, fmt.Errorf("GetDocuments: %w", err)
	}
	return resp.Documents, nil
}

// GetClaims fetches claim nodes from a space's knowledge layer.
func (c *Client) GetClaims(slug string, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 20
	}
	u := fmt.Sprintf("%s/app/%s/knowledge?tab=claims&limit=%d", c.base, slug, limit)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	var resp struct {
		Claims []Node `json:"claims"`
	}
	if err := c.do(req, &resp); err != nil {
		return nil, fmt.Errorf("GetClaims: %w", err)
	}
	return resp.Claims, nil
}

// GetFeed fetches recent posts from a space's feed.
func (c *Client) GetFeed(slug string, limit int) ([]Node, error) {
	if limit <= 0 {
		limit = 20
	}
	u := fmt.Sprintf("%s/app/%s/feed?limit=%d", c.base, slug, limit)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	var resp struct {
		Posts []Node `json:"posts"`
	}
	if err := c.do(req, &resp); err != nil {
		return nil, fmt.Errorf("GetFeed: %w", err)
	}
	return resp.Posts, nil
}

// LatestByTitle finds the most recent node matching a title prefix.
// Searches tasks, documents, and claims. Returns nil if not found.
func (c *Client) LatestByTitle(slug, prefix string) *Node {
	tasks, _ := c.GetTasks(slug, "")
	for i := len(tasks) - 1; i >= 0; i-- {
		if strings.HasPrefix(tasks[i].Title, prefix) {
			return &tasks[i]
		}
	}
	docs, _ := c.GetDocuments(slug, 50)
	for i := range docs {
		if strings.HasPrefix(docs[i].Title, prefix) {
			return &docs[i]
		}
	}
	claims, _ := c.GetClaims(slug, 50)
	for i := range claims {
		if strings.HasPrefix(claims[i].Title, prefix) {
			return &claims[i]
		}
	}
	return nil
}

// PostUpdate posts to the feed (social — visible to followers).
func (c *Client) PostUpdate(slug, title, body string) error {
	_, err := c.PostOp(slug, map[string]string{
		"op":    "express",
		"title": title,
		"body":  body,
	})
	return err
}

// CreateDocument creates a document node in the knowledge layer.
// Documents are institutional knowledge — specs, reports, reflections.
// NOT feed posts. Use PostUpdate for social visibility.
// causes is the list of node IDs that triggered this document (Invariant 2 — CAUSALITY).
func (c *Client) CreateDocument(slug, title, body string, causes []string) (*Node, error) {
	fields := map[string]any{
		"op":          "intend",
		"kind":        "document",
		"title":       title,
		"description": body,
	}
	if len(causes) > 0 {
		fields["causes"] = causes
	}
	resp, err := c.postOpAny(slug, fields)
	if err != nil {
		return nil, err
	}
	return resp.Node, nil
}

// AssertClaim creates a knowledge claim — a factual assertion that can be
// challenged, verified, or retracted. Use for lessons learned, verdicts,
// architectural decisions — anything that should be verifiable.
// causes is the list of node IDs that triggered this claim (Invariant 2 — CAUSALITY).
func (c *Client) AssertClaim(slug, title, body string, causes []string) (*Node, error) {
	fields := map[string]any{
		"op":    "assert",
		"title": title,
		"body":  body,
	}
	if len(causes) > 0 {
		fields["causes"] = causes
	}
	resp, err := c.postOpAny(slug, fields)
	if err != nil {
		return nil, err
	}
	return resp.Node, nil
}

// NextLessonNumber calls the server-side aggregate to find the highest existing
// "Lesson N" number and returns N+1. Returns 1 if no numbered lessons exist.
// Server-side MAX avoids client-side scan truncation as lesson count grows
// (Invariant 13: BOUNDED — correct at any count, O(1) per call).
func (c *Client) NextLessonNumber(slug string) int {
	u := fmt.Sprintf("%s/app/%s/knowledge?op=max_lesson", c.base, slug)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return 1
	}
	c.setHeaders(req)
	var resp struct {
		MaxLesson int `json:"max_lesson"`
	}
	if err := c.do(req, &resp); err != nil {
		return 1
	}
	return resp.MaxLesson + 1
}

// AskQuestion creates a question node. If Mind is configured for the space,
// it will auto-answer from the space's documents. Use for self-queries —
// "does this already exist?" "what primitive maps to X?"
func (c *Client) AskQuestion(slug, title, body string) (*Node, error) {
	resp, err := c.PostOp(slug, map[string]string{
		"op":          "intend",
		"kind":        "question",
		"title":       title,
		"description": body,
	})
	if err != nil {
		return nil, err
	}
	return resp.Node, nil
}

// StartThread creates a discussion thread. Use for deliberations that
// need multiple responses — architecture discussions, trade-off analysis.
func (c *Client) StartThread(slug, title, body string) (*Node, error) {
	resp, err := c.PostOp(slug, map[string]string{
		"op":    "discuss",
		"title": title,
		"body":  body,
	})
	if err != nil {
		return nil, err
	}
	return resp.Node, nil
}

// Agent is an agent definition from the lovyou.ai database.
type Agent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Display     string `json:"display"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Prompt      string `json:"prompt"`
	Model       string `json:"model"`
	Active      bool   `json:"active"`
	SessionID   string `json:"session_id"` // UUID for persistent Claude CLI session
}

// ListAgents fetches all active agents from the space.
func (c *Client) ListAgents(slug string) ([]Agent, error) {
	u := fmt.Sprintf("%s/app/%s/people?format=agents", c.base, slug)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	var resp struct {
		Agents []Agent `json:"agents"`
	}
	if err := c.do(req, &resp); err != nil {
		return nil, fmt.Errorf("ListAgents: %w", err)
	}
	return resp.Agents, nil
}

// EscalateTask sets a task to "escalated" state and creates a notification
// for the space owner. Called when a Builder returns ACTION: ESCALATE.
func (c *Client) EscalateTask(slug, taskID, reason, assigneeID string) error {
	body := map[string]string{
		"space_slug": slug,
		"task_id":    taskID,
		"reason":     reason,
	}
	if assigneeID != "" {
		body["assignee_id"] = assigneeID
	}
	return c.postJSON("/api/hive/escalation", body, nil)
}

// UpdateAgentSession persists a new session ID for the named agent persona.
// Called after each successful Claude CLI call so the next iteration can resume warm.
func (c *Client) UpdateAgentSession(slug, name, sessionID string) error {
	u := fmt.Sprintf("%s/app/%s/agents/%s/session", c.base, slug, name)
	body := fmt.Sprintf(`{"session_id":%q}`, sessionID)
	req, err := http.NewRequest("PATCH", u, strings.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, nil)
}

// GetAgent fetches an agent by ID. Falls back to name match if ID not found.
func (c *Client) GetAgent(slug, idOrName string) (*Agent, error) {
	agents, err := c.ListAgents(slug)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if a.ID == idOrName || a.Name == idOrName {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("agent %q not found", idOrName)
}

// AssignTask assigns a task to a specific agent by ID or name.
func (c *Client) AssignTask(slug, nodeID, assignee string) error {
	_, err := c.PostOp(slug, map[string]string{
		"op":       "assign",
		"node_id":  nodeID,
		"assignee": assignee,
	})
	return err
}

// NodeExists checks whether a node with the given ID exists in the space.
// Returns false if the node is not found (HTTP 404) or on network error.
// Used to validate LLM-generated cause IDs before posting nodes (Lesson 170).
func (c *Client) NodeExists(slug, id string) bool {
	u := fmt.Sprintf("%s/app/%s/node/%s?format=json", c.base, slug, id)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return false
	}
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	// Drain body to allow connection reuse.
	_, _ = io.ReadAll(resp.Body)
	return resp.StatusCode == http.StatusOK
}

// PostDiagnostic sends a phase event to the /api/hive/diagnostic endpoint.
// The event is stored in the site's database so /hive/feed works in production.
// Returns nil on success. Non-fatal: callers should log but not abort on error.
// payload must be pre-marshalled JSON.
func (c *Client) PostDiagnostic(payload []byte) error {
	return c.postJSON("/api/hive/diagnostic", payload, nil)
}

// MirrorToSite pushes a hive event update back to the site via
// /api/hive/mirror so site nodes can be stamped with hive_chain_ref.
// Called after hive emits a terminal spec event (e.g. hive.spec.completed).
// body is an arbitrary JSON-serialisable payload; out is optional.
// The successful call is recorded on the chain as site.op.mirrored by
// the caller — this method does not emit.
func (c *Client) MirrorToSite(body any, out any) error {
	return c.postJSON("/api/hive/mirror", body, out)
}

// postJSON marshals body as JSON and POSTs it to path (relative to c.base).
// When body is a []byte or json.RawMessage, it is sent as-is without
// re-marshalling (avoids base64 encoding []byte). Pass nil for body to
// send an empty body, and nil for out to ignore the response payload.
func (c *Client) postJSON(path string, body any, out any) error {
	var reader io.Reader
	switch v := body.(type) {
	case nil:
		// empty body
	case []byte:
		reader = bytes.NewReader(v)
	case json.RawMessage:
		reader = bytes.NewReader(v)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("postJSON marshal: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest("POST", c.base+path, reader)
	if err != nil {
		return fmt.Errorf("postJSON request: %w", err)
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
}

func (c *Client) do(req *http.Request, result any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	if result != nil && len(data) > 0 {
		if err := json.Unmarshal(data, result); err != nil {
			return fmt.Errorf("decode: %w (body: %s)", err, string(data))
		}
	}
	return nil
}
