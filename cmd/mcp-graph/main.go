// Command mcp-graph is an MCP server that exposes transpara.ai graph operations
// as tools callable by Claude or any MCP-compatible client.
//
// The server speaks the MCP protocol over stdio (newline-delimited JSON-RPC 2.0).
// Wire it into cmd/mind via --mcp-config to let the Mind execute graph operations
// from within conversations.
//
// Tools provided:
//   - graph.intend    — create a task/node in a space
//   - graph.respond   — post a response to a node
//   - graph.search    — search nodes in a space
//   - graph.getBoard  — list task nodes on the board
//   - graph.getNode   — get details of a specific node
//
// Configuration via environment variables:
//
//	TRANSPARA_API_KEY   — required. Bearer token for transpara.ai API.
//	TRANSPARA_BASE_URL  — optional. Defaults to https://transpara.ai.
//	TRANSPARA_SPACE     — optional. Default space slug. Defaults to "hive".
//
// Usage:
//
//	TRANSPARA_API_KEY=lv_... ./mcp-graph
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// JSON-RPC 2.0 types
// ────────────────────────────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"` // nil for notifications
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ────────────────────────────────────────────────────────────────────────────
// MCP types
// ────────────────────────────────────────────────────────────────────────────

type toolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string                `json:"type"`
	Properties map[string]schemaProp `json:"properties"`
	Required   []string              `json:"required,omitempty"`
}

type schemaProp struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ────────────────────────────────────────────────────────────────────────────
// Server config
// ────────────────────────────────────────────────────────────────────────────

// maxResponseBytes caps API response body reads to prevent unbounded memory use (invariant 13: BOUNDED).
const maxResponseBytes = 1 << 20 // 1 MiB

type server struct {
	apiKey   string
	baseURL  string
	defSpace string
	client   *http.Client
}

func newServer() *server {
	apiKey := os.Getenv("TRANSPARA_API_KEY")
	baseURL := strings.TrimRight(os.Getenv("TRANSPARA_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "https://transpara.ai"
	}
	defSpace := os.Getenv("TRANSPARA_SPACE")
	if defSpace == "" {
		defSpace = "hive"
	}
	return &server{
		apiKey:   apiKey,
		baseURL:  baseURL,
		defSpace: defSpace,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *server) spaceFor(args map[string]any) string {
	if v, ok := args["space"].(string); ok && v != "" {
		return v
	}
	return s.defSpace
}

// ────────────────────────────────────────────────────────────────────────────
// Tool definitions
// ────────────────────────────────────────────────────────────────────────────

var tools = []toolDef{
	{
		Name:        "graph.intend",
		Description: "Create a task or node in a space on transpara.ai. Returns the created node ID.",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"space":       {Type: "string", Description: "Space slug (e.g. 'hive'). Defaults to the server's default space."},
				"title":       {Type: "string", Description: "Title of the task/node."},
				"description": {Type: "string", Description: "Optional body text."},
				"kind":        {Type: "string", Description: "Node kind: task (default), project, goal, post."},
				"assignee":    {Type: "string", Description: "Optional assignee username."},
			},
			Required: []string{"title"},
		},
	},
	{
		Name:        "graph.respond",
		Description: "Post a response to an existing node (conversation message, task comment, etc.).",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"space":     {Type: "string", Description: "Space slug. Defaults to the server's default space."},
				"parent_id": {Type: "string", Description: "ID of the parent node to respond to."},
				"body":      {Type: "string", Description: "Response text."},
			},
			Required: []string{"parent_id", "body"},
		},
	},
	{
		Name:        "graph.search",
		Description: "Search for nodes in a space by keyword. Returns matching tasks, posts, and threads.",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"space": {Type: "string", Description: "Space slug. Defaults to the server's default space."},
				"query": {Type: "string", Description: "Search query."},
			},
			Required: []string{"query"},
		},
	},
	{
		Name:        "graph.getBoard",
		Description: "Get the task board for a space. Returns all open tasks.",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"space": {Type: "string", Description: "Space slug. Defaults to the server's default space."},
			},
		},
	},
	{
		Name:        "graph.getNode",
		Description: "Get details of a specific node by ID, including its children and ops history.",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"space":   {Type: "string", Description: "Space slug. Defaults to the server's default space."},
				"node_id": {Type: "string", Description: "Node ID."},
			},
			Required: []string{"node_id"},
		},
	},
}

// ────────────────────────────────────────────────────────────────────────────
// Tool handlers
// ────────────────────────────────────────────────────────────────────────────

func (s *server) callTool(name string, args map[string]any) toolResult {
	switch name {
	case "graph.intend":
		return s.toolIntend(args)
	case "graph.respond":
		return s.toolRespond(args)
	case "graph.search":
		return s.toolSearch(args)
	case "graph.getBoard":
		return s.toolGetBoard(args)
	case "graph.getNode":
		return s.toolGetNode(args)
	default:
		return errResult(fmt.Sprintf("unknown tool: %s", name))
	}
}

