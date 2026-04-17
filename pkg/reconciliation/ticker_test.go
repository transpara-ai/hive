package reconciliation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lovyou-ai/eventgraph/go/pkg/types"
	"github.com/lovyou-ai/hive/pkg/runner"
)

// fakeState is an in-memory State implementation: one watermark per
// space and a set of opIDs that have been anchored (site.op.received).
type fakeState struct {
	mu         sync.Mutex
	watermark  map[string]time.Time
	anchored   map[string]bool
	loadErr    error
	saveErr    error
	hasErr     error
	loadCalls  int32
	saveCalls  int32
	savedValue map[string]time.Time
}

func newFakeState() *fakeState {
	return &fakeState{
		watermark:  map[string]time.Time{},
		anchored:   map[string]bool{},
		savedValue: map[string]time.Time{},
	}
}

func (s *fakeState) LoadWatermark(_ context.Context, space string) (time.Time, error) {
	atomic.AddInt32(&s.loadCalls, 1)
	if s.loadErr != nil {
		return time.Time{}, s.loadErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.watermark[space], nil
}

func (s *fakeState) SaveWatermark(_ context.Context, space string, w time.Time) error {
	atomic.AddInt32(&s.saveCalls, 1)
	if s.saveErr != nil {
		return s.saveErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watermark[space] = w
	s.savedValue[space] = w
	return nil
}

func (s *fakeState) HasSiteOpReceived(_ context.Context, opID string) (bool, error) {
	if s.hasErr != nil {
		return false, s.hasErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.anchored[opID], nil
}

// fakeSource returns a fixed slice. Cycles over the same slice on repeat
// calls — the ticker's responsibility is to respect the watermark.
type fakeSource struct {
	ops []runner.OpEvent
	err error
}

func (s *fakeSource) ListOpsSince(_ string, _ time.Time) ([]runner.OpEvent, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.ops, nil
}

// fakeDispatcher counts anchor + translate invocations and allows injecting
// per-op anchor errors by op.ID.
type fakeDispatcher struct {
	mu            sync.Mutex
	anchorCount   int32
	translateCount int32
	anchorErr     map[string]error
	translateDone chan struct{}
}

func newFakeDispatcher() *fakeDispatcher {
	return &fakeDispatcher{
		anchorErr:     map[string]error{},
		translateDone: make(chan struct{}, 32),
	}
}

func (d *fakeDispatcher) AnchorSiteOp(_ context.Context, op runner.OpEvent) (types.EventID, error) {
	atomic.AddInt32(&d.anchorCount, 1)
	d.mu.Lock()
	err := d.anchorErr[op.ID]
	d.mu.Unlock()
	if err != nil {
		return types.EventID{}, err
	}
	id, _ := types.NewEventIDFromNew()
	return id, nil
}

func (d *fakeDispatcher) EmitSiteOp(_ context.Context, _ runner.OpEvent, _ types.EventID) error {
	atomic.AddInt32(&d.translateCount, 1)
	d.translateDone <- struct{}{}
	return nil
}

// mustTick invokes tick and waits for N async translates to complete or fails.
func (d *fakeDispatcher) waitForTranslates(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-d.translateDone:
		case <-time.After(2 * time.Second):
			t.Fatalf("translate %d/%d did not complete within 2s", i+1, n)
		}
	}
}

func TestTick_EmptySourceDoesNothing(t *testing.T) {
	state := newFakeState()
	src := &fakeSource{}
	disp := newFakeDispatcher()
	ticker := newTickerWithState(state, disp, src, "space-1")

	ticker.tick(context.Background())

	if got := atomic.LoadInt32(&disp.anchorCount); got != 0 {
		t.Errorf("anchorCount = %d; want 0 for empty source", got)
	}
	if got := atomic.LoadInt32(&state.saveCalls); got != 0 {
		t.Errorf("saveWatermark called %d times on empty cycle; want 0", got)
	}
}

func TestTick_AgentKindSkipped(t *testing.T) {
	t0 := time.Now().Add(-time.Minute)
	state := newFakeState()
	src := &fakeSource{ops: []runner.OpEvent{
		{ID: "op-1", Op: "respond", ActorKind: "agent", CreatedAt: t0},
	}}
	disp := newFakeDispatcher()
	ticker := newTickerWithState(state, disp, src, "space-1")

	ticker.tick(context.Background())

	if got := atomic.LoadInt32(&disp.anchorCount); got != 0 {
		t.Errorf("anchorCount = %d; want 0 — agent-kind must not re-anchor", got)
	}
	// Watermark MUST still advance — otherwise a stuck agent op holds it back.
	state.mu.Lock()
	wm := state.watermark["space-1"]
	state.mu.Unlock()
	if !wm.Equal(t0) {
		t.Errorf("watermark = %v; want %v — watermark must advance past skipped ops", wm, t0)
	}
}

func TestTick_AlreadyAnchoredSkipped(t *testing.T) {
	t0 := time.Now()
	state := newFakeState()
	state.anchored["op-existing"] = true

	src := &fakeSource{ops: []runner.OpEvent{
		{ID: "op-existing", Op: "respond", Actor: "alice", CreatedAt: t0},
	}}
	disp := newFakeDispatcher()
	ticker := newTickerWithState(state, disp, src, "space-1")

	ticker.tick(context.Background())

	if got := atomic.LoadInt32(&disp.anchorCount); got != 0 {
		t.Errorf("anchorCount = %d; want 0 — already-anchored op must not re-anchor", got)
	}
	if got := atomic.LoadInt32(&disp.translateCount); got != 0 {
		t.Errorf("translateCount = %d; want 0 — already-anchored op must not translate", got)
	}
}

func TestTick_HumanOpAnchoredAndTranslated(t *testing.T) {
	t0 := time.Now()
	state := newFakeState()
	src := &fakeSource{ops: []runner.OpEvent{
		{ID: "op-human", Op: "respond", Actor: "alice", CreatedAt: t0},
	}}
	disp := newFakeDispatcher()
	ticker := newTickerWithState(state, disp, src, "space-1")

	ticker.tick(context.Background())
	disp.waitForTranslates(t, 1)

	if got := atomic.LoadInt32(&disp.anchorCount); got != 1 {
		t.Errorf("anchorCount = %d; want 1", got)
	}
	if got := atomic.LoadInt32(&disp.translateCount); got != 1 {
		t.Errorf("translateCount = %d; want 1", got)
	}

	state.mu.Lock()
	wm := state.watermark["space-1"]
	state.mu.Unlock()
	if !wm.Equal(t0) {
		t.Errorf("watermark = %v; want %v", wm, t0)
	}
}

func TestTick_AnchorErrorDoesNotStopLoop(t *testing.T) {
	t0 := time.Now().Add(-2 * time.Minute)
	t1 := time.Now().Add(-time.Minute)
	state := newFakeState()
	src := &fakeSource{ops: []runner.OpEvent{
		{ID: "op-bad", Op: "respond", Actor: "alice", CreatedAt: t0},
		{ID: "op-good", Op: "respond", Actor: "bob", CreatedAt: t1},
	}}
	disp := newFakeDispatcher()
	disp.anchorErr["op-bad"] = errors.New("chain write failed")
	ticker := newTickerWithState(state, disp, src, "space-1")

	ticker.tick(context.Background())
	disp.waitForTranslates(t, 1)

	// Both anchor calls ran (one failed, one succeeded); only the
	// successful op was translated.
	if got := atomic.LoadInt32(&disp.anchorCount); got != 2 {
		t.Errorf("anchorCount = %d; want 2 — loop must continue past a failed anchor", got)
	}
	if got := atomic.LoadInt32(&disp.translateCount); got != 1 {
		t.Errorf("translateCount = %d; want 1 — only the good op translates", got)
	}

	// Watermark advanced to the newest op in the batch regardless of
	// per-op anchor outcome: progress beats perfection.
	state.mu.Lock()
	wm := state.watermark["space-1"]
	state.mu.Unlock()
	if !wm.Equal(t1) {
		t.Errorf("watermark = %v; want %v — must advance past both ops", wm, t1)
	}
}

func TestTick_WatermarkOnlySavedWhenAdvanced(t *testing.T) {
	// Preload watermark at t_now; source returns an older op.
	tNow := time.Now()
	tOld := tNow.Add(-time.Hour)
	state := newFakeState()
	state.watermark["space-1"] = tNow

	src := &fakeSource{ops: []runner.OpEvent{
		{ID: "op-old", Op: "respond", Actor: "alice", CreatedAt: tOld},
	}}
	disp := newFakeDispatcher()
	ticker := newTickerWithState(state, disp, src, "space-1")

	ticker.tick(context.Background())
	disp.waitForTranslates(t, 1)

	// The op is anchored (not yet in state.anchored), but the watermark
	// shouldn't be rolled backwards.
	if got := atomic.LoadInt32(&state.saveCalls); got != 0 {
		t.Errorf("saveWatermark called %d times; want 0 when no advance", got)
	}
	state.mu.Lock()
	wm := state.watermark["space-1"]
	state.mu.Unlock()
	if !wm.Equal(tNow) {
		t.Errorf("watermark = %v; want unchanged %v", wm, tNow)
	}
}

func TestTick_LoadWatermarkErrorAborts(t *testing.T) {
	state := newFakeState()
	state.loadErr = errors.New("db down")
	src := &fakeSource{ops: []runner.OpEvent{
		{ID: "op-1", Op: "respond", Actor: "alice", CreatedAt: time.Now()},
	}}
	disp := newFakeDispatcher()
	ticker := newTickerWithState(state, disp, src, "space-1")

	ticker.tick(context.Background())

	if got := atomic.LoadInt32(&disp.anchorCount); got != 0 {
		t.Errorf("anchorCount = %d; want 0 — tick must abort when watermark load fails", got)
	}
}
