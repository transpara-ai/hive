package loop

import (
	"strings"

	"github.com/transpara-ai/hive/pkg/checkpoint"
)

type checkpointResponseContext struct {
	Intent  string
	Next    string
	Context string
}

func (l *Loop) captureBoundary(trigger checkpoint.BoundaryTrigger, response string) {
	if l.sink == nil {
		return
	}
	ctx := checkpointContextFromResponse(response)
	if contextual, ok := l.sink.(checkpoint.ContextSink); ok {
		contextual.OnBoundaryWithContext(trigger, l.currentSnapshot(), ctx.Intent, ctx.Next, ctx.Context)
		return
	}
	l.sink.OnBoundary(trigger, l.currentSnapshot())
}

func checkpointContextFromResponse(response string) checkpointResponseContext {
	cleaned := checkpointResponseBody(response)
	ctx := checkpointResponseContext{
		Context: truncateCheckpointText(cleaned, 1600),
	}

	for _, line := range strings.Split(cleaned, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "NEXT:") {
			ctx.Next = truncateCheckpointText(strings.TrimSpace(line[len("NEXT:"):]), 280)
			continue
		}
		if ctx.Intent == "" {
			ctx.Intent = truncateCheckpointText(line, 280)
		}
	}

	if ctx.Intent == "" {
		ctx.Intent = truncateCheckpointText(cleaned, 280)
	}
	return ctx
}

func checkpointResponseBody(response string) string {
	lines := strings.Split(response, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "/signal") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func truncateCheckpointText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
