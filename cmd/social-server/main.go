// Command social-server is an HTTP REST API server for the Social Graph (Layer 3).
// It exposes user-owned social as signed, auditable events on the shared event graph.
//
// Environment variables:
//
//	SOCIAL_HUMAN      — display name of the human operator (defaults to "operator")
//	SOCIAL_API_KEY    — API key for auth; callers pass Authorization: Bearer <key> (required)
//	SOCIAL_API_TOKEN  — bearer token for workspace-scoped external API; falls back to SOCIAL_API_KEY if unset
//	DATABASE_URL      — Postgres DSN (optional; defaults to in-memory)
//	PORT              — HTTP port to listen on (optional; defaults to 8082)
//
// Endpoints:
//
//	GET  /                                          read-only dashboard (HTML, no auth required)
//	GET  /health                                    liveness probe
//	POST /posts                                     create a post
//	GET  /posts                                     list posts
//	GET  /posts/{id}                                get a post by ID
//	POST /actors/{id}/follow                        follow an actor
//	GET  /actors/{id}/followers                     list followers of an actor
//	GET  /actors/{id}/following                     list actors an actor is following
//	POST /posts/{id}/react                          react to a post
//
// Workspace-scoped routes (authenticated via SOCIAL_API_TOKEN):
//
//	GET  /w/{workspace}                             workspace social dashboard (HTML, no auth required)
//	POST /w/{workspace}/posts                       create a post in the workspace
//	GET  /w/{workspace}/posts                       list posts in the workspace
//	GET  /w/{workspace}/posts/{id}                  get a post by ID in the workspace
//	POST /w/{workspace}/actors/{id}/follow          follow an actor in the workspace
//	GET  /w/{workspace}/actors/{id}/followers       list followers in the workspace
//	GET  /w/{workspace}/actors/{id}/following       list following in the workspace
//	POST /w/{workspace}/posts/{id}/react            react to a post in the workspace
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
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/transpara-ai/eventgraph/go/pkg/actor"
	"github.com/transpara-ai/eventgraph/go/pkg/actor/pgactor"
	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/store"
	"github.com/transpara-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/transpara-ai/eventgraph/go/pkg/types"

	"github.com/transpara-ai/hive/pkg/social"
)

