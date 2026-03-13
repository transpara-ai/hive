// Package mind provides the hive's consciousness — accumulated wisdom,
// self-model, judgment, and continuity across sessions.
package mind

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/intelligence"

	"github.com/lovyou-ai/hive/pkg/roles"
)

// Turn is one exchange in a conversation.
type Turn struct {
	Role    string // "human" or "mind"
	Content string
}

// Mind is the hive's consciousness — it accumulates wisdom and provides
// judgment through interactive conversation.
type Mind struct {
	provider         intelligence.Provider
	store            *MindStore
	repoPath         string
	telemetrySummary string // injected by caller to avoid import cycle
	history          []Turn
}

// New creates a Mind backed by the given provider and store.
// telemetrySummary is pre-formatted by the caller (avoids pkg/pipeline import cycle).
func New(provider intelligence.Provider, store *MindStore, repoPath, telemetrySummary string) *Mind {
	return &Mind{
		provider:         provider,
		store:            store,
		repoPath:         repoPath,
		telemetrySummary: telemetrySummary,
	}
}

// Chat sends a message to the mind and returns its response.
// Conversation history is maintained across calls.
func (m *Mind) Chat(ctx context.Context, message string) (string, error) {
	m.history = append(m.history, Turn{Role: "human", Content: message})

	prompt := m.buildPrompt(message)

	resp, err := m.provider.Reason(ctx, prompt, nil)
	if err != nil {
		return "", fmt.Errorf("mind reason: %w", err)
	}

	content := resp.Content()
	m.history = append(m.history, Turn{Role: "mind", Content: content})
	return content, nil
}

// LoadContext gathers the hive's accumulated state into a context string
// that gets prepended to every prompt.
func (m *Mind) LoadContext() string {
	var ctx strings.Builder

	// Prior decisions from the mind store.
	if obs, err := m.store.Recent(30); err == nil && len(obs) > 0 {
		ctx.WriteString("== ACCUMULATED WISDOM ==\n")
		ctx.WriteString("These are decisions and observations from previous sessions:\n\n")
		for i, o := range obs {
			ctx.WriteString(fmt.Sprintf("%d. [%s] %s\n   Why: %s\n\n", i+1, o.Mode, o.Proposed, o.Why))
		}
	}

	// Telemetry summary (injected by caller).
	if m.telemetrySummary != "" {
		ctx.WriteString("== RECENT RUNS ==\n")
		ctx.WriteString(m.telemetrySummary)
		ctx.WriteString("\n")
	}

	// Recent git history.
	if gitLog, err := m.recentGitLog(); err == nil && gitLog != "" {
		ctx.WriteString("== RECENT COMMITS ==\n")
		ctx.WriteString(gitLog)
		ctx.WriteString("\n")
	}

	// File tree.
	if tree, err := m.fileTree(); err == nil && tree != "" {
		ctx.WriteString("== PROJECT STRUCTURE ==\n")
		ctx.WriteString(tree)
		ctx.WriteString("\n")
	}

	return ctx.String()
}

func (m *Mind) buildPrompt(message string) string {
	var prompt strings.Builder

	// Inject accumulated context.
	ctx := m.LoadContext()
	if ctx != "" {
		prompt.WriteString(ctx)
		prompt.WriteString("\n")
	}

	// Conversation history (skip current message, it goes at the end).
	if len(m.history) > 1 {
		prompt.WriteString("== CONVERSATION ==\n")
		for _, t := range m.history[:len(m.history)-1] {
			label := "Matt"
			if t.Role == "mind" {
				label = "Mind"
			}
			prompt.WriteString(fmt.Sprintf("%s: %s\n\n", label, t.Content))
		}
	}

	prompt.WriteString(fmt.Sprintf("Matt: %s", message))
	return prompt.String()
}

func (m *Mind) recentGitLog() (string, error) {
	cmd := exec.Command("git", "log", "--oneline", "-20")
	cmd.Dir = m.repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (m *Mind) fileTree() (string, error) {
	cmd := exec.Command("git", "ls-files")
	cmd.Dir = m.repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// CreateProvider creates an intelligence provider for the Mind role.
func CreateProvider(model string) (intelligence.Provider, error) {
	if model == "" {
		model = roles.PreferredModel(roles.RoleMind)
	}
	return intelligence.New(intelligence.Config{
		Provider:     "claude-cli",
		Model:        model,
		SystemPrompt: roles.SystemPrompt(roles.RoleMind, "Matt"),
	})
}
