package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

// fakeDispatcher records AnchorSiteOp and EmitSiteOp calls. The anchor
// returns anchorID if non-empty and anchorErr otherwise. EmitSiteOp runs
// synchronously for test determinism — the handler spawns it in a
// goroutine, so tests wait on translateDone.
type fakeDispatcher struct {
	mu            sync.Mutex
	anchorCalls   int32
	translates    []OpEvent
	anchorID      types.EventID
	anchorErr     error
	translateErr  error
	translateDone chan struct{}
}

func newFakeDispatcher(t *testing.T) *fakeDispatcher {
	t.Helper()
	id, err := types.NewEventIDFromNew()
	if err != nil {
		t.Fatalf("NewEventIDFromNew: %v", err)
	}
	return &fakeDispatcher{
		anchorID:      id,
		translateDone: make(chan struct{}, 16),
	}
}

func (f *fakeDispatcher) AnchorSiteOp(_ context.Context, _ OpEvent) (types.EventID, error) {
	atomic.AddInt32(&f.anchorCalls, 1)
	if f.anchorErr != nil {
		return types.EventID{}, f.anchorErr
	}
	return f.anchorID, nil
}

func (f *fakeDispatcher) EmitSiteOp(_ context.Context, op OpEvent, _ types.EventID) error {
	f.mu.Lock()
	f.translates = append(f.translates, op)
	f.mu.Unlock()
	defer func() { f.translateDone <- struct{}{} }()
	return f.translateErr
}

func (f *fakeDispatcher) translateCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.translates)
}

// postOp fires a single webhook payload against a handler built around a
// dispatcher. Returns the http.ResponseRecorder so tests can inspect body
// and status.
func postOp(t *testing.T, d Dispatcher, op OpEvent) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("marshal op: %v", err)
	}
	el := &EventListener{dispatcher: d, ctx: context.Background()}
	req := httptest.NewRequest("POST", "/event", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	el.handleEvent(rec, req)
	return rec
}

func TestHandleEvent_AgentKindSkipsDispatch(t *testing.T) {
	d := newFakeDispatcher(t)
	rec := postOp(t, d, OpEvent{ID: "op1", Op: "respond", ActorKind: "agent"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if got := atomic.LoadInt32(&d.anchorCalls); got != 0 {
		t.Errorf("anchor calls = %d; want 0 for agent actor", got)
	}
}

func TestHandleEvent_AnchorReturnsChainRef(t *testing.T) {
	d := newFakeDispatcher(t)
	rec := postOp(t, d, OpEvent{ID: "op2", Op: "respond", Actor: "alice"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["hive_chain_ref"] != d.anchorID.String() {
		t.Errorf("hive_chain_ref = %q; want %q", body["hive_chain_ref"], d.anchorID.String())
	}
	if got := atomic.LoadInt32(&d.anchorCalls); got != 1 {
		t.Errorf("anchor calls = %d; want 1", got)
	}
}

func TestHandleEvent_AnchorFailureReturns500(t *testing.T) {
	d := newFakeDispatcher(t)
	d.anchorErr = fmt.Errorf("chain write failed")
	rec := postOp(t, d, OpEvent{ID: "op3", Op: "respond", Actor: "alice"})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500", rec.Code)
	}
	if d.translateCount() != 0 {
		t.Errorf("translate ran despite anchor failure")
	}
}

func TestHandleEvent_TranslateRunsAsync(t *testing.T) {
	d := newFakeDispatcher(t)
	rec := postOp(t, d, OpEvent{ID: "op4", Op: "intend", Actor: "alice"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	// Webhook has returned; translate should fire on its own goroutine.
	select {
	case <-d.translateDone:
	case <-time.After(2 * time.Second):
		t.Fatal("translate did not run within 2s of webhook response")
	}
	if d.translateCount() != 1 {
		t.Errorf("translate count = %d; want 1", d.translateCount())
	}
}

func TestHandleEvent_NilDispatcherLogOnly(t *testing.T) {
	rec := postOp(t, nil, OpEvent{ID: "op5", Op: "respond", Actor: "alice"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "no dispatcher" {
		t.Errorf("body status = %q; want 'no dispatcher'", body["status"])
	}
}

func TestEventListenerAddressWholeDomain(t *testing.T) {
	tests := []struct {
		name   string
		config EventListenerConfig
		want   string
	}{
		{name: "no address or port", want: "127.0.0.1:8081"},
		{name: "port only", config: EventListenerConfig{Port: "9090"}, want: "127.0.0.1:9090"},
		{name: "explicit address", config: EventListenerConfig{Addr: ":8081"}, want: ":8081"},
		{name: "explicit address overrides port", config: EventListenerConfig{Addr: "0.0.0.0:9090", Port: "ignored"}, want: "0.0.0.0:9090"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newEventListenerServer(context.Background(), nil, tt.config)
			if server.Addr != tt.want {
				t.Fatalf("listener address = %q, want %q", server.Addr, tt.want)
			}
		})
	}
}

func TestEventListenerBearerAuthRefusalPaths(t *testing.T) {
	tests := []struct {
		name          string
		authorization string
	}{
		{name: "absent"},
		{name: "wrong scheme", authorization: "Basic abc"},
		{name: "wrong bearer", authorization: "Bearer wrong"},
		{name: "empty bearer", authorization: "Bearer "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newFakeDispatcher(t)
			body, err := json.Marshal(OpEvent{ID: "op-auth", Op: "respond", Actor: "alice"})
			if err != nil {
				t.Fatalf("marshal op: %v", err)
			}
			server := newEventListenerServer(context.Background(), d, EventListenerConfig{BearerToken: "expected"})
			req := httptest.NewRequest("POST", "/event", bytes.NewReader(body))
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()
			server.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if got := atomic.LoadInt32(&d.anchorCalls); got != 0 {
				t.Fatalf("unauthorized request reached dispatcher %d time(s)", got)
			}
			if rec.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("WWW-Authenticate = %q, want Bearer", rec.Header().Get("WWW-Authenticate"))
			}
		})
	}
}

func TestEventListenerBearerAuthAcceptsMatchingToken(t *testing.T) {
	d := newFakeDispatcher(t)
	body, err := json.Marshal(OpEvent{ID: "op-auth-ok", Op: "respond", Actor: "alice"})
	if err != nil {
		t.Fatalf("marshal op: %v", err)
	}
	server := newEventListenerServer(context.Background(), d, EventListenerConfig{BearerToken: "expected"})
	req := httptest.NewRequest("POST", "/event", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer expected")
	rec := httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := atomic.LoadInt32(&d.anchorCalls); got != 1 {
		t.Fatalf("authorized request reached dispatcher %d time(s), want 1", got)
	}
}