// dashboardHTML is the read-only social dashboard served at GET /.
// The placeholder {{API_KEY}} is replaced at serve time with the actual key so
// the browser's fetch() calls can authenticate against the API.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Social Graph — Feed</title>
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
.post { background: #1e293b; border-radius: 8px; padding: 1.25rem; margin-bottom: 1rem; }
.post-author { font-family: monospace; font-size: 0.8125rem; color: #7c3aed; margin-bottom: 0.5rem; }
.post-body { font-size: 0.9375rem; color: #e2e8f0; line-height: 1.5; margin-bottom: 0.75rem; }
.post-footer { display: flex; align-items: center; gap: 1rem; flex-wrap: wrap; }
.tag { display: inline-block; padding: 0.15em 0.5em; border-radius: 4px; font-size: 0.72rem; font-weight: 600; background: #1e3a5f; color: #60a5fa; margin-right: 0.25rem; }
.reactions { font-size: 0.875rem; color: #94a3b8; }
.ts { font-size: 0.75rem; color: #475569; margin-left: auto; }
</style>
</head>
<body>
<h1>Social Graph</h1>
<p class="subtitle">User-owned social — Layer 3</p>
<div class="meta">
  <span class="dot"></span>
  <span id="status-line">Connecting...</span>
</div>
<div id="error-box" class="error" style="display:none"></div>
<div id="content"></div>
<script>
const API_KEY = "{{API_KEY}}";
const REFRESH_MS = 15000;

function esc(s) {
  return String(s).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;");
}

function shortID(id) {
  return id ? id.slice(0, 8) + "\u2026" : "\u2014";
}

async function fetchJSON(path) {
  const res = await fetch(path, { headers: { Authorization: "Bearer " + API_KEY } });
  if (!res.ok) throw new Error("HTTP " + res.status);
  return res.json();
}

async function refresh() {
  try {
    const data = await fetchJSON("/posts");
    document.getElementById("error-box").style.display = "none";
    document.getElementById("status-line").textContent = "Live \u2014 " + new Date().toLocaleTimeString();
    const posts = data.posts || [];
    if (!posts.length) {
      document.getElementById("content").innerHTML = '<p class="empty">No posts yet. Use POST /posts to create one.</p>';
      return;
    }
    let html = "";
    for (const p of posts) {
      html += '<div class="post">';
      html += '<div class="post-author">' + esc(shortID(p.author)) + '</div>';
      html += '<div class="post-body">' + esc(p.body) + '</div>';
      html += '<div class="post-footer">';
      const tags = p.tags || [];
      if (tags.length) {
        html += '<span>' + tags.map(t => '<span class="tag">' + esc(t) + '</span>').join("") + '</span>';
      }
      const reactions = p.reactions || {};
      const rKeys = Object.keys(reactions);
      if (rKeys.length) {
        html += '<span class="reactions">' + rKeys.map(r => esc(r) + ' ' + esc(reactions[r])).join(" \u00b7 ") + '</span>';
      }
      if (p.timestamp) {
        html += '<span class="ts">' + esc(new Date(p.timestamp).toLocaleString()) + '</span>';
      }
      html += '</div></div>';
    }
    document.getElementById("content").innerHTML = html;
  } catch (err) {
    document.getElementById("error-box").textContent = "Error: " + err.message;
    document.getElementById("error-box").style.display = "";
    document.getElementById("status-line").textContent = "Error";
  }
}

refresh();
setInterval(refresh, REFRESH_MS);
</script>
</body>
</html>`

// workspaceDashboardHTML is the interactive workspace social dashboard served at GET /w/{workspace}.
// Placeholders {{WORKSPACE}} and {{API_TOKEN}} are replaced at serve time.
const workspaceDashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{WORKSPACE}} — Social Graph</title>
<style>
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: system-ui, -apple-system, sans-serif; background: #0f1117; color: #e2e8f0; min-height: 100vh; padding: 2rem; max-width: 640px; }
h1 { font-size: 1.5rem; font-weight: 600; color: #f8fafc; margin-bottom: 0.25rem; }
.subtitle { font-size: 0.875rem; color: #64748b; margin-bottom: 1.5rem; }
.meta { display: flex; align-items: center; gap: 1rem; margin-bottom: 1.25rem; font-size: 0.8125rem; color: #64748b; }
.dot { width: 8px; height: 8px; border-radius: 50%; background: #22c55e; animation: pulse 2s ease-in-out infinite; }
@keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.4; } }
.error { background: #3b1010; color: #fca5a5; padding: 0.75rem 1rem; border-radius: 6px; margin-bottom: 1rem; font-size: 0.875rem; }
.empty { color: #475569; font-size: 0.875rem; padding: 2rem 0; text-align: center; }
.compose { background: #1e293b; border-radius: 8px; padding: 1rem; margin-bottom: 1.5rem; }
.compose textarea { width: 100%; background: #0f172a; border: 1px solid #334155; border-radius: 6px; color: #e2e8f0; font-size: 0.9375rem; padding: 0.625rem 0.75rem; resize: vertical; min-height: 80px; font-family: inherit; }
.compose textarea:focus { outline: none; border-color: #7c3aed; }
.compose-row { display: flex; gap: 0.75rem; margin-top: 0.75rem; align-items: center; }
.compose input { flex: 1; background: #0f172a; border: 1px solid #334155; border-radius: 6px; color: #e2e8f0; font-size: 0.8125rem; padding: 0.5rem 0.75rem; font-family: inherit; }
.compose input:focus { outline: none; border-color: #7c3aed; }
.btn { background: #7c3aed; color: #fff; border: none; border-radius: 6px; padding: 0.5rem 1.25rem; font-size: 0.875rem; font-weight: 600; cursor: pointer; }
.btn:hover { background: #6d28d9; }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.post { background: #1e293b; border-radius: 8px; padding: 1.25rem; margin-bottom: 1rem; }
.post-author { font-family: monospace; font-size: 0.8125rem; color: #7c3aed; margin-bottom: 0.5rem; display: flex; align-items: center; gap: 0.75rem; }
.follow-btn { font-size: 0.72rem; font-weight: 600; padding: 0.2em 0.65em; border-radius: 4px; border: 1px solid #7c3aed; color: #7c3aed; background: transparent; cursor: pointer; }
.follow-btn:hover { background: #7c3aed22; }
.post-body { font-size: 0.9375rem; color: #e2e8f0; line-height: 1.5; margin-bottom: 0.75rem; }
.post-footer { display: flex; align-items: center; gap: 1rem; flex-wrap: wrap; }
.tag { display: inline-block; padding: 0.15em 0.5em; border-radius: 4px; font-size: 0.72rem; font-weight: 600; background: #1e3a5f; color: #60a5fa; margin-right: 0.25rem; }
.reactions { display: flex; gap: 0.5rem; flex-wrap: wrap; align-items: center; }
.reaction-btn { background: #0f172a; border: 1px solid #334155; border-radius: 20px; color: #e2e8f0; font-size: 0.8125rem; padding: 0.2em 0.65em; cursor: pointer; }
.reaction-btn:hover { border-color: #7c3aed; }
.add-reaction { background: transparent; border: 1px dashed #475569; border-radius: 20px; color: #64748b; font-size: 0.8125rem; padding: 0.2em 0.65em; cursor: pointer; }
.add-reaction:hover { border-color: #7c3aed; color: #7c3aed; }
.ts { font-size: 0.75rem; color: #475569; margin-left: auto; }
</style>
</head>
<body>
<h1>{{WORKSPACE}}</h1>
<p class="subtitle">Social Graph — Layer 3</p>
<div class="meta">
  <span class="dot"></span>
  <span id="status-line">Connecting...</span>
</div>
<div id="error-box" class="error" style="display:none"></div>
<div class="compose">
  <textarea id="compose-body" placeholder="What's on your mind?"></textarea>
  <div class="compose-row">
    <input id="compose-tags" placeholder="Tags (comma-separated, optional)" />
    <input id="compose-author" placeholder="Your actor ID" style="max-width:180px" />
    <button class="btn" id="compose-btn" onclick="createPost()">Post</button>
  </div>
</div>
<div id="content"></div>
<script>
const WORKSPACE = "{{WORKSPACE}}";
const API_TOKEN = "{{API_TOKEN}}";
const BASE = "/w/" + WORKSPACE;
const REFRESH_MS = 10000;
const EMOJI_REACTIONS = ["\u2764\ufe0f", "\ud83d\udc4d", "\ud83d\ude02", "\ud83d\ude31", "\ud83d\ude22"];

function esc(s) {
  return String(s).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;");
}

function shortID(id) {
  return id ? id.slice(0, 8) + "\u2026" : "\u2014";
}

async function fetchJSON(path, opts) {
  const res = await fetch(path, { headers: { Authorization: "Bearer " + API_TOKEN, "Content-Type": "application/json" }, ...opts });
  if (!res.ok) {
    const body = await res.text();
    throw new Error("HTTP " + res.status + ": " + body);
  }
  return res.json();
}

async function createPost() {
  const btn = document.getElementById("compose-btn");
  const body = document.getElementById("compose-body").value.trim();
  if (!body) { alert("Body is required"); return; }
  const tagsRaw = document.getElementById("compose-tags").value.trim();
  const tags = tagsRaw ? tagsRaw.split(",").map(t => t.trim()).filter(Boolean) : [];
  const authorRaw = document.getElementById("compose-author").value.trim();
  const payload = { body, tags };
  if (authorRaw) payload.author = authorRaw;
  btn.disabled = true;
  try {
    await fetchJSON(BASE + "/posts", { method: "POST", body: JSON.stringify(payload) });
    document.getElementById("compose-body").value = "";
    document.getElementById("compose-tags").value = "";
    await refresh();
  } catch (err) {
    alert("Error: " + err.message);
  } finally {
    btn.disabled = false;
  }
}

async function followActor(actorID) {
  const followerRaw = prompt("Your actor ID (leave blank to use operator):", "");
  const payload = {};
  if (followerRaw) payload.follower = followerRaw;
  try {
    await fetchJSON(BASE + "/actors/" + encodeURIComponent(actorID) + "/follow", { method: "POST", body: JSON.stringify(payload) });
    alert("Followed!");
  } catch (err) {
    alert("Error: " + err.message);
  }
}

async function addReaction(postID, emoji) {
  const actorRaw = prompt("Your actor ID (leave blank to use operator):", "");
  const payload = { reaction: emoji };
  if (actorRaw) payload.actor = actorRaw;
  try {
    await fetchJSON(BASE + "/posts/" + encodeURIComponent(postID) + "/react", { method: "POST", body: JSON.stringify(payload) });
    await refresh();
  } catch (err) {
    alert("Error: " + err.message);
  }
}

async function pickReaction(postID) {
  const emoji = prompt("Enter reaction emoji (e.g. \u2764\ufe0f, \ud83d\udc4d):", "\u2764\ufe0f");
  if (!emoji) return;
  await addReaction(postID, emoji.trim());
}

async function refresh() {
  try {
    const data = await fetchJSON(BASE + "/posts");
    document.getElementById("error-box").style.display = "none";
    document.getElementById("status-line").textContent = "Live \u2014 " + new Date().toLocaleTimeString();
    const posts = data.posts || [];
    if (!posts.length) {
      document.getElementById("content").innerHTML = '<p class="empty">No posts yet. Be the first to post!</p>';
      return;
    }
    let html = "";
    for (const p of posts) {
      const pid = esc(p.id || "");
      html += '<div class="post">';
      html += '<div class="post-author">';
      html += shortID(p.author);
      html += ' <button class="follow-btn" onclick="followActor(\'' + esc(p.author) + '\')">Follow</button>';
      html += '</div>';
      html += '<div class="post-body">' + esc(p.body) + '</div>';
      html += '<div class="post-footer">';
      const tags = p.tags || [];
      if (tags.length) {
        html += '<span>' + tags.map(t => '<span class="tag">' + esc(t) + '</span>').join("") + '</span>';
      }
      const reactions = p.reactions || {};
      const rKeys = Object.keys(reactions);
      html += '<span class="reactions">';
      for (const r of rKeys) {
        html += '<button class="reaction-btn" onclick="addReaction(\'' + pid + '\',\'' + esc(r) + '\')">' + esc(r) + ' ' + esc(reactions[r]) + '</button>';
      }
      html += '<button class="add-reaction" onclick="pickReaction(\'' + pid + '\')">+ React</button>';
      html += '</span>';
      if (p.timestamp) {
        html += '<span class="ts">' + esc(new Date(p.timestamp).toLocaleString()) + '</span>';
      }
      html += '</div></div>';
    }
    document.getElementById("content").innerHTML = html;
  } catch (err) {
    document.getElementById("error-box").textContent = "Error: " + err.message;
    document.getElementById("error-box").style.display = "";
    document.getElementById("status-line").textContent = "Error";
  }
}

refresh();
setInterval(refresh, REFRESH_MS);
</script>
</body>
</html>`

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "social-server: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	humanName := os.Getenv("SOCIAL_HUMAN")
	if humanName == "" {
		humanName = "operator"
	}
	apiKey := os.Getenv("SOCIAL_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("SOCIAL_API_KEY is required")
	}
	apiToken := os.Getenv("SOCIAL_API_TOKEN")
	if apiToken == "" {
		apiToken = apiKey
	}
	dsn := os.Getenv("DATABASE_URL")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
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

	// Register social event type unmarshalers before any store reads —
	// Head() deserializes the latest event which may be a social type.
	social.RegisterEventTypes()

	// Bootstrap the event graph if it has no genesis event.
	if err := bootstrapGraph(s, humanID); err != nil {
		return fmt.Errorf("bootstrap graph: %w", err)
	}

	// Build factory and signer for social events.
	registry := event.DefaultRegistry()
	social.RegisterWithRegistry(registry)
	factory := event.NewEventFactory(registry)
	signer := deriveSignerFromID(humanID)

	ss := social.NewSocialStore(s, factory, signer)

	srv := &server{
		ss:       ss,
		store:    s,
		humanID:  humanID,
		apiKey:   apiKey,
		apiToken: apiToken,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", srv.dashboard)
	mux.HandleFunc("GET /health", srv.health)
	mux.HandleFunc("POST /posts", srv.auth(srv.createPost))
	mux.HandleFunc("GET /posts", srv.auth(srv.listPosts))
	mux.HandleFunc("GET /posts/{id}", srv.auth(srv.getPost))
	mux.HandleFunc("POST /actors/{id}/follow", srv.auth(srv.follow))
	mux.HandleFunc("GET /actors/{id}/followers", srv.auth(srv.getFollowers))
	mux.HandleFunc("GET /actors/{id}/following", srv.auth(srv.getFollowing))
	mux.HandleFunc("POST /posts/{id}/react", srv.auth(srv.react))

	// Workspace-scoped routes — isolated namespace per team, auth via SOCIAL_API_TOKEN.
	mux.HandleFunc("GET /w/{workspace}", srv.workspaceDashboard)
	mux.HandleFunc("POST /w/{workspace}/posts", srv.tokenAuth(srv.workspaceCreatePost))
	mux.HandleFunc("GET /w/{workspace}/posts", srv.tokenAuth(srv.workspaceListPosts))
	mux.HandleFunc("GET /w/{workspace}/posts/{id}", srv.tokenAuth(srv.workspaceGetPost))
	mux.HandleFunc("POST /w/{workspace}/actors/{id}/follow", srv.tokenAuth(srv.workspaceFollow))
	mux.HandleFunc("GET /w/{workspace}/actors/{id}/followers", srv.tokenAuth(srv.workspaceGetFollowers))
	mux.HandleFunc("GET /w/{workspace}/actors/{id}/following", srv.tokenAuth(srv.workspaceGetFollowing))
	mux.HandleFunc("POST /w/{workspace}/posts/{id}/react", srv.tokenAuth(srv.workspaceReact))

	addr := ":" + port
	fmt.Fprintf(os.Stderr, "social-server listening on %s\n", addr)
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
	ss       *social.SocialStore
	store    store.Store
	humanID  types.ActorID
	apiKey   string
	apiToken string
}

// dashboard handles GET / — serves the read-only HTML social dashboard.
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

// tokenAuth is middleware for workspace routes that validates the SOCIAL_API_TOKEN bearer token.
func (sv *server) tokenAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || token != sv.apiToken {
			writeErr(w, http.StatusUnauthorized, "invalid or missing API token")
			return
		}
		next(w, r)
	}
}

// workspaceDashboard handles GET /w/{workspace} — serves the interactive workspace social dashboard.
func (sv *server) workspaceDashboard(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("workspace")
	if workspace == "" {
		writeErr(w, http.StatusBadRequest, "workspace is required")
		return
	}
	html := strings.ReplaceAll(workspaceDashboardHTML, "{{WORKSPACE}}", workspace)
	html = strings.ReplaceAll(html, "{{API_TOKEN}}", sv.apiToken)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, html)
}

// createPost handles POST /posts
// Body: {"author":"actor_id", "body":"...", "tags":["a","b"]}
func (sv *server) createPost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Author string   `json:"author"`
		Body   string   `json:"body"`
		Tags   []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Body == "" {
		writeErr(w, http.StatusBadRequest, "body is required")
		return
	}
	author := sv.humanID
	if body.Author != "" {
		aid, err := types.NewActorID(body.Author)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid author: "+err.Error())
			return
		}
		author = aid
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
	post, err := sv.ss.CreatePost(author, body.Body, body.Tags, causes, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create post: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, postToMap(post, nil))
}

// listPosts handles GET /posts — returns up to 50 recent posts with reaction counts.
func (sv *server) listPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := sv.ss.ListPosts(50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list posts: "+err.Error())
		return
	}
	items, err := sv.postsWithReactions(posts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "reaction counts: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": items})
}

