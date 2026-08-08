package hive

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

const (
	FactoryRuntimeSnapshotSchemaVersion = "factory-runtime-snapshot/v1"
	FactoryRuntimeSnapshotPath          = "/api/hive/factory/v1/runtime-snapshot"
	FactoryRuntimeHeartbeatInterval     = 5 * time.Second
	FactoryRuntimeHeartbeatFreshness    = 15 * time.Second
	FactoryRuntimeMaxResponseBytes      = 256 * 1024
)

type FactoryRuntimeSchedulerState string

const (
	FactoryRuntimeStarting  FactoryRuntimeSchedulerState = "starting"
	FactoryRuntimePolling   FactoryRuntimeSchedulerState = "polling"
	FactoryRuntimeExecuting FactoryRuntimeSchedulerState = "executing"
	FactoryRuntimeDegraded  FactoryRuntimeSchedulerState = "degraded"
	FactoryRuntimeStopping  FactoryRuntimeSchedulerState = "stopping"
)

func (s FactoryRuntimeSchedulerState) valid() bool {
	switch s {
	case FactoryRuntimeStarting, FactoryRuntimePolling, FactoryRuntimeExecuting, FactoryRuntimeDegraded, FactoryRuntimeStopping:
		return true
	default:
		return false
	}
}

type FactoryRuntimeSnapshot struct {
	SchemaVersion           string                        `json:"schema_version"`
	DaemonInstanceID        string                        `json:"daemon_instance_id"`
	BootID                  string                        `json:"boot_id"`
	RecoveryGeneration      int                           `json:"recovery_generation"`
	Sequence                uint64                        `json:"sequence"`
	ProcessStartedAt        time.Time                     `json:"process_started_at"`
	ObservedAt              time.Time                     `json:"observed_at"`
	LastHeartbeatAt         time.Time                     `json:"last_heartbeat_at"`
	LastSchedulerProgressAt time.Time                     `json:"last_scheduler_progress_at"`
	SchedulerState          FactoryRuntimeSchedulerState  `json:"scheduler_state"`
	ConfiguredWorkers       int                           `json:"configured_workers"`
	ActiveWorkers           int                           `json:"active_workers"`
	AvailableWorkers        int                           `json:"available_workers"`
	QueuedOrders            int                           `json:"queued_orders"`
	SchedulableOrders       int                           `json:"schedulable_orders"`
	Assignments             []factoryv1.RuntimeAssignment `json:"assignments"`
	LastErrorSummary        string                        `json:"last_error_summary,omitempty"`
}

type factoryRuntimeSchedulerSource interface {
	RuntimeSnapshot() factoryv1.SchedulerRuntimeSnapshot
}

type FactoryRuntimeMonitorConfig struct {
	InstanceID         string
	BootID             string
	RecoveryGeneration int
	Clock              factoryv1.Clock
	HeartbeatInterval  time.Duration
}

// FactoryRuntimeMonitor owns only boot-scoped, in-memory observation state.
// It never writes EventGraph or Work and never exposes credentials.
type FactoryRuntimeMonitor struct {
	scheduler factoryRuntimeSchedulerSource
	clock     factoryv1.Clock
	interval  time.Duration

	mu                 sync.Mutex
	instanceID         string
	bootID             string
	recoveryGeneration int
	processStartedAt   time.Time
	lastHeartbeatAt    time.Time
	lastProgressAt     time.Time
	state              FactoryRuntimeSchedulerState
	lastError          string
	sequence           uint64
}

func NewFactoryRuntimeMonitor(scheduler factoryRuntimeSchedulerSource, config FactoryRuntimeMonitorConfig) (*FactoryRuntimeMonitor, error) {
	if scheduler == nil {
		return nil, errors.New("factory runtime monitor requires scheduler")
	}
	if strings.TrimSpace(config.InstanceID) == "" {
		return nil, errors.New("factory runtime monitor requires daemon instance id")
	}
	if config.RecoveryGeneration < 0 {
		return nil, errors.New("factory runtime recovery generation must be non-negative")
	}
	if config.Clock == nil {
		config.Clock = factoryv1.WallClock{}
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = FactoryRuntimeHeartbeatInterval
	}
	if config.HeartbeatInterval < 100*time.Millisecond || config.HeartbeatInterval > time.Minute {
		return nil, errors.New("factory runtime heartbeat interval must be between 100ms and 1m")
	}
	bootID := strings.TrimSpace(config.BootID)
	if bootID == "" {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return nil, fmt.Errorf("generate factory runtime boot id: %w", err)
		}
		bootID = hex.EncodeToString(raw[:])
	}
	now := config.Clock.Now().UTC()
	return &FactoryRuntimeMonitor{
		scheduler: scheduler, clock: config.Clock, interval: config.HeartbeatInterval,
		instanceID: strings.TrimSpace(config.InstanceID), bootID: bootID,
		recoveryGeneration: config.RecoveryGeneration, processStartedAt: now,
		lastHeartbeatAt: now, lastProgressAt: now, state: FactoryRuntimeStarting, sequence: 1,
	}, nil
}

