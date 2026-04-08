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

// dispatchMu serializes dispatch calls to prevent chain integrity violations.
// The eventgraph requires each append to reference the actual head; concurrent
// appends race and the loser fails. This mutex ensures only one dispatch
// touches the chain at a time.
var dispatchMu sync.Mutex

// Site op event types — bridged from the site's webhook into the eventgraph bus.
var (
	EventTypeSiteRespond  = types.MustEventType("hive.site.respond")
	EventTypeSiteExpress  = types.MustEventType("hive.site.express")
	EventTypeSiteAssert   = types.MustEventType("hive.site.assert")
	EventTypeSiteProgress = types.MustEventType("hive.site.progress")
)

// SiteOpContent wraps a site op payload for non-task events.
type SiteOpContent struct {
	hiveContent
	SiteOp    string          `json:"SiteOp"`
	SpaceID   string          `json:"SpaceID"`
	NodeID    string          `json:"NodeID"`
	NodeTitle string          `json:"NodeTitle"`
	Actor     string          `json:"Actor"`
	ActorID   string          `json:"ActorID"`
	Payload   json.RawMessage `json:"Payload,omitempty"`
	eventType string
}

func (c SiteOpContent) EventTypeName() string { return c.eventType }

func init() {
	event.RegisterContentUnmarshaler("hive.site.respond", event.Unmarshal[SiteOpContent])
	event.RegisterContentUnmarshaler("hive.site.express", event.Unmarshal[SiteOpContent])
	event.RegisterContentUnmarshaler("hive.site.assert", event.Unmarshal[SiteOpContent])
	event.RegisterContentUnmarshaler("hive.site.progress", event.Unmarshal[SiteOpContent])
}

// EmitSiteOp translates a site webhook op into eventgraph bus events.
// Task ops (intend, assign, complete) go through the TaskStore which emits
// work.task.* events that agents already subscribe to. Non-task ops are
// emitted as hive.site.* events.
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

// dispatchAssign records an assignment on the eventgraph.
// The site payload must include the target task — but we don't have a mapping
// from site node IDs to eventgraph task IDs yet. Log and skip for now.
func (r *Runtime) dispatchAssign(op runner.OpEvent) error {
	log.Printf("[dispatch] assign received for node %s — site→eventgraph task ID mapping not yet implemented", op.NodeID)
	return nil
}

// dispatchComplete records a completion on the eventgraph.
// Same limitation as assign — needs site→eventgraph task ID mapping.
func (r *Runtime) dispatchComplete(op runner.OpEvent) error {
	log.Printf("[dispatch] complete received for node %s — site→eventgraph task ID mapping not yet implemented", op.NodeID)
	return nil
}

// emitSiteEvent creates and appends a hive.site.* event to the eventgraph.
func (r *Runtime) emitSiteEvent(eventType types.EventType, op runner.OpEvent) error {
	causes, err := r.headCauses()
	if err != nil {
		return err
	}
	content := SiteOpContent{
		SiteOp:    op.Op,
		SpaceID:   op.SpaceID,
		NodeID:    op.NodeID,
		NodeTitle: op.NodeTitle,
		Actor:     op.Actor,
		ActorID:   op.ActorID,
		Payload:   op.Payload,
		eventType: eventType.String(),
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