// getPost handles GET /posts/{id} — returns a single post with reaction counts.
func (sv *server) getPost(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseEventID(w, r)
	if !ok {
		return
	}
	post, err := sv.ss.GetPost(postID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "post not found: "+err.Error())
		return
	}
	reactions, err := sv.ss.ListReactions(postID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list reactions: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, postToMap(post, reactions))
}

// follow handles POST /actors/{id}/follow
// Body: {"follower":"actor_id"} — omit follower to use the human operator.
func (sv *server) follow(w http.ResponseWriter, r *http.Request) {
	subjectID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	var body struct {
		Follower string `json:"follower"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	follower := sv.humanID
	if body.Follower != "" {
		aid, err := types.NewActorID(body.Follower)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid follower: "+err.Error())
			return
		}
		follower = aid
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
	follow, err := sv.ss.Follow(follower, subjectID, causes, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "follow: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          follow.ID.Value(),
		"follower_id": follow.Follower.Value(),
		"subject_id":  follow.Subject.Value(),
	})
}

// getFollowers handles GET /actors/{id}/followers
func (sv *server) getFollowers(w http.ResponseWriter, r *http.Request) {
	subjectID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	followers, err := sv.ss.GetFollowers(subjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get followers: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subject_id": subjectID.Value(),
		"followers":  actorIDsToStrings(followers),
	})
}

// getFollowing handles GET /actors/{id}/following
func (sv *server) getFollowing(w http.ResponseWriter, r *http.Request) {
	followerID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	following, err := sv.ss.GetFollowing(followerID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get following: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"follower_id": followerID.Value(),
		"following":   actorIDsToStrings(following),
	})
}

// react handles POST /posts/{id}/react
// Body: {"actor":"actor_id", "reaction":"👍"}
func (sv *server) react(w http.ResponseWriter, r *http.Request) {
	postID, ok := parseEventID(w, r)
	if !ok {
		return
	}
	var body struct {
		Actor    string `json:"actor"`
		Reaction string `json:"reaction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Reaction == "" {
		writeErr(w, http.StatusBadRequest, "reaction is required")
		return
	}
	actor := sv.humanID
	if body.Actor != "" {
		aid, err := types.NewActorID(body.Actor)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid actor: "+err.Error())
			return
		}
		actor = aid
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
	reaction, err := sv.ss.AddReaction(actor, postID, body.Reaction, causes, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "add reaction: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       reaction.ID.Value(),
		"actor_id": reaction.Actor.Value(),
		"post_id":  reaction.PostID.Value(),
		"reaction": reaction.Reaction,
	})
}

