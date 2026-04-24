package localapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	db := testDB(t)
	s := NewStore(db)
	handler := NewServer(s, "dev")
	return httptest.NewServer(handler)
}

func doReq(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer dev")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func readJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v (body: %s)", err, string(data))
	}
	return m
}

func TestRoundTrip_CreateAndListTasks(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a task via intend op.
	resp := doReq(t, "POST", ts.URL+"/app/hive/op",
		`{"op":"intend","title":"build widget","description":"make it good","priority":"high"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("intend: got status %d, want 200", resp.StatusCode)
	}
	result := readJSON(t, resp)
	if result["op"] != "intend" {
		t.Fatalf("expected op=intend, got %v", result["op"])
	}
	node, ok := result["node"].(map[string]any)
	if !ok {
		t.Fatal("expected node in response")
	}
	nodeID := node["id"].(string)
	if nodeID == "" {
		t.Fatal("expected non-empty node id")
	}
	if node["title"] != "build widget" {
		t.Fatalf("title mismatch: got %v", node["title"])
	}

	// List board — task should appear.
	resp = doReq(t, "GET", ts.URL+"/app/hive/board", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("board: got status %d, want 200", resp.StatusCode)
	}
	board := readJSON(t, resp)
	nodes, ok := board["nodes"].([]any)
	if !ok {
		t.Fatal("expected nodes array")
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node on board, got %d", len(nodes))
	}

	first := nodes[0].(map[string]any)
	if first["title"] != "build widget" {
		t.Fatalf("board node title mismatch: got %v", first["title"])
	}
}

func TestRoundTrip_CompleteTask(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a task.
	resp := doReq(t, "POST", ts.URL+"/app/hive/op",
		`{"op":"intend","title":"ephemeral task"}`)
	result := readJSON(t, resp)
	node := result["node"].(map[string]any)
	nodeID := node["id"].(string)

	// Complete it.
	resp = doReq(t, "POST", ts.URL+"/app/hive/op",
		fmt.Sprintf(`{"op":"complete","node_id":"%s"}`, nodeID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete: got status %d, want 200", resp.StatusCode)
	}
	cResult := readJSON(t, resp)
	if cResult["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", cResult["status"])
	}

	// Board should be empty (done tasks excluded).
	resp = doReq(t, "GET", ts.URL+"/app/hive/board", "")
	board := readJSON(t, resp)
	nodes := board["nodes"].([]any)
	if len(nodes) != 0 {
		t.Fatalf("expected empty board after complete, got %d nodes", len(nodes))
	}
}

func TestRoundTrip_OpenTask(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create and complete a task.
	resp := doReq(t, "POST", ts.URL+"/app/hive/op",
		`{"op":"intend","title":"reopenable task"}`)
	result := readJSON(t, resp)
	node := result["node"].(map[string]any)
	nodeID := node["id"].(string)

	resp = doReq(t, "POST", ts.URL+"/app/hive/op",
		fmt.Sprintf(`{"op":"complete","node_id":"%s"}`, nodeID))
	resp.Body.Close()

	// Board should be empty.
	resp = doReq(t, "GET", ts.URL+"/app/hive/board", "")
	board := readJSON(t, resp)
	if len(board["nodes"].([]any)) != 0 {
		t.Fatal("expected empty board after complete")
	}

	// Reopen the task.
	resp = doReq(t, "POST", ts.URL+"/app/hive/op",
		fmt.Sprintf(`{"op":"open","node_id":"%s"}`, nodeID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open: got status %d, want 200", resp.StatusCode)
	}
	oResult := readJSON(t, resp)
	if oResult["status"] != "ok" {
		t.Fatalf("expected status=ok, got %v", oResult["status"])
	}

	// Direct state check: GET the node and confirm state == "open".
	resp = doReq(t, "GET", ts.URL+"/app/hive/node/"+nodeID, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get reopened node: got status %d, want 200", resp.StatusCode)
	}
	got := readJSON(t, resp)
	if got["state"] != NodeStateOpen {
		t.Fatalf("state after reopen: got %v, want %q", got["state"], NodeStateOpen)
	}

	// Board should show the task again.
	resp = doReq(t, "GET", ts.URL+"/app/hive/board", "")
	board = readJSON(t, resp)
	nodes := board["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node on board after reopen, got %d", len(nodes))
	}
	first := nodes[0].(map[string]any)
	if first["title"] != "reopenable task" {
		t.Fatalf("board node title mismatch: got %v", first["title"])
	}
}

func TestOp_MissingNodeID(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	for _, op := range []string{"complete", "open", "edit", "claim", "assign"} {
		t.Run(op, func(t *testing.T) {
			body := fmt.Sprintf(`{"op":%q}`, op)
			resp := doReq(t, "POST", ts.URL+"/app/hive/op", body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("%s missing node_id: got status %d, want 400", op, resp.StatusCode)
			}
		})
	}
}

func TestOp_NonexistentNodeID(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	missing := "00000000-0000-0000-0000-000000000000"
	cases := []struct {
		op   string
		body string
	}{
		{"complete", fmt.Sprintf(`{"op":"complete","node_id":%q}`, missing)},
		{"open", fmt.Sprintf(`{"op":"open","node_id":%q}`, missing)},
		{"edit", fmt.Sprintf(`{"op":"edit","node_id":%q,"state":"escalated"}`, missing)},
		{"claim", fmt.Sprintf(`{"op":"claim","node_id":%q}`, missing)},
		{"assign", fmt.Sprintf(`{"op":"assign","node_id":%q,"assignee":"alice"}`, missing)},
	}

	for _, c := range cases {
		t.Run(c.op, func(t *testing.T) {
			resp := doReq(t, "POST", ts.URL+"/app/hive/op", c.body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("%s missing node: got status %d, want 404", c.op, resp.StatusCode)
			}
		})
	}
}

func TestOp_OpenAlreadyOpen(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp := doReq(t, "POST", ts.URL+"/app/hive/op",
		`{"op":"intend","title":"never completed"}`)
	result := readJSON(t, resp)
	nodeID := result["node"].(map[string]any)["id"].(string)

	resp = doReq(t, "POST", ts.URL+"/app/hive/op",
		fmt.Sprintf(`{"op":"open","node_id":%q}`, nodeID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("open already-open: got status %d, want 409", resp.StatusCode)
	}
}

func TestOp_AssignMissingAssignee(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp := doReq(t, "POST", ts.URL+"/app/hive/op",
		`{"op":"intend","title":"to assign"}`)
	result := readJSON(t, resp)
	nodeID := result["node"].(map[string]any)["id"].(string)

	resp = doReq(t, "POST", ts.URL+"/app/hive/op",
		fmt.Sprintf(`{"op":"assign","node_id":%q}`, nodeID))
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("assign missing assignee: got status %d, want 400", resp.StatusCode)
	}
}

func TestRoundTrip_NodeExists(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Create a task.
	resp := doReq(t, "POST", ts.URL+"/app/hive/op",
		`{"op":"intend","title":"findable task"}`)
	result := readJSON(t, resp)
	node := result["node"].(map[string]any)
	nodeID := node["id"].(string)

	// GET existing node — 200.
	resp = doReq(t, "GET", ts.URL+"/app/hive/node/"+nodeID, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get existing node: got status %d, want 200", resp.StatusCode)
	}
	got := readJSON(t, resp)
	if got["title"] != "findable task" {
		t.Fatalf("node title mismatch: got %v", got["title"])
	}

	// GET nonexistent node — 404.
	resp = doReq(t, "GET", ts.URL+"/app/hive/node/nonexistent-id", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get missing node: got status %d, want 404", resp.StatusCode)
	}
}

func TestHealth(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Health does not require auth.
	req, _ := http.NewRequest("GET", ts.URL+"/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()
	// Health endpoint doesn't require auth in the mux — but our mux wraps it
	// without auth, so it should return 200.
	// Actually the health route is NOT wrapped with auth, so no Bearer needed.
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: got status %d, want 200", resp.StatusCode)
	}
	if string(body) != "ok" {
		t.Fatalf("health: got %q, want %q", string(body), "ok")
	}
}

func TestUnauthorized(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Request without Bearer token should get 401.
	req, _ := http.NewRequest("GET", ts.URL+"/app/hive/board", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestUnknownOp(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp := doReq(t, "POST", ts.URL+"/app/hive/op",
		`{"op":"teleport"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown op: got status %d, want 400", resp.StatusCode)
	}
}
