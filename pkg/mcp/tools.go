package mcp

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"

	"github.com/lovyou-ai/eventgraph/go/pkg/actor"
	"github.com/lovyou-ai/eventgraph/go/pkg/event"
	"github.com/lovyou-ai/eventgraph/go/pkg/store"
	"github.com/lovyou-ai/eventgraph/go/pkg/statestore"
	"github.com/lovyou-ai/eventgraph/go/pkg/trust"
	"github.com/lovyou-ai/eventgraph/go/pkg/types"
)

// Deps holds the dependencies shared by all tool handlers.
type Deps struct {
	Store  store.Store
	Actors actor.IActorStore
	States statestore.IStateStore
	Trust  *trust.DefaultTrustModel

	AgentID  types.ActorID        // the calling agent's identity
	HumanID  types.ActorID        // the human operator's identity
	ConvID   types.ConversationID // conversation grouping for emitted events
	MinTrust float64              // minimum trust for write tools (0 = allow all)
}

// RegisterAllTools registers all MCP tools on the server, with authority
// checking on write tools and audit logging on all tool calls.
func RegisterAllTools(s *Server, deps Deps) {
	// Shared signer for audit + emit events.
	// TODO(M3): Use the agent's persistent signing key from the actor store
	// instead of an ephemeral key. Currently signatures are structurally valid
	// but unverifiable across restarts because the public key is not registered.
	_, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(fmt.Sprintf("generate MCP signer key: %v", err))
	}
	signer := &ed25519Signer{key: privKey}
	registry := event.DefaultRegistry()
	factory := event.NewEventFactory(registry)

	audit := NewAuditLogger(deps.Store, factory, signer, deps.AgentID, deps.ConvID)
	authCheck := NewAuthorityChecker(deps.Actors, deps.Trust, deps.AgentID, deps.MinTrust)

	registerReadTools(s, deps, audit)
	registerSelfTools(s, deps, audit)
	registerWriteTools(s, deps, audit, authCheck, factory, signer)
}

// regTool registers a tool with audit logging.
func regTool(s *Server, audit *AuditLogger, name, description string, schema json.RawMessage, handler Handler) {
	s.RegisterTool(name, description, schema, WrapHandler(audit, name, handler))
}

