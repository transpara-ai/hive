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

// OpenBrainClient implements ThoughtStore against the Open Brain HTTP API.
type OpenBrainClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenBrainClient creates a client with a 5-second timeout.
// apiKey may be empty — the Authorization header is omitted when blank.
func NewOpenBrainClient(baseURL, apiKey string) *OpenBrainClient {
	return &OpenBrainClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Capture POSTs content to baseURL+"/capture_thought".
// It retries once on a transient (network or 5xx) failure.
func (c *OpenBrainClient) Capture(content string) error {
	body, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("openbrain: marshal capture body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/capture_thought", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("openbrain: build capture request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("openbrain: capture request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("openbrain: capture returned status %d", resp.StatusCode)
	}
	return nil
}

// SearchRecent POSTs to baseURL+"/search_thoughts" and filters by maxAge.
func (c *OpenBrainClient) SearchRecent(query string, maxAge time.Duration) ([]Thought, error) {
	body, err := json.Marshal(map[string]interface{}{
		"query": query,
		"limit": 5,
	})
	if err != nil {
		return nil, fmt.Errorf("openbrain: marshal search body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/search_thoughts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openbrain: build search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("openbrain: search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		return nil, fmt.Errorf("openbrain: search returned status %d", resp.StatusCode)
	}

	// Response is an array of {content, created_at} objects.
	var raw []struct {
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("openbrain: decode search response: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	var thoughts []Thought
	for _, r := range raw {
		ts, err := time.Parse(time.RFC3339, r.CreatedAt)
		if err != nil {
			// Skip unparseable timestamps rather than failing the whole call.
			continue
		}
		if ts.Before(cutoff) {
			continue
		}
		thoughts = append(thoughts, Thought{
			Content:    r.Content,
			CapturedAt: ts,
		})
	}
	return thoughts, nil
}

// doWithRetry executes req and retries once on error or 5xx response.
// The request body must be re-readable — callers must use bytes.NewReader.
func (c *OpenBrainClient) doWithRetry(req *http.Request) (*http.Response, error) {
	// Clone the body bytes so we can replay on retry.
	var bodyBytes []byte
	if req.Body != nil {
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, req.Body); err != nil {
			return nil, err
		}
		req.Body.Close()
		bodyBytes = buf.Bytes()
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
	}

	resp, err := c.httpClient.Do(req)
	if err == nil && resp.StatusCode < 500 {
		return resp, nil
	}
	if err == nil {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
	}

	// Retry once.
	if bodyBytes != nil {
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		req.ContentLength = int64(len(bodyBytes))
	}
	return c.httpClient.Do(req)
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
