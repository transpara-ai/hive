package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/decision"
)

// InvokeAgent runs any agent by name. Reads persona from the graph first
// (lovyou.ai agent_personas table), falls back to agents/{name}.md file.
// This is how pipeline roles communicate with the broader hive — 50+ agents
// available on demand, not hardcoded in the pipeline sequence.
func (r *Runner) InvokeAgent(ctx context.Context, name, task string) (string, error) {
	// Graph first: read agent from the API.
	var prompt, model string
	if r.cfg.APIClient != nil {
		if agent, err := r.cfg.APIClient.GetAgent(r.cfg.SpaceSlug, name); err == nil {
			prompt = agent.Prompt
			model = agent.Model
			log.Printf("[invoke:%s] loaded from graph (model=%s)", name, model)
		}
	}
	// Fallback: local file.
	if prompt == "" {
		prompt = LoadRolePrompt(r.cfg.HiveDir, name)
	}
	if prompt == "" {
		return "", fmt.Errorf("agent %q not found (graph or agents/)", name)
	}

	op, canOperate := r.cfg.Provider.(decision.IOperator)
	if !canOperate {
		// Fallback to Reason.
		fullPrompt := fmt.Sprintf("You are the %s.\n\n%s\n\n## Task\n%s", name, prompt, task)
		resp, err := r.cfg.Provider.Reason(ctx, fullPrompt, nil)
		if err != nil {
			return "", err
		}
		r.cost.Record(resp.Usage())
		return resp.Content(), nil
	}

	instruction := fmt.Sprintf("You are the %s.\n\n%s\n\n## Task\n%s", name, prompt, task)
	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     r.cfg.HiveDir,
		Instruction: instruction,
	})
	if err != nil {
		return "", err
	}
	r.cost.Record(result.Usage)
	r.dailyBudget.Record(result.Usage.CostUSD)
	log.Printf("[invoke:%s] done (cost=$%.4f)", name, result.Usage.CostUSD)
	return result.Summary, nil
}

// runSpawner checks if the hive needs new agents or if existing agents
// need to be wired into the pipeline. The Spawner is the SELF-EVOLVE
// invariant made concrete: every unhandled failure is a missing agent.
// Not a pipeline role — invoked on demand when gaps are detected.
func (r *Runner) runSpawner(ctx context.Context) {
	if !r.cfg.OneShot && r.tick%16 != 0 {
		return
	}

	log.Printf("[spawner] tick %d: checking for agency gaps", r.tick)

	op, canOperate := r.cfg.Provider.(decision.IOperator)
	if !canOperate {
		log.Printf("[spawner] provider does not support Operate, skipping")
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	apiKey := os.Getenv("LOVYOU_API_KEY")

	// List existing agent definitions.
	var agentFiles []string
	agentsDir := filepath.Join(r.cfg.HiveDir, "agents")
	if entries, err := os.ReadDir(agentsDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".md") && e.Name() != "CONTEXT.md" && e.Name() != "METHOD.md" {
				agentFiles = append(agentFiles, strings.TrimSuffix(e.Name(), ".md"))
			}
		}
	}

	instruction := fmt.Sprintf(`You are the Spawner. Your job: ensure the hive has every agent it needs.

## Current Agent Definitions (%d files in agents/)
%s

## Invariant: SELF-EVOLVE
Every unhandled failure is a missing agent. If a problem occurs and no agent exists to fix it, create the agent.

## Your Tools
- Use knowledge.search to find recent diagnostics and pipeline failures
- Read loop/diagnostics.jsonl for failure patterns
- Check the board for tasks that reveal missing capabilities:
  curl -s -H "Authorization: Bearer %s" -H "Accept: application/json" "%s/app/%s/board"

## Steps
1. Read recent diagnostics — are there recurring failures no agent handles?
2. Check the board — are there "Add agent:" tasks created by other roles?
3. Compare agent definitions to pipeline wiring — are any defined but not wired?
4. If a new agent is needed:
   a. Write its role prompt to agents/<name>.md
   b. Create a task to wire it into the pipeline
5. If an existing agent needs adjustment:
   a. Edit its prompt in agents/<name>.md
   b. Note what changed and why

## Rules
- Only create an agent if there's evidence of a gap (diagnostic, repeated failure, explicit request)
- Don't duplicate existing roles — search knowledge first
- Agent prompts should be 10-30 lines, specific about what the agent watches and does
- Every new agent gets a task to wire it into the pipeline

If no gaps are found, say "No agency gaps detected."`,
		len(agentFiles), strings.Join(agentFiles, ", "), apiKey, r.cfg.APIBase, r.cfg.SpaceSlug)

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     r.cfg.HiveDir,
		Instruction: instruction,
	})
	if err != nil {
		log.Printf("[spawner] Operate error: %v", err)
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	r.cost.Record(result.Usage)
	r.dailyBudget.Record(result.Usage.CostUSD)
	log.Printf("[spawner] done (cost=$%.4f)", result.Usage.CostUSD)

	if r.cfg.OneShot {
		r.done = true
	}
}
