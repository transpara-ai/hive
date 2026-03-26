// Command mcp-knowledge is an MCP server that gives agents a navigable
// knowledge tree — the "light switch" for the hive's institutional memory.
//
// It indexes ALL knowledge sources:
//   - /site/content/reference/ — 201 primitives, 13 layers, 13 grammars
//   - /site/content/posts/ — blog posts (derivation stories)
//   - /hive/loop/ — state, backlog, reflections, scout/build/critique reports
//   - /hive/agents/ — role prompts, CONTEXT.md
//   - /hive/docs/ — specs, design docs
//   - All repo CLAUDE.md files
//
// The server speaks MCP over stdio (JSON-RPC 2.0).
//
// Tools:
//   - knowledge.topics    — list top-level categories or children of a topic
//   - knowledge.get       — get full content of a knowledge item
//   - knowledge.search    — search across all sources by keyword
//   - knowledge.primitives — list primitives, optionally filtered by layer
//   - knowledge.grammar   — get grammar ops for a product layer
//
// Configuration:
//
//	HIVE_DIR    — path to hive repo (default: cwd)
//	SITE_DIR    — path to site repo (default: ../site)
//	WORKSPACE   — path to workspace root (default: parent of HIVE_DIR)
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ─── JSON-RPC 2.0 ───────────────────────────────────────────────────────

type rpcRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
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

// ─── MCP types ───────────────────────────────────────────────────────────

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

// ─── Knowledge tree ──────────────────────────────────────────────────────

// topic is a node in the knowledge tree.
type topic struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"` // "category", "file", "primitive", "grammar", "post"
	Summary  string  `json:"summary,omitempty"`
	Children []topic `json:"children,omitempty"`
	Path     string  `json:"path,omitempty"` // file path for "file" kind
}

type knowledgeServer struct {
	hiveDir   string
	siteDir   string
	workspace string
	tree      []topic // top-level categories
}

func newKnowledgeServer() *knowledgeServer {
	hiveDir := envOr("HIVE_DIR", ".")
	siteDir := envOr("SITE_DIR", filepath.Join(hiveDir, "..", "site"))
	workspace := envOr("WORKSPACE", filepath.Join(hiveDir, ".."))

	abs := func(p string) string {
		a, _ := filepath.Abs(p)
		return a
	}

	s := &knowledgeServer{
		hiveDir:   abs(hiveDir),
		siteDir:   abs(siteDir),
		workspace: abs(workspace),
	}
	s.buildTree()
	return s
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// buildTree constructs the knowledge hierarchy from the filesystem.
func (s *knowledgeServer) buildTree() {
	s.tree = []topic{
		s.buildOntology(),
		s.buildArchitecture(),
		s.buildHiveLoop(),
		s.buildBlog(),
		s.buildAgents(),
		s.buildDocs(),
	}
}

func (s *knowledgeServer) buildOntology() topic {
	t := topic{ID: "ontology", Name: "Ontology", Kind: "category", Summary: "201 primitives, 13 layers, 13 grammars — the architecture's foundation"}

	// Layers
	layersDir := filepath.Join(s.siteDir, "content", "reference", "fundamentals")
	if entries, err := os.ReadDir(layersDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			t.Children = append(t.Children, topic{
				ID: "ontology/" + name, Name: name, Kind: "file",
				Summary: "Layer definition + primitives",
				Path:    filepath.Join(layersDir, e.Name()),
			})
		}
	}

	// Grammars
	grammarsDir := filepath.Join(s.siteDir, "content", "reference", "grammars")
	if entries, err := os.ReadDir(grammarsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			t.Children = append(t.Children, topic{
				ID: "ontology/grammar-" + name, Name: "Grammar: " + name, Kind: "grammar",
				Summary: "Domain grammar operations",
				Path:    filepath.Join(grammarsDir, e.Name()),
			})
		}
	}

	// Standalone reference files
	for _, name := range []string{"grammar.md", "cognitive-grammar.md", "higher-order-ops.md", "code-graph.md", "agent-primitives.md"} {
		p := filepath.Join(s.siteDir, "content", "reference", name)
		if _, err := os.Stat(p); err == nil {
			t.Children = append(t.Children, topic{
				ID: "ontology/" + strings.TrimSuffix(name, ".md"), Name: strings.TrimSuffix(name, ".md"), Kind: "file",
				Path: p,
			})
		}
	}

	return t
}

