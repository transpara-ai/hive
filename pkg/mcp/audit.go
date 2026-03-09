package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// AuditLogger emits tool-call audit events on the graph.
// Every tool invocation is recorded so the Guardian can monitor agent activity.
type AuditLogger struct {
	store   StoreAppender
	factory *event.EventFactory
	signer  event.Signer
	agentID types.ActorID
	convID  types.ConversationID
}

// StoreAppender is the subset of store.Store needed for audit logging.
type StoreAppender interface {
	Head() (types.Option[event.Event], error)
	Append(ev event.Event) (event.Event, error)
}

// NewAuditLogger creates an audit logger. If store is nil, logging is a no-op.
func NewAuditLogger(s StoreAppender, factory *event.EventFactory, signer event.Signer, agentID types.ActorID, convID types.ConversationID) *AuditLogger {
	if s == nil {
		return nil
	}
	return &AuditLogger{
		store:   s,
		factory: factory,
		signer:  signer,
		agentID: agentID,
		convID:  convID,
	}
}

// LogToolCall records that a tool was called and whether it succeeded.
func (a *AuditLogger) LogToolCall(toolName string, args map[string]any, result ToolCallResult) {
	if a == nil {
		return
	}

	// Build a summary of the call for the audit trail.
	argsJSON, _ := json.Marshal(args)
	summary := fmt.Sprintf("tool=%s args=%s", toolName, string(argsJSON))
	if len(summary) > 500 {
		summary = summary[:500] + "..."
	}

	content := event.AgentActedContent{
		AgentID: a.agentID,
		Action:  "mcp.tool_call:" + toolName,
		Target:  summary,
	}

	head, err := a.store.Head()
	if err != nil || !head.IsSome() {
		return // can't log without a chain head — don't block the tool call
	}

	et, err := types.NewEventType("agent.acted")
	if err != nil {
		return
	}

	ev, err := a.factory.Create(
		et, a.agentID, content,
		[]types.EventID{head.Unwrap().ID()},
		a.convID, a.store, a.signer,
	)
	if err != nil {
		return
	}

	a.store.Append(ev)
}

// WrapHandler wraps a tool handler with audit logging.
func WrapHandler(audit *AuditLogger, name string, handler Handler) Handler {
	if audit == nil {
		return handler
	}
	return func(args map[string]any) (ToolCallResult, error) {
		result, err := handler(args)
		if err != nil {
			audit.LogToolCall(name, args, ErrorResult(err.Error()))
		} else {
			audit.LogToolCall(name, args, result)
		}
		return result, err
	}
}
