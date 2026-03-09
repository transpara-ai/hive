package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

func TestWriteConfig(t *testing.T) {
	dir := t.TempDir()
	agentID := types.MustActorID("actor_agent_00000000000000000000001")
	humanID := types.MustActorID("actor_human_00000000000000000000001")
	convID, err := types.NewConversationID("conv_test_0000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}

	path, err := WriteConfig(dir, "/usr/local/bin/mcp-server", "postgres://localhost/hive", agentID, humanID, convID)
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Base(path) != ".mcp.json" {
		t.Errorf("expected .mcp.json, got %s", filepath.Base(path))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}

	hive, ok := cfg.MCPServers["hive"]
	if !ok {
		t.Fatal("missing 'hive' server in config")
	}
	if hive.Command != "/usr/local/bin/mcp-server" {
		t.Errorf("command = %q", hive.Command)
	}

	// Verify args contain the DSN and IDs.
	args := hive.Args
	found := map[string]bool{}
	for i, a := range args {
		if a == "--store" && i+1 < len(args) {
			found["store"] = args[i+1] == "postgres://localhost/hive"
		}
		if a == "--agent-id" && i+1 < len(args) {
			found["agent-id"] = args[i+1] == agentID.Value()
		}
		if a == "--human-id" && i+1 < len(args) {
			found["human-id"] = args[i+1] == humanID.Value()
		}
	}
	for k, v := range found {
		if !v {
			t.Errorf("arg %s not found or wrong value", k)
		}
	}
}
