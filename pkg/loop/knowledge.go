package loop

import (
	"fmt"
	"strings"
	"time"

	"github.com/transpara-ai/hive/pkg/knowledge"
)

// enrichKnowledgeObservation appends distilled institutional knowledge to the
// observation for ANY agent (not role-gated). No-op when the knowledge store
// is nil or the loop hasn't stabilized yet.
func (l *Loop) enrichKnowledgeObservation(obs string) string {
	if l.config.KnowledgeStore == nil {
		return obs
	}
	if l.iteration < 10 {
		return obs
	}

	filter := knowledge.KnowledgeFilter{
		Role:          string(l.agent.Role()),
		MinConfidence: 0.3,
		MaxAge:        72 * time.Hour,
	}
	insights := l.config.KnowledgeStore.Query(filter, knowledge.MaxEnrichmentItems)
	if len(insights) == 0 {
		return obs
	}

	return obs + formatKnowledgeBlock(insights)
}

// formatKnowledgeBlock renders a set of insights as a structured text block
// for injection into an agent's observation context.
func formatKnowledgeBlock(insights []knowledge.KnowledgeInsight) string {
	var sb strings.Builder
	sb.WriteString("\n=== INSTITUTIONAL KNOWLEDGE ===\n")
	sb.WriteString("The following insights are distilled from the civilization's\n")
	sb.WriteString("accumulated experience. Consider them when making decisions.\n\n")

	for i, ins := range insights {
		if i >= knowledge.MaxEnrichmentItems {
			break
		}
		summary := ins.Summary
		if len(summary) > knowledge.MaxItemChars {
			summary = summary[:knowledge.MaxItemChars] + "..."
		}
		sb.WriteString(fmt.Sprintf("[%d] (domain: %s, confidence: %.2f, evidence: %d events)\n",
			i+1, ins.Domain, ins.Confidence, ins.EvidenceCount))
		sb.WriteString(fmt.Sprintf("    %s\n\n", summary))
	}

	sb.WriteString("===\n")

	result := sb.String()
	if len(result) > knowledge.MaxBlockChars {
		result = result[:knowledge.MaxBlockChars]
	}
	return result
}
