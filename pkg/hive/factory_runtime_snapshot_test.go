package hive

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

type missionControlClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *missionControlClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *missionControlClock) Add(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	c.mu.Unlock()
}

type runtimeSchedulerStub struct {
	mu       sync.Mutex
	snapshot factoryv1.SchedulerRuntimeSnapshot
}

func (s *runtimeSchedulerStub) RuntimeSnapshot() factoryv1.SchedulerRuntimeSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := s.snapshot
	copy.Assignments = append([]factoryv1.RuntimeAssignment(nil), s.snapshot.Assignments...)
	return copy
}

func (s *runtimeSchedulerStub) Set(snapshot factoryv1.SchedulerRuntimeSnapshot) {
	s.mu.Lock()
	s.snapshot = snapshot
	s.mu.Unlock()
}

func TestHIVEMCT2RuntimeSnapshotAuthenticationAndValidation(t *testing.T) {
	now := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	clock := &missionControlClock{now: now}
	scheduler := &runtimeSchedulerStub{snapshot: factoryv1.SchedulerRuntimeSnapshot{
		Sequence: 4, ConfiguredWorkers: 3, ActiveWorkers: 1, AvailableWorkers: 2,
		QueuedOrders: 2, SchedulableOrders: 1, LastProgressAt: now,
		Assignments: []factoryv1.RuntimeAssignment{{
			OrderID: "FO-MC-1", OrderVersion: "1.0.0", DocumentSHA256: strings.Repeat("a", 64),
			Stage: factoryv1.StageWriteCode, AttemptID: "attempt-1", ProviderID: "codex-cli", ModelID: "gpt-5.6-sol", AssignedAt: now,
		}},
	}}
	monitor, err := NewFactoryRuntimeMonitor(scheduler, FactoryRuntimeMonitorConfig{
		InstanceID: "factory-daemon-test", BootID: "boot-test", RecoveryGeneration: 2,
		Clock: clock, HeartbeatInterval: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	monitor.SetState(FactoryRuntimeExecuting, nil)
	handler, err := NewFactoryRuntimeSnapshotHandler(monitor, "runtime-secret")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	unauthorized, err := http.Get(server.URL + FactoryRuntimeSnapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.StatusCode)
	}
	postRequest, _ := http.NewRequest(http.MethodPost, server.URL+FactoryRuntimeSnapshotPath, nil)
	postRequest.Header.Set("Authorization", "Bearer runtime-secret")
	postResponse, err := server.Client().Do(postRequest)
	if err != nil {
		t.Fatal(err)
	}
	postResponse.Body.Close()
	if postResponse.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", postResponse.StatusCode)
	}

	client := FactoryRuntimeClient{Endpoint: server.URL + FactoryRuntimeSnapshotPath, APIKey: "runtime-secret", Client: server.Client()}
	snapshot, err := client.Fetch(context.Background(), now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SchemaVersion != FactoryRuntimeSnapshotSchemaVersion || snapshot.BootID != "boot-test" || snapshot.RecoveryGeneration != 2 || snapshot.ActiveWorkers != 1 || snapshot.AvailableWorkers != 2 || len(snapshot.Assignments) != 1 || !FactoryRuntimeSnapshotCurrentHealthy(snapshot, now) {
		t.Fatalf("HIVE-MC-T2 snapshot = %+v", snapshot)
	}
	if snapshot.Assignments[0].ProviderID != "codex-cli" || snapshot.Assignments[0].ModelID != "gpt-5.6-sol" {
		t.Fatalf("HIVE-MC-T2 assignment = %+v", snapshot.Assignments[0])
	}

	scheduler.Set(factoryv1.SchedulerRuntimeSnapshot{Sequence: 5, ConfiguredWorkers: 3, AvailableWorkers: 3, LastProgressAt: now})
	zero := monitor.Snapshot()
	if zero.ActiveWorkers != 0 || zero.AvailableWorkers != 3 || len(zero.Assignments) != 0 {
		t.Fatalf("HIVE-MC-T2 zero utilization = %+v", zero)
	}
	fullAssignments := make([]factoryv1.RuntimeAssignment, 0, 3)
	for i := 0; i < 3; i++ {
		fullAssignments = append(fullAssignments, factoryv1.RuntimeAssignment{
			OrderID: "FO-MC-FULL-" + string(rune('1'+i)), OrderVersion: "1.0.0", DocumentSHA256: strings.Repeat(string(rune('b'+i)), 64),
			Stage: factoryv1.StageWriteCode, AttemptID: "attempt-full-" + string(rune('1'+i)), ProviderID: "codex-cli", ModelID: "gpt-5.6-sol", AssignedAt: now,
		})
	}
	scheduler.Set(factoryv1.SchedulerRuntimeSnapshot{Sequence: 6, ConfiguredWorkers: 3, ActiveWorkers: 3, AvailableWorkers: 0, QueuedOrders: 4, SchedulableOrders: 3, LastProgressAt: now, Assignments: fullAssignments})
	full := monitor.Snapshot()
	if full.ActiveWorkers != 3 || full.AvailableWorkers != 0 || full.QueuedOrders != 4 || full.SchedulableOrders != 3 || len(full.Assignments) != 3 || full.Sequence <= zero.Sequence {
		t.Fatalf("HIVE-MC-T2 full utilization = %+v; zero sequence=%d", full, zero.Sequence)
	}
}

func TestHIVEMCT2HeartbeatAdvancesWhileSchedulerIsBlocked(t *testing.T) {
	scheduler := &runtimeSchedulerStub{snapshot: factoryv1.SchedulerRuntimeSnapshot{ConfiguredWorkers: 3, AvailableWorkers: 3}}
	monitor, err := NewFactoryRuntimeMonitor(scheduler, FactoryRuntimeMonitorConfig{InstanceID: "blocked-daemon", BootID: "blocked-boot", Clock: factoryv1.WallClock{}, HeartbeatInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	before := monitor.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	go monitor.RunHeartbeat(ctx)
	t.Cleanup(cancel)
	deadline := time.After(750 * time.Millisecond)
	for {
		after := monitor.Snapshot()
		if after.Sequence > before.Sequence && after.LastHeartbeatAt.After(before.LastHeartbeatAt) {
			cancel()
			return
		}
		select {
		case <-deadline:
			t.Fatalf("heartbeat did not advance independently: before=%+v after=%+v", before, after)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestHIVEMCT3RuntimeRestartExpiryAndFailClosedArithmetic(t *testing.T) {
	now := time.Date(2026, 8, 8, 10, 0, 20, 0, time.UTC)
	base := FactoryRuntimeSnapshot{
		SchemaVersion: FactoryRuntimeSnapshotSchemaVersion, DaemonInstanceID: "factory-daemon",
		BootID: "boot-a", RecoveryGeneration: 1, Sequence: 10,
		ProcessStartedAt: now.Add(-time.Minute), ObservedAt: now,
		LastHeartbeatAt: now.Add(-14 * time.Second), LastSchedulerProgressAt: now.Add(-time.Second),
		SchedulerState: FactoryRuntimePolling, ConfiguredWorkers: 3, ActiveWorkers: 0,
		AvailableWorkers: 3, QueuedOrders: 0, SchedulableOrders: 0, Assignments: []factoryv1.RuntimeAssignment{},
	}
	if err := ValidateFactoryRuntimeSnapshot(base, now, nil); err != nil || !FactoryRuntimeSnapshotCurrentHealthy(base, now) {
		t.Fatalf("14-second snapshot rejected: %v %+v", err, base)
	}
	expired := base
	expired.LastHeartbeatAt = now.Add(-16 * time.Second)
	if ValidateFactoryRuntimeSnapshot(expired, now, nil) != nil && FactoryRuntimeSnapshotCurrentHealthy(expired, now) {
		t.Fatal("expired snapshot became healthy")
	}
	regressed := base
	regressed.Sequence = 9
	if err := ValidateFactoryRuntimeSnapshot(regressed, now, &base); err == nil || !strings.Contains(err.Error(), "regressed") {
		t.Fatalf("sequence regression error = %v", err)
	}
	restarted := base
	restarted.BootID = "boot-b"
	restarted.Sequence = 1
	restarted.ProcessStartedAt = base.ProcessStartedAt.Add(30 * time.Second)
	if err := ValidateFactoryRuntimeSnapshot(restarted, now, &base); err != nil {
		t.Fatalf("new boot rejected: %v", err)
	}
	olderBoot := restarted
	olderBoot.BootID = "boot-a"
	olderBoot.ProcessStartedAt = base.ProcessStartedAt
	if err := ValidateFactoryRuntimeSnapshot(olderBoot, now, &restarted); err == nil || !strings.Contains(err.Error(), "older") {
		t.Fatalf("older boot accepted: %v", err)
	}
	recoveryConflict := base
	recoveryConflict.RecoveryGeneration++
	if err := ValidateFactoryRuntimeSnapshot(recoveryConflict, now, &base); err == nil || !strings.Contains(err.Error(), "recovery") {
		t.Fatalf("same-boot recovery conflict accepted: %v", err)
	}
	unknownSchema := base
	unknownSchema.SchemaVersion = "factory-runtime-snapshot/future"
	if err := ValidateFactoryRuntimeSnapshot(unknownSchema, now, nil); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("unknown schema accepted: %v", err)
	}
	invalid := base
	invalid.ActiveWorkers = 1
	if err := ValidateFactoryRuntimeSnapshot(invalid, now, nil); err == nil {
		t.Fatal("invalid arithmetic/assignment count accepted")
	}
	stopping := base
	stopping.SchedulerState = FactoryRuntimeStopping
	if FactoryRuntimeSnapshotCurrentHealthy(stopping, now) {
		t.Fatal("stopping runtime became healthy")
	}
}

func TestHIVEMCT3RuntimeClientTimeoutAndUnknownPayload(t *testing.T) {
	external := FactoryRuntimeClient{Endpoint: "https://example.com/runtime", APIKey: "must-not-leave-loopback"}
	if _, err := external.Fetch(context.Background(), time.Now().UTC(), nil); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("external runtime endpoint accepted: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"schema_version":"future"}`))
	}))
	defer server.Close()
	client := FactoryRuntimeClient{Endpoint: server.URL, APIKey: "secret", Client: &http.Client{Timeout: 10 * time.Millisecond}}
	if _, err := client.Fetch(context.Background(), time.Now().UTC(), nil); err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("request timeout did not fail closed: %v", err)
	}
}

func TestHIVEMCT2RuntimeListenAddressRejectsNonLoopback(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "[::1]:0", "localhost:0"} {
		if err := ValidateFactoryRuntimeListenAddress(address); err != nil {
			t.Fatalf("loopback %q rejected: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:8080", "192.0.2.1:8080", ":8080", "invalid"} {
		if err := ValidateFactoryRuntimeListenAddress(address); err == nil {
			t.Fatalf("non-loopback %q accepted", address)
		}
	}
}