// RunHeartbeat keeps liveness independent from runner calls.
func (m *FactoryRuntimeMonitor) RunHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.SetState(FactoryRuntimeStopping, nil)
			return
		case <-ticker.C:
			m.Heartbeat()
		}
	}
}

func (m *FactoryRuntimeMonitor) Heartbeat() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastHeartbeatAt = m.clock.Now().UTC()
	m.sequence++
}

func (m *FactoryRuntimeMonitor) SetState(state FactoryRuntimeSchedulerState, cycleErr error) {
	if !state.valid() {
		state = FactoryRuntimeDegraded
		cycleErr = errors.New("unknown scheduler state")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = state
	m.lastProgressAt = m.clock.Now().UTC()
	if cycleErr != nil {
		m.lastError = boundedRuntimeError(cycleErr.Error())
	} else if state != FactoryRuntimeDegraded {
		m.lastError = ""
	}
	m.sequence++
}

func boundedRuntimeError(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "factory scheduler cycle reported an error"
}

func (m *FactoryRuntimeMonitor) Snapshot() FactoryRuntimeSnapshot {
	scheduler := m.scheduler.RuntimeSnapshot()
	m.mu.Lock()
	defer m.mu.Unlock()
	progress := m.lastProgressAt
	if scheduler.LastProgressAt.After(progress) {
		progress = scheduler.LastProgressAt
	}
	assignments := append([]factoryv1.RuntimeAssignment(nil), scheduler.Assignments...)
	return FactoryRuntimeSnapshot{
		SchemaVersion:    FactoryRuntimeSnapshotSchemaVersion,
		DaemonInstanceID: m.instanceID, BootID: m.bootID,
		RecoveryGeneration: m.recoveryGeneration,
		Sequence:           m.sequence + scheduler.Sequence,
		ProcessStartedAt:   m.processStartedAt, ObservedAt: m.clock.Now().UTC(),
		LastHeartbeatAt: m.lastHeartbeatAt, LastSchedulerProgressAt: progress,
		SchedulerState:    m.state,
		ConfiguredWorkers: scheduler.ConfiguredWorkers, ActiveWorkers: scheduler.ActiveWorkers,
		AvailableWorkers: scheduler.AvailableWorkers, QueuedOrders: scheduler.QueuedOrders,
		SchedulableOrders: scheduler.SchedulableOrders, Assignments: assignments,
		LastErrorSummary: m.lastError,
	}
}

func ValidateFactoryRuntimeSnapshot(snapshot FactoryRuntimeSnapshot, now time.Time, previous *FactoryRuntimeSnapshot) error {
	if snapshot.SchemaVersion != FactoryRuntimeSnapshotSchemaVersion {
		return fmt.Errorf("unsupported runtime schema %q", snapshot.SchemaVersion)
	}
	if strings.TrimSpace(snapshot.DaemonInstanceID) == "" || strings.TrimSpace(snapshot.BootID) == "" || snapshot.RecoveryGeneration < 0 || snapshot.Sequence == 0 {
		return errors.New("runtime identity is incomplete")
	}
	if !snapshot.SchedulerState.valid() {
		return fmt.Errorf("unknown scheduler state %q", snapshot.SchedulerState)
	}
	if snapshot.ConfiguredWorkers < 1 || snapshot.ConfiguredWorkers > 64 || snapshot.ActiveWorkers < 0 || snapshot.ActiveWorkers > snapshot.ConfiguredWorkers || snapshot.AvailableWorkers != snapshot.ConfiguredWorkers-snapshot.ActiveWorkers {
		return errors.New("runtime worker arithmetic is invalid")
	}
	if snapshot.QueuedOrders < 0 || snapshot.SchedulableOrders < 0 || snapshot.SchedulableOrders > snapshot.QueuedOrders || len(snapshot.Assignments) != snapshot.ActiveWorkers {
		return errors.New("runtime demand or assignment count is invalid")
	}
	if snapshot.ProcessStartedAt.IsZero() || snapshot.ObservedAt.IsZero() || snapshot.LastHeartbeatAt.IsZero() || snapshot.LastSchedulerProgressAt.IsZero() || snapshot.ProcessStartedAt.After(snapshot.LastHeartbeatAt) || snapshot.ProcessStartedAt.After(snapshot.LastSchedulerProgressAt) || snapshot.LastHeartbeatAt.After(snapshot.ObservedAt) || snapshot.LastSchedulerProgressAt.After(snapshot.ObservedAt) || snapshot.ObservedAt.After(now.Add(time.Second)) {
		return errors.New("runtime timestamps are invalid")
	}
	seenOrder, seenAttempt := map[string]struct{}{}, map[string]struct{}{}
	for _, assignment := range snapshot.Assignments {
		if strings.TrimSpace(assignment.OrderID) == "" || strings.TrimSpace(assignment.OrderVersion) == "" || !isExactGitOrDocumentHash(assignment.DocumentSHA256) ||
			factoryv1.StageIndex(assignment.Stage) < 0 || strings.TrimSpace(assignment.AttemptID) == "" || strings.TrimSpace(assignment.ProviderID) == "" || strings.TrimSpace(assignment.ModelID) == "" ||
			assignment.AssignedAt.IsZero() || assignment.AssignedAt.Before(snapshot.ProcessStartedAt) || assignment.AssignedAt.After(snapshot.ObservedAt) {
			return errors.New("runtime assignment identity is invalid")
		}
		if _, exists := seenOrder[assignment.OrderID]; exists {
			return errors.New("runtime assignments contain duplicate order")
		}
		seenOrder[assignment.OrderID] = struct{}{}
		if _, exists := seenAttempt[assignment.AttemptID]; exists {
			return errors.New("runtime assignments contain duplicate attempt")
		}
		seenAttempt[assignment.AttemptID] = struct{}{}
	}
	if previous != nil {
		sameBoot := previous.DaemonInstanceID == snapshot.DaemonInstanceID && previous.BootID == snapshot.BootID
		if sameBoot {
			if previous.RecoveryGeneration != snapshot.RecoveryGeneration {
				return errors.New("runtime recovery identity changed within one boot")
			}
			if snapshot.ProcessStartedAt != previous.ProcessStartedAt {
				return errors.New("runtime process identity changed within one boot")
			}
			if snapshot.Sequence < previous.Sequence {
				return errors.New("runtime sequence regressed within one boot")
			}
		} else {
			if !snapshot.ProcessStartedAt.After(previous.ProcessStartedAt) {
				return errors.New("runtime snapshot returned an older or conflicting boot")
			}
			if snapshot.DaemonInstanceID == previous.DaemonInstanceID && snapshot.RecoveryGeneration < previous.RecoveryGeneration {
				return errors.New("runtime recovery generation regressed across boots")
			}
		}
	}
	return nil
}

func isExactGitOrDocumentHash(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}

func FactoryRuntimeSnapshotCurrentHealthy(snapshot FactoryRuntimeSnapshot, now time.Time) bool {
	if ValidateFactoryRuntimeSnapshot(snapshot, now, nil) != nil || now.Sub(snapshot.LastHeartbeatAt) > FactoryRuntimeHeartbeatFreshness {
		return false
	}
	if snapshot.SchedulerState == FactoryRuntimeStopping || snapshot.SchedulerState == FactoryRuntimeDegraded {
		return false
	}
	return true
}

func NewFactoryRuntimeSnapshotHandler(monitor *FactoryRuntimeMonitor, apiKey string) (http.Handler, error) {
	if monitor == nil || strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("factory runtime handler requires monitor and bearer key")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+FactoryRuntimeSnapshotPath, func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if len(provided) != len(apiKey) || subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		snapshot := monitor.Snapshot()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(snapshot)
	})
	return mux, nil
}

