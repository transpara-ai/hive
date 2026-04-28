package hive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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
	EventTypeSiteDelete   = types.MustEventType("hive.site.delete")
	EventTypeSiteMirror   = types.MustEventType("hive.site.mirror")
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
	event.RegisterContentUnmarshaler("hive.site.delete", event.Unmarshal[SiteOpContent])
	event.RegisterContentUnmarshaler("hive.site.mirror", event.Unmarshal[SiteOpContent])
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
	case "intake":
		err = r.dispatchRefineryIntake(op, causes)
	case "attach":
		err = r.dispatchRefineryAttach(op, causes)
	case "respond":
		err = r.emitSiteEvent(EventTypeSiteRespond, op, causes)
	case "express":
		err = r.emitSiteEvent(EventTypeSiteExpress, op, causes)
	case "assert":
		err = r.emitSiteEvent(EventTypeSiteAssert, op, causes)
	case "delete":
		err = r.emitSiteEvent(EventTypeSiteDelete, op, causes)
	case "hive_mirror":
		err = r.emitSiteEvent(EventTypeSiteMirror, op, causes)
	case "progress":
		if isRefineryStateTransition(op.Payload) {
			err = r.dispatchRefineryStateTransition(op, causes)
		} else {
			err = r.emitSiteEvent(EventTypeSiteProgress, op, causes)
		}
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

type refineryIntakePayload struct {
	ClassifierKind   string `json:"classifier_kind"`
	RecommendedState string `json:"recommended_state"`
	PersistedState   string `json:"persisted_state"`
	DuplicateCount   int    `json:"duplicate_count"`
	ArtifactCount    int    `json:"artifact_count"`
	Question         string `json:"question"`
	DesiredOutcome   string `json:"desired_outcome"`
	Rationale        string `json:"rationale"`
}

type refineryAttachPayload struct {
	ParentID string `json:"parent_id"`
	Filename string `json:"filename"`
	Hash     string `json:"hash"`
}

type refineryStatePayload struct {
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	Reason    string `json:"reason"`
}

func (r *Runtime) dispatchRefineryIntake(op runner.OpEvent, causes []types.EventID) error {
	var p refineryIntakePayload
	_ = json.Unmarshal(op.Payload, &p)
	now := time.Now().UTC()
	if p.PersistedState == "" {
		p.PersistedState = "intake.raw"
	}
	if err := r.bridgeEmitRefineryIntakeReceived(event.RefineryIntakeReceivedContent{
		IntakeRef:      op.NodeID,
		SpaceID:        op.SpaceID,
		Title:          op.NodeTitle,
		Actor:          op.Actor,
		ActorID:        op.ActorID,
		Question:       p.Question,
		DesiredOutcome: p.DesiredOutcome,
		ArtifactCount:  p.ArtifactCount,
		ReceivedAt:     now,
	}); err != nil {
		return fmt.Errorf("dispatch refinery intake received: %w", err)
	}
	receivedID := r.bridgeAgent.LastEvent()
	if err := r.bridgeEmitRefineryIntakeClassified(event.RefineryIntakeClassifiedContent{
		IntakeRef:        op.NodeID,
		ClassifierKind:   p.ClassifierKind,
		RecommendedState: p.RecommendedState,
		PersistedState:   p.PersistedState,
		DuplicateCount:   p.DuplicateCount,
		ArtifactCount:    p.ArtifactCount,
		Rationale:        p.Rationale,
		ClassifiedAt:     now,
	}); err != nil {
		return fmt.Errorf("dispatch refinery intake classified: %w", err)
	}
	if err := r.bridgeEmitRefineryStateTransitioned(event.RefineryStateTransitionedContent{
		IntakeRef:      op.NodeID,
		FromState:      "",
		ToState:        p.PersistedState,
		Reason:         "all submissions begin at raw intake for provenance; classifier recommendation is advisory",
		TransitionedAt: now,
	}); err != nil {
		return fmt.Errorf("dispatch refinery intake landed: %w", err)
	}
	r.recordTranslation(op, receivedID)
	if err := r.autoAdvanceRefineryIntake(op, p); err != nil {
		log.Printf("[dispatch] refinery auto-advance skipped for %s: %v", op.NodeID, err)
	}
	return nil
}

func (r *Runtime) autoAdvanceRefineryIntake(op runner.OpEvent, p refineryIntakePayload) error {
	target := strings.TrimSpace(p.RecommendedState)
	if strings.TrimSpace(p.ClassifierKind) != "spec" || target == "" || target == p.PersistedState {
		return nil
	}
	if !isSafeRefineryAutoTarget(target) {
		return nil
	}
	base := refinerySiteAPIBase(r.siteAPIBase)
	if base == "" {
		return fmt.Errorf("site API base unavailable")
	}
	payload, _ := json.Marshal(map[string]string{
		"space_id": op.SpaceID,
		"node_id":  op.NodeID,
		"state":    target,
		"reason":   "classifier recommendation after raw intake; raw intake provenance is preserved in EventGraph",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/api/hive/refinery/state", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(os.Getenv("LOVYOU_API_KEY")); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("site state endpoint returned %s", resp.Status)
	}
	return nil
}

func isSafeRefineryAutoTarget(state string) bool {
	switch state {
	case "requirement.clarifying", "requirement.investigating", "spec.draft", "needs_attention":
		return true
	default:
		return false
	}
}

func refinerySiteAPIBase(configured string) string {
	if base := strings.TrimSpace(os.Getenv("SITE_BASE_URL")); base != "" {
		return base
	}
	if base := strings.TrimSpace(os.Getenv("LOVYOU_BASE_URL")); base != "" {
		return base
	}
	configured = strings.TrimSpace(configured)
	if configured != "" && configured != "https://lovyou.ai" {
		return configured
	}
	return "http://localhost:8201"
}

func (r *Runtime) dispatchRefineryAttach(op runner.OpEvent, causes []types.EventID) error {
	var p refineryAttachPayload
	_ = json.Unmarshal(op.Payload, &p)
	if p.ParentID == "" {
		p.ParentID = op.NodeID
	}
	if p.Filename == "" {
		p.Filename = op.NodeTitle
	}
	if err := r.bridgeEmitRefineryArtifactAttached(event.RefineryArtifactAttachedContent{
		IntakeRef:   p.ParentID,
		ArtifactRef: op.NodeID,
		Filename:    p.Filename,
		Hash:        p.Hash,
		AttachedAt:  time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("dispatch refinery artifact attached: %w", err)
	}
	r.recordTranslation(op, r.bridgeAgent.LastEvent())
	return nil
}

func isRefineryStateTransition(raw json.RawMessage) bool {
	var p refineryStatePayload
	if len(raw) == 0 || json.Unmarshal(raw, &p) != nil {
		return false
	}
	return p.ToState != "" || p.FromState != ""
}

func (r *Runtime) dispatchRefineryStateTransition(op runner.OpEvent, causes []types.EventID) error {
	var p refineryStatePayload
	_ = json.Unmarshal(op.Payload, &p)
	if err := r.bridgeEmitRefineryStateTransitioned(event.RefineryStateTransitionedContent{
		IntakeRef:      op.NodeID,
		FromState:      p.FromState,
		ToState:        p.ToState,
		Reason:         p.Reason,
		TransitionedAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("dispatch refinery state transitioned: %w", err)
	}
	r.recordTranslation(op, r.bridgeAgent.LastEvent())
	return nil
}
