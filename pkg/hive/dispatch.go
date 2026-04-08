package hive

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/runner"
)

// dispatchMu serializes webhook dispatch calls against each other.
// This prevents two concurrent webhooks from reading the same chain head
// and racing on Append. It does NOT protect against concurrent appends
// from the agent loop (Runtime.emit) — those follow the same best-effort
// pattern as the rest of the runtime.
var dispatchMu sync.Mutex

// Site op event types — bridged from the site's webhook into the eventgraph bus.
var (
	EventTypeSiteRespond  = types.MustEventType("hive.site.respond")
	EventTypeSiteExpress  = types.MustEventType("hive.site.express")
	EventTypeSiteAssert   = types.MustEventType("hive.site.assert")
	EventTypeSiteProgress = types.MustEventType("hive.site.progress")
)

// SiteOpContent wraps a site op payload for non-task events.
// EventType is exported so it survives JSON round-trip through Postgres.
type SiteOpContent struct {
	hiveContent
	EventType string          `json:"EventType"`
	SiteOp    string          `json:"SiteOp"`
	SpaceID   string          `json:"SpaceID"`
	NodeID    string          `json:"NodeID"`
	NodeTitle string          `json:"NodeTitle"`
	Actor     string          `json:"Actor"`
	ActorID   string          `json:"ActorID"`
	Payload   json.RawMessage `json:"Payload,omitempty"`
}

func (c SiteOpContent) EventTypeName() string { return c.EventType }

func init() {
	event.RegisterContentUnmarshaler("hive.site.respond", event.Unmarshal[SiteOpContent])
	event.RegisterContentUnmarshaler("hive.site.express", event.Unmarshal[SiteOpContent])
	event.RegisterContentUnmarshaler("hive.site.assert", event.Unmarshal[SiteOpContent])
	event.RegisterContentUnmarshaler("hive.site.progress", event.Unmarshal[SiteOpContent])
}

// EmitSiteOp translates a site webhook op into eventgraph bus events.
// Task ops use the TaskStore where implemented:
//   - intend → TaskStore.Create() → work.task.created
//   - assign → NOT YET IMPLEMENTED (needs site→eventgraph task ID mapping)
//   - complete → NOT YET IMPLEMENTED (needs site→eventgraph task ID mapping)
//
// Non-task ops are emitted as hive.site.* events.
func (r *Runtime) EmitSiteOp(ctx context.Context, op runner.OpEvent) error {
	dispatchMu.Lock()
	defer dispatchMu.Unlock()

	switch op.Op {
	case "intend":
		return r.dispatchIntend(op)
	case "assign":
		return r.dispatchAssign(op)
	case "complete":
		return r.dispatchComplete(op)
	case "respond":
		return r.emitSiteEvent(EventTypeSiteRespond, op)
	case "express":
		return r.emitSiteEvent(EventTypeSiteExpress, op)
	case "assert":
		return r.emitSiteEvent(EventTypeSiteAssert, op)
	case "progress":
		return r.emitSiteEvent(EventTypeSiteProgress, op)
	default:
		log.Printf("[dispatch] unhandled site op: %s", op.Op)
		return nil
	}
}

// dispatchIntend creates a task on the eventgraph from a site "intend" op.
func (r *Runtime) dispatchIntend(op runner.OpEvent) error {
	title := op.NodeTitle
	if title == "" {
		title = "Site task (no title)"
	}
	// Extract description from payload if available.
	var desc string
	if len(op.Payload) > 0 {
		var p struct {
			Description string `json:"description"`
			Body        string `json:"body"`
		}
		if json.Unmarshal(op.Payload, &p) == nil {
			desc = p.Description
			if desc == "" {
				desc = p.Body
			}
		}
	}

	causes, err := r.headCauses()
	if err != nil {
		return err
	}
	task, err := r.tasks.Create(r.humanID, title, desc, causes, r.convID)
	if err != nil {
		return fmt.Errorf("dispatch intend: %w", err)
	}
	log.Printf("[dispatch] task created: %s (eventgraph ID %s)", title, task.ID)
	return nil
}

// dispatchAssign is a stub — site→eventgraph task ID mapping is not yet
// implemented. Logs the event and returns an error so the caller knows
// the op was not processed.
func (r *Runtime) dispatchAssign(op runner.OpEvent) error {
	log.Printf("[dispatch] assign received for node %s — site→eventgraph task ID mapping not yet implemented", op.NodeID)
	return fmt.Errorf("assign: site→eventgraph task ID mapping not yet implemented")
}

// dispatchComplete is a stub — site→eventgraph task ID mapping is not yet
// implemented. Logs the event and returns an error so the caller knows
// the op was not processed.
func (r *Runtime) dispatchComplete(op runner.OpEvent) error {
	log.Printf("[dispatch] complete received for node %s — site→eventgraph task ID mapping not yet implemented", op.NodeID)
	return fmt.Errorf("complete: site→eventgraph task ID mapping not yet implemented")
}

// emitSiteEvent creates and appends a hive.site.* event to the eventgraph.
func (r *Runtime) emitSiteEvent(eventType types.EventType, op runner.OpEvent) error {
	causes, err := r.headCauses()
	if err != nil {
		return err
	}
	content := SiteOpContent{
		EventType: eventType.String(),
		SiteOp:    op.Op,
		SpaceID:   op.SpaceID,
		NodeID:    op.NodeID,
		NodeTitle: op.NodeTitle,
		Actor:     op.Actor,
		ActorID:   op.ActorID,
		Payload:   op.Payload,
	}
	ev, err := r.factory.Create(eventType, r.humanID, content, causes, r.convID, r.store, r.signer)
	if err != nil {
		return fmt.Errorf("dispatch %s: create event: %w", op.Op, err)
	}
	if _, err := r.store.Append(ev); err != nil {
		return fmt.Errorf("dispatch %s: append: %w", op.Op, err)
	}
	log.Printf("[dispatch] emitted %s: actor=%s node=%s", eventType, op.Actor, op.NodeTitle)
	return nil
}

// headCauses returns the current chain head as a single-element cause slice.
func (r *Runtime) headCauses() ([]types.EventID, error) {
	head, err := r.store.Head()
	if err != nil {
		return nil, fmt.Errorf("head: %w", err)
	}
	if !head.IsSome() {
		return nil, fmt.Errorf("empty event store")
	}
	return []types.EventID{head.Unwrap().ID()}, nil
}
