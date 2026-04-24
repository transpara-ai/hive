package localapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// NewServer creates an http.Handler that mirrors the lovyou.ai REST API
// surface, backed by a local Store. The apiKey is checked on every request
// via Bearer token in the Authorization header.
func NewServer(store *Store, apiKey string) http.Handler {
	s := &server{store: store, apiKey: apiKey}

	mux := http.NewServeMux()

	// Board / node routes.
	mux.HandleFunc("GET /app/{slug}/board", s.auth(s.handleBoard))
	mux.HandleFunc("POST /app/{slug}/op", s.auth(s.handleOp))
	mux.HandleFunc("GET /app/{slug}/node/{id}", s.auth(s.handleGetNode))
	mux.HandleFunc("GET /app/{slug}/documents", s.auth(s.handleDocuments))
	mux.HandleFunc("GET /app/{slug}/knowledge", s.auth(s.handleKnowledge))
	mux.HandleFunc("GET /app/{slug}/feed", s.auth(s.handleFeed))

	// Agent routes.
	mux.HandleFunc("GET /app/{slug}/people", s.auth(s.handleListAgents))
	mux.HandleFunc("PATCH /app/{slug}/agents/{name}/session", s.auth(s.handleUpdateSession))

	// Hive internal routes.
	mux.HandleFunc("POST /api/hive/escalation", s.auth(s.handleEscalation))
	mux.HandleFunc("POST /api/hive/diagnostic", s.auth(s.handleDiagnostic))

	// Health.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return mux
}

type server struct {
	store  *Store
	apiKey string
}

// auth wraps a handler with Bearer token verification.
func (s *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+s.apiKey {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

// ---------------------------------------------------------------------------
// Route handlers
// ---------------------------------------------------------------------------

func (s *server) handleBoard(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	spaceID := s.store.ResolveSpaceID(slug)
	nodes, err := s.store.ListNodes(spaceID, "", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []Node{}
	}
	nodes = filterExcludeDone(nodes)
	if assignee := r.URL.Query().Get("assignee"); assignee != "" {
		nodes = filterByAssignee(nodes, assignee)
	}
	s.store.enrichChildCounts(nodes)
	writeJSON(w, map[string]any{"nodes": nodes})
}

func (s *server) handleOp(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	spaceID := s.store.ResolveSpaceID(slug)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	op := stringField(m, "op", "")
	if op == "" {
		http.Error(w, `missing "op" field`, http.StatusBadRequest)
		return
	}

	// requireNodeID extracts node_id and returns "" after writing a 400 if
	// it is missing. Callers must early-return when the second value is false.
	requireNodeID := func() (string, bool) {
		nodeID := stringField(m, "node_id", "")
		if nodeID == "" {
			http.Error(w, `missing "node_id" field`, http.StatusBadRequest)
			return "", false
		}
		return nodeID, true
	}

	switch op {
	case OpIntend:
		kind := stringField(m, "kind", "task")
		n := Node{
			Kind:     kind,
			Title:    stringField(m, "title", ""),
			Body:     stringField(m, "description", ""),
			Priority: stringField(m, "priority", ""),
		}
		created, err := s.store.CreateNode(spaceID, n)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"op": OpIntend, "node": created})

	case OpComplete:
		nodeID, ok := requireNodeID()
		if !ok {
			return
		}
		if err := s.store.CompleteNode(spaceID, nodeID); err != nil {
			s.writeStoreErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"op": OpComplete, "status": "ok"})

	case OpOpen:
		nodeID, ok := requireNodeID()
		if !ok {
			return
		}
		if err := s.store.OpenNode(spaceID, nodeID); err != nil {
			s.writeStoreErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"op": OpOpen, "status": "ok"})

	case OpEdit:
		nodeID, ok := requireNodeID()
		if !ok {
			return
		}
		state := stringField(m, "state", "")
		if state == "" {
			http.Error(w, `missing "state" field`, http.StatusBadRequest)
			return
		}
		if err := s.store.UpdateNodeStateInSpace(spaceID, nodeID, state); err != nil {
			s.writeStoreErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"op": OpEdit, "status": "ok"})

	case OpClaim:
		nodeID, ok := requireNodeID()
		if !ok {
			return
		}
		if err := s.store.ClaimNode(spaceID, nodeID, "agent"); err != nil {
			s.writeStoreErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"op": OpClaim, "status": "ok"})

	case OpAssign:
		nodeID, ok := requireNodeID()
		if !ok {
			return
		}
		assignee := stringField(m, "assignee", "")
		if err := s.store.ClaimNode(spaceID, nodeID, assignee); err != nil {
			s.writeStoreErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"op": OpAssign, "status": "ok"})

	case OpRespond:
		n := Node{
			Kind:     "comment",
			ParentID: stringField(m, "parent_id", ""),
			Body:     stringField(m, "body", ""),
		}
		created, err := s.store.CreateNode(spaceID, n)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"op": OpRespond, "node": created})

	case OpExpress:
		n := Node{
			Kind:  "post",
			Title: stringField(m, "title", ""),
			Body:  stringField(m, "body", ""),
		}
		created, err := s.store.CreateNode(spaceID, n)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"op": OpExpress, "node": created})

	case OpAssert:
		n := Node{
			Kind:  "claim",
			Title: stringField(m, "title", ""),
			Body:  stringField(m, "body", ""),
		}
		created, err := s.store.CreateNode(spaceID, n)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"op": OpAssert, "node": created})

	case OpDiscuss:
		n := Node{
			Kind:  "thread",
			Title: stringField(m, "title", ""),
			Body:  stringField(m, "body", ""),
		}
		created, err := s.store.CreateNode(spaceID, n)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"op": OpDiscuss, "node": created})

	default:
		http.Error(w, "unknown op: "+op, http.StatusBadRequest)
	}
}