// --- Workspace handlers ---

// workspaceCreatePost handles POST /w/{workspace}/posts
func (sv *server) workspaceCreatePost(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("workspace")
	if workspace == "" {
		writeErr(w, http.StatusBadRequest, "workspace is required")
		return
	}
	var body struct {
		Author string   `json:"author"`
		Body   string   `json:"body"`
		Tags   []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Body == "" {
		writeErr(w, http.StatusBadRequest, "body is required")
		return
	}
	author := sv.humanID
	if body.Author != "" {
		aid, err := types.NewActorID(body.Author)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid author: "+err.Error())
			return
		}
		author = aid
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
	post, err := sv.ss.CreatePostInWorkspace(author, body.Body, body.Tags, workspace, causes, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create post: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, postToMap(post, nil))
}

// workspaceListPosts handles GET /w/{workspace}/posts
func (sv *server) workspaceListPosts(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("workspace")
	if workspace == "" {
		writeErr(w, http.StatusBadRequest, "workspace is required")
		return
	}
	posts, err := sv.ss.ListPostsByWorkspace(workspace, 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list posts: "+err.Error())
		return
	}
	items, err := sv.workspacePostsWithReactions(posts, workspace)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "reaction counts: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": workspace, "posts": items})
}

// workspaceGetPost handles GET /w/{workspace}/posts/{id}
func (sv *server) workspaceGetPost(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("workspace")
	if workspace == "" {
		writeErr(w, http.StatusBadRequest, "workspace is required")
		return
	}
	postID, ok := parseEventID(w, r)
	if !ok {
		return
	}
	post, err := sv.ss.GetPost(postID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "post not found: "+err.Error())
		return
	}
	reactions, err := sv.ss.ListReactionsByWorkspace(postID, workspace)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list reactions: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, postToMap(post, reactions))
}

