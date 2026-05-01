package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// PhaseCommand represents a parsed /phase command from an agent response.
type PhaseCommand struct {
	Action  string
	Payload json.RawMessage
}

type phaseGatePayload struct {
	Phase    string   `json:"phase"`
	Title    string   `json:"title"`
	Criteria []string `json:"criteria"`
}

type phaseApprovePayload struct {
	GateID  string `json:"gate_id"`
	Summary string `json:"summary"`
}

type phaseRejectPayload struct {
	GateID string `json:"gate_id"`
	Reason string `json:"reason"`
}

// parsePhaseCommands extracts /phase commands from an agent's response.
func parsePhaseCommands(response string) []PhaseCommand {
	var commands []PhaseCommand
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "/phase ") {
			continue
		}
		rest := strings.TrimPrefix(trimmed, "/phase ")
		idx := strings.Index(rest, " ")
		if idx < 0 {
			continue
		}
		action := strings.ToLower(rest[:idx])
		jsonStr := strings.TrimSpace(rest[idx+1:])
		switch action {
		case "gate", "approve", "reject":
			commands = append(commands, PhaseCommand{Action: action, Payload: json.RawMessage(jsonStr)})
		}
	}
	return commands
}

func executePhaseCommands(
	commands []PhaseCommand,
	gates *work.PhaseGateStore,
	agentID types.ActorID,
	causes []types.EventID,
	convID types.ConversationID,
) int {
	executed := 0
	for _, cmd := range commands {
		var err error
		switch cmd.Action {
		case "gate":
			err = execPhaseGate(cmd.Payload, gates, agentID, causes, convID)
		case "approve":
			err = execPhaseApprove(cmd.Payload, gates, agentID, causes, convID)
		case "reject":
			err = execPhaseReject(cmd.Payload, gates, agentID, causes, convID)
		}
		if err != nil {
			fmt.Printf("warning: /phase %s failed: %v\n", cmd.Action, err)
		} else {
			executed++
		}
	}
	return executed
}

func execPhaseGate(payload json.RawMessage, gates *work.PhaseGateStore, agentID types.ActorID, causes []types.EventID, convID types.ConversationID) error {
	var p phaseGatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	gate, err := gates.Declare(agentID, p.Phase, p.Title, p.Criteria, causes, convID)
	if err != nil {
		return err
	}
	fmt.Printf("  → phase gate declared: %s — %s\n", gate.ID.Value(), gate.Title)
	return nil
}

func execPhaseApprove(payload json.RawMessage, gates *work.PhaseGateStore, agentID types.ActorID, causes []types.EventID, convID types.ConversationID) error {
	var p phaseApprovePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	gateID, err := types.NewEventID(p.GateID)
	if err != nil {
		return fmt.Errorf("invalid gate_id: %w", err)
	}
	if err := gates.Approve(agentID, gateID, p.Summary, causes, convID); err != nil {
		return err
	}
	fmt.Printf("  → phase gate approved: %s\n", p.GateID)
	return nil
}

func execPhaseReject(payload json.RawMessage, gates *work.PhaseGateStore, agentID types.ActorID, causes []types.EventID, convID types.ConversationID) error {
	var p phaseRejectPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	gateID, err := types.NewEventID(p.GateID)
	if err != nil {
		return fmt.Errorf("invalid gate_id: %w", err)
	}
	if err := gates.Reject(agentID, gateID, p.Reason, causes, convID); err != nil {
		return err
	}
	fmt.Printf("  → phase gate rejected: %s\n", p.GateID)
	return nil
}
