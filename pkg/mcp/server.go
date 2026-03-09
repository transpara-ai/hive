package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Handler processes an MCP tool call.
type Handler func(args map[string]any) (ToolCallResult, error)

// Server is an MCP server that communicates over stdio.
type Server struct {
	name     string
	version  string
	tools    []Tool
	handlers map[string]Handler
}

// NewServer creates an MCP server with the given identity.
func NewServer(name, version string) *Server {
	return &Server{
		name:     name,
		version:  version,
		handlers: make(map[string]Handler),
	}
}

// RegisterTool registers a tool with its handler.
func (s *Server) RegisterTool(name, description string, schema json.RawMessage, handler Handler) {
	s.tools = append(s.tools, Tool{
		Name:        name,
		Description: description,
		InputSchema: schema,
	})
	s.handlers[name] = handler
}

// Run starts the server, reading JSON-RPC from stdin and writing to stdout.
func (s *Server) Run() error {
	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(writer, nil, -32700, "parse error")
			continue
		}

		resp := s.handleRequest(req)
		if resp != nil {
			s.writeResponse(writer, *resp)
		}
	}
}

func (s *Server) handleRequest(req Request) *Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		return nil // notification, no response
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "ping":
		return s.success(req.ID, json.RawMessage(`{}`))
	default:
		return s.errorResp(req.ID, -32601, fmt.Sprintf("method not found: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req Request) *Response {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapability{
			Tools: &ToolCapability{},
		},
		ServerInfo: AppInfo{Name: s.name, Version: s.version},
	}
	data, _ := json.Marshal(result)
	return s.success(req.ID, data)
}

func (s *Server) handleToolsList(req Request) *Response {
	result := ToolsListResult{Tools: s.tools}
	data, _ := json.Marshal(result)
	return s.success(req.ID, data)
}

func (s *Server) handleToolsCall(req Request) *Response {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.errorResp(req.ID, -32602, "invalid params")
	}

	handler, ok := s.handlers[params.Name]
	if !ok {
		return s.errorResp(req.ID, -32602, fmt.Sprintf("unknown tool: %s", params.Name))
	}

	result, err := handler(params.Arguments)
	if err != nil {
		result = ErrorResult(err.Error())
	}

	data, _ := json.Marshal(result)
	return s.success(req.ID, data)
}

func (s *Server) success(id json.RawMessage, result json.RawMessage) *Response {
	return &Response{JSONRPC: "2.0", ID: id, Result: result}
}

func (s *Server) errorResp(id json.RawMessage, code int, msg string) *Response {
	return &Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: msg}}
}

func (s *Server) writeResponse(w io.Writer, resp Response) {
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	w.Write(data)
}

func (s *Server) writeError(w io.Writer, id json.RawMessage, code int, msg string) {
	s.writeResponse(w, Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	})
}
