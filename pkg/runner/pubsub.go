package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Dispatcher translates site webhook ops into eventgraph bus events.
// Implement on hive.Runtime (or any coordinator) and pass to StartEventListener.
// If nil, the listener runs in log-only mode.
type Dispatcher interface {
	EmitSiteOp(ctx context.Context, event OpEvent) error
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
	var event OpEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	// Don't react to our own events (avoid infinite loops).
	if event.ActorKind == "agent" {
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("[pubsub] event: op=%s actor=%s node=%s", event.Op, event.Actor, event.NodeTitle)

	if el.dispatcher != nil {
		go func() {
			if err := el.dispatcher.EmitSiteOp(el.ctx, event); err != nil {
				log.Printf("[pubsub] dispatch error: %v", err)
			}
		}()
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"received"}`)
}