func (s *knowledgeServer) buildArchitecture() topic {
	t := topic{ID: "architecture", Name: "Architecture", Kind: "category", Summary: "How the system works — repos, code structure, CLAUDE.md files"}

	// CLAUDE.md from each repo
	repos := []string{"hive", "site", "eventgraph", "agent", "work"}
	for _, repo := range repos {
		p := filepath.Join(s.workspace, repo, "CLAUDE.md")
		if _, err := os.Stat(p); err == nil {
			t.Children = append(t.Children, topic{
				ID: "architecture/" + repo, Name: repo + " (CLAUDE.md)", Kind: "file",
				Summary: repo + " repo architecture and conventions",
				Path:    p,
			})
		}
	}

	// eventgraph decision tree
	decisionDir := filepath.Join(s.workspace, "eventgraph", "go", "pkg", "decision")
	if _, err := os.Stat(decisionDir); err == nil {
		t.Children = append(t.Children, topic{
			ID: "architecture/decision-tree", Name: "Decision Tree Engine", Kind: "category",
			Summary: "eventgraph/go/pkg/decision/ — tree.go, evaluate.go, evolve.go. The pipeline should run on this.",
		})
	}

	return t
}

func (s *knowledgeServer) buildHiveLoop() topic {
	t := topic{ID: "loop", Name: "Hive Loop", Kind: "category", Summary: "Pipeline state, backlog, reflections, reports — current operational state"}

	files := []struct{ name, summary string }{
		{"state.md", "Current state, lessons, scout directive"},
		{"backlog.md", "Ideas, directions, futures — the PM reads this"},
		{"reflections.md", "Append-only iteration reflections (COVER/BLIND/ZOOM/FORMALIZE)"},
		{"scout.md", "Latest scout gap report"},
		{"build.md", "Latest build report"},
		{"critique.md", "Latest critique report"},
		{"council.md", "Latest council deliberation"},
	}
	for _, f := range files {
		p := filepath.Join(s.hiveDir, "loop", f.name)
		if _, err := os.Stat(p); err == nil {
			t.Children = append(t.Children, topic{
				ID: "loop/" + strings.TrimSuffix(f.name, ".md"), Name: f.name, Kind: "file",
				Summary: f.summary, Path: p,
			})
		}
	}

	return t
}

func (s *knowledgeServer) buildBlog() topic {
	t := topic{ID: "blog", Name: "Blog", Kind: "category", Summary: "Derivation stories, technical posts — the 'why' behind decisions"}

	postsDir := filepath.Join(s.siteDir, "content", "posts")
	if entries, err := os.ReadDir(postsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			// Extract title from first # line
			summary := extractFirstHeading(filepath.Join(postsDir, e.Name()))
			t.Children = append(t.Children, topic{
				ID: "blog/" + name, Name: name, Kind: "post",
				Summary: summary, Path: filepath.Join(postsDir, e.Name()),
			})
		}
	}

	return t
}

func (s *knowledgeServer) buildAgents() topic {
	t := topic{ID: "agents", Name: "Agents", Kind: "category", Summary: "Role prompts, CONTEXT.md — agent definitions and shared knowledge"}

	agentsDir := filepath.Join(s.hiveDir, "agents")
	if entries, err := os.ReadDir(agentsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			t.Children = append(t.Children, topic{
				ID: "agents/" + name, Name: name, Kind: "file",
				Path: filepath.Join(agentsDir, e.Name()),
			})
		}
	}

	return t
}

func (s *knowledgeServer) buildDocs() topic {
	t := topic{ID: "docs", Name: "Docs", Kind: "category", Summary: "Specs, design docs, migration plans"}

	docsDir := filepath.Join(s.hiveDir, "docs")
	if entries, err := os.ReadDir(docsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			summary := extractFirstHeading(filepath.Join(docsDir, e.Name()))
			t.Children = append(t.Children, topic{
				ID: "docs/" + name, Name: name, Kind: "file",
				Summary: summary, Path: filepath.Join(docsDir, e.Name()),
			})
		}
	}

	return t
}

// ─── Tool implementations ────────────────────────────────────────────────

