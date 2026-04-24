package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
)

// councilMember represents one agent's voice in the council.
type councilMember struct {
	role     string
	prompt   string // loaded from agents/{role}.md
	response string // their contribution
}

// RunCouncil convenes all agents for a deliberation session.
// Each agent receives shared context and speaks from their perspective.
// Uses the runner Config — same provider, API client, paths.
func RunCouncil(ctx context.Context, cfg Config) error {
	log.Println("[council] ═══ Convening the hive council ═══")
	start := time.Now()

	hiveDir := cfg.HiveDir
	if hiveDir == "" {
		log.Println("[council] HiveDir not set, cannot load agent prompts")
		return fmt.Errorf("HiveDir required for council")
	}

	// Load shared context that every agent sees.
	sharedContext := loadCouncilContext(hiveDir, cfg.RepoPath)

	// Load all agent roles.
	members := loadCouncilMembers(hiveDir)
	if len(members) == 0 {
		return fmt.Errorf("no agent prompts found in %s/agents/", hiveDir)
	}

	log.Printf("[council] %d agents assembled: %s", len(members), memberNames(members))

	// Each agent deliberates (concurrently for speed).
	// Use Operate() if available so agents can search the knowledge layer.
	var wg sync.WaitGroup
	var mu sync.Mutex
	totalCost := 0.0

	op, canOperate := cfg.Provider.(decision.IOperator)

	for i := range members {
		wg.Add(1)
		go func(m *councilMember) {
			defer wg.Done()

			if canOperate {
				// Operate mode: agent can search knowledge, read code, check the board.
				apiKey := os.Getenv("LOVYOU_API_KEY")
				instruction := buildCouncilOperateInstruction(m.role, m.prompt, cfg.CouncilTopic, cfg.SpaceSlug, apiKey, cfg.APIBase)
				result, err := op.Operate(ctx, decision.OperateTask{
					WorkDir:     cfg.HiveDir,
					Instruction: instruction,
				})
				if err != nil {
					log.Printf("[council] %s: error: %v", m.role, err)
					m.response = fmt.Sprintf("(could not contribute: %v)", err)
					return
				}

				mu.Lock()
				totalCost += result.Usage.CostUSD
				mu.Unlock()

				m.response = result.Summary
				log.Printf("[council] %s spoke ($%.4f)", m.role, result.Usage.CostUSD)
			} else {
				// Reason fallback: static context, no search.
				prompt := buildCouncilPrompt(m.role, m.prompt, sharedContext, cfg.CouncilTopic)
				resp, err := cfg.Provider.Reason(ctx, prompt, nil)
				if err != nil {
					log.Printf("[council] %s: error: %v", m.role, err)
					m.response = fmt.Sprintf("(could not contribute: %v)", err)
					return
				}

				mu.Lock()
				totalCost += resp.Usage().CostUSD
				mu.Unlock()

				m.response = resp.Content()
				log.Printf("[council] %s spoke ($%.4f)", m.role, resp.Usage().CostUSD)
			}
		}(&members[i])
	}

	wg.Wait()
	NewDailyBudget(cfg.HiveDir).Record(totalCost)
	log.Printf("[council] all agents spoke (total cost: $%.4f)", totalCost)

	// Synthesize — the Reflector summarizes.
	report := synthesizeCouncil(members)

	// Write the council report.
	reportPath := filepath.Join(hiveDir, "loop", "council.md")
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		log.Printf("[council] write report error: %v", err)
	} else {
		log.Printf("[council] report written to loop/council.md")
	}

	// Print to stdout.
	fmt.Println(report)

	elapsed := time.Since(start).Round(time.Second)
	log.Printf("[council] ═══ Council complete (%s, $%.4f) ═══", elapsed, totalCost)

	// Post summary to lovyou.ai if API client is available.
	if cfg.APIClient != nil {
		title := fmt.Sprintf("Council report — %s", time.Now().Format("2006-01-02"))
		_ = cfg.APIClient.PostUpdate(cfg.SpaceSlug, title, truncateForPost(report, 2000))
		log.Println("[council] posted to feed")
	}

	return nil
}