func registerReadTools(s *Server, deps Deps, audit *AuditLogger) {
	// query_events — query recent events with optional filters
	regTool(s, audit, "query_events",
		"Query events from the graph. Returns recent events, optionally filtered by type or source.",
		mustSchema(`{
			"type": "object",
			"properties": {
				"limit":  {"type": "integer", "description": "Max events to return (default 20, max 100)"},
				"type":   {"type": "string", "description": "Filter by event type (e.g. 'trust.updated')"},
				"source": {"type": "string", "description": "Filter by source actor ID"}
			}
		}`),
		func(args map[string]any) (ToolCallResult, error) {
			limit := intArg(args, "limit", 20)
			if limit > 100 {
				limit = 100
			}

			if typeFilter, ok := stringArg(args, "type"); ok {
				et, err := types.NewEventType(typeFilter)
				if err != nil {
					return ErrorResult(fmt.Sprintf("invalid event type: %v", err)), nil
				}
				p, err := deps.Store.ByType(et, limit, types.None[types.Cursor]())
				if err != nil {
					return ErrorResult(fmt.Sprintf("query error: %v", err)), nil
				}
				return eventsToResult(p.Items()), nil
			}

			if source, ok := stringArg(args, "source"); ok {
				aid, err := types.NewActorID(source)
				if err != nil {
					return ErrorResult(fmt.Sprintf("invalid source actor ID: %v", err)), nil
				}
				p, err := deps.Store.BySource(aid, limit, types.None[types.Cursor]())
				if err != nil {
					return ErrorResult(fmt.Sprintf("query error: %v", err)), nil
				}
				return eventsToResult(p.Items()), nil
			}

			p, err := deps.Store.Recent(limit, types.None[types.Cursor]())
			if err != nil {
				return ErrorResult(fmt.Sprintf("query error: %v", err)), nil
			}
			return eventsToResult(p.Items()), nil
		},
	)

	// get_event — get a single event by ID
	regTool(s, audit, "get_event",
		"Get a single event by its ID, with full details including causes and content.",
		mustSchema(`{
			"type": "object",
			"properties": {
				"id": {"type": "string", "description": "Event ID (UUID v7)"}
			},
			"required": ["id"]
		}`),
		func(args map[string]any) (ToolCallResult, error) {
			id, ok := stringArg(args, "id")
			if !ok {
				return ErrorResult("id is required"), nil
			}
			eid, err := types.NewEventID(id)
			if err != nil {
				return ErrorResult(fmt.Sprintf("invalid event ID: %v", err)), nil
			}
			ev, err := deps.Store.Get(eid)
			if err != nil {
				return ErrorResult(fmt.Sprintf("not found: %v", err)), nil
			}
			return eventToResult(ev), nil
		},
	)

	// get_actor — look up an actor by ID
	regTool(s, audit, "get_actor",
		"Look up an actor by ID. Returns display name, type, status, and metadata.",
		mustSchema(`{
			"type": "object",
			"properties": {
				"id": {"type": "string", "description": "Actor ID (e.g. 'actor_abc123...')"}
			},
			"required": ["id"]
		}`),
		func(args map[string]any) (ToolCallResult, error) {
			id, ok := stringArg(args, "id")
			if !ok {
				return ErrorResult("id is required"), nil
			}
			aid, err := types.NewActorID(id)
			if err != nil {
				return ErrorResult(fmt.Sprintf("invalid actor ID: %v", err)), nil
			}
			a, err := deps.Actors.Get(aid)
			if err != nil {
				return ErrorResult(fmt.Sprintf("not found: %v", err)), nil
			}
			return actorToResult(a), nil
		},
	)

	// list_actors — list actors with filters
	regTool(s, audit, "list_actors",
		"List actors in the system. Optionally filter by type or status.",
		mustSchema(`{
			"type": "object",
			"properties": {
				"type":   {"type": "string", "description": "Filter by actor type: Human, AI, System, Committee, RulesEngine"},
				"status": {"type": "string", "description": "Filter by status: active, suspended, memorial"}
			}
		}`),
		func(args map[string]any) (ToolCallResult, error) {
			filter := actor.ActorFilter{Limit: 100}
			if t, ok := stringArg(args, "type"); ok {
				at := event.ActorType(t)
				if !at.IsValid() {
					return ErrorResult(fmt.Sprintf("invalid actor type: %q (valid: Human, AI, System, Committee, RulesEngine)", t)), nil
				}
				filter.Type = types.Some(at)
			}
			if st, ok := stringArg(args, "status"); ok {
				status, err := types.NewActorStatus(st)
				if err != nil {
					return ErrorResult(fmt.Sprintf("invalid status: %v", err)), nil
				}
				filter.Status = types.Some(status)
			}
			page, err := deps.Actors.List(filter)
			if err != nil {
				return ErrorResult(fmt.Sprintf("list error: %v", err)), nil
			}
			return actorsToResult(page.Items()), nil
		},
	)

	// get_trust — get trust score for an actor or between two actors
	regTool(s, audit, "get_trust",
		"Get trust score for an actor. If 'from' is provided, returns directional trust from→to.",
		mustSchema(`{
			"type": "object",
			"properties": {
				"actor": {"type": "string", "description": "Actor ID to get trust for"},
				"from":  {"type": "string", "description": "Optional: get directional trust from this actor toward 'actor'"}
			},
			"required": ["actor"]
		}`),
		func(args map[string]any) (ToolCallResult, error) {
			actorID, ok := stringArg(args, "actor")
			if !ok {
				return ErrorResult("actor is required"), nil
			}

			aid, err := types.NewActorID(actorID)
			if err != nil {
				return ErrorResult(fmt.Sprintf("invalid actor ID: %v", err)), nil
			}
			a, err := deps.Actors.Get(aid)
			if err != nil {
				return ErrorResult(fmt.Sprintf("actor not found: %v", err)), nil
			}

			if fromID, ok := stringArg(args, "from"); ok {
				fid, err := types.NewActorID(fromID)
				if err != nil {
					return ErrorResult(fmt.Sprintf("invalid from actor ID: %v", err)), nil
				}
				from, err := deps.Actors.Get(fid)
				if err != nil {
					return ErrorResult(fmt.Sprintf("from actor not found: %v", err)), nil
				}
				metrics, err := deps.Trust.Between(nil, from, a)
				if err != nil {
					return ErrorResult(fmt.Sprintf("trust error: %v", err)), nil
				}
				return trustToResult(metrics), nil
			}

			metrics, err := deps.Trust.Score(nil, a)
			if err != nil {
				return ErrorResult(fmt.Sprintf("trust error: %v", err)), nil
			}
			return trustToResult(metrics), nil
		},
	)
}