// writeStoreErr maps store errors to HTTP responses:
//   - ErrNotFound    → 404 (node does not exist or belongs to a different space)
//   - ErrInvalidState → 409 (state-specific precondition failed)
//   - anything else  → 500
func (s *server) writeStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrInvalidState):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) handleGetNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	node, err := s.store.GetNode(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, node)
}

func (s *server) handleDocuments(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	spaceID := s.store.ResolveSpaceID(slug)
	nodes, err := s.store.ListNodes(spaceID, "document", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []Node{}
	}
	writeJSON(w, map[string]any{"documents": nodes})
}

func (s *server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	spaceID := s.store.ResolveSpaceID(slug)

	if r.URL.Query().Get("op") == "max_lesson" {
		max := s.store.MaxLessonNumber(spaceID)
		writeJSON(w, map[string]any{"max_lesson": max})
		return
	}

	nodes, err := s.store.ListNodes(spaceID, "claim", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []Node{}
	}
	writeJSON(w, map[string]any{"claims": nodes})
}

func (s *server) handleFeed(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	spaceID := s.store.ResolveSpaceID(slug)
	nodes, err := s.store.ListNodes(spaceID, "post", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if nodes == nil {
		nodes = []Node{}
	}
	writeJSON(w, map[string]any{"posts": nodes})
}

func (s *server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.store.ListAgents()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if agents == nil {
		agents = []Agent{}
	}
	writeJSON(w, map[string]any{"agents": agents})
}

func (s *server) handleUpdateSession(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var body struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateAgentSession(name, body.SessionID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateNodeState(body.TaskID, NodeStateEscalated); err != nil {
		s.writeStoreErr(w, err)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

func (s *server) handleDiagnostic(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.InsertDiagnostic(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeJSON marshals v as JSON and writes it to w with application/json
// content type. Errors are written as 500.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// stringField extracts a string value from a map[string]any, returning
// fallback if the key is missing or not a string.
func stringField(m map[string]any, key, fallback string) string {
	v, ok := m[key]
	if !ok {
		return fallback
	}
	s, ok := v.(string)
	if !ok {
		return fallback
	}
	return s
}
