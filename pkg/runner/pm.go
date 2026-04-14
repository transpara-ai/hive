package runner

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/lovyou-ai/eventgraph/go/pkg/decision"
)

// runPM reads the board, finds the pinned goal, and creates ONE milestone
// if the board has no open work. Simple. DB-only. No file reads.
func (r *Runner) runPM(ctx context.Context) {
	if !r.cfg.OneShot && r.tick%16 != 0 {
		return
	}

	log.Printf("[pm] tick %d: deciding next priority", r.tick)

	op, canOperate := r.cfg.Provider.(decision.IOperator)
	if !canOperate {
		log.Printf("[pm] provider does not support Operate, skipping")
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	apiKey := os.Getenv("LOVYOU_API_KEY")
	slug := r.cfg.SpaceSlug

	// Build repo list from registry.
	repoList := ""
	if r.cfg.Registry != nil {
		repoList = r.cfg.Registry.Summary()
	} else {
		var names []string
		for name := range r.cfg.RepoMap {
			names = append(names, name)
		}
		repoList = "Repos: " + strings.Join(names, ", ")
	}

	instruction := fmt.Sprintf(`You are the PM. Create ONE milestone for the next piece of work.

## Rules
1. Read the board FIRST. If there are open tasks, do NOT create more — the hive has work.
2. Find the PINNED goal on the board — it contains a priority queue. Work it top to bottom.
3. Create exactly ONE milestone (high priority task) for the next item in the queue.
4. The milestone title should be clear and actionable. The body should have 2-4 specific subtask descriptions.
5. Include "**Target repo:** <name>" in the body so the pipeline knows which repo to work on.
6. Do NOT read files. Do NOT search knowledge. Just read the board and create the milestone.

## How to read the board
curl -s -H "Authorization: Bearer %s" -H "Accept: application/json" "%s/app/%s/board"

Look for:
- "pinned": true — the Director's goal with the priority queue
- "state": "open" or "active" — existing work (if any exists, STOP — don't create more)

## How to create a milestone
curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "%s/app/%s/op" -d '{"op":"intend","kind":"task","title":"<TITLE>","description":"<BODY WITH TARGET REPO AND SUBTASKS>","priority":"high","causes":["<PINNED_GOAL_ID>"]}'

## %s

If the board already has open tasks: respond with "Board has work. No action needed." and stop.
If there is no pinned goal: respond with "No pinned goal. Waiting for Director." and stop.
`, apiKey, r.cfg.APIBase, slug, apiKey, r.cfg.APIBase, slug, repoList)

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     r.cfg.HiveDir,
		Instruction: instruction,
	})
	if err != nil {
		log.Printf("[pm] Operate error: %v", err)
	} else {
		r.cost.Record(result.Usage)
		r.dailyBudget.Record(result.Usage.CostUSD)
		log.Printf("[pm] decision made (cost=$%.4f)", result.Usage.CostUSD)
	}

	if r.cfg.OneShot {
		r.done = true
	}
}
