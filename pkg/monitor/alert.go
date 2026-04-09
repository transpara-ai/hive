// Package monitor provides configurable alerting for k3s cluster metrics.
//
// The alerting engine evaluates node and pod metrics against configurable
// thresholds and produces alerts with three states: OK, Warning, Critical.
// Alerts can be delivered via webhook notifications.
package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/lovyou-ai/hive/pkg/config"
	"github.com/lovyou-ai/hive/pkg/store"
)

// AlertState represents the severity of an alert.
type AlertState string

const (
	AlertOK       AlertState = "ok"
	AlertWarning  AlertState = "warning"
	AlertCritical AlertState = "critical"
)

// ResourceKind identifies which resource is being monitored.
type ResourceKind string

const (
	ResourceCPU    ResourceKind = "cpu"
	ResourceMemory ResourceKind = "memory"
	ResourceDisk   ResourceKind = "disk"
)

// TargetKind identifies whether the alert targets a node or pod.
type TargetKind string

const (
	TargetNode TargetKind = "node"
	TargetPod  TargetKind = "pod"
)

// Alert is a single alerting event produced by the engine.
type Alert struct {
	State      AlertState   `json:"state"`
	Resource   ResourceKind `json:"resource"`
	Target     TargetKind   `json:"target"`
	TargetName string       `json:"target_name"`
	Namespace  string       `json:"namespace,omitempty"` // only for pods
	Value      float64      `json:"value"`               // observed fraction 0.0-1.0
	Threshold  float64      `json:"threshold"`            // threshold that was breached
	Message    string       `json:"message"`
	FiredAt    time.Time    `json:"fired_at"`
}

// AlertRule allows per-target threshold overrides. When set, these take
// precedence over the global AlertThreshold for the matching target.
type AlertRule struct {
	TargetKind TargetKind   `json:"target_kind"`
	TargetName string       `json:"target_name"` // exact match; empty = all targets of this kind
	Resource   ResourceKind `json:"resource"`
	Warning    float64      `json:"warning"`  // fraction 0.0-1.0
	Critical   float64      `json:"critical"` // fraction 0.0-1.0
}

// Validate checks that the rule's thresholds are sensible.
func (r AlertRule) Validate() error {
	if r.Warning < 0 || r.Warning > 1 {
		return fmt.Errorf("alert rule: warning threshold must be 0.0-1.0, got %.3f", r.Warning)
	}
	if r.Critical < 0 || r.Critical > 1 {
		return fmt.Errorf("alert rule: critical threshold must be 0.0-1.0, got %.3f", r.Critical)
	}
	if r.Warning > r.Critical {
		return fmt.Errorf("alert rule: warning (%.3f) must be <= critical (%.3f)", r.Warning, r.Critical)
	}
	if r.Resource == "" {
		return fmt.Errorf("alert rule: resource is required")
	}
	if r.TargetKind == "" {
		return fmt.Errorf("alert rule: target_kind is required")
	}
	return nil
}

// WebhookConfig holds the destination for alert notifications.
type WebhookConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"` // extra headers (e.g. Authorization)
	Timeout time.Duration     `json:"timeout"`           // 0 means 10s default
}

func (w WebhookConfig) effectiveTimeout() time.Duration {
	if w.Timeout > 0 {
		return w.Timeout
	}
	return 10 * time.Second
}

// WebhookPayload is the JSON body sent to webhook endpoints.
type WebhookPayload struct {
	Alerts    []Alert   `json:"alerts"`
	FiredAt   time.Time `json:"fired_at"`
	AlertCount int      `json:"alert_count"`
}

// ── Engine ──────────────────────────────────────────────────────────────

// Engine evaluates cluster snapshots against thresholds and fires alerts.
// It is safe for concurrent use.
type Engine struct {
	mu         sync.RWMutex
	thresholds config.AlertThreshold
	rules      []AlertRule
	webhooks   []WebhookConfig
	alerts     []Alert
	client     *http.Client
}

// NewEngine returns an alerting engine with the given global thresholds.
func NewEngine(thresholds config.AlertThreshold) *Engine {
	return &Engine{
		thresholds: thresholds,
		client:     &http.Client{},
	}
}

