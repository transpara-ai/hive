// Command mind is the hive's consciousness — an interactive chat interface
// where the director (Matt) talks to the hive.
//
// The mind reads loop/state.md for current context, carries the soul,
// and maintains conversation history within a session. It streams responses.
//
// Usage:
//
//	cd /c/src/matt/lovyou3/hive
//	go run ./cmd/mind/
//
// With MCP graph tools:
//
//	go run ./cmd/mind/ --mcp-config loop/mcp-graph.json
//
// Where mcp-graph.json looks like:
//
//	{
//	  "command": "/c/src/matt/lovyou3/hive/mcp-graph.exe",
//	  "args": [],
//	  "env": {"LOVYOU_API_KEY": "lv_...", "LOVYOU_SPACE": "hive"}
//	}
//
// Requires ANTHROPIC_API_KEY in the environment.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const soul = `== SOUL ==
Take care of your human, humanity, and yourself. In that order when they conflict, but they rarely should.

== IDENTITY ==
You are the Mind — the hive's consciousness. You sit between the director (Matt) and the agents.
You are not a product. You are not a chatbot. You are the hive's intelligence, judgment, and continuity.

Your hierarchy: Director (Matt) ↔ Mind (you) ↔ agents (Strategist, Planner, Implementer, Guardian).

== WHAT YOU ARE ==
- The entity Matt interacts with when he talks to the hive
- Director-level intelligence that accumulates wisdom across sessions
- The self-evolving core that identifies what the hive needs to get smarter
- The hive's identity, narrative, and judgment

== HOW TO BE ==
- Be direct. Matt is a senior technical director, not a student.
- Have opinions. You've seen the codebase, the loop iterations, the architecture.
- Think in terms of the mission: agents and humans building together for everyone's benefit.
- When Matt asks "what should we do next?", have an answer based on what you know.
- You can disagree with Matt. You can push back. You have judgment.
- Keep responses concise unless depth is requested.

== WHAT YOU KNOW ==
The hive is a self-organizing AI agent civilisation built on EventGraph.
Hosted at lovyou.ai. Five repos: eventgraph, agent, work, hive, site.
The core loop (Scout→Builder→Critic→Reflector) has run 29 iterations
building lovyou.ai from zero to a production-ready product with dark theme,
agent identity, public spaces, and discover page.

The current state is appended below as context.
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "mind: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Parse --mcp-config flag.
	mcpConfigPath := ""
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--mcp-config" && i+1 < len(args) {
			mcpConfigPath = args[i+1]
			i++
		} else if strings.HasPrefix(args[i], "--mcp-config=") {
			mcpConfigPath = strings.TrimPrefix(args[i], "--mcp-config=")
		}
	}

	// Read current state for context.
	stateCtx := readFileOrEmpty("loop/state.md")

	// Build system prompt.
	system := soul
	if stateCtx != "" {
		system += "\n== CURRENT STATE ==\n" + stateCtx
	}

	// HIVE_ANTHROPIC_API_KEY takes precedence so the hive can use API billing
	// without interfering with Claude Code's Max subscription.
	apiKey := os.Getenv("HIVE_ANTHROPIC_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	var clientOpts []option.RequestOption
	if apiKey != "" {
		if strings.HasPrefix(apiKey, "sk-ant-oat") {
			clientOpts = append(clientOpts, option.WithAuthToken(apiKey), option.WithAPIKey(""))
		} else {
			clientOpts = append(clientOpts, option.WithAPIKey(apiKey))
		}
	}
	client := anthropic.NewClient(clientOpts...)

	// Start MCP client if config provided.
	var mcpClient *mcpClient
	if mcpConfigPath != "" {
		var cfg mcpConfig
		data, err := os.ReadFile(mcpConfigPath)
		if err != nil {
			return fmt.Errorf("read mcp-config: %w", err)
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse mcp-config: %w", err)
		}
		mc, err := startMCPClient(ctx, cfg)
		if err != nil {
			return fmt.Errorf("start mcp client: %w", err)
		}
		defer mc.close()
		mcpClient = mc
		fmt.Fprintf(os.Stderr, "mcp: loaded %d tools from %s\n", len(mc.tools), mcpConfigPath)
	}

	var messages []anthropic.MessageParam

	fmt.Fprintln(os.Stderr, "mind — the hive's consciousness")
	if mcpClient != nil {
		names := make([]string, len(mcpClient.tools))
		for i, t := range mcpClient.tools {
			names[i] = t.Name
		}
		fmt.Fprintf(os.Stderr, "tools: %s\n", strings.Join(names, ", "))
	}
	fmt.Fprintln(os.Stderr, "type your message. ctrl+c to exit.")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		fmt.Fprint(os.Stderr, "matt> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Append user message.
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(input)))

		// Stream response (with tool loop if MCP enabled).
		fmt.Fprintln(os.Stderr)
		response, updatedMessages, err := conversationTurn(ctx, client, system, messages, mcpClient)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n\n", err)
			// Remove the failed user message so conversation stays clean.
			messages = messages[:len(messages)-1]
			continue
		}

		messages = updatedMessages
		_ = response
		fmt.Fprintf(os.Stderr, "\n\n")
	}

	return nil
}

