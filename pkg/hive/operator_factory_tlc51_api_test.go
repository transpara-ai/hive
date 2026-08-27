package hive

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/store"
)

type staticFactoryTLC51MissionControlSource struct {
	value FactoryTLC51MissionControlEnvelope
}

func (source staticFactoryTLC51MissionControlSource) BuildFactoryTLC51MissionControl(context.Context) FactoryTLC51MissionControlEnvelope {
	return source.value
}

func TestFactoryTLC51MissionControlRouteIsOptInReadOnlyAndStrictlyAuthenticated(t *testing.T) {
	value := FactoryTLC51MissionControlEnvelope{
		SchemaVersion: "factory-tlc51-mission-control-envelope/v1",
		GeneratedAt:   time.Date(2026, 8, 27, 14, 0, 0, 0, time.UTC),
		Orders:        nil, Errors: []string{}, AuthorityGranted: false,
	}
	source := staticFactoryTLC51MissionControlSource{value: value}
	graph := store.NewInMemoryStore()
	handler := NewOperatorProjectionServer(graph, "non-dev-test-token", 10, WithFactoryTLC51MissionControl(source))

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, FactoryTLC51MissionControlPath, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, FactoryTLC51MissionControlPath, nil)
	request.Header.Set("Authorization", "Bearer non-dev-test-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var got FactoryTLC51MissionControlEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != value.SchemaVersion || got.GeneratedAt != value.GeneratedAt || got.AuthorityGranted {
		t.Fatalf("projection = %+v", got)
	}

	emptyKey := NewOperatorProjectionServer(graph, "", 10, WithFactoryTLC51MissionControl(source))
	emptyResponse := httptest.NewRecorder()
	emptyKey.ServeHTTP(emptyResponse, httptest.NewRequest(http.MethodGet, FactoryTLC51MissionControlPath, nil))
	if emptyResponse.Code != http.StatusUnauthorized {
		t.Fatalf("empty-key status = %d", emptyResponse.Code)
	}

	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, FactoryTLC51MissionControlPath, nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want method not allowed", post.Code)
	}
}

func TestFactoryTLC51DaemonRequiresClientAndScheduler(t *testing.T) {
	if daemon, err := NewFactoryTLC51Daemon(nil, nil); err == nil || daemon != nil {
		t.Fatalf("daemon = %v error = %v", daemon, err)
	}
}
