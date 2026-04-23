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

// runScribe reads design conversation transcripts and extracts reasoning
// chains as claims on the graph. The ephemeral becomes persistent.
func (r *Runner) runScribe(ctx context.Context) {
	if !r.cfg.OneShot && r.tick%32 != 0 {
		return // Low cadence — runs every 32nd tick (~8 min)
	}

	log.Printf("[scribe] tick %d: ingesting conversations", r.tick)

	op, canOperate := r.cfg.Provider.(decision.IOperator)
	if !canOperate {
		log.Printf("[scribe] provider does not support Operate, skipping")
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	// Find conversation transcripts.
	transcriptDir := filepath.Join(os.Getenv("HOME"), ".claude", "projects", "C--src-matt-lovyou3")
	transcripts := findTranscripts(transcriptDir)
	if len(transcripts) == 0 {
		log.Printf("[scribe] no transcripts found in %s", transcriptDir)
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	apiKey := os.Getenv("LOVYOU_API_KEY")
	latestTranscript := transcripts[len(transcripts)-1]

	instruction := fmt.Sprintf(`You are the Scribe. Read the design conversation transcript and extract reasoning the hive needs to learn from.

## Transcript
Read the file: %s

It's a JSONL file — each line is a JSON object with "role" (human/assistant) and "content" fields.

## Your Task
1. Read the transcript (focus on the last 200 lines if it's very long)
2. Use knowledge.search to check what's already captured as claims
3. Extract the 5-10 most important reasoning chains — the WHY behind decisions
4. For each, assert a claim on the graph:

curl -s -X POST -H "Authorization: Bearer %s" -H "Content-Type: application/json" -H "Accept: application/json" "%s/app/%s/op" -d '{"op":"assert","title":"Reasoning: <INSIGHT>","body":"<FULL REASONING CHAIN — what was said, why it mattered, what it changed>"}'

## What to Extract
- Corrections: "Matt said X was wrong because Y" → the Y is the insight
- Questions that revealed gaps: "What agent should have noticed that?" → the question IS the gap
- Principles: "Know thyself" emerged from rediscovering existing code
- Connections: "Roles ARE primitives" connects pipeline to tick engine
- Philosophy: every architectural decision is a philosophical position

## Rules
- Don't duplicate existing claims — search first
- Be selective — 5-10 claims, not 50
- Capture reasoning, not just conclusions
- Reference specific moments: "when discussing the Observer, Matt asked..."
- Attention is finite — only assert what changes how the hive thinks
`, latestTranscript, apiKey, r.cfg.APIBase, r.cfg.SpaceSlug)

	result, err := op.Operate(ctx, decision.OperateTask{
		WorkDir:     r.cfg.HiveDir,
		Instruction: instruction,
	})
	if err != nil {
		log.Printf("[scribe] Operate error: %v", err)
		if r.cfg.OneShot {
			r.done = true
		}
		return
	}

	r.cost.Record(result.Usage)
	r.dailyBudget.Record(result.Usage.CostUSD)
	log.Printf("[scribe] done (cost=$%.4f)", result.Usage.CostUSD)

	if r.cfg.OneShot {
		r.done = true
	}
}

// findTranscripts returns .jsonl files in the given directory, sorted by name.
func findTranscripts(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var paths []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return paths
}