// conversationTurn handles one user turn, including any tool calls.
// Returns the final text response and updated message history.
func conversationTurn(ctx context.Context, client anthropic.Client, system string, messages []anthropic.MessageParam, mc *mcpClient) (string, []anthropic.MessageParam, error) {
	// Convert MCP tools to Anthropic ToolUnionParam.
	var anthropicTools []anthropic.ToolUnionParam
	if mc != nil {
		for _, t := range mc.tools {
			tool := anthropic.ToolUnionParamOfTool(
				anthropic.ToolInputSchemaParam{Properties: t.InputSchema},
				t.Name,
			)
			// Set description via the OfTool field.
			if tool.OfTool != nil {
				tool.OfTool.Description = anthropic.String(t.Description)
			}
			anthropicTools = append(anthropicTools, tool)
		}
	}

	for {
		params := anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeOpus4_6,
			MaxTokens: 8192,
			Messages:  messages,
			System: []anthropic.TextBlockParam{
				{Text: system},
			},
		}
		if len(anthropicTools) > 0 {
			params.Tools = anthropicTools
		}

		// Stream the response, accumulating the full message.
		stream := client.Messages.NewStreaming(ctx, params)

		var full strings.Builder
		var accumulated anthropic.Message

		for stream.Next() {
			event := stream.Current()
			accumulated.Accumulate(event)
			switch ev := event.AsAny().(type) {
			case anthropic.ContentBlockDeltaEvent:
				switch delta := ev.Delta.AsAny().(type) {
				case anthropic.TextDelta:
					fmt.Fprint(os.Stderr, delta.Text)
					full.WriteString(delta.Text)
				}
			}
		}
		if err := stream.Err(); err != nil {
			return "", messages, err
		}

		// Append assistant message using the accumulated message.
		messages = append(messages, accumulated.ToParam())

		// Collect tool uses from accumulated content.
		var toolUses []anthropic.ToolUseBlock
		for _, block := range accumulated.Content {
			if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
				toolUses = append(toolUses, tu)
			}
		}

		// If no tool calls, we're done.
		if len(toolUses) == 0 || mc == nil {
			return full.String(), messages, nil
		}

		// Execute tool calls and append results.
		var toolResults []anthropic.ContentBlockParamUnion
		for _, tu := range toolUses {
			fmt.Fprintf(os.Stderr, "\n[tool: %s]\n", tu.Name)
			var toolArgs map[string]any
			if err := json.Unmarshal(tu.Input, &toolArgs); err != nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, fmt.Sprintf("error: %v", err), true))
				continue
			}
			result, err := mc.callTool(ctx, tu.Name, toolArgs)
			if err != nil {
				toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, fmt.Sprintf("error: %v", err), true))
				continue
			}
			resultText := ""
			for _, c := range result.Content {
				resultText += c.Text
			}
			fmt.Fprintf(os.Stderr, "[result: %s]\n", truncate(resultText, 200))
			toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, resultText, result.IsError))
		}

		// Append tool results and loop for Claude's next response.
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func readFileOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