func (s *knowledgeServer) handleTopics(args map[string]any) string {
	parent, _ := args["parent"].(string)

	var items []topic
	if parent == "" {
		// Top-level categories
		for _, t := range s.tree {
			items = append(items, topic{ID: t.ID, Name: t.Name, Kind: t.Kind, Summary: t.Summary})
		}
	} else {
		// Find the parent and return its children
		node := s.findTopic(parent)
		if node == nil {
			return fmt.Sprintf("Topic %q not found", parent)
		}
		items = node.Children
	}

	var sb strings.Builder
	for _, item := range items {
		sb.WriteString(fmt.Sprintf("- **%s** (%s)", item.Name, item.Kind))
		if item.Summary != "" {
			sb.WriteString(fmt.Sprintf(" — %s", item.Summary))
		}
		sb.WriteString(fmt.Sprintf("\n  ID: `%s`\n", item.ID))
	}
	if sb.Len() == 0 {
		return "(no children)"
	}
	return sb.String()
}

func (s *knowledgeServer) handleGet(args map[string]any) string {
	id, _ := args["id"].(string)
	if id == "" {
		return "Error: id is required"
	}

	node := s.findTopic(id)
	if node == nil {
		return fmt.Sprintf("Topic %q not found", id)
	}

	if node.Path == "" {
		// Category node — return summary + children list
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n%s\n\n## Children:\n", node.Name, node.Summary))
		for _, c := range node.Children {
			sb.WriteString(fmt.Sprintf("- %s (`%s`)\n", c.Name, c.ID))
		}
		return sb.String()
	}

	// File node — return content (bounded)
	data, err := os.ReadFile(node.Path)
	if err != nil {
		return fmt.Sprintf("Error reading %s: %v", node.Path, err)
	}
	content := string(data)
	if len(content) > 8000 {
		content = content[:8000] + "\n\n... (truncated at 8000 chars. Use knowledge.search for specific content.)"
	}
	return content
}

func (s *knowledgeServer) handleSearch(args map[string]any) string {
	query, _ := args["query"].(string)
	if query == "" {
		return "Error: query is required"
	}
	query = strings.ToLower(query)

	maxResults := 10
	var results []string

	// Search all file-backed topics
	s.walkTopics(s.tree, func(t *topic) {
		if len(results) >= maxResults {
			return
		}
		// Match on name, summary, or ID
		if strings.Contains(strings.ToLower(t.Name), query) ||
			strings.Contains(strings.ToLower(t.Summary), query) ||
			strings.Contains(strings.ToLower(t.ID), query) {
			results = append(results, fmt.Sprintf("- **%s** (`%s`) — %s", t.Name, t.ID, t.Summary))
			return
		}
		// Match on file content (first 4000 chars)
		if t.Path != "" {
			data, err := os.ReadFile(t.Path)
			if err == nil {
				content := strings.ToLower(string(data))
				if len(content) > 4000 {
					content = content[:4000]
				}
				if strings.Contains(content, query) {
					results = append(results, fmt.Sprintf("- **%s** (`%s`) — content match", t.Name, t.ID))
				}
			}
		}
	})

	if len(results) == 0 {
		return fmt.Sprintf("No results for %q", query)
	}
	return strings.Join(results, "\n")
}

func (s *knowledgeServer) handlePrimitives(args map[string]any) string {
	layer, _ := args["layer"].(string)

	// Find ontology layers and return primitive tables
	var results []string
	for _, t := range s.tree {
		if t.ID != "ontology" {
			continue
		}
		for _, child := range t.Children {
			if child.Kind != "file" || !strings.Contains(child.ID, "layer") {
				continue
			}
			if layer != "" && !strings.Contains(child.Name, layer) {
				continue
			}
			data, _ := os.ReadFile(child.Path)
			if data != nil {
				content := string(data)
				if len(content) > 3000 {
					content = content[:3000] + "\n..."
				}
				results = append(results, fmt.Sprintf("## %s\n\n%s", child.Name, content))
			}
		}
	}

	if len(results) == 0 {
		return "No primitive layers found" + func() string {
			if layer != "" {
				return fmt.Sprintf(" matching %q", layer)
			}
			return ""
		}()
	}
	return strings.Join(results, "\n\n---\n\n")
}

func (s *knowledgeServer) handleGrammar(args map[string]any) string {
	name, _ := args["name"].(string)
	if name == "" {
		return "Error: name is required (e.g. 'work', 'knowledge', 'social')"
	}

	for _, t := range s.tree {
		if t.ID != "ontology" {
			continue
		}
		for _, child := range t.Children {
			if child.Kind != "grammar" {
				continue
			}
			if strings.Contains(strings.ToLower(child.Name), strings.ToLower(name)) {
				data, err := os.ReadFile(child.Path)
				if err != nil {
					return fmt.Sprintf("Error: %v", err)
				}
				content := string(data)
				if len(content) > 6000 {
					content = content[:6000] + "\n..."
				}
				return content
			}
		}
	}
	return fmt.Sprintf("Grammar %q not found", name)
}

