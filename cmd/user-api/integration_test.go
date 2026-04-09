package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lovyou-ai/hive/pkg/userapi"
)

// newTestServer creates the full handler stack (mux + middleware) backed by
// an in-memory store and returns an httptest.Server ready for requests.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store := userapi.NewMemStore()
	mux := userapi.NewMux(store)
	handler := userapi.Chain(mux,
		userapi.RecoverMiddleware,
		userapi.LogMiddleware,
	)
	return httptest.NewServer(handler)
}

// postJSON is a test helper that POSTs JSON to the server and returns the response.
func postJSON(t *testing.T, url string, v any) *http.Response {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// putJSON is a test helper that PUTs JSON to the server and returns the response.
func putJSON(t *testing.T, url string, v any) *http.Response {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	return resp
}

// doDelete is a test helper that sends a DELETE request.
func doDelete(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	return resp
}

// decode unmarshals a JSON response body into v and closes the body.
func decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestIntegration_AllEndpoints(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()
	base := srv.URL

	// --- 1. Health check ---
	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// --- 2. List users (empty) ---
	resp, err = http.Get(base + "/api/v1/users")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var users []userapi.User
	decode(t, resp, &users)
	if len(users) != 0 {
		t.Fatalf("list: expected 0 users, got %d", len(users))
	}

	// --- 3. Create a user ---
	createResp := postJSON(t, base+"/api/v1/users", userapi.CreateRequest{
		Name:  "Alice",
		Email: "alice@example.com",
	})
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", createResp.StatusCode)
	}
	var alice userapi.User
	decode(t, createResp, &alice)
	if alice.Name != "Alice" || alice.Email != "alice@example.com" {
		t.Fatalf("create: unexpected user %+v", alice)
	}
	if alice.ID == "" {
		t.Fatal("create: missing ID")
	}

	// --- 4. Create a second user ---
	createResp2 := postJSON(t, base+"/api/v1/users", userapi.CreateRequest{
		Name:  "Bob",
		Email: "bob@example.com",
	})
	if createResp2.StatusCode != http.StatusCreated {
		t.Fatalf("create bob: expected 201, got %d", createResp2.StatusCode)
	}
	var bob userapi.User
	decode(t, createResp2, &bob)

	// --- 5. List users (should have 2) ---
	resp, err = http.Get(base + "/api/v1/users")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	decode(t, resp, &users)
	if len(users) != 2 {
		t.Fatalf("list: expected 2 users, got %d", len(users))
	}

	// --- 6. Get user by ID ---
	resp, err = http.Get(base + "/api/v1/users/" + alice.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", resp.StatusCode)
	}
	var got userapi.User
	decode(t, resp, &got)
	if got.ID != alice.ID || got.Name != "Alice" {
		t.Fatalf("get: unexpected user %+v", got)
	}

	// --- 7. Get non-existent user → 404 ---
	resp, err = http.Get(base + "/api/v1/users/does-not-exist")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get missing: expected 404, got %d", resp.StatusCode)
	}

	// --- 8. Update user ---
	updateResp := putJSON(t, base+"/api/v1/users/"+alice.ID, userapi.UpdateRequest{
		Name:  "Alice Updated",
		Email: "alice-new@example.com",
	})
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", updateResp.StatusCode)
	}
	var updated userapi.User
	decode(t, updateResp, &updated)
	if updated.Name != "Alice Updated" || updated.Email != "alice-new@example.com" {
		t.Fatalf("update: unexpected user %+v", updated)
	}
	if updated.ID != alice.ID {
		t.Fatalf("update: ID changed from %s to %s", alice.ID, updated.ID)
	}

	// --- 9. Verify update persisted ---
	resp, err = http.Get(base + "/api/v1/users/" + alice.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	decode(t, resp, &got)
	if got.Name != "Alice Updated" {
		t.Fatalf("get after update: name not updated, got %q", got.Name)
	}

	// --- 10. Duplicate email → 409 ---
	dupResp := postJSON(t, base+"/api/v1/users", userapi.CreateRequest{
		Name:  "Evil Alice",
		Email: "alice-new@example.com",
	})
	dupResp.Body.Close()
	if dupResp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate email: expected 409, got %d", dupResp.StatusCode)
	}

	// --- 11. Delete user ---
	delResp := doDelete(t, base+"/api/v1/users/"+bob.ID)
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", delResp.StatusCode)
	}

	// --- 12. Delete again → 404 ---
	delResp2 := doDelete(t, base+"/api/v1/users/"+bob.ID)
	delResp2.Body.Close()
	if delResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("delete again: expected 404, got %d", delResp2.StatusCode)
	}

	// --- 13. List users (should have 1 after delete) ---
	resp, err = http.Get(base + "/api/v1/users")
	if err != nil {
		t.Fatalf("list final: %v", err)
	}
	decode(t, resp, &users)
	if len(users) != 1 {
		t.Fatalf("list final: expected 1 user, got %d", len(users))
	}
	if users[0].ID != alice.ID {
		t.Fatalf("list final: expected Alice, got %+v", users[0])
	}

	// --- 14. Validation: missing fields → 422 ---
	badResp := postJSON(t, base+"/api/v1/users", userapi.CreateRequest{Name: ""})
	badResp.Body.Close()
	if badResp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("validation: expected 422, got %d", badResp.StatusCode)
	}

	// --- 15. Update non-existent → 404 ---
	upMissing := putJSON(t, base+"/api/v1/users/does-not-exist", userapi.UpdateRequest{
		Name:  "Ghost",
		Email: "ghost@example.com",
	})
	upMissing.Body.Close()
	if upMissing.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: expected 404, got %d", upMissing.StatusCode)
	}
}
