package hive

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/hive/pkg/runner"
	"github.com/transpara-ai/work"
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
// anchorID is the ID of the site.op.received event written synchronously
// by AnchorSiteOp — all translated events list this as their cause, so
// the chain records a direct edge from anchor to translation.
//
// Task ops use the TaskStore where implemented:
//   - intend → TaskStore.Create() → work.task.created
//   - assign → NOT YET IMPLEMENTED (needs site→eventgraph task ID mapping)
//   - complete → NOT YET IMPLEMENTED (needs site→eventgraph task ID mapping)
//
// Non-task ops are emitted as hive.site.* events. Any translation failure
// is recorded as site.op.rejected with the original external_ref.
func (r *Runtime) EmitSiteOp(ctx context.Context, op runner.OpEvent, anchorID types.EventID) error {
	dispatchMu.Lock()
	defer dispatchMu.Unlock()

	causes := []types.EventID{anchorID}
	var err error
	switch op.Op {
	case "intend":
		err = r.dispatchIntend(op, causes)
	case "assign":
		err = r.dispatchAssign(op, causes)
	case "complete":
		err = r.dispatchComplete(op, causes)
	case "respond":
		err = r.emitSiteEvent(EventTypeSiteRespond, op, causes)
	case "express":
		err = r.emitSiteEvent(EventTypeSiteExpress, op, causes)
	case "assert":
		err = r.emitSiteEvent(EventTypeSiteAssert, op, causes)
	case "progress":
		err = r.emitSiteEvent(EventTypeSiteProgress, op, causes)
	default:
		log.Printf("[dispatch] unhandled site op: %s", op.Op)
		r.recordRejection(op, fmt.Sprintf("unhandled op kind: %s", op.Op))
		return nil
	}

	if err != nil {
		r.recordRejection(op, err.Error())
		return err
	}
	return nil
}

// recordRejection emits site.op.rejected on the chain. Best-effort —
// a failure to record the rejection is logged but not surfaced, because
// the original translation error has already been returned.
func (r *Runtime) recordRejection(op runner.OpEvent, reason string) {
	content := event.SiteOpRejectedContent{
		ExternalRef: event.ExternalRef{System: "site", ID: op.ID},
		Reason:      reason,
		RejectedAt:  time.Now().UTC(),
	}
	if err := r.bridgeEmitRejected(content); err != nil {
		log.Printf("[dispatch] emit rejected failed: %v", err)
	}
}

// recordTranslation emits site.op.translated on the chain with the bus
// event ID produced by the successful dispatch.
func (r *Runtime) recordTranslation(op runner.OpEvent, busEventID types.EventID) {
	content := event.SiteOpTranslatedContent{
		ExternalRef:  event.ExternalRef{System: "site", ID: op.ID},
		BusEventID:   busEventID,
		TranslatedAt: time.Now().UTC(),
	}
	if err := r.bridgeEmitTranslated(content); err != nil {
		log.Printf("[dispatch] emit translated failed: %v", err)
	}
}

// intendPayload holds the parsed fields from an intend op's JSON payload.
type intendPayload struct {
	Desc     string
	Priority work.TaskPriority
}

// parseIntendPayload extracts description, body, and priority from a raw
// JSON payload. When both Description and Body are present, they are
// combined so the full content is preserved (TaskStore.Create accepts a
// single description string). Priority is validated against the known
// constants; invalid values are replaced with DefaultPriority.
func parseIntendPayload(raw json.RawMessage) intendPayload {
	var result intendPayload
	if len(raw) > 0 {
		var p struct {
			Description string `json:"description"`
			Body        string `json:"body"`
			Priority    string `json:"priority"`
		}
		if json.Unmarshal(raw, &p) == nil {
			switch {
			case p.Description != "" && p.Body != "":
				result.Desc = p.Description + "\n\n" + p.Body
			case p.Description != "":
				result.Desc = p.Description
			default:
				result.Desc = p.Body
			}
			if p.Priority != "" {
				result.Priority = work.TaskPriority(p.Priority)
			}
		}
	}

	// Validate and default priority.
	if result.Priority != "" {
		switch result.Priority {
		case work.PriorityLow, work.PriorityMedium, work.PriorityHigh, work.PriorityCritical:
			// valid
		default:
			log.Printf("[dispatch] ignoring invalid priority %q, using default", result.Priority)
			result.Priority = ""
		}
	}
	if result.Priority == "" {
		result.Priority = work.DefaultPriority
	}
	return result
}

// dispatchIntend creates a task on the eventgraph from a site "intend" op.
// The task's causes list contains only the anchor event ID — the chain
// records a direct edge from anchor to task-created.
func (r *Runtime) dispatchIntend(op runner.OpEvent, causes []types.EventID) error {
	title := op.NodeTitle
	if title == "" {
		title = "Site task (no title)"
	}

	parsed := parseIntendPayload(op.Payload)

	task, err := r.tasks.Create(r.humanID, title, parsed.Desc, causes, r.convID, parsed.Priority)
	if err != nil {
		return fmt.Errorf("dispatch intend: %w", err)
	}
	log.Printf("[dispatch] task created: %s (eventgraph ID %s)", title, task.ID)
	r.recordTranslation(op, task.ID)
	return nil
}

// dispatchAssign is a stub — site→eventgraph task ID mapping is not yet
// implemented. Logs the event and returns an error so the caller knows
// the op was not processed.
func (r *Runtime) dispatchAssign(op runner.OpEvent, causes []types.EventID) error {
	_ = causes
	log.Printf("[dispatch] assign received for node %s — site→eventgraph task ID mapping not yet implemented", op.NodeID)
	return fmt.Errorf("assign: site→eventgraph task ID mapping not yet implemented")
}

// dispatchComplete is a stub — site→eventgraph task ID mapping is not yet
// implemented. Logs the event and returns an error so the caller knows
// the op was not processed.
func (r *Runtime) dispatchComplete(op runner.OpEvent, causes []types.EventID) error {
	_ = causes
	log.Printf("[dispatch] complete received for node %s — site→eventgraph task ID mapping not yet implemented", op.NodeID)
	return fmt.Errorf("complete: site→eventgraph task ID mapping not yet implemented")
}

// emitSiteEvent creates and appends a hive.site.* event to the eventgraph.
func (r *Runtime) emitSiteEvent(eventType types.EventType, op runner.OpEvent, causes []types.EventID) error {
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
	stored, err := r.store.Append(ev)
	if err != nil {
		return fmt.Errorf("dispatch %s: append: %w", op.Op, err)
	}
	log.Printf("[dispatch] emitted %s: actor=%s node=%s", eventType, op.Actor, op.NodeTitle)
	r.recordTranslation(op, stored.ID())
	return nil
}