// ─── Tree helpers ────────────────────────────────────────────────────────

func (s *knowledgeServer) findTopic(id string) *topic {
	var found *topic
	s.walkTopics(s.tree, func(t *topic) {
		if t.ID == id {
			found = t
		}
	})
	return found
}

func (s *knowledgeServer) walkTopics(topics []topic, fn func(*topic)) {
	for i := range topics {
		fn(&topics[i])
		s.walkTopics(topics[i].Children, fn)
	}
}

func extractFirstHeading(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

// ─── Tool definitions ────────────────────────────────────────────────────

var tools = []toolDef{
	{
		Name:        "knowledge.topics",
		Description: "List knowledge categories or children of a topic. Call with no parent to see top-level categories. Use the returned IDs to drill deeper.",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"parent": {Type: "string", Description: "Topic ID to list children of. Empty for top-level categories."},
			},
		},
	},
	{
		Name:        "knowledge.get",
		Description: "Get the full content of a knowledge item by ID. Returns file content for files, or summary + children list for categories.",
		InputSchema: inputSchema{
			Type:     "object",
			Properties: map[string]schemaProp{
				"id": {Type: "string", Description: "Topic ID (e.g. 'ontology/layer-1-agency', 'blog/post45-agents-that-work', 'loop/backlog')."},
			},
			Required: []string{"id"},
		},
	},
	{
		Name:        "knowledge.search",
		Description: "Search across ALL knowledge sources by keyword. Searches names, summaries, and file content. Returns up to 10 matches.",
		InputSchema: inputSchema{
			Type:     "object",
			Properties: map[string]schemaProp{
				"query": {Type: "string", Description: "Search keyword (e.g. 'decision tree', 'council', 'Layer 3')."},
			},
			Required: []string{"query"},
		},
	},
	{
		Name:        "knowledge.primitives",
		Description: "List primitives from the 201-primitive ontology. Optionally filter by layer number or name.",
		InputSchema: inputSchema{
			Type: "object",
			Properties: map[string]schemaProp{
				"layer": {Type: "string", Description: "Filter by layer (e.g. '0', '1', 'agency', 'foundation'). Empty for all layers."},
			},
		},
	},
	{
		Name:        "knowledge.grammar",
		Description: "Get the grammar operations for a product layer (e.g. 'work', 'knowledge', 'social', 'market').",
		InputSchema: inputSchema{
			Type:     "object",
			Properties: map[string]schemaProp{
				"name": {Type: "string", Description: "Grammar name (e.g. 'work', 'knowledge', 'social')."},
			},
			Required: []string{"name"},
		},
	},
}

// ─── MCP protocol ────────────────────────────────────────────────────────

func main() {
	srv := newKnowledgeServer()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1 MiB buffer

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		var resp rpcResponse
		resp.JSONRPC = "2.0"
		if req.ID != nil {
			resp.ID = *req.ID
		}

		switch req.Method {
		case "initialize":
			resp.Result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":   map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{
					"name":    "mcp-knowledge",
					"version": "0.1.0",
				},
			}

		case "notifications/initialized":
			continue // no response needed

		case "tools/list":
			resp.Result = map[string]any{"tools": tools}

		case "tools/call":
			var params toolCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = &rpcError{Code: -32602, Message: "invalid params"}
			} else {
				resp.Result = srv.dispatch(params)
			}

		default:
			resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
		}

		out, _ := json.Marshal(resp)
		fmt.Fprintf(os.Stdout, "%s\n", out)
	}
}

func (s *knowledgeServer) dispatch(params toolCallParams) toolResult {
	var text string

	switch params.Name {
	case "knowledge.topics":
		text = s.handleTopics(params.Arguments)
	case "knowledge.get":
		text = s.handleGet(params.Arguments)
	case "knowledge.search":
		text = s.handleSearch(params.Arguments)
	case "knowledge.primitives":
		text = s.handlePrimitives(params.Arguments)
	case "knowledge.grammar":
		text = s.handleGrammar(params.Arguments)
	default:
		return toolResult{
			Content: []toolContent{{Type: "text", Text: "Unknown tool: " + params.Name}},
			IsError: true,
		}
	}

	return toolResult{Content: []toolContent{{Type: "text", Text: text}}}
}