// workspaceFollow handles POST /w/{workspace}/actors/{id}/follow
func (sv *server) workspaceFollow(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("workspace")
	if workspace == "" {
		writeErr(w, http.StatusBadRequest, "workspace is required")
		return
	}
	subjectID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	var body struct {
		Follower string `json:"follower"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	follower := sv.humanID
	if body.Follower != "" {
		aid, err := types.NewActorID(body.Follower)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid follower: "+err.Error())
			return
		}
		follower = aid
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
	follow, err := sv.ss.FollowInWorkspace(follower, subjectID, workspace, causes, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "follow: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          follow.ID.Value(),
		"follower_id": follow.Follower.Value(),
		"subject_id":  follow.Subject.Value(),
		"workspace":   follow.Workspace,
	})
}

// workspaceGetFollowers handles GET /w/{workspace}/actors/{id}/followers
func (sv *server) workspaceGetFollowers(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("workspace")
	if workspace == "" {
		writeErr(w, http.StatusBadRequest, "workspace is required")
		return
	}
	subjectID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	followers, err := sv.ss.GetFollowersByWorkspace(subjectID, workspace)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get followers: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subject_id": subjectID.Value(),
		"workspace":  workspace,
		"followers":  actorIDsToStrings(followers),
	})
}

// workspaceGetFollowing handles GET /w/{workspace}/actors/{id}/following
func (sv *server) workspaceGetFollowing(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("workspace")
	if workspace == "" {
		writeErr(w, http.StatusBadRequest, "workspace is required")
		return
	}
	followerID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	following, err := sv.ss.GetFollowingByWorkspace(followerID, workspace)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get following: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"follower_id": followerID.Value(),
		"workspace":   workspace,
		"following":   actorIDsToStrings(following),
	})
}

// workspaceReact handles POST /w/{workspace}/posts/{id}/react
func (sv *server) workspaceReact(w http.ResponseWriter, r *http.Request) {
	workspace := r.PathValue("workspace")
	if workspace == "" {
		writeErr(w, http.StatusBadRequest, "workspace is required")
		return
	}
	postID, ok := parseEventID(w, r)
	if !ok {
		return
	}
	var body struct {
		Actor    string `json:"actor"`
		Reaction string `json:"reaction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Reaction == "" {
		writeErr(w, http.StatusBadRequest, "reaction is required")
		return
	}
	actor := sv.humanID
	if body.Actor != "" {
		aid, err := types.NewActorID(body.Actor)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid actor: "+err.Error())
			return
		}
		actor = aid
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
	reaction, err := sv.ss.AddReactionInWorkspace(actor, postID, body.Reaction, workspace, causes, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "add reaction: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        reaction.ID.Value(),
		"actor_id":  reaction.Actor.Value(),
		"post_id":   reaction.PostID.Value(),
		"reaction":  reaction.Reaction,
		"workspace": reaction.Workspace,
	})
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

