package hive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

type siteMirrorPayload struct {
	NodeID       string `json:"node_id"`
	HiveTaskID   string `json:"hive_task_id"`
	HiveChainRef string `json:"hive_chain_ref"`
	EventType    string `json:"event_type"`
	State        string `json:"state"`
	Summary      string `json:"summary"`
}

func (r *Runtime) mirrorTaskCompletion(ctx context.Context, task work.Task, summary string) error {
	if strings.TrimSpace(r.siteAPIBase) == "" {
		return nil
	}
	siteNodeID, siteOpID, err := r.findSiteNodeForTask(task.ID)
	if err != nil {
		return err
	}
	if siteNodeID == "" {
		ancestorNodeID, ancestorTaskID, err := r.findSiteNodeForAncestorTask(task.ID, 5)
		if err != nil || ancestorNodeID == "" {
			return err
		}
		progressSummary := fmt.Sprintf("Hive completed child task `%s`: %s\n\n%s\n\nAncestor Site-originated task: `%s`.", task.ID.Value(), task.Title, strings.TrimSpace(summary), ancestorTaskID.Value())
		return r.mirrorTaskProgress(ctx, ancestorNodeID, task.ID.Value(), task.ID.Value(), work.EventTypeTaskCompleted.Value(), progressSummary)
	}
	payload := siteMirrorPayload{
		NodeID:       siteNodeID,
		HiveTaskID:   task.ID.Value(),
		HiveChainRef: task.ID.Value(),
		EventType:    work.EventTypeTaskCompleted.Value(),
		State:        "done",
		Summary:      summary,
	}
	status, err := r.postSiteMirror(ctx, payload)
	if err != nil {
		return err
	}
	if r.bridgeAgent != nil && siteOpID != "" {
		ref := event.ExternalRef{System: "site", ID: siteOpID}
		mirrorEventID := task.ID
		_ = r.bridgeEmitMirrored(event.SiteOpMirroredContent{
			ExternalRef:   ref,
			MirrorEventID: mirrorEventID,
			HTTPStatus:    status,
			MirroredAt:    time.Now().UTC(),
		})
	}
	return nil
}

func (r *Runtime) mirrorTaskProgress(ctx context.Context, siteNodeID, hiveTaskID, eventID, eventType, summary string) error {
	if strings.TrimSpace(r.siteAPIBase) == "" || strings.TrimSpace(siteNodeID) == "" {
		return nil
	}
	payload := siteMirrorPayload{
		NodeID:       siteNodeID,
		HiveTaskID:   hiveTaskID,
		HiveChainRef: eventID,
		EventType:    eventType,
		Summary:      summary,
	}
	_, err := r.postSiteMirror(ctx, payload)
	return err
}

func (r *Runtime) findTaskCreated(taskID types.EventID) (work.TaskCreatedContent, bool, error) {
	page, err := r.store.ByType(work.EventTypeTaskCreated, 2000, types.None[types.Cursor]())
	if err != nil {
		return work.TaskCreatedContent{}, false, fmt.Errorf("list task created events: %w", err)
	}
	for _, ev := range page.Items() {
		if ev.ID().Value() != taskID.Value() {
			continue
		}
		c, ok := ev.Content().(work.TaskCreatedContent)
		return c, ok, nil
	}
	return work.TaskCreatedContent{}, false, nil
}

func (r *Runtime) postSiteMirror(ctx context.Context, payload siteMirrorPayload) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	base := strings.TrimRight(r.siteAPIBase, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/hive/mirror", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("site mirror returned HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func (r *Runtime) findSiteNodeForTask(taskID types.EventID) (nodeID string, siteOpID string, err error) {
	translated, err := r.store.ByType(event.EventTypeSiteOpTranslated, 1000, types.None[types.Cursor]())
	if err != nil {
		return "", "", fmt.Errorf("list site translations: %w", err)
	}
	for _, ev := range translated.Items() {
		c, ok := ev.Content().(event.SiteOpTranslatedContent)
		if !ok || c.BusEventID.Value() != taskID.Value() {
			continue
		}
		siteOpID = c.ExternalRef.ID
		break
	}
	if siteOpID == "" {
		return "", "", nil
	}

	received, err := r.store.ByType(event.EventTypeSiteOpReceived, 1000, types.None[types.Cursor]())
	if err != nil {
		return "", "", fmt.Errorf("list site op anchors: %w", err)
	}
	for _, ev := range received.Items() {
		c, ok := ev.Content().(event.SiteOpReceivedContent)
		if ok && c.ExternalRef.ID == siteOpID {
			return c.NodeID, siteOpID, nil
		}
	}
	return "", siteOpID, nil
}

func (r *Runtime) findSiteNodeForAncestorTask(taskID types.EventID, maxDepth int) (nodeID string, ancestorTaskID types.EventID, err error) {
	if maxDepth <= 0 {
		return "", types.EventID{}, nil
	}
	deps, err := r.store.ByType(work.EventTypeTaskDependencyAdded, 2000, types.None[types.Cursor]())
	if err != nil {
		return "", types.EventID{}, fmt.Errorf("list task dependencies: %w", err)
	}
	for _, ev := range deps.Items() {
		c, ok := ev.Content().(work.TaskDependencyContent)
		if !ok || c.TaskID.Value() != taskID.Value() {
			continue
		}
		if nodeID, _, err := r.findSiteNodeForTask(c.DependsOnID); err != nil || nodeID != "" {
			return nodeID, c.DependsOnID, err
		}
		if nodeID, ancestor, err := r.findSiteNodeForAncestorTask(c.DependsOnID, maxDepth-1); err != nil || nodeID != "" {
			return nodeID, ancestor, err
		}
	}
	return "", types.EventID{}, nil
}
