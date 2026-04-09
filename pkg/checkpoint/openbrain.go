package checkpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ThoughtStore is the coupling boundary between the checkpoint package and
// any thought-capture backend. The rest of the checkpoint package depends only
// on this interface, not on Open Brain specifics.
type ThoughtStore interface {
	// SearchRecent returns thoughts matching query that were captured within maxAge.
	SearchRecent(query string, maxAge time.Duration) ([]Thought, error)

	// Capture records a thought in the store.
	Capture(content string) error
}

// Thought is a single captured thought record.
type Thought struct {
	Content    string
	CapturedAt time.Time
}

// ─── OpenBrainClient ────────────────────────────────────────────────────────

// OpenBrainClient implements ThoughtStore via Open Brain's JSON-RPC 2.0 MCP
// endpoint. The endpoint is a Supabase Edge Function that speaks MCP over
// HTTP streaming (SSE). Authentication is via ?key= query parameter.
type OpenBrainClient struct {
	endpoint   string // full URL including ?key= param
	httpClient *http.Client
	nextID     int
}

// NewOpenBrainClient creates a client targeting the Open Brain MCP endpoint.
// baseURL is the edge function URL (e.g. https://xxx.supabase.co/functions/v1/open-brain-mcp).
// apiKey is the MCP access key appended as ?key= query parameter.
func NewOpenBrainClient(baseURL, apiKey string) *OpenBrainClient {
	endpoint := strings.TrimRight(baseURL, "/")
	if apiKey != "" {
		endpoint += "?key=" + apiKey
	}
	return &OpenBrainClient{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Capture stores a thought via the capture_thought MCP tool.
func (c *OpenBrainClient) Capture(content string) error {
	_, err := c.callTool("capture_thought", map[string]interface{}{
		"content": content,
	})
	if err != nil {
		return fmt.Errorf("openbrain: capture: %w", err)
	}
	return nil
}

// SearchRecent queries thoughts via the search_thoughts MCP tool, filtering by maxAge.
func (c *OpenBrainClient) SearchRecent(query string, maxAge time.Duration) ([]Thought, error) {
	result, err := c.callTool("search_thoughts", map[string]interface{}{
		"query":     query,
		"limit":     5,
		"threshold": 0.5,
	})
	if err != nil {
		return nil, fmt.Errorf("openbrain: search: %w", err)
	}

	// The result is a JSON-RPC content array: [{"type":"text","text":"..."}]
	// The text field contains the formatted thought list from Open Brain.
	// Parse the thoughts from the text content.
	return c.parseSearchResult(result, maxAge)
}

// callTool sends a JSON-RPC 2.0 tools/call request and returns the result text.
func (c *OpenBrainClient) callTool(toolName string, args map[string]interface{}) (string, error) {
	c.nextID++
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
		"id": c.nextID,
	}

	body, err := json.Marshal(rpcReq)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	resp, err := c.doWithRetry(body)
	if err != nil {
		return "", err
	}

	return resp, nil
}

// doWithRetry sends a JSON-RPC request and parses the SSE response.
// Retries once on network error or 5xx.
func (c *OpenBrainClient) doWithRetry(body []byte) (string, error) {
	result, err := c.doOnce(body)
	if err != nil {
		// Retry once.
		result, err = c.doOnce(body)
	}
	return result, err
}

// doOnce sends body to the endpoint and parses the SSE response.
func (c *OpenBrainClient) doOnce(body []byte) (string, error) {
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return "", fmt.Errorf("server error: status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	// Parse SSE response. Look for "data:" lines containing JSON-RPC result.
	return c.parseSSE(resp.Body)
}

// parseSSE reads an SSE stream and extracts the JSON-RPC result text.
// Format: "event: message\ndata: {json-rpc response}\n\n"
func (c *OpenBrainClient) parseSSE(r io.Reader) (string, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read SSE: %w", err)
	}

	// Find the last "data:" line (SSE may have multiple events).
	body := string(raw)
	var lastData string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			lastData = strings.TrimPrefix(line, "data:")
			lastData = strings.TrimSpace(lastData)
		}
	}

	if lastData == "" {
		// Response may be plain JSON (not SSE). Try parsing raw body directly.
		lastData = strings.TrimSpace(body)
	}

	// Parse JSON-RPC response.
	var rpcResp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(lastData), &rpcResp); err != nil {
		return "", fmt.Errorf("parse JSON-RPC response: %w (raw: %.200s)", err, lastData)
	}
	if rpcResp.Error != nil {
		return "", fmt.Errorf("JSON-RPC error: %s", rpcResp.Error.Message)
	}

	// Concatenate all text content blocks.
	var sb strings.Builder
	for _, c := range rpcResp.Result.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String(), nil
}

// parseSearchResult extracts thoughts from the search_thoughts tool response.
// The response text contains formatted thought entries with timestamps.
func (c *OpenBrainClient) parseSearchResult(text string, maxAge time.Duration) ([]Thought, error) {
	if text == "" {
		return nil, nil
	}

	cutoff := time.Now().Add(-maxAge)

	// Try parsing as JSON array first (some responses are structured).
	var structured []struct {
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal([]byte(text), &structured); err == nil && len(structured) > 0 {
		var thoughts []Thought
		for _, s := range structured {
			ts, err := time.Parse(time.RFC3339, s.CreatedAt)
			if err != nil {
				continue
			}
			if ts.Before(cutoff) {
				continue
			}
			thoughts = append(thoughts, Thought{Content: s.Content, CapturedAt: ts})
		}
		return thoughts, nil
	}

	// Fallback: treat as a single text response. The content itself is the thought.
	// Use current time as CapturedAt since the timestamp isn't structured.
	return []Thought{{Content: text, CapturedAt: time.Now()}}, nil
}

// ─── StubThoughtStore ───────────────────────────────────────────────────────

// StubThoughtStore is an in-memory ThoughtStore for use in tests.
type StubThoughtStore struct {
	Thoughts []Thought
}

// NewStubThoughtStore returns an empty StubThoughtStore.
func NewStubThoughtStore() *StubThoughtStore {
	return &StubThoughtStore{}
}

// Capture appends content with the current timestamp.
func (s *StubThoughtStore) Capture(content string) error {
	s.Thoughts = append(s.Thoughts, Thought{
		Content:    content,
		CapturedAt: time.Now(),
	})
	return nil
}

// SearchRecent returns thoughts whose Content contains query (case-insensitive
// substring match) and whose CapturedAt is within maxAge of now.
func (s *StubThoughtStore) SearchRecent(query string, maxAge time.Duration) ([]Thought, error) {
	cutoff := time.Now().Add(-maxAge)
	lower := strings.ToLower(query)

	var results []Thought
	for _, t := range s.Thoughts {
		if t.CapturedAt.Before(cutoff) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(t.Content), lower) {
			continue
		}
		results = append(results, t)
	}
	return results, nil
}