func (s *server) toolIntend(args map[string]any) toolResult {
	space := s.spaceFor(args)
	title, _ := args["title"].(string)
	if title == "" {
		return errResult("title is required")
	}
	payload := map[string]string{
		"op":    "intend",
		"title": title,
	}
	if desc, ok := args["description"].(string); ok && desc != "" {
		payload["description"] = desc
	}
	if kind, ok := args["kind"].(string); ok && kind != "" {
		payload["kind"] = kind
	}
	if assignee, ok := args["assignee"].(string); ok && assignee != "" {
		payload["assignee"] = assignee
	}

	body, _ := json.Marshal(payload)
	resp, err := s.apiPost("/app/"+url.PathEscape(space)+"/op", body)
	if err != nil {
		return errResult(err.Error())
	}
	return okResult(resp)
}

func (s *server) toolRespond(args map[string]any) toolResult {
	space := s.spaceFor(args)
	parentID, _ := args["parent_id"].(string)
	msgBody, _ := args["body"].(string)
	if parentID == "" || msgBody == "" {
		return errResult("parent_id and body are required")
	}

	payload, _ := json.Marshal(map[string]string{
		"op":        "respond",
		"parent_id": parentID,
		"body":      msgBody,
	})
	resp, err := s.apiPost("/app/"+url.PathEscape(space)+"/op", payload)
	if err != nil {
		return errResult(err.Error())
	}
	return okResult(resp)
}

func (s *server) toolSearch(args map[string]any) toolResult {
	space := s.spaceFor(args)
	query, _ := args["query"].(string)
	if query == "" {
		return errResult("query is required")
	}
	resp, err := s.apiGet("/app/" + url.PathEscape(space) + "/board?q=" + url.QueryEscape(query))
	if err != nil {
		return errResult(err.Error())
	}
	return okResult(resp)
}

func (s *server) toolGetBoard(args map[string]any) toolResult {
	space := s.spaceFor(args)
	resp, err := s.apiGet("/app/" + url.PathEscape(space) + "/board")
	if err != nil {
		return errResult(err.Error())
	}
	return okResult(resp)
}

func (s *server) toolGetNode(args map[string]any) toolResult {
	space := s.spaceFor(args)
	nodeID, _ := args["node_id"].(string)
	if nodeID == "" {
		return errResult("node_id is required")
	}
	if strings.ContainsAny(nodeID, "/?#") {
		return errResult("node_id contains invalid characters")
	}
	resp, err := s.apiGet("/app/" + url.PathEscape(space) + "/node/" + url.PathEscape(nodeID))
	if err != nil {
		return errResult(err.Error())
	}
	return okResult(resp)
}

// ────────────────────────────────────────────────────────────────────────────
// HTTP helpers
// ────────────────────────────────────────────────────────────────────────────

func (s *server) apiGet(path string) (string, error) {
	req, err := http.NewRequest("GET", s.baseURL+path, nil)
	if err != nil {
		return "", err
	}
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	return string(b), nil
}

func (s *server) apiPost(path string, body []byte) (string, error) {
	req, err := http.NewRequest("POST", s.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	return string(b), nil
}

func okResult(text string) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: text}}}
}

func errResult(msg string) toolResult {
	return toolResult{IsError: true, Content: []toolContent{{Type: "text", Text: msg}}}
}

// ────────────────────────────────────────────────────────────────────────────
// MCP protocol loop
// ────────────────────────────────────────────────────────────────────────────

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mcp-graph: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	srv := newServer()
	enc := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(enc, nil, -32700, "parse error")
			continue
		}

		// Notifications have no ID — no response required.
		if req.ID == nil {
			continue
		}

		id := *req.ID

		switch req.Method {
		case "initialize":
			enc.Encode(rpcResponse{
				JSONRPC: "2.0",
				ID:      id,
				Result: map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{"tools": map[string]any{}},
					"serverInfo":      map[string]any{"name": "mcp-graph", "version": "1.0.0"},
				},
			})

		case "tools/list":
			enc.Encode(rpcResponse{
				JSONRPC: "2.0",
				ID:      id,
				Result:  map[string]any{"tools": tools},
			})

		case "tools/call":
			var p toolCallParams
			if err := json.Unmarshal(req.Params, &p); err != nil {
				sendError(enc, id, -32602, "invalid params: "+err.Error())
				continue
			}
			result := srv.callTool(p.Name, p.Arguments)
			enc.Encode(rpcResponse{
				JSONRPC: "2.0",
				ID:      id,
				Result:  result,
			})

		default:
			sendError(enc, id, -32601, "method not found: "+req.Method)
		}
	}

	return scanner.Err()
}

func sendError(enc *json.Encoder, id json.RawMessage, code int, msg string) {
	enc.Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	})
}