func loadCouncilContext(hiveDir, repoPath string) string {
	var parts []string

	// State (scout section + lessons).
	if data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "state.md")); err == nil {
		s := string(data)
		// Extract lessons section.
		if idx := strings.Index(s, "## Lessons"); idx >= 0 {
			parts = append(parts, "## Lessons from 230+ iterations\n"+s[idx:])
		}
		// Extract scout section.
		if idx := strings.Index(s, "## What the Scout Should Focus On Next"); idx >= 0 {
			end := strings.Index(s[idx+40:], "\n## ")
			if end > 0 {
				parts = append(parts, s[idx:idx+40+end])
			} else {
				parts = append(parts, s[idx:])
			}
		}
	}

	// Vision.
	if data, err := os.ReadFile(filepath.Join(hiveDir, "docs", "VISION.md")); err == nil {
		s := string(data)
		if len(s) > 2000 {
			s = s[:2000] + "\n..."
		}
		parts = append(parts, "## Vision\n"+s)
	}

	// Limitations.
	if data, err := os.ReadFile(filepath.Join(hiveDir, "docs", "LIMITATIONS.md")); err == nil {
		s := string(data)
		if len(s) > 1500 {
			s = s[:1500] + "\n..."
		}
		parts = append(parts, "## Known Limitations\n"+s)
	}

	// Target repo CLAUDE.md.
	if repoPath != "" {
		if data, err := os.ReadFile(filepath.Join(repoPath, "CLAUDE.md")); err == nil {
			s := string(data)
			if len(s) > 2000 {
				s = s[:2000] + "\n..."
			}
			parts = append(parts, "## Target Product\n"+s)
		}
	}

	// Recent reflections (last 2000 chars for rich context).
	if data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "reflections.md")); err == nil {
		s := string(data)
		if len(s) > 2000 {
			s = s[len(s)-2000:]
		}
		parts = append(parts, "## Recent Reflections\n"+s)
	}

	// The full list of agents in the civilization.
	if entries, err := os.ReadDir(filepath.Join(hiveDir, "agents")); err == nil {
		var roles []string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") && e.Name() != "CONTEXT.md" && e.Name() != "METHOD.md" {
				roles = append(roles, strings.TrimSuffix(e.Name(), ".md"))
			}
		}
		parts = append(parts, fmt.Sprintf("## Current Civilization\n%d agents: %s", len(roles), strings.Join(roles, ", ")))
	}

	return strings.Join(parts, "\n\n---\n\n")
}

func loadCouncilMembers(hiveDir string) []councilMember {
	agentsDir := filepath.Join(hiveDir, "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return nil
	}

	var members []councilMember
	for _, e := range entries {
		name := e.Name()
		// Skip non-role files.
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if name == "CONTEXT.md" || name == "METHOD.md" {
			continue
		}

		role := strings.TrimSuffix(name, ".md")
		data, err := os.ReadFile(filepath.Join(agentsDir, name))
		if err != nil {
			continue
		}

		members = append(members, councilMember{
			role:   role,
			prompt: string(data),
		})
	}
	return members
}

func memberNames(members []councilMember) string {
	names := make([]string, len(members))
	for i, m := range members {
		names[i] = m.role
	}
	return strings.Join(names, ", ")
}

func buildCouncilPrompt(role, rolePrompt, sharedContext, topic string) string {
	question := `Speak from your role's perspective. In 5-10 lines, address:

1. **What I see:** From my role's vantage point, what's the most important thing right now?
2. **What worries me:** What risk or gap is invisible to other agents but visible to me?
3. **What I'd change:** If I could direct the next 5 iterations, what would I prioritize?

Be specific. Name files, features, patterns. Don't repeat what others would say — contribute what ONLY your role can see.`

	if topic != "" {
		question = fmt.Sprintf("The Director has focused this council on a specific question:\n\n**%s**\n\nSpeak from your role's perspective. Be deep, be honest, be specific. This is not a status update — it is a deliberation. Think before you speak. Disagree with other roles if you must. Name what's missing, what's wrong, what's invisible. 10-20 lines.", topic)
	}

	return fmt.Sprintf(`You are the %s, attending a council meeting of the hive.

## Your Role
%s

## Shared Context (what everyone sees)
%s

## Your Task

%s`, role, rolePrompt, sharedContext, question)
}

func synthesizeCouncil(members []councilMember) string {
	var b strings.Builder
	b.WriteString("# Hive Council Report\n\n")
	b.WriteString(fmt.Sprintf("*%d agents convened. Each spoke from their role.*\n\n", len(members)))
	b.WriteString("---\n\n")

	for _, m := range members {
		b.WriteString(fmt.Sprintf("## %s\n\n", strings.ToUpper(m.role[:1])+m.role[1:]))
		if m.response != "" {
			b.WriteString(m.response)
		} else {
			b.WriteString("*(no response)*")
		}
		b.WriteString("\n\n---\n\n")
	}

	return b.String()
}

// buildCouncilOperateInstruction creates the instruction for a council member
// using Operate(). The agent can search knowledge, read code, and check the
// board before speaking — grounded deliberation, not blind opinion.
func buildCouncilOperateInstruction(role, rolePrompt, topic, spaceSlug, apiKey, apiBase string) string {
	question := "What's the most important thing right now from your perspective? What risk or gap is invisible to others? What would you prioritize?"
	if topic != "" {
		question = fmt.Sprintf("The Director has focused this council on: **%s**\n\nDeliberate on this question from your role's perspective. Be deep, specific, and honest.", topic)
	}

	return fmt.Sprintf(`You are the %s, attending a council meeting of the hive.

## Your Role
%s

## Your Tools
Before speaking, SEARCH for relevant knowledge to ground your perspective:
- Use knowledge.search to find prior work, decisions, and context
- Use knowledge.primitives to check which ontology primitives are relevant
- Use knowledge.get to read specific docs, blog posts, or reflections
- Use Bash to check the board: curl -s -H "Authorization: Bearer %s" -H "Accept: application/json" "%s/app/%s/board"

## Your Task
%s

Search first, then speak. Ground your perspective in what you find.
Respond in 10-20 lines. Be specific — name files, features, primitives.
Disagree with other roles if you must. Name what's missing.`, role, rolePrompt, apiKey, apiBase, spaceSlug, question)
}

func truncateForPost(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
