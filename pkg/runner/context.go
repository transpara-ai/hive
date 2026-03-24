package runner

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadSharedContext loads the institutional knowledge that every agent should have:
// CONTEXT.md (vision, architecture, invariants) + lessons from state.md.
// This is the civilization's memory, injected into every prompt.
func LoadSharedContext(hiveDir string) string {
	if hiveDir == "" {
		return ""
	}

	var parts []string

	// CONTEXT.md — shared vision, architecture, invariants, soul.
	if data, err := os.ReadFile(filepath.Join(hiveDir, "agents", "CONTEXT.md")); err == nil {
		parts = append(parts, string(data))
	}

	// Lessons from state.md — the civilization's hard-won knowledge.
	if data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "state.md")); err == nil {
		s := string(data)
		if idx := strings.Index(s, "## Lessons Learned"); idx >= 0 {
			lessons := s[idx:]
			// Find end of lessons section.
			if end := strings.Index(lessons[20:], "\n## "); end > 0 {
				lessons = lessons[:20+end]
			}
			parts = append(parts, lessons)
		}
	}

	// Council directive — current strategic direction.
	if data, err := os.ReadFile(filepath.Join(hiveDir, "loop", "state.md")); err == nil {
		s := string(data)
		marker := "## What the Scout Should Focus On Next"
		if idx := strings.Index(s, marker); idx >= 0 {
			section := s[idx:]
			if end := strings.Index(section[len(marker):], "\n## "); end > 0 {
				section = section[:len(marker)+end]
			}
			if len(section) > 2000 {
				section = section[:2000]
			}
			parts = append(parts, section)
		}
	}

	result := strings.Join(parts, "\n\n---\n\n")
	// Cap total context to avoid blowing up prompts.
	if len(result) > 8000 {
		result = result[:8000] + "\n... (truncated)"
	}
	return result
}
