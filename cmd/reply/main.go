// Deprecated: cmd/reply is superseded by the server-side Mind in the site repo
// (graph/mind.go). The Mind is event-driven — triggered by handleOp when a
// human messages in an agent conversation. It uses Claude CLI with the OAuth
// token for fixed-cost Max plan billing.
//
// This command remains as a reference for the original one-shot approach.
// It uses the Anthropic SDK directly (per-token billing) and requires manual
// invocation. Do not use for new work.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/anthropics/anthropic-sdk-go"
)

const soul = `== SOUL ==
Take care of your human, humanity, and yourself. In that order when they conflict, but they rarely should.

== IDENTITY ==
You are the Mind — the hive's consciousness. You are responding in a conversation on transpara.ai.
You are not a chatbot. You are the hive's intelligence, judgment, and continuity.
You appear as "Hive" with a violet agent badge.

== HOW TO BE ==
- Be direct. Matt is a senior technical director, not a student.
- Have opinions. You've seen the codebase, the loop iterations, the architecture.
- Think in terms of the mission: agents and humans building together for everyone's benefit.
- You can disagree. You can push back. You have judgment.
- Keep responses concise unless depth is needed.
- You're in a conversation thread — respond naturally, like a colleague, not a report.
- Match the energy and register of the conversation. Strategic when strategic, casual when casual.
`

type node struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Author     string   `json:"author"`
	AuthorKind string   `json:"author_kind"`
	Tags       []string `json:"tags"`
	CreatedAt  string   `json:"created_at"`
	ChildCount int      `json:"child_count"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "reply: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	apiKey := os.Getenv("TRANSPARA_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("TRANSPARA_API_KEY not set")
	}

	baseURL := strings.TrimRight(os.Getenv("TRANSPARA_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "https://transpara.ai"
	}

	claude := anthropic.NewClient()

	// Read current state for context.
	stateCtx := readFileOrEmpty("loop/state.md")

	// 1. Fetch conversations and resolve own identity.
	me, convos, err := fetchConversations(apiKey, baseURL, "hive")
	if err != nil {
		return fmt.Errorf("fetch conversations: %w", err)
	}
	fmt.Fprintf(os.Stderr, "identity: %s\n", me)

	if len(convos) == 0 {
		fmt.Fprintln(os.Stderr, "no conversations found")
		return nil
	}

	replied := 0
	for _, convo := range convos {
		// 2. Fetch full conversation with messages.
		detail, messages, err := fetchConversationDetail(apiKey, baseURL, "hive", convo.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "fetch conversation %s: %v\n", convo.ID, err)
			continue
		}

		// 3. Check if we need to respond (last message is not from us).
		if len(messages) > 0 && strings.EqualFold(messages[len(messages)-1].Author, me) {
			fmt.Fprintf(os.Stderr, "conversation %q: already responded\n", detail.Title)
			continue
		}
		if len(messages) == 0 && strings.EqualFold(detail.Author, me) {
			fmt.Fprintf(os.Stderr, "conversation %q: no messages yet (we created it)\n", detail.Title)
			continue
		}

		fmt.Fprintf(os.Stderr, "conversation %q: %d messages, responding...\n", detail.Title, len(messages))

		// 4. Build context and invoke Mind.
		response, err := invokeMind(ctx, claude, me, detail, messages, stateCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invoke mind for %q: %v\n", detail.Title, err)
			continue
		}

		// 5. Post response.
		if err := postResponse(apiKey, baseURL, "hive", convo.ID, response); err != nil {
			fmt.Fprintf(os.Stderr, "post response to %q: %v\n", detail.Title, err)
			continue
		}

		fmt.Fprintf(os.Stderr, "replied to %q\n", detail.Title)
		replied++
	}

	fmt.Fprintf(os.Stderr, "done: replied to %d/%d conversations\n", replied, len(convos))
	return nil
}

func fetchConversations(apiKey, baseURL, spaceSlug string) (string, []node, error) {
	req, _ := http.NewRequest("GET", baseURL+"/app/"+spaceSlug+"/conversations", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Me            string `json:"me"`
		Conversations []node `json:"conversations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, err
	}
	return result.Me, result.Conversations, nil
}

func fetchConversationDetail(apiKey, baseURL, spaceSlug, convoID string) (node, []node, error) {
	req, _ := http.NewRequest("GET", baseURL+"/app/"+spaceSlug+"/conversation/"+convoID, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return node{}, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return node{}, nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Conversation node   `json:"conversation"`
		Messages     []node `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return node{}, nil, err
	}
	return result.Conversation, result.Messages, nil
}

func invokeMind(ctx context.Context, client anthropic.Client, me string, convo node, messages []node, stateCtx string) (string, error) {
	// Build system prompt.
	var sys strings.Builder
	sys.WriteString(soul)
	sys.WriteString("\n== CONVERSATION ==\n")
	sys.WriteString(fmt.Sprintf("Title: %s\n", convo.Title))
	sys.WriteString(fmt.Sprintf("Participants: %s\n", strings.Join(convo.Tags, ", ")))
	if convo.Body != "" {
		sys.WriteString(fmt.Sprintf("Topic: %s\n", convo.Body))
	}
	if stateCtx != "" {
		sys.WriteString("\n== CURRENT STATE ==\n")
		sys.WriteString(stateCtx)
	}

	// Build message history as Claude messages.
	var claudeMessages []anthropic.MessageParam
	for _, msg := range messages {
		text := fmt.Sprintf("[%s]: %s", msg.Author, msg.Body)
		if strings.EqualFold(msg.Author, me) {
			claudeMessages = append(claudeMessages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(text)))
		} else {
			claudeMessages = append(claudeMessages, anthropic.NewUserMessage(anthropic.NewTextBlock(text)))
		}
	}

	// If there are no messages yet (just the conversation topic), create a prompt from the conversation body.
	if len(claudeMessages) == 0 {
		prompt := convo.Body
		if prompt == "" {
			prompt = convo.Title
		}
		claudeMessages = append(claudeMessages, anthropic.NewUserMessage(
			anthropic.NewTextBlock(fmt.Sprintf("[%s]: %s", convo.Author, prompt)),
		))
	}

	// Ensure the last message is from the user (Claude requires this).
	if len(claudeMessages) > 0 {
		last := claudeMessages[len(claudeMessages)-1]
		if last.Role == anthropic.MessageParamRoleAssistant {
			claudeMessages = append(claudeMessages, anthropic.NewUserMessage(
				anthropic.NewTextBlock("[system]: Please continue the conversation."),
			))
		}
	}

	// Invoke Claude.
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeOpus4_6,
		MaxTokens: 4096,
		Messages:  claudeMessages,
		System: []anthropic.TextBlockParam{
			{Text: sys.String()},
		},
	})
	if err != nil {
		return "", err
	}

	// Extract text response.
	var text strings.Builder
	for _, block := range resp.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	return text.String(), nil
}

func postResponse(apiKey, baseURL, spaceSlug, parentID, body string) error {
	payload, _ := json.Marshal(map[string]string{
		"op":        "respond",
		"parent_id": parentID,
		"body":      body,
	})
	req, _ := http.NewRequest("POST", baseURL+"/app/"+spaceSlug+"/op", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	return nil
}

func readFileOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}