// AddRule registers a per-target threshold override.
func (e *Engine) AddRule(r AlertRule) error {
	if err := r.Validate(); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, r)
	return nil
}

// AddWebhook registers a webhook endpoint for alert delivery.
func (e *Engine) AddWebhook(w WebhookConfig) error {
	if w.URL == "" {
		return fmt.Errorf("webhook: URL is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.webhooks = append(e.webhooks, w)
	return nil
}

// Alerts returns a copy of the most recent alert set.
func (e *Engine) Alerts() []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Alert, len(e.alerts))
	copy(out, e.alerts)
	return out
}

// Evaluate inspects a snapshot and produces alerts for any threshold breaches.
// It replaces the previous alert set and fires webhooks for non-OK alerts.
func (e *Engine) Evaluate(ctx context.Context, snap store.Snapshot) []Alert {
	now := time.Now()
	var alerts []Alert

	for _, n := range snap.Nodes {
		alerts = append(alerts, e.evaluateNode(n, now)...)
	}
	for _, p := range snap.Pods {
		alerts = append(alerts, e.evaluatePod(p, now)...)
	}

	e.mu.Lock()
	e.alerts = alerts
	webhooks := make([]WebhookConfig, len(e.webhooks))
	copy(webhooks, e.webhooks)
	e.mu.Unlock()

	// Fire webhooks for non-OK alerts.
	firing := filterFiring(alerts)
	if len(firing) > 0 && len(webhooks) > 0 {
		e.fireWebhooks(ctx, webhooks, firing, now)
	}

	return alerts
}

// ── Node evaluation ─────────────────────────────────────────────────────

func (e *Engine) evaluateNode(n store.NodeMetrics, now time.Time) []Alert {
	var alerts []Alert

	cpuWarn, cpuCrit := e.thresholdsFor(TargetNode, n.Name, ResourceCPU)
	memWarn, memCrit := e.thresholdsFor(TargetNode, n.Name, ResourceMemory)
	diskWarn, diskCrit := e.thresholdsFor(TargetNode, n.Name, ResourceDisk)

	alerts = append(alerts, classify(
		n.CPUUsage, cpuWarn, cpuCrit,
		ResourceCPU, TargetNode, n.Name, "", now,
	))

	alerts = append(alerts, classify(
		n.MemoryUsage, memWarn, memCrit,
		ResourceMemory, TargetNode, n.Name, "", now,
	))

	// Disk usage -1 means unavailable; skip.
	if n.DiskUsage >= 0 {
		alerts = append(alerts, classify(
			n.DiskUsage, diskWarn, diskCrit,
			ResourceDisk, TargetNode, n.Name, "", now,
		))
	}

	return alerts
}

// ── Pod evaluation ──────────────────────────────────────────────────────

func (e *Engine) evaluatePod(p store.PodMetrics, now time.Time) []Alert {
	var alerts []Alert

	// Pod CPU usage is absolute (cores). Convert to fraction of limit if available.
	cpuFrac := podCPUFraction(p)
	memFrac := podMemFraction(p)

	cpuWarn, cpuCrit := e.thresholdsFor(TargetPod, p.Name, ResourceCPU)
	memWarn, memCrit := e.thresholdsFor(TargetPod, p.Name, ResourceMemory)

	if cpuFrac >= 0 {
		alerts = append(alerts, classify(
			cpuFrac, cpuWarn, cpuCrit,
			ResourceCPU, TargetPod, p.Name, p.Namespace, now,
		))
	}

	if memFrac >= 0 {
		alerts = append(alerts, classify(
			memFrac, memWarn, memCrit,
			ResourceMemory, TargetPod, p.Name, p.Namespace, now,
		))
	}

	return alerts
}

// podCPUFraction returns CPU usage as a fraction of the limit.
// Returns -1 if the pod has no CPU limit set.
func podCPUFraction(p store.PodMetrics) float64 {
	if p.CPULimit <= 0 {
		return -1
	}
	return p.CPUUsage / p.CPULimit
}

