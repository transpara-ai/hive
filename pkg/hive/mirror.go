package hive

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

type siteTaskMirrorPayload struct {
	NodeID       string `json:"node_id"`
	HiveTaskID   string `json:"hive_task_id"`
	HiveChainRef string `json:"hive_chain_ref"`
	EventType    string `json:"event_type"`
	State        string `json:"state"`
	Summary      string `json:"summary,omitempty"`
}

type siteTaskMirrorTarget struct {
	siteOpID string
	nodeID   string
}

func (r *Runtime) mirrorTaskCompletion(ctx context.Context, task work.Task, summary string) {
	if r.apiClient == nil {
		return
	}
	target, ok, err := r.findSiteMirrorTarget(task.ID)
	if err != nil {
		log.Printf("[mirror] find site target for task %s failed: %v", task.ID.Value(), err)
		return
	}
	if !ok {
		return
	}
	chainRef := task.ID
	if completedID, found, err := r.findCompletedEventForTask(task.ID); err != nil {
		log.Printf("[mirror] find completion event for task %s failed: %v", task.ID.Value(), err)
	} else if found {
		chainRef = completedID
	}

	payload := siteTaskMirrorPayload{
		NodeID:       target.nodeID,
		HiveTaskID:   task.ID.Value(),
		HiveChainRef: chainRef.Value(),
		EventType:    work.EventTypeTaskCompleted.Value(),
		State:        "done",
		Summary:      summary,
	}
	if err := r.apiClient.MirrorToSite(payload, nil); err != nil {
		log.Printf("[mirror] POST /api/hive/mirror failed for task %s: %v", task.ID.Value(), err)
		return
	}
	if target.siteOpID != "" {
		if err := r.bridgeEmitMirrored(event.SiteOpMirroredContent{
			ExternalRef:   event.ExternalRef{System: "site", ID: target.siteOpID},
			MirrorEventID: chainRef,
			HTTPStatus:    http.StatusOK,
			MirroredAt:    time.Now().UTC(),
		}); err != nil {
			log.Printf("[mirror] emit site.op.mirrored failed: %v", err)
		}
	}
}

func (r *Runtime) findSiteMirrorTarget(taskID types.EventID) (siteTaskMirrorTarget, bool, error) {
	if target, ok, err := r.findDirectSiteMirrorTarget(taskID); err != nil || ok {
		return target, ok, err
	}
	return r.findAncestorSiteMirrorTarget(taskID, 8)
}

func (r *Runtime) findDirectSiteMirrorTarget(taskID types.EventID) (siteTaskMirrorTarget, bool, error) {
	translated, err := r.store.ByType(event.EventTypeSiteOpTranslated, 1000, types.None[types.Cursor]())
	if err != nil {
		return siteTaskMirrorTarget{}, false, fmt.Errorf("site.op.translated: %w", err)
	}
	for _, ev := range translated.Items() {
		c, ok := ev.Content().(event.SiteOpTranslatedContent)
		if !ok || c.BusEventID != taskID {
			continue
		}
		nodeID, ok, err := r.findSiteNodeByOpID(c.ExternalRef.ID)
		if err != nil || !ok {
			return siteTaskMirrorTarget{}, ok, err
		}
		return siteTaskMirrorTarget{siteOpID: c.ExternalRef.ID, nodeID: nodeID}, true, nil
	}
	return siteTaskMirrorTarget{}, false, nil
}

func (r *Runtime) findAncestorSiteMirrorTarget(taskID types.EventID, maxDepth int) (siteTaskMirrorTarget, bool, error) {
	if maxDepth <= 0 {
		return siteTaskMirrorTarget{}, false, nil
	}
	deps, err := r.store.ByType(work.EventTypeTaskDependencyAdded, 1000, types.None[types.Cursor]())
	if err != nil {
		return siteTaskMirrorTarget{}, false, fmt.Errorf("work.task.dependency.added: %w", err)
	}
	for _, ev := range deps.Items() {
		c, ok := ev.Content().(work.TaskDependencyContent)
		if !ok || c.TaskID != taskID {
			continue
		}
		if target, ok, err := r.findDirectSiteMirrorTarget(c.DependsOnID); err != nil || ok {
			return target, ok, err
		}
		if target, ok, err := r.findAncestorSiteMirrorTarget(c.DependsOnID, maxDepth-1); err != nil || ok {
			return target, ok, err
		}
	}
	return siteTaskMirrorTarget{}, false, nil
}

func (r *Runtime) findSiteNodeByOpID(siteOpID string) (string, bool, error) {
	received, err := r.store.ByType(event.EventTypeSiteOpReceived, 1000, types.None[types.Cursor]())
	if err != nil {
		return "", false, fmt.Errorf("site.op.received: %w", err)
	}
	for _, ev := range received.Items() {
		c, ok := ev.Content().(event.SiteOpReceivedContent)
		if ok && c.ExternalRef.ID == siteOpID && c.NodeID != "" {
			return c.NodeID, true, nil
		}
	}
	return "", false, nil
}

func (r *Runtime) findCompletedEventForTask(taskID types.EventID) (types.EventID, bool, error) {
	completed, err := r.store.ByType(work.EventTypeTaskCompleted, 1000, types.None[types.Cursor]())
	if err != nil {
		return types.EventID{}, false, fmt.Errorf("work.task.completed: %w", err)
	}
	for _, ev := range completed.Items() {
		c, ok := ev.Content().(work.TaskCompletedContent)
		if ok && c.TaskID == taskID {
			return ev.ID(), true, nil
		}
	}
	return types.EventID{}, false, nil
}
