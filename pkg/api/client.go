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
	DueDate    string `json:"due_date,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
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

// PostOp sends a grammar operation to the space.
// fields is a flat map sent as JSON body (must include "op").
func (c *Client) PostOp(slug string, fields map[string]string) (*OpResponse, error) {
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
		return nil, fmt.Errorf("PostOp(%s): %w", fields["op"], err)
	}
	return &resp, nil
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
func (c *Client) CreateTask(slug, title, description, priority string) (*Node, error) {
	fields := map[string]string{
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
	resp, err := c.PostOp(slug, fields)
	if err != nil {
		return nil, err
	}
	return resp.Node, nil
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
func (c *Client) CreateDocument(slug, title, body string) (*Node, error) {
	resp, err := c.PostOp(slug, map[string]string{
		"op":          "intend",
		"kind":        "document",
		"title":       title,
		"description": body,
	})
	if err != nil {
		return nil, err
	}
	return resp.Node, nil
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
