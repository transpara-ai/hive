package sdr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/membrane"
)

// TestEndToEndSmokeTest proves the full membrane loop:
//   ai-sdr (mock) → poller → bridge (mock) → human decision → dispatcher → ai-sdr (mock)
//
// This is the smoke test that validates the entire data flow without
// requiring real services running.
func TestEndToEndSmokeTest(t *testing.T) {
	// --- Mock AI-SDR ---
	var mu sync.Mutex
	var approvedDrafts []string
	var copilotMode bool = true

	sdrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/health":
			json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})

		case r.Method == "GET" && r.URL.Path == "/copilot/drafts":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"drafts": []map[string]interface{}{
					{
						"interaction_id":   "int-smoke-001",
						"lead_id":          "lead-smoke-001",
						"lead_name":        "Sarah Chen",
						"company":          "Aurex Mining",
						"email":            "sarah@aurexmining.com",
						"subject":          "Operational visibility for your 8 sites",
						"body_text":        "Hi Sarah, I noticed Aurex Mining runs AVEVA PI across 8 sites...",
						"score_total":      62,
						"score_company_fit": 18,
						"score_pain_need":  15,
						"score_authority":  14,
						"score_timing":     15,
						"current_state":    "ready_for_outreach",
						"internal_notes":   "Good ICP fit, PI user, multi-site",
						"created_at":       "2026-03-30T10:00:00Z",
					},
				},
				"count": 1,
			})

		case r.Method == "GET" && r.URL.Path == "/api/leads":
			// No qualified handoffs for this test
			json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []interface{}{},
				"total": 0,
			})

		case r.Method == "GET" && r.URL.Path == "/api/dashboard/stats":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"active_leads":    47,
				"response_rate":   18.5,
				"avg_score":       72.3,
				"qualified_count": 4,
			})

		case r.Method == "POST" && r.URL.Path == "/copilot/drafts/int-smoke-001/approve":
			mu.Lock()
			approvedDrafts = append(approvedDrafts, "int-smoke-001")
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]string{
				"status": "ok", "action": "approved",
				"interaction_id": "int-smoke-001", "task_id": "task-001",
			})

		case r.Method == "POST" && r.URL.Path == "/api/settings/mode":
			var body map[string]bool
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			copilotMode = body["copilot_mode"]
			mu.Unlock()
			json.NewEncoder(w).Encode(body)

		default:
			http.Error(w, fmt.Sprintf("unexpected: %s %s", r.Method, r.URL.Path), 404)
		}
	}))
	defer sdrServer.Close()

	// --- Mock Bridge (site) ---
	var bridgeActions []map[string]interface{}
	var bridgeEvents []map[string]interface{}
	var decisionsReturned bool

	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/bridge/action":
			var action map[string]interface{}
			json.NewDecoder(r.Body).Decode(&action)
			mu.Lock()
			bridgeActions = append(bridgeActions, action)
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]string{"status": "ok", "action_id": "act-smoke-001"})
			w.WriteHeader(201)

		case r.Method == "POST" && r.URL.Path == "/api/bridge/event":
			var event map[string]interface{}
			json.NewDecoder(r.Body).Decode(&event)
			mu.Lock()
			bridgeEvents = append(bridgeEvents, event)
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			w.WriteHeader(201)

		case r.Method == "GET" && r.URL.Path == "/api/bridge/decisions":
			mu.Lock()
			returned := decisionsReturned
			decisionsReturned = true
			mu.Unlock()

			if returned {
				// Second poll: no more decisions
				json.NewEncoder(w).Encode([]interface{}{})
			} else {
				// First poll: human approved the draft
				json.NewEncoder(w).Encode([]map[string]interface{}{
					{
						"id":              "act-smoke-001",
						"agent_name":      "sdr",
						"action_type":     "approval",
						"status":          "approved",
						"decided_by":      "user-matt",
						"decision_notes":  "Looks good, send it",
						"domain_data":     json.RawMessage(`{"interaction_id":"int-smoke-001"}`),
					},
				})
			}

		default:
			http.Error(w, fmt.Sprintf("unexpected: %s %s", r.Method, r.URL.Path), 404)
		}
	}))
	defer bridgeServer.Close()

	ctx := context.Background()
	bands := membrane.TrustBands{RequiredBelow: 0.3, RecommendedBelow: 0.6}

	sdrClient := membrane.NewHTTPServiceClient(sdrServer.URL, "bearer", nil)
	poller := &Poller{
		client:       sdrClient,
		bridgeClient: membrane.NewHTTPServiceClient(bridgeServer.URL, "bearer", nil),
	}
	dispatcher := &Dispatcher{
		sdrClient:    sdrClient,
		bridgeClient: membrane.NewHTTPServiceClient(bridgeServer.URL, "bearer", nil),
	}

	// === STEP 1: Health check ===
	t.Log("Step 1: Health check")
	health, err := poller.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("health check: %v", err)
	}
	if health.Status != "healthy" {
		t.Fatalf("ai-sdr status = %q, want healthy", health.Status)
	}

	// === STEP 2: Set operating mode based on trust ===
	t.Log("Step 2: Set operating mode (trust=0.1, copilot=true)")
	if err := dispatcher.SetOperatingMode(ctx, 0.1, bands); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	mu.Lock()
	if !copilotMode {
		t.Fatal("copilot_mode should be true at low trust")
	}
	mu.Unlock()

	// === STEP 3: Poll for drafts → creates bridge action ===
	t.Log("Step 3: Poll drafts → bridge action")
	created, err := poller.PollDrafts(ctx, 0.1, bands)
	if err != nil {
		t.Fatalf("poll drafts: %v", err)
	}
	if created != 1 {
		t.Fatalf("created = %d, want 1", created)
	}
	mu.Lock()
	if len(bridgeActions) != 1 {
		t.Fatalf("bridge actions = %d, want 1", len(bridgeActions))
	}
	action := bridgeActions[0]
	if action["agent_name"] != "sdr" {
		t.Errorf("agent_name = %v, want sdr", action["agent_name"])
	}
	if action["action_type"] != "approval" {
		t.Errorf("action_type = %v, want approval", action["action_type"])
	}
	if action["authority"] != "required" {
		t.Errorf("authority = %v, want required (low trust)", action["authority"])
	}
	mu.Unlock()

	// === STEP 4: Poll dashboard stats → bridge event ===
	t.Log("Step 4: Poll stats → bridge event")
	stats, err := poller.PollStats(ctx)
	if err != nil {
		t.Fatalf("poll stats: %v", err)
	}
	if stats.ActiveLeads != 47 {
		t.Errorf("active_leads = %d, want 47", stats.ActiveLeads)
	}

	// === STEP 5: Human approves in bridge → dispatcher picks it up ===
	t.Log("Step 5: Dispatch human decision → ai-sdr approve")
	dispatched, err := dispatcher.PollAndDispatch(ctx)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if dispatched != 1 {
		t.Fatalf("dispatched = %d, want 1", dispatched)
	}

	// === STEP 6: Verify ai-sdr received the approval ===
	t.Log("Step 6: Verify ai-sdr approved the draft")
	mu.Lock()
	if len(approvedDrafts) != 1 {
		t.Fatalf("approved drafts = %d, want 1", len(approvedDrafts))
	}
	if approvedDrafts[0] != "int-smoke-001" {
		t.Errorf("approved = %q, want int-smoke-001", approvedDrafts[0])
	}
	mu.Unlock()

	// === STEP 7: Test high trust → autonomous mode ===
	t.Log("Step 7: High trust → copilot off")
	if err := dispatcher.SetOperatingMode(ctx, 0.8, bands); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	mu.Lock()
	if copilotMode {
		t.Fatal("copilot_mode should be false at high trust")
	}
	mu.Unlock()

	// === STEP 8: At high trust, poller skips drafts (ai-sdr handles autonomously) ===
	t.Log("Step 8: High trust → poller skips drafts")
	created, err = poller.PollDrafts(ctx, 0.8, bands)
	if err != nil {
		t.Fatalf("poll drafts high trust: %v", err)
	}
	if created != 0 {
		t.Errorf("created = %d, want 0 at high trust", created)
	}

	// === STEP 9: Second dispatch poll returns empty (no more decisions) ===
	t.Log("Step 9: Second dispatch poll → empty")
	dispatched, err = dispatcher.PollAndDispatch(ctx)
	if err != nil {
		t.Fatalf("dispatch 2: %v", err)
	}
	if dispatched != 0 {
		t.Errorf("dispatched = %d, want 0 (no more decisions)", dispatched)
	}

	// === SUMMARY ===
	t.Log("=== SMOKE TEST PASSED ===")
	t.Logf("Full loop verified:")
	t.Logf("  ai-sdr draft → poller → bridge action (authority=required)")
	t.Logf("  human approves → dispatcher → ai-sdr /copilot/drafts/{id}/approve")
	t.Logf("  trust 0.1 → copilot=true | trust 0.8 → copilot=false")
	t.Logf("  high trust → poller skips drafts (autonomous mode)")

	_ = time.Now() // use time package
}
