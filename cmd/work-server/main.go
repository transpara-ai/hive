// Command work-server is an HTTP REST API server for the Work Graph (Layer 1).
// It exposes task management as signed, auditable events on the shared event graph.
//
// Environment variables:
//
//	WORK_HUMAN   — display name of the human operator (required)
//	WORK_API_KEY — API key for auth; callers pass Authorization: Bearer <key> (required)
//	DATABASE_URL — Postgres DSN (optional; defaults to in-memory)
//	PORT         — HTTP port to listen on (optional; defaults to 8080)
//
// Endpoints:
//
//	GET  /                         read-only dashboard (HTML, no auth required)
//	POST /tasks                    create a task
//	GET  /tasks                    list tasks (?open=true, ?priority=high, ?assignee=<actor_id>)
//	GET  /tasks/{id}               get full task details (title, description, priority, status, assignee, blocked)
//	GET  /tasks/{id}/status        get task status
//	GET  /tasks/{id}/events        get audit trail (ordered work.task.* events for this task, including comments)
//	POST /tasks/{id}/assign        assign task (body: {"assignee":"..."})
//	POST /tasks/{id}/complete      complete task (body: {"summary":"..."})
//	POST /tasks/{id}/comment       add a comment (body: {"body":"..."})
//	GET  /tasks/{id}/comments      list comments for a task
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/actor/pgactor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/work"
)