// podMemFraction returns memory usage as a fraction of the limit.
// Returns -1 if the pod has no memory limit set.
func podMemFraction(p store.PodMetrics) float64 {
	if p.MemoryLimit <= 0 {
		return -1
	}
	return float64(p.MemoryBytes) / float64(p.MemoryLimit)
}

// ── Threshold resolution ────────────────────────────────────────────────

// thresholdsFor returns the (warning, critical) thresholds for a specific
// target and resource. Per-target rules take precedence over global defaults.
func (e *Engine) thresholdsFor(kind TargetKind, name string, res ResourceKind) (warn, crit float64) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check rules from most to least specific.
	for _, r := range e.rules {
		if r.TargetKind != kind || r.Resource != res {
			continue
		}
		// Exact match on name, or wildcard (empty name).
		if r.TargetName == name || r.TargetName == "" {
			return r.Warning, r.Critical
		}
	}

	// Fall back to global thresholds.
	return e.globalThresholds(kind, res)
}

// globalThresholds returns the default thresholds from config.AlertThreshold.
func (e *Engine) globalThresholds(kind TargetKind, res ResourceKind) (warn, crit float64) {
	t := e.thresholds
	switch {
	case kind == TargetNode && res == ResourceCPU:
		return t.CPUWarning, t.CPUCritical
	case kind == TargetNode && res == ResourceMemory:
		return t.MemWarning, t.MemCritical
	case kind == TargetNode && res == ResourceDisk:
		return t.DiskWarning, t.DiskCritical
	case kind == TargetPod && res == ResourceCPU:
		return t.CPUWarning, t.CPUCritical
	case kind == TargetPod && res == ResourceMemory:
		return t.MemWarning, t.MemCritical
	default:
		return 0.80, 0.95 // safe fallback
	}
}

// ── Classification ──────────────────────────────────────────────────────

// classify produces an Alert for a given metric value against warning/critical thresholds.
func classify(value, warn, crit float64, res ResourceKind, target TargetKind, name, ns string, now time.Time) Alert {
	var state AlertState
	var threshold float64

	switch {
	case value >= crit:
		state = AlertCritical
		threshold = crit
	case value >= warn:
		state = AlertWarning
		threshold = warn
	default:
		state = AlertOK
		threshold = warn
	}

	return Alert{
		State:      state,
		Resource:   res,
		Target:     target,
		TargetName: name,
		Namespace:  ns,
		Value:      value,
		Threshold:  threshold,
		Message:    formatMessage(state, res, target, name, ns, value, threshold),
		FiredAt:    now,
	}
}

func formatMessage(state AlertState, res ResourceKind, target TargetKind, name, ns string, value, threshold float64) string {
	targetStr := string(target) + " " + name
	if ns != "" {
		targetStr = string(target) + " " + ns + "/" + name
	}

	switch state {
	case AlertCritical:
		return fmt.Sprintf("%s %s at %.1f%% (critical threshold %.1f%%)", targetStr, res, value*100, threshold*100)
	case AlertWarning:
		return fmt.Sprintf("%s %s at %.1f%% (warning threshold %.1f%%)", targetStr, res, value*100, threshold*100)
	default:
		return fmt.Sprintf("%s %s at %.1f%% (ok)", targetStr, res, value*100)
	}
}

// filterFiring returns only non-OK alerts.
func filterFiring(alerts []Alert) []Alert {
	var firing []Alert
	for _, a := range alerts {
		if a.State != AlertOK {
			firing = append(firing, a)
		}
	}
	return firing
}

// ── Webhook delivery ────────────────────────────────────────────────────

// fireWebhooks sends alerts to all configured webhook endpoints.
// Errors are logged but do not block the alerting pipeline.
func (e *Engine) fireWebhooks(ctx context.Context, webhooks []WebhookConfig, alerts []Alert, now time.Time) {
	payload := WebhookPayload{
		Alerts:     alerts,
		FiredAt:    now,
		AlertCount: len(alerts),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return // encoding a known struct should not fail
	}

	for _, wh := range webhooks {
		e.sendWebhook(ctx, wh, body)
	}
}

func (e *Engine) sendWebhook(ctx context.Context, wh WebhookConfig, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, wh.effectiveTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	for k, v := range wh.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