func ValidateFactoryRuntimeListenAddress(address string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return fmt.Errorf("runtime listen address: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return errors.New("factory runtime listener must bind to loopback")
	}
	return nil
}

type FactoryRuntimeClient struct {
	Endpoint string
	APIKey   string
	Client   *http.Client
}

func (c FactoryRuntimeClient) Fetch(ctx context.Context, now time.Time, previous *FactoryRuntimeSnapshot) (FactoryRuntimeSnapshot, error) {
	endpoint := strings.TrimSpace(c.Endpoint)
	if endpoint == "" || strings.TrimSpace(c.APIKey) == "" {
		return FactoryRuntimeSnapshot{}, errors.New("factory runtime client is unconfigured")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return FactoryRuntimeSnapshot{}, errors.New("factory runtime endpoint is invalid")
	}
	host := parsed.Hostname()
	if !strings.EqualFold(host, "localhost") {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return FactoryRuntimeSnapshot{}, errors.New("factory runtime endpoint must be loopback")
		}
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return FactoryRuntimeSnapshot{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := client.Do(req)
	if err != nil {
		return FactoryRuntimeSnapshot{}, fmt.Errorf("factory runtime request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return FactoryRuntimeSnapshot{}, fmt.Errorf("factory runtime returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, FactoryRuntimeMaxResponseBytes+1))
	if err != nil {
		return FactoryRuntimeSnapshot{}, fmt.Errorf("read factory runtime response: %w", err)
	}
	if len(body) > FactoryRuntimeMaxResponseBytes {
		return FactoryRuntimeSnapshot{}, errors.New("factory runtime response exceeds limit")
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var snapshot FactoryRuntimeSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return FactoryRuntimeSnapshot{}, fmt.Errorf("decode factory runtime response: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return FactoryRuntimeSnapshot{}, errors.New("factory runtime response has trailing data")
	}
	if err := ValidateFactoryRuntimeSnapshot(snapshot, now.UTC(), previous); err != nil {
		return FactoryRuntimeSnapshot{}, err
	}
	sort.Slice(snapshot.Assignments, func(i, j int) bool { return snapshot.Assignments[i].OrderID < snapshot.Assignments[j].OrderID })
	return snapshot, nil
}