// dashboardHTML is the read-only monitoring dashboard served at GET /.
// The placeholder {{API_KEY}} is replaced at serve time with the actual key so
// the browser's fetch() calls can authenticate against GET /tasks.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Work Graph — Live Dashboard</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: system-ui, -apple-system, sans-serif; background: #0f1117; color: #e2e8f0; min-height: 100vh; padding: 2rem; }
h1 { font-size: 1.5rem; font-weight: 600; color: #f8fafc; margin-bottom: 0.25rem; }
.subtitle { font-size: 0.875rem; color: #64748b; margin-bottom: 1.5rem; }
.meta { display: flex; align-items: center; gap: 1rem; margin-bottom: 1.25rem; font-size: 0.8125rem; color: #64748b; }
.dot { width: 8px; height: 8px; border-radius: 50%; background: #22c55e; animation: pulse 2s ease-in-out infinite; }
@keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.4; } }
.error { background: #3b1010; color: #fca5a5; padding: 0.75rem 1rem; border-radius: 6px; margin-bottom: 1rem; font-size: 0.875rem; }
.empty { color: #475569; font-size: 0.875rem; padding: 2rem 0; text-align: center; }
table { width: 100%; border-collapse: collapse; font-size: 0.875rem; }
th { text-align: left; padding: 0.5rem 0.75rem; color: #64748b; font-weight: 500; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; border-bottom: 1px solid #1e293b; }
td { padding: 0.625rem 0.75rem; border-bottom: 1px solid #1e293b; vertical-align: middle; }
tr:hover td { background: #1e293b; }
.title { font-weight: 500; color: #f1f5f9; max-width: 28rem; }
.desc { font-size: 0.75rem; color: #64748b; margin-top: 0.125rem; }
.badge { display: inline-block; padding: 0.2em 0.55em; border-radius: 4px; font-size: 0.72rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.03em; }
.s-open       { background: #1e3a5f; color: #60a5fa; }
.s-in_progress{ background: #3b2400; color: #fb923c; }
.s-completed  { background: #052e16; color: #4ade80; }
.s-blocked    { background: #3b0a0a; color: #f87171; }
.p-high   { background: #3b0a0a; color: #f87171; }
.p-medium { background: #3b2400; color: #fb923c; }
.p-low    { background: #1e293b; color: #94a3b8; }
.blocked-yes { color: #f87171; font-weight: 600; }
.blocked-no  { color: #334155; }
.assignee { font-family: monospace; font-size: 0.75rem; color: #7c3aed; max-width: 12rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
</head>
<body>
<h1>Work Graph</h1>
<p class="subtitle">Live pipeline dashboard — read-only</p>
<div class="meta">
  <span class="dot"></span>
  <span id="status-line">Connecting...</span>
  <span id="countdown"></span>
</div>
<div id="error-box" class="error" style="display:none"></div>
<table id="task-table" style="display:none">
  <thead>
    <tr>
      <th>Task</th>
      <th>Status</th>
      <th>Priority</th>
      <th>Assignee</th>
      <th>Blocked</th>
    </tr>
  </thead>
  <tbody id="task-body"></tbody>
</table>
<div id="empty-msg" class="empty" style="display:none">No tasks yet.</div>
<script>
const API_KEY = "{{API_KEY}}";
const REFRESH_MS = 10000;
let timer, countdown, nextAt;

function esc(s) {
  return String(s).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;");
}

function badge(cls, text) {
  return '<span class="badge ' + cls + '">' + esc(text) + '</span>';
}

function statusBadge(s) {
  const map = { open: "s-open", in_progress: "s-in_progress", completed: "s-completed", blocked: "s-blocked" };
  return badge(map[s] || "s-open", s || "open");
}

function priorityBadge(p) {
  const map = { high: "p-high", medium: "p-medium", low: "p-low" };
  return badge(map[p] || "p-low", p || "");
}

function shortID(id) {
  return id ? id.slice(0, 8) + "\u2026" : "\u2014";
}

async function refresh() {
  try {
    const res = await fetch("/tasks", { headers: { Authorization: "Bearer " + API_KEY } });
    if (!res.ok) throw new Error("HTTP " + res.status);
    const data = await res.json();
    const tasks = data.tasks || [];
    document.getElementById("error-box").style.display = "none";

    const tbody = document.getElementById("task-body");
    if (tasks.length === 0) {
      document.getElementById("task-table").style.display = "none";
      document.getElementById("empty-msg").style.display = "block";
    } else {
      document.getElementById("task-table").style.display = "table";
      document.getElementById("empty-msg").style.display = "none";
      tbody.innerHTML = tasks.map(t => {
        const blockedCell = t.blocked
          ? '<span class="blocked-yes">\u26a0 blocked</span>'
          : '<span class="blocked-no">\u2014</span>';
        const assigneeCell = t.assignee
          ? '<span class="assignee" title="' + esc(t.assignee) + '">' + esc(shortID(t.assignee)) + '</span>'
          : '<span class="blocked-no">\u2014</span>';
        return '<tr>'
          + '<td><div class="title">' + esc(t.title) + '</div>'
          + (t.description ? '<div class="desc">' + esc(t.description.slice(0, 80)) + (t.description.length > 80 ? "\u2026" : "") + '</div>' : '')
          + '</td>'
          + '<td>' + statusBadge(t.status) + '</td>'
          + '<td>' + priorityBadge(t.priority) + '</td>'
          + '<td>' + assigneeCell + '</td>'
          + '<td>' + blockedCell + '</td>'
          + '</tr>';
      }).join("");
    }

    const now = new Date();
    document.getElementById("status-line").textContent =
      "Updated " + now.toLocaleTimeString() + " \u2014 " + tasks.length + " task" + (tasks.length === 1 ? "" : "s");
    nextAt = Date.now() + REFRESH_MS;
  } catch (err) {
    const box = document.getElementById("error-box");
    box.textContent = "Fetch failed: " + err.message;
    box.style.display = "block";
    document.getElementById("status-line").textContent = "Error \u2014 retrying in 10s";
    nextAt = Date.now() + REFRESH_MS;
  }
}

function tick() {
  const secs = Math.max(0, Math.round((nextAt - Date.now()) / 1000));
  document.getElementById("countdown").textContent = secs > 0 ? "(next refresh in " + secs + "s)" : "";
}

refresh();
setInterval(refresh, REFRESH_MS);
setInterval(tick, 1000);
</script>
</body>
</html>`

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	humanName := os.Getenv("WORK_HUMAN")
	if humanName == "" {
		return fmt.Errorf("WORK_HUMAN env var is required (display name of the human operator)")
	}
	apiKey := os.Getenv("WORK_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("WORK_API_KEY env var is required")
	}
	dsn := os.Getenv("DATABASE_URL")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Open shared pool for Postgres, or nil for in-memory.
	var pool *pgxpool.Pool
	if dsn != "" {
		fmt.Fprintf(os.Stderr, "Postgres: %s\n", dsn)
		var err error
		pool, err = pgxpool.New(ctx, dsn)
		if err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
		defer pool.Close()
	}

	s, err := openStore(ctx, pool)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "store close: %v\n", err)
		}
	}()

	actors, err := openActorStore(ctx, pool)
	if err != nil {
		return fmt.Errorf("actor store: %w", err)
	}

	// Bootstrap human actor — same key-derivation pattern as cmd/hive.
	if pool != nil {
		fmt.Fprintln(os.Stderr, "WARNING: CLI key derivation is insecure for persistent Postgres stores.")
		fmt.Fprintln(os.Stderr, "         Production should use Google auth. Proceeding for development.")
	}
	humanID, err := registerHuman(actors, humanName)
	if err != nil {
		return fmt.Errorf("register human: %w", err)
	}

	// Register work event type unmarshalers before any store reads —
	// Head() deserializes the latest event which may be a work type.
	work.RegisterEventTypes()

	// Bootstrap the event graph if it has no genesis event.
	if err := bootstrapGraph(s, humanID); err != nil {
		return fmt.Errorf("bootstrap graph: %w", err)
	}

	// Build factory and signer for work events.
	registry := event.DefaultRegistry()
	work.RegisterWithRegistry(registry)
	factory := event.NewEventFactory(registry)
	signer := deriveSignerFromID(humanID)

	ts := work.NewTaskStore(s, factory, signer)

	srv := &server{
		ts:      ts,
		store:   s,
		humanID: humanID,
		apiKey:  apiKey,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", srv.dashboard)
	mux.HandleFunc("GET /health", srv.health)
	mux.HandleFunc("POST /tasks", srv.auth(srv.createTask))
	mux.HandleFunc("GET /tasks", srv.auth(srv.listTasks))
	mux.HandleFunc("GET /tasks/{id}", srv.auth(srv.getTask))
	mux.HandleFunc("GET /tasks/{id}/status", srv.auth(srv.getTaskStatus))
	mux.HandleFunc("GET /tasks/{id}/events", srv.auth(srv.getTaskEvents))
	mux.HandleFunc("POST /tasks/{id}/assign", srv.auth(srv.assignTask))
	mux.HandleFunc("POST /tasks/{id}/complete", srv.auth(srv.completeTask))
	mux.HandleFunc("POST /tasks/{id}/comment", srv.auth(srv.addComment))
	mux.HandleFunc("GET /tasks/{id}/comments", srv.auth(srv.listComments))

	addr := ":" + port
	fmt.Fprintf(os.Stderr, "work-server listening on %s\n", addr)
	httpSrv := &http.Server{Addr: addr, Handler: corsMiddleware(mux)}
	go func() {
		<-ctx.Done()
		httpSrv.Shutdown(context.Background()) //nolint:errcheck
	}()
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}

// server holds shared dependencies for HTTP handlers.
type server struct {
	ts      *work.TaskStore
	store   store.Store
	humanID types.ActorID
	apiKey  string
}

// dashboard handles GET / — serves the read-only HTML monitoring dashboard.
// No auth required; the API key is injected into the page so the browser's
// fetch() calls can authenticate against GET /tasks.
func (sv *server) dashboard(w http.ResponseWriter, r *http.Request) {
	html := strings.ReplaceAll(dashboardHTML, "{{API_KEY}}", sv.apiKey)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

// health handles GET /health — used by Fly.io and load balancers to check liveness.
func (sv *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// corsMiddleware adds Access-Control-Allow-Origin headers so the REST API can be
// called directly from web browsers. Handles preflight OPTIONS requests.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// auth is middleware that validates the Authorization: Bearer <key> header.
func (sv *server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || token != sv.apiKey {
			writeErr(w, http.StatusUnauthorized, "invalid or missing API key")
			return
		}
		next(w, r)
	}
}

// createTask handles POST /tasks
// Body: {"title":"...", "description":"...", "priority":"high"}
func (sv *server) createTask(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Priority    string `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Title == "" {
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}
	causes, err := sv.currentCauses()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get causes: "+err.Error())
		return
	}
	convID, err := newConversationID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "conversation id: "+err.Error())
		return
	}
	task, err := sv.ts.Create(sv.humanID, body.Title, body.Description, causes, convID, work.TaskPriority(body.Priority))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create task: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          task.ID.Value(),
		"title":       task.Title,
		"description": task.Description,
		"priority":    string(task.Priority),
		"created_by":  task.CreatedBy.Value(),
	})
}

// listTasks handles GET /tasks
// Query params: ?open=true, ?priority=high, ?assignee=<actor_id>
func (sv *server) listTasks(w http.ResponseWriter, r *http.Request) {
	openOnly := r.URL.Query().Get("open") == "true"
	priorityFilter := r.URL.Query().Get("priority")
	assigneeFilter := r.URL.Query().Get("assignee")

	summaries, err := sv.ts.ListSummaries(100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list tasks: "+err.Error())
		return
	}

	if openOnly {
		filtered := make([]work.TaskSummary, 0, len(summaries))
		for _, s := range summaries {
			if s.Status != work.StatusCompleted && !s.Blocked {
				filtered = append(filtered, s)
			}
		}
		summaries = filtered
	}

	if assigneeFilter != "" {
		aid, err := types.NewActorID(assigneeFilter)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid assignee: "+err.Error())
			return
		}
		filtered := make([]work.TaskSummary, 0, len(summaries))
		for _, s := range summaries {
			if s.Assignee == aid {
				filtered = append(filtered, s)
			}
		}
		summaries = filtered
	}

	if priorityFilter != "" {
		p := work.TaskPriority(priorityFilter)
		filtered := make([]work.TaskSummary, 0, len(summaries))
		for _, s := range summaries {
			if s.Task.Priority == p {
				filtered = append(filtered, s)
			}
		}
		summaries = filtered
	}

	items := make([]map[string]any, 0, len(summaries))
	for _, s := range summaries {
		items = append(items, map[string]any{
			"id":          s.Task.ID.Value(),
			"title":       s.Task.Title,
			"description": s.Task.Description,
			"priority":    string(s.Task.Priority),
			"created_by":  s.Task.CreatedBy.Value(),
			"status":      string(s.Status),
			"assignee":    s.Assignee.Value(),
			"blocked":     s.Blocked,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": items})
}

// getTaskStatus handles GET /tasks/{id}/status
func (sv *server) getTaskStatus(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseTaskID(w, r)
	if !ok {
		return
	}
	status, err := sv.ts.GetStatus(taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get status: "+err.Error())
		return
	}
	priority, err := sv.ts.GetPriority(taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get priority: "+err.Error())
		return
	}
	blocked, err := sv.ts.IsBlocked(taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "blocked check: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       taskID.Value(),
		"status":   string(status),
		"priority": string(priority),
		"blocked":  blocked,
	})
}

// assignTask handles POST /tasks/{id}/assign
// Body: {"assignee":"actor_id"} — omit assignee to assign to the human operator.
func (sv *server) assignTask(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseTaskID(w, r)
	if !ok {
		return
	}
	var body struct {
		Assignee string `json:"assignee"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	assignee := sv.humanID
	if body.Assignee != "" {
		aid, err := types.NewActorID(body.Assignee)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid assignee: "+err.Error())
			return
		}
		assignee = aid
	}
	causes, err := sv.currentCauses()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get causes: "+err.Error())
		return
	}
	convID, err := newConversationID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "conversation id: "+err.Error())
		return
	}
	if err := sv.ts.Assign(sv.humanID, taskID, assignee, causes, convID); err != nil {
		writeErr(w, http.StatusInternalServerError, "assign: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id":  taskID.Value(),
		"assignee": assignee.Value(),
	})
}

// completeTask handles POST /tasks/{id}/complete
// Body: {"summary":"..."}
func (sv *server) completeTask(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseTaskID(w, r)
	if !ok {
		return
	}
	var body struct {
		Summary string `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	causes, err := sv.currentCauses()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get causes: "+err.Error())
		return
	}
	convID, err := newConversationID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "conversation id: "+err.Error())
		return
	}
	if err := sv.ts.Complete(sv.humanID, taskID, body.Summary, causes, convID); err != nil {
		writeErr(w, http.StatusInternalServerError, "complete: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"task_id": taskID.Value(),
		"status":  "completed",
	})
}

// addComment handles POST /tasks/{id}/comment
// Body: {"body":"..."}
func (sv *server) addComment(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseTaskID(w, r)
	if !ok {
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Body == "" {
		writeErr(w, http.StatusBadRequest, "body is required")
		return
	}
	causes, err := sv.currentCauses()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get causes: "+err.Error())
		return
	}
	convID, err := newConversationID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "conversation id: "+err.Error())
		return
	}
	if err := sv.ts.AddComment(taskID, body.Body, sv.humanID, causes, convID); err != nil {
		writeErr(w, http.StatusInternalServerError, "add comment: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"task_id": taskID.Value(),
		"status":  "commented",
	})
}

// listComments handles GET /tasks/{id}/comments
// Returns all comments for the task in chronological order.
func (sv *server) listComments(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseTaskID(w, r)
	if !ok {
		return
	}
	comments, err := sv.ts.ListComments(taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list comments: "+err.Error())
		return
	}
	items := make([]map[string]any, 0, len(comments))
	for _, c := range comments {
		items = append(items, map[string]any{
			"id":        c.ID.Value(),
			"task_id":   c.TaskID.Value(),
			"body":      c.Body,
			"author_id": c.AuthorID.Value(),
			"timestamp": c.Timestamp.String(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"task_id": taskID.Value(), "comments": items})
}

// getTask handles GET /tasks/{id}
// Returns full task details: title, description, priority, status, assignee, blocked.
func (sv *server) getTask(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseTaskID(w, r)
	if !ok {
		return
	}

	// Fetch the creation event for base task fields.
	ev, err := sv.store.Get(taskID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "task not found: "+err.Error())
		return
	}
	c, ok := ev.Content().(work.TaskCreatedContent)
	if !ok {
		writeErr(w, http.StatusNotFound, "event is not a task")
		return
	}

	status, err := sv.ts.GetStatus(taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get status: "+err.Error())
		return
	}
	priority, err := sv.ts.GetPriority(taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get priority: "+err.Error())
		return
	}
	blocked, err := sv.ts.IsBlocked(taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "blocked check: "+err.Error())
		return
	}

	// Find current assignee: assigned events are returned newest-first, so the first match wins.
	var assignee string
	assignedPage, err := sv.store.ByType(work.EventTypeTaskAssigned, 1000, types.None[types.Cursor]())
	if err == nil {
		for _, ae := range assignedPage.Items() {
			ac, ok := ae.Content().(work.TaskAssignedContent)
			if ok && ac.TaskID == taskID {
				assignee = ac.AssignedTo.Value()
				break
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          taskID.Value(),
		"title":       c.Title,
		"description": c.Description,
		"priority":    string(priority),
		"status":      string(status),
		"created_by":  c.CreatedBy.Value(),
		"assignee":    assignee,
		"blocked":     blocked,
	})
}

// getTaskEvents handles GET /tasks/{id}/events
// Returns the ordered audit trail of all work.task.* events causally linked to this task.
func (sv *server) getTaskEvents(w http.ResponseWriter, r *http.Request) {
	taskID, ok := parseTaskID(w, r)
	if !ok {
		return
	}

	var collected []event.Event

	// Include the task creation event itself.
	if ev, err := sv.store.Get(taskID); err == nil {
		if _, ok := ev.Content().(work.TaskCreatedContent); ok {
			collected = append(collected, ev)
		}
	}

	// Scan all other work event types for events that reference this task.
	for _, et := range []types.EventType{
		work.EventTypeTaskAssigned,
		work.EventTypeTaskCompleted,
		work.EventTypeTaskDependencyAdded,
		work.EventTypeTaskPrioritySet,
		work.EventTypeTaskComment,
	} {
		page, err := sv.store.ByType(et, 1000, types.None[types.Cursor]())
		if err != nil {
			continue
		}
		for _, ev := range page.Items() {
			if taskIDFromContent(ev.Content()) == taskID {
				collected = append(collected, ev)
			}
		}
	}

	// Sort chronologically (oldest first) for a readable audit trail.
	sort.Slice(collected, func(i, j int) bool {
		return collected[i].Timestamp().Value().Before(collected[j].Timestamp().Value())
	})

	items := make([]map[string]any, 0, len(collected))
	for _, ev := range collected {
		items = append(items, map[string]any{
			"id":        ev.ID().Value(),
			"type":      ev.Type().Value(),
			"source":    ev.Source().Value(),
			"timestamp": ev.Timestamp().String(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"task_id": taskID.Value(), "events": items})
}

// taskIDFromContent extracts the TaskID field from a work event content struct.
// Returns zero value EventID if the content type does not reference a task ID.
func taskIDFromContent(content any) types.EventID {
	switch c := content.(type) {
	case work.TaskAssignedContent:
		return c.TaskID
	case work.TaskCompletedContent:
		return c.TaskID
	case work.TaskDependencyContent:
		return c.TaskID
	case work.TaskPrioritySetContent:
		return c.TaskID
	case work.CommentContent:
		return c.TaskID
	}
	return types.EventID{}
}

// --- Helpers ---

// currentCauses fetches the current graph head to use as a cause for new events.
func (sv *server) currentCauses() ([]types.EventID, error) {
	head, err := sv.store.Head()
	if err != nil {
		return nil, err
	}
	if head.IsSome() {
		return []types.EventID{head.Unwrap().ID()}, nil
	}
	return nil, nil
}

// parseTaskID extracts and validates the {id} path parameter from the request.
func parseTaskID(w http.ResponseWriter, r *http.Request) (types.EventID, bool) {
	idStr := r.PathValue("id")
	if idStr == "" {
		writeErr(w, http.StatusBadRequest, "task id is required")
		return types.EventID{}, false
	}
	taskID, err := types.NewEventID(idStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid task id: "+err.Error())
		return types.EventID{}, false
	}
	return taskID, true
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// writeErr writes a JSON error response.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- Infrastructure helpers (mirror of cmd/work patterns) ---

func openStore(ctx context.Context, pool *pgxpool.Pool) (store.Store, error) {
	if pool == nil {
		fmt.Fprintln(os.Stderr, "Store: in-memory")
		return store.NewInMemoryStore(), nil
	}
	fmt.Fprintln(os.Stderr, "Store: postgres")
	return pgstore.NewPostgresStoreFromPool(ctx, pool)
}

func openActorStore(ctx context.Context, pool *pgxpool.Pool) (actor.IActorStore, error) {
	if pool == nil {
		fmt.Fprintln(os.Stderr, "Actor store: in-memory")
		return actor.NewInMemoryActorStore(), nil
	}
	fmt.Fprintln(os.Stderr, "Actor store: postgres")
	return pgactor.NewPostgresActorStoreFromPool(ctx, pool)
}

// bootstrapGraph emits the genesis event if the store is empty. Idempotent.
func bootstrapGraph(s store.Store, humanID types.ActorID) error {
	head, err := s.Head()
	if err != nil {
		return fmt.Errorf("check head: %w", err)
	}
	if head.IsSome() {
		return nil // already bootstrapped
	}
	fmt.Fprintln(os.Stderr, "Bootstrapping event graph...")
	registry := event.DefaultRegistry()
	bsFactory := event.NewBootstrapFactory(registry)
	bsSigner := &bootstrapSigner{humanID: humanID}
	bootstrap, err := bsFactory.Init(humanID, bsSigner)
	if err != nil {
		return fmt.Errorf("create genesis event: %w", err)
	}
	if _, err := s.Append(bootstrap); err != nil {
		return fmt.Errorf("append genesis event: %w", err)
	}
	fmt.Fprintln(os.Stderr, "Event graph bootstrapped.")
	return nil
}

// bootstrapSigner provides a minimal Signer for the genesis event.
type bootstrapSigner struct {
	humanID types.ActorID
}

func (b *bootstrapSigner) Sign(data []byte) (types.Signature, error) {
	h := sha256.Sum256([]byte("signer:" + b.humanID.Value()))
	priv := ed25519.NewKeyFromSeed(h[:])
	sig := ed25519.Sign(priv, data)
	return types.NewSignature(sig)
}

// registerHuman bootstraps a human operator in the actor store.
// WARNING: derives key from display name — insecure for production persistent stores.
// Mirrors cmd/hive registerHuman exactly so the same name produces the same ActorID.
func registerHuman(actors actor.IActorStore, displayName string) (types.ActorID, error) {
	h := sha256.Sum256([]byte("human:" + displayName))
	priv := ed25519.NewKeyFromSeed(h[:])
	pub := priv.Public().(ed25519.PublicKey)
	pk, err := types.NewPublicKey([]byte(pub))
	if err != nil {
		return types.ActorID{}, fmt.Errorf("public key: %w", err)
	}
	a, err := actors.Register(pk, displayName, event.ActorTypeHuman)
	if err != nil {
		return types.ActorID{}, err
	}
	return a.ID(), nil
}

// ed25519Signer implements event.Signer for work-emitted events.
type ed25519Signer struct {
	key ed25519.PrivateKey
}

func (s *ed25519Signer) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}

// deriveSignerFromID creates a deterministic Ed25519 signer from an ActorID.
// Stable across restarts — the same humanID always produces the same key.
func deriveSignerFromID(id types.ActorID) *ed25519Signer {
	h := sha256.Sum256([]byte("signer:" + id.Value()))
	priv := ed25519.NewKeyFromSeed(h[:])
	return &ed25519Signer{key: priv}
}

// newConversationID generates a unique ConversationID for this HTTP request.
func newConversationID() (types.ConversationID, error) {
	id, err := types.NewEventIDFromNew()
	if err != nil {
		return types.ConversationID{}, err
	}
	return types.NewConversationID("work-server-" + id.Value())
}
