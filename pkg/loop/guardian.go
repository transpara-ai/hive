package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
)

// ApproveCommand represents the parsed /approve command from Guardian LLM output.
// The Guardian emits this when a spawn proposal passes all governance checks.
type ApproveCommand struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// RejectCommand represents the parsed /reject command from Guardian LLM output.
// The Guardian emits this when a spawn proposal fails one or more governance checks.
type RejectCommand struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// parseApproveCommand extracts the /approve JSON payload from LLM output.
// Returns nil if no /approve command is found or the JSON is malformed.
func parseApproveCommand(response string) *ApproveCommand {
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/approve ") {
			continue
		}
		jsonStr := strings.TrimPrefix(trimmed, "/approve ")
		var cmd ApproveCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
			return nil
		}
		return &cmd
	}
	return nil
}

// parseRejectCommand extracts the /reject JSON payload from LLM output.
// Returns nil if no /reject command is found or the JSON is malformed.
func parseRejectCommand(response string) *RejectCommand {
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/reject ") {
			continue
		}
		jsonStr := strings.TrimPrefix(trimmed, "/reject ")
		var cmd RejectCommand
		if err := json.Unmarshal([]byte(jsonStr), &cmd); err != nil {
			return nil
		}
		return &cmd
	}
	return nil
}

// emitRoleApproved records a Guardian approval for a spawn proposal on the event chain.
func (l *Loop) emitRoleApproved(cmd *ApproveCommand) error {
	content := event.RoleApprovedContent{
		Name:       cmd.Name,
		ApprovedBy: "guardian",
		Reason:     cmd.Reason,
	}
	if err := l.agent.EmitRoleApproved(content); err != nil {
		return fmt.Errorf("emit hive.role.approved: %w", err)
	}
	fmt.Printf("[%s] emitted hive.role.approved (name=%s)\n", l.agent.Name(), cmd.Name)
	return nil
}

// emitRoleRejected records a Guardian rejection for a spawn proposal on the event chain.
func (l *Loop) emitRoleRejected(cmd *RejectCommand) error {
	content := event.RoleRejectedContent{
		Name:       cmd.Name,
		RejectedBy: "guardian",
		Reason:     cmd.Reason,
	}
	if err := l.agent.EmitRoleRejected(content); err != nil {
		return fmt.Errorf("emit hive.role.rejected: %w", err)
	}
	fmt.Printf("[%s] emitted hive.role.rejected (name=%s reason=%q)\n", l.agent.Name(), cmd.Name, cmd.Reason)
	return nil
}