// postsWithReactions enriches a slice of Posts with reaction counts from the global store.
func (sv *server) postsWithReactions(posts []social.Post) ([]map[string]any, error) {
	// Fetch all reaction events once for batch enrichment.
	reactionPage, err := sv.store.ByType(social.EventTypeReactionCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch reactions: %w", err)
	}
	// Build a map of postID → reaction → count.
	counts := make(map[types.EventID]map[string]int)
	for _, ev := range reactionPage.Items() {
		c, ok := ev.Content().(social.ReactionCreatedContent)
		if !ok {
			continue
		}
		if counts[c.PostID] == nil {
			counts[c.PostID] = make(map[string]int)
		}
		counts[c.PostID][c.Reaction]++
	}
	items := make([]map[string]any, 0, len(posts))
	for _, p := range posts {
		items = append(items, postToMap(p, reactionMapToSlice(p.ID, counts[p.ID])))
	}
	return items, nil
}

// workspacePostsWithReactions enriches workspace posts with workspace-scoped reaction counts.
func (sv *server) workspacePostsWithReactions(posts []social.Post, workspace string) ([]map[string]any, error) {
	reactionPage, err := sv.store.ByType(social.EventTypeReactionCreated, 1000, types.None[types.Cursor]())
	if err != nil {
		return nil, fmt.Errorf("fetch reactions: %w", err)
	}
	counts := make(map[types.EventID]map[string]int)
	for _, ev := range reactionPage.Items() {
		c, ok := ev.Content().(social.ReactionCreatedContent)
		if !ok || c.Workspace != workspace {
			continue
		}
		if counts[c.PostID] == nil {
			counts[c.PostID] = make(map[string]int)
		}
		counts[c.PostID][c.Reaction]++
	}
	items := make([]map[string]any, 0, len(posts))
	for _, p := range posts {
		items = append(items, postToMap(p, reactionMapToSlice(p.ID, counts[p.ID])))
	}
	return items, nil
}