func registerSelfTools(s *Server, deps Deps, audit *AuditLogger) {
	// query_self — agent's own identity, trust, authority
	regTool(s, audit, "query_self",
		"Get your own actor info: ID, display name, type, trust level.",
		mustSchema(`{"type": "object", "properties": {}}`),
		func(args map[string]any) (ToolCallResult, error) {
			a, err := deps.Actors.Get(deps.AgentID)
			if err != nil {
				return ErrorResult(fmt.Sprintf("self lookup failed: %v", err)), nil
			}
			metrics, _ := deps.Trust.Score(nil, a)

			info := map[string]any{
				"id":           a.ID().Value(),
				"display_name": a.DisplayName(),
				"type":         string(a.Type()),
				"status":       string(a.Status()),
				"trust_score":  metrics.Overall().Value(),
				"confidence":   metrics.Confidence().Value(),
				"trend":        metrics.Trend().Value(),
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			return TextResult(string(data)), nil
		},
	)

	// query_human — human operator info
	regTool(s, audit, "query_human",
		"Get the human operator's info: ID, display name.",
		mustSchema(`{"type": "object", "properties": {}}`),
		func(args map[string]any) (ToolCallResult, error) {
			a, err := deps.Actors.Get(deps.HumanID)
			if err != nil {
				return ErrorResult(fmt.Sprintf("human lookup failed: %v", err)), nil
			}
			info := map[string]any{
				"id":           a.ID().Value(),
				"display_name": a.DisplayName(),
				"type":         string(a.Type()),
				"status":       string(a.Status()),
			}
			data, _ := json.MarshalIndent(info, "", "  ")
			return TextResult(string(data)), nil
		},
	)
}

func registerWriteTools(s *Server, deps Deps, audit *AuditLogger, authCheck *AuthorityChecker, factory *event.EventFactory, signer event.Signer) {
	// emit_event — record an event on the graph (authority-checked)
	emitHandler := func(args map[string]any) (ToolCallResult, error) {
		eventType, ok := stringArg(args, "event_type")
		if !ok {
			return ErrorResult("event_type is required"), nil
		}
		contentRaw, ok := args["content"]
		if !ok {
			return ErrorResult("content is required"), nil
		}

		// Marshal content to JSON for deserialization into typed structs.
		contentJSON, err := json.Marshal(contentRaw)
		if err != nil {
			return ErrorResult(fmt.Sprintf("invalid content: %v", err)), nil
		}

		content, err := unmarshalAgentContent(eventType, contentJSON, deps.AgentID)
		if err != nil {
			return ErrorResult(err.Error()), nil
		}

		// Get the chain head as cause.
		head, err := deps.Store.Head()
		if err != nil {
			return ErrorResult(fmt.Sprintf("store error: %v", err)), nil
		}
		var causes []types.EventID
		if head.IsSome() {
			causes = []types.EventID{head.Unwrap().ID()}
		} else {
			return ErrorResult("graph has no events (not bootstrapped)"), nil
		}

		et, err := types.NewEventType(eventType)
		if err != nil {
			return ErrorResult(fmt.Sprintf("invalid event type: %v", err)), nil
		}

		ev, err := factory.Create(et, deps.AgentID, content, causes, deps.ConvID, deps.Store, signer)
		if err != nil {
			return ErrorResult(fmt.Sprintf("create event: %v", err)), nil
		}

		stored, err := deps.Store.Append(ev)
		if err != nil {
			return ErrorResult(fmt.Sprintf("append event: %v", err)), nil
		}

		return eventToResult(stored), nil
	}

	// Chain: authority check → audit log → handler
	wrapped := WrapHandler(audit, "emit_event", WrapWriteHandler(authCheck, "emit_event", emitHandler))

	s.RegisterTool("emit_event",
		"Record an event on the graph. Use this to log observations, actions, decisions, learnings, "+
			"communications, escalations, or refusals. The event is signed and causally linked.",
		mustSchema(`{
			"type": "object",
			"properties": {
				"event_type": {
					"type": "string",
					"description": "Event type. One of: agent.observed, agent.evaluated, agent.decided, agent.acted, agent.learned, agent.communicated, agent.escalated, agent.refused"
				},
				"content": {
					"type": "object",
					"description": "Event content fields (varies by type). Common fields: Action, Target, Subject, Reason, Lesson, Source, Observation, Recipient, Channel"
				}
			},
			"required": ["event_type", "content"]
		}`),
		wrapped,
	)
}

// unmarshalAgentContent deserializes JSON into the correct EventContent type.
// The agentID is injected since the agent shouldn't need to specify its own ID.
func unmarshalAgentContent(eventType string, data []byte, agentID types.ActorID) (event.EventContent, error) {
	switch eventType {
	case "agent.observed":
		var c event.AgentObservedContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("invalid content for %s: %v", eventType, err)
		}
		c.AgentID = agentID
		return c, nil

	case "agent.evaluated":
		var c event.AgentEvaluatedContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("invalid content for %s: %v", eventType, err)
		}
		c.AgentID = agentID
		return c, nil

	case "agent.decided":
		var c event.AgentDecidedContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("invalid content for %s: %v", eventType, err)
		}
		c.AgentID = agentID
		return c, nil

	case "agent.acted":
		var c event.AgentActedContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("invalid content for %s: %v", eventType, err)
		}
		c.AgentID = agentID
		return c, nil

	case "agent.learned":
		var c event.AgentLearnedContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("invalid content for %s: %v", eventType, err)
		}
		c.AgentID = agentID
		return c, nil

	case "agent.communicated":
		var c event.AgentCommunicatedContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("invalid content for %s: %v", eventType, err)
		}
		c.AgentID = agentID
		return c, nil

	case "agent.escalated":
		var c event.AgentEscalatedContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("invalid content for %s: %v", eventType, err)
		}
		c.AgentID = agentID
		return c, nil

	case "agent.refused":
		var c event.AgentRefusedContent
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("invalid content for %s: %v", eventType, err)
		}
		c.AgentID = agentID
		return c, nil

	default:
		return nil, fmt.Errorf("unsupported event type: %s (supported: agent.observed, agent.evaluated, agent.decided, agent.acted, agent.learned, agent.communicated, agent.escalated, agent.refused)", eventType)
	}
}

// ed25519Signer implements event.Signer for MCP-emitted events.
type ed25519Signer struct {
	key ed25519.PrivateKey
}

func (s *ed25519Signer) Sign(data []byte) (types.Signature, error) {
	sig := ed25519.Sign(s.key, data)
	return types.NewSignature(sig)
}

// --- helpers ---

func mustSchema(s string) json.RawMessage {
	var v json.RawMessage
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(fmt.Sprintf("invalid schema JSON: %v", err))
	}
	return v
}

func stringArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func intArg(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	// JSON numbers decode as float64
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return def
}
