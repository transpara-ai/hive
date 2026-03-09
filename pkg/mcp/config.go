package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// ServerConfig describes an MCP server for Claude CLI.
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPConfig is the top-level config file format for Claude CLI.
type MCPConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// WriteConfig writes an MCP config file for Claude CLI that points to the
// hive MCP server binary. Returns the path to the config file.
func WriteConfig(dir string, mcpBinary string, dsn string, agentID types.ActorID, humanID types.ActorID, convID types.ConversationID) (string, error) {
	cfg := MCPConfig{
		MCPServers: map[string]ServerConfig{
			"hive": {
				Command: mcpBinary,
				Args: []string{
					"--agent-id", agentID.Value(),
					"--human-id", humanID.Value(),
					"--conv-id", convID.Value(),
				},
				Env: map[string]string{
					"DATABASE_URL": dsn,
				},
			},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal MCP config: %w", err)
	}

	path := filepath.Join(dir, ".mcp.json")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write MCP config: %w", err)
	}

	return path, nil
}