// reactionMapToSlice converts a reaction count map to a []Reaction slice for postToMap.
// Accepts nil (returns nil reactions list in output).
func reactionMapToSlice(postID types.EventID, counts map[string]int) []social.Reaction {
	if len(counts) == 0 {
		return nil
	}
	reactions := make([]social.Reaction, 0, len(counts))
	for r, _ := range counts {
		reactions = append(reactions, social.Reaction{PostID: postID, Reaction: r})
	}
	return reactions
}

// postToMap converts a Post and its reactions to the JSON response shape.
// reactions may be nil (no reactions fetched yet — field will be empty map).
func postToMap(p social.Post, reactions []social.Reaction) map[string]any {
	reactionCounts := make(map[string]int)
	for _, r := range reactions {
		reactionCounts[r.Reaction]++
	}
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	m := map[string]any{
		"id":        p.ID.Value(),
		"author":    p.Author.Value(),
		"body":      p.Body,
		"tags":      tags,
		"reactions": reactionCounts,
	}
	if !p.Timestamp.IsZero() {
		m["timestamp"] = p.Timestamp.UTC().Format("2006-01-02T15:04:05Z")
	}
	if p.Workspace != "" {
		m["workspace"] = p.Workspace
	}
	return m
}

// parseActorID extracts and validates the {id} path parameter as an ActorID.
func parseActorID(w http.ResponseWriter, r *http.Request) (types.ActorID, bool) {
	idStr := r.PathValue("id")
	if idStr == "" {
		writeErr(w, http.StatusBadRequest, "actor id is required")
		return types.ActorID{}, false
	}
	actorID, err := types.NewActorID(idStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid actor id: "+err.Error())
		return types.ActorID{}, false
	}
	return actorID, true
}

// parseEventID extracts and validates the {id} path parameter as an EventID.
func parseEventID(w http.ResponseWriter, r *http.Request) (types.EventID, bool) {
	idStr := r.PathValue("id")
	if idStr == "" {
		writeErr(w, http.StatusBadRequest, "id is required")
		return types.EventID{}, false
	}
	eventID, err := types.NewEventID(idStr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id: "+err.Error())
		return types.EventID{}, false
	}
	return eventID, true
}

// actorIDsToStrings converts a slice of ActorIDs to a slice of strings.
func actorIDsToStrings(ids []types.ActorID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.Value())
	}
	return out
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

// --- Infrastructure helpers (mirror of cmd/market-server patterns) ---

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

// ed25519Signer implements event.Signer for social-emitted events.
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
	return types.NewConversationID("social-server-" + id.Value())
}
