package hive

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOperatorProjectionServerRequiresBearerToken(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)
	handler := NewOperatorProjectionServer(s, "secret", 50)

	req := httptest.NewRequest(http.MethodGet, "/api/hive/operator-projection", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status without token = %d, want %d", resp.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/hive/operator-projection", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status with token = %d, want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
}

func TestOperatorProjectionServerReturnsProjectionJSON(t *testing.T) {
	s, actorID, appendEvent := newOperatorProjectionStore(t)
	requestID := newTestEventID(t)
	appendEvent(EventTypeAuthorityRequestRecorded, AuthorityRequestRecordedContent{
		RequestID:        requestID,
		RequestingActor:  actorID,
		ActionName:       "agent.spawn.persistent",
		Target:           "builder",
		Environment:      "production",
		RequestedOutcome: "persistent identity",
		Justification:    "operator approved persistence trial",
		RiskSummary:      "persistent agent creation",
	})

	handler := NewOperatorProjectionServer(s, "", 50)
	req := httptest.NewRequest(http.MethodGet, "/api/hive/operator-projection", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var projection OperatorProjection
	if err := json.Unmarshal(resp.Body.Bytes(), &projection); err != nil {
		t.Fatalf("decode projection: %v", err)
	}
	if projection.Source != "eventgraph" {
		t.Fatalf("source = %q, want eventgraph", projection.Source)
	}
	if len(projection.PendingApprovals) != 1 || projection.PendingApprovals[0].RequestID != requestID.Value() {
		t.Fatalf("pending approvals = %+v", projection.PendingApprovals)
	}
	if projection.GeneratedAt.IsZero() {
		t.Fatal("generated_at is zero")
	}
}

func TestOperatorProjectionServerHealth(t *testing.T) {
	s, _, _ := newOperatorProjectionStore(t)
	handler := NewOperatorProjectionServer(s, "secret", 50)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", resp.Code, http.StatusOK)
	}
	if resp.Body.String() != "ok" {
		t.Fatalf("health body = %q, want ok", resp.Body.String())
	}
}
