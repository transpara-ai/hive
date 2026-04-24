package runner

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// Dispatcher translates site webhook ops into eventgraph bus events.
// Implement on hive.Runtime (or any coordinator) and pass to StartEventListener.
// If nil, the listener runs in log-only mode.
//
// The two-phase contract splits the anchor (synchronous, emits
// site.op.received and returns an event ID) from the translation
// (asynchronous, causally linked to the anchor). Callers receive the
// anchor ID as hive_chain_ref before the translation goroutine runs —
// so the response to the webhook is a cryptographic receipt that the
// op was observed, even if translation later fails.
type Dispatcher interface {
	AnchorSiteOp(ctx context.Context, op OpEvent) (types.EventID, error)
	EmitSiteOp(ctx context.Context, op OpEvent, anchorID types.EventID) error
}

// EventListener receives op events from the site's webhook and dispatches
// to the right agent. This is the pub/sub backbone — events arrive, agents react.
type EventListener struct {
	dispatcher Dispatcher
	ctx        context.Context
	port       string
}

// OpEvent is the JSON payload from the site's webhook.
type OpEvent struct {
	ID        string          `json:"id"`
	SpaceID   string          `json:"space_id"`
	NodeID    string          `json:"node_id"`
	NodeTitle string          `json:"node_title"`
	Actor     string          `json:"actor"`
	ActorID   string          `json:"actor_id"`
	ActorKind string          `json:"actor_kind"`
	Op        string          `json:"op"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

// StartEventListener starts an HTTP server that receives webhook events
// from the site and dispatches to agents. If d is nil, runs in log-only mode.
func StartEventListener(ctx context.Context, d Dispatcher, port string) error {
	el := &EventListener{dispatcher: d, ctx: ctx, port: port}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /event", el.handleEvent)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	log.Printf("[pubsub] listening on :%s", port)
	return server.ListenAndServe()
}

func (el *EventListener) handleEvent(w http.ResponseWriter, r *http.Request) {
	var op OpEvent
	if err := json.NewDecoder(r.Body).Decode(&op); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// Self-loop guard: don't re-process events produced by the hive itself.
	if op.ActorKind == "agent" {
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("[pubsub] event: op=%s actor=%s node=%s", op.Op, op.Actor, op.NodeTitle)

	if el.dispatcher == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "no dispatcher"})
		return
	}

	// Synchronous anchor: must succeed before we acknowledge the op.
	// Anchor failures return 500 — the site should retry.
	anchorID, err := el.dispatcher.AnchorSiteOp(r.Context(), op)
	if err != nil {
		log.Printf("[pubsub] anchor error: %v", err)
		http.Error(w, "anchor failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"hive_chain_ref": anchorID.Value()})

	// Asynchronous translate: errors are recorded as site.op.rejected by
	// the dispatcher. The webhook has already been acknowledged — the
	// anchor is on the chain, and reconciliation can retry if needed.
	go func() {
		if err := el.dispatcher.EmitSiteOp(el.ctx, op, anchorID); err != nil {
			log.Printf("[pubsub] translate error: %v", err)
		}
	}()
}
