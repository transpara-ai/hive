// Command market-server is an HTTP REST API server for the Market Graph (Layer 2).
// It exposes portable reputation as signed, auditable events on the shared event graph.
//
// Environment variables:
//
//	MARKET_HUMAN   — display name of the human operator (defaults to "operator")
//	MARKET_API_KEY — API key for auth; callers pass Authorization: Bearer <key> (required)
//	DATABASE_URL   — Postgres DSN (optional; defaults to in-memory)
//	PORT           — HTTP port to listen on (optional; defaults to 8081)
//
// Endpoints:
//
//	GET  /                          read-only dashboard (HTML, no auth required)
//	GET  /health                    liveness probe
//	GET  /actors                    list all actors with reputation data
//	POST /actors/{id}/endorse       endorse an actor for a skill
//	GET  /actors/{id}/reputation    get accumulated skill endorsements for an actor
//	POST /actors/{id}/review        leave a review for an actor after a task
//	GET  /actors/{id}/reviews       list all reviews for an actor
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

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/actor/pgactor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/store/pgstore"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"

	"github.com/lovyou-ai/hive/pkg/market"
)

// dashboardHTML is the read-only reputation dashboard served at GET /.
// The placeholder {{API_KEY}} is replaced at serve time with the actual key so
// the browser's fetch() calls can authenticate against the API.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Market Graph — Reputation Dashboard</title>
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
.actor-card { background: #1e293b; border-radius: 8px; padding: 1.25rem; margin-bottom: 1.25rem; }
.actor-id { font-family: monospace; font-size: 0.8125rem; color: #7c3aed; margin-bottom: 0.75rem; }
.card-section { margin-top: 0.75rem; }
.card-section h3 { font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.06em; color: #475569; margin-bottom: 0.5rem; }
table { width: 100%; border-collapse: collapse; font-size: 0.875rem; }
th { text-align: left; padding: 0.4rem 0.6rem; color: #64748b; font-weight: 500; font-size: 0.72rem; text-transform: uppercase; letter-spacing: 0.05em; border-bottom: 1px solid #334155; }
td { padding: 0.5rem 0.6rem; border-bottom: 1px solid #1e293b; vertical-align: middle; }
.skill { display: inline-block; padding: 0.2em 0.55em; border-radius: 4px; font-size: 0.72rem; font-weight: 600; background: #1e3a5f; color: #60a5fa; }
.count { font-weight: 600; color: #f1f5f9; }
.rating { color: #fbbf24; font-weight: 600; }
.note { color: #94a3b8; font-size: 0.8125rem; }
.reviewer { font-family: monospace; font-size: 0.72rem; color: #7c3aed; }
</style>
</head>
<body>
<h1>Market Graph</h1>
<p class="subtitle">Portable reputation — Layer 2</p>
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
    const data = await fetchJSON("/actors");
    document.getElementById("error-box").style.display = "none";
    document.getElementById("status-line").textContent = "Live \u2014 " + new Date().toLocaleTimeString();
    const actors = data.actors || [];
    if (!actors.length) {
      document.getElementById("content").innerHTML = '<p class="empty">No reputation data yet. Use POST /actors/{id}/endorse or POST /actors/{id}/review to add data.</p>';
      return;
    }
    let html = "";
    for (const a of actors) {
      html += '<div class="actor-card">';
      html += '<div class="actor-id">' + esc(a.id) + '</div>';
      const skills = a.skills || {};
      const skillKeys = Object.keys(skills);
      if (skillKeys.length > 0) {
        html += '<div class="card-section"><h3>Endorsements</h3><table><thead><tr><th>Skill</th><th>Count</th></tr></thead><tbody>';
        for (const sk of skillKeys) {
          html += '<tr><td><span class="skill">' + esc(sk) + '</span></td><td class="count">' + esc(skills[sk]) + '</td></tr>';
        }
        html += '</tbody></table></div>';
      }
      const reviews = a.reviews || [];
      if (reviews.length > 0) {
        html += '<div class="card-section"><h3>Reviews</h3><table><thead><tr><th>Reviewer</th><th>Rating</th><th>Note</th></tr></thead><tbody>';
        for (const rv of reviews) {
          html += '<tr><td class="reviewer">' + esc(shortID(rv.reviewer_id)) + '</td>';
          html += '<td class="rating">' + esc(rv.rating) + '/5</td>';
          html += '<td class="note">' + esc(rv.note || "") + '</td></tr>';
        }
        html += '</tbody></table></div>';
      }
      html += '</div>';
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
		fmt.Fprintf(os.Stderr, "market-server: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	humanName := os.Getenv("MARKET_HUMAN")
	if humanName == "" {
		humanName = "operator"
	}
	apiKey := os.Getenv("MARKET_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("MARKET_API_KEY is required")
	}
	dsn := os.Getenv("DATABASE_URL")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
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

	// Register market event type unmarshalers before any store reads —
	// Head() deserializes the latest event which may be a market type.
	market.RegisterEventTypes()

	// Bootstrap the event graph if it has no genesis event.
	if err := bootstrapGraph(s, humanID); err != nil {
		return fmt.Errorf("bootstrap graph: %w", err)
	}

	// Build factory and signer for market events.
	registry := event.DefaultRegistry()
	market.RegisterWithRegistry(registry)
	factory := event.NewEventFactory(registry)
	signer := deriveSignerFromID(humanID)

	rs := market.NewReputationStore(s, factory, signer)

	srv := &server{
		rs:      rs,
		store:   s,
		humanID: humanID,
		apiKey:  apiKey,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", srv.dashboard)
	mux.HandleFunc("GET /health", srv.health)
	mux.HandleFunc("GET /actors", srv.auth(srv.listActors))
	mux.HandleFunc("POST /actors/{id}/endorse", srv.auth(srv.endorse))
	mux.HandleFunc("GET /actors/{id}/reputation", srv.auth(srv.getReputation))
	mux.HandleFunc("POST /actors/{id}/review", srv.auth(srv.addReview))
	mux.HandleFunc("GET /actors/{id}/reviews", srv.auth(srv.listReviews))

	addr := ":" + port
	fmt.Fprintf(os.Stderr, "market-server listening on %s\n", addr)
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
	rs      *market.ReputationStore
	store   store.Store
	humanID types.ActorID
	apiKey  string
}

// dashboard handles GET / — serves the read-only HTML reputation dashboard.
// No auth required; the API key is injected into the page so the browser's
// fetch() calls can authenticate against the API.
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

// endorse handles POST /actors/{id}/endorse
// Body: {"endorser":"actor_id", "skill":"go"} — omit endorser to use the human operator.
func (sv *server) endorse(w http.ResponseWriter, r *http.Request) {
	subjectID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	var body struct {
		Endorser string `json:"endorser"`
		Skill    string `json:"skill"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Skill == "" {
		writeErr(w, http.StatusBadRequest, "skill is required")
		return
	}
	endorser := sv.humanID
	if body.Endorser != "" {
		aid, err := types.NewActorID(body.Endorser)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid endorser: "+err.Error())
			return
		}
		endorser = aid
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
	endorsement, err := sv.rs.AddEndorsement(endorser, subjectID, body.Skill, causes, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "add endorsement: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          endorsement.ID.Value(),
		"endorser_id": endorsement.EndorserID.Value(),
		"subject_id":  endorsement.SubjectID.Value(),
		"skill":       endorsement.Skill,
	})
}

// getReputation handles GET /actors/{id}/reputation
// Returns accumulated endorsement counts per skill for the actor.
func (sv *server) getReputation(w http.ResponseWriter, r *http.Request) {
	subjectID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	rep, err := sv.rs.GetReputation(subjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get reputation: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subject_id": rep.SubjectID.Value(),
		"skills":     rep.SkillCounts,
	})
}

// addReview handles POST /actors/{id}/review
// Body: {"reviewer":"actor_id", "task_id":"event_id", "rating":5, "note":"..."}
// reviewer defaults to the human operator if omitted. task_id is optional.
func (sv *server) addReview(w http.ResponseWriter, r *http.Request) {
	subjectID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	var body struct {
		Reviewer string `json:"reviewer"`
		TaskID   string `json:"task_id"`
		Rating   int    `json:"rating"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Rating < 1 || body.Rating > 5 {
		writeErr(w, http.StatusBadRequest, "rating must be between 1 and 5")
		return
	}
	reviewer := sv.humanID
	if body.Reviewer != "" {
		aid, err := types.NewActorID(body.Reviewer)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid reviewer: "+err.Error())
			return
		}
		reviewer = aid
	}
	var taskID types.EventID
	if body.TaskID != "" {
		tid, err := types.NewEventID(body.TaskID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid task_id: "+err.Error())
			return
		}
		taskID = tid
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
	review, err := sv.rs.AddReview(reviewer, subjectID, taskID, body.Rating, body.Note, causes, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "add review: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          review.ID.Value(),
		"reviewer_id": review.ReviewerID.Value(),
		"subject_id":  review.SubjectID.Value(),
		"task_id":     review.TaskID.Value(),
		"rating":      review.Rating,
		"note":        review.Note,
	})
}

// listReviews handles GET /actors/{id}/reviews
// Returns all reviews for the given actor.
func (sv *server) listReviews(w http.ResponseWriter, r *http.Request) {
	subjectID, ok := parseActorID(w, r)
	if !ok {
		return
	}
	reviews, err := sv.rs.ListReviews(subjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list reviews: "+err.Error())
		return
	}
	items := make([]map[string]any, 0, len(reviews))
	for _, rv := range reviews {
		items = append(items, map[string]any{
			"id":          rv.ID.Value(),
			"reviewer_id": rv.ReviewerID.Value(),
			"subject_id":  rv.SubjectID.Value(),
			"task_id":     rv.TaskID.Value(),
			"rating":      rv.Rating,
			"note":        rv.Note,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subject_id": subjectID.Value(),
		"reviews":    items,
	})
}

// listActors handles GET /actors — returns all actors with reputation data.
// Used by the dashboard to enumerate known subjects.
func (sv *server) listActors(w http.ResponseWriter, r *http.Request) {
	// Collect unique subject IDs from endorsement and review events.
	subjectSet := make(map[types.ActorID]bool)
	endPage, err := sv.store.ByType(market.EventTypeEndorsement, 1000, types.None[types.Cursor]())
	if err == nil {
		for _, ev := range endPage.Items() {
			if c, ok := ev.Content().(market.EndorsementContent); ok {
				subjectSet[c.SubjectID] = true
			}
		}
	}
	revPage, err := sv.store.ByType(market.EventTypeReview, 1000, types.None[types.Cursor]())
	if err == nil {
		for _, ev := range revPage.Items() {
			if c, ok := ev.Content().(market.ReviewContent); ok {
				subjectSet[c.SubjectID] = true
			}
		}
	}

	type actorData struct {
		ID      string           `json:"id"`
		Skills  map[string]int   `json:"skills"`
		Reviews []map[string]any `json:"reviews"`
	}
	result := make([]actorData, 0, len(subjectSet))
	for subjectID := range subjectSet {
		rep, err := sv.rs.GetReputation(subjectID)
		if err != nil {
			continue
		}
		reviews, err := sv.rs.ListReviews(subjectID)
		if err != nil {
			continue
		}
		revItems := make([]map[string]any, 0, len(reviews))
		for _, rv := range reviews {
			revItems = append(revItems, map[string]any{
				"id":          rv.ID.Value(),
				"reviewer_id": rv.ReviewerID.Value(),
				"rating":      rv.Rating,
				"note":        rv.Note,
			})
		}
		result = append(result, actorData{
			ID:      subjectID.Value(),
			Skills:  rep.SkillCounts,
			Reviews: revItems,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"actors": result})
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

// parseActorID extracts and validates the {id} path parameter from the request.
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

// --- Infrastructure helpers (mirror of cmd/work-server patterns) ---

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

// ed25519Signer implements event.Signer for market-emitted events.
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
	return types.NewConversationID("market-server-" + id.Value())
}
