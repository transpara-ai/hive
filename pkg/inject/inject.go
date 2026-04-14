package inject

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/lovyou-ai/work"
)

// TitleFromFilename derives a human-readable title from a file path.
// It strips the directory, removes the last extension, replaces - _ and
// remaining dots with spaces, and title-cases each word.
func TitleFromFilename(path string) string {
	name := filepath.Base(path)
	// filepath.Base("") returns "." — treat as empty.
	if name == "." {
		return ""
	}
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	name = strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(name)
	words := strings.Fields(name)
	for i, w := range words {
		words[i] = titleWord(w)
	}
	return strings.Join(words, " ")
}

func titleWord(s string) string {
	if s == "" {
		return s
	}
	lower := strings.ToLower(s)
	runes := []rune(lower)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// Options holds the inputs for building an inject payload and event.
type Options struct {
	FileContent string
	SourceFile  string
	Title       string
	Description string
	Priority    string
	Actor       string
}

// Payload is the JSON body embedded in an Event.
type Payload struct {
	Description string `json:"description"`
	Body        string `json:"body,omitempty"`
	SourceFile  string `json:"source_file"`
	Priority    string `json:"priority"`
}

// BuildPayload constructs a Payload from Options.
// If Description is set it becomes the payload description and FileContent
// goes into Body; otherwise FileContent is used as the description directly.
// Priority defaults to "medium" when empty.
func BuildPayload(opts Options) Payload {
	priority := opts.Priority
	if priority == "" {
		priority = string(work.DefaultPriority)
	}
	p := Payload{
		SourceFile: opts.SourceFile,
		Priority:   priority,
	}
	if opts.Description != "" {
		p.Description = opts.Description
		p.Body = opts.FileContent
	} else {
		p.Description = opts.FileContent
	}
	return p
}

// Event is the top-level object posted to the hive webhook endpoint.
type Event struct {
	ID        string          `json:"id"`
	NodeTitle string          `json:"node_title"`
	Op        string          `json:"op"`
	Actor     string          `json:"actor"`
	ActorKind string          `json:"actor_kind"`
	Payload   json.RawMessage `json:"payload"`
}

// validPriorities is the set of accepted priority values.
var validPriorities = map[work.TaskPriority]bool{
	work.PriorityLow:      true,
	work.PriorityMedium:   true,
	work.PriorityHigh:     true,
	work.PriorityCritical: true,
}

// BuildEvent constructs an Event from Options.
// Title defaults to TitleFromFilename(opts.SourceFile) when empty.
// Actor is passed through as-is; callers are responsible for supplying a value.
// Returns an error if Priority is non-empty and not one of: low, medium, high, critical.
func BuildEvent(opts Options) (Event, error) {
	if opts.Priority != "" && !validPriorities[work.TaskPriority(opts.Priority)] {
		return Event{}, fmt.Errorf("invalid priority %q: must be low, medium, high, or critical", opts.Priority)
	}

	title := opts.Title
	if title == "" {
		title = TitleFromFilename(opts.SourceFile)
	}
	payload := BuildPayload(opts)
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("marshal payload: %w", err)
	}
	return Event{
		ID:        "idea-file-" + uuid.New().String(),
		NodeTitle: title,
		Op:        "intend",
		Actor:     opts.Actor,
		ActorKind: "human",
		Payload:   payloadJSON,
	}, nil
}

// Post marshals ev to JSON and HTTP POSTs it to http://<addr>/event.
// addr must be in host:port format.
// Returns an error if the request fails or the server returns non-200.
func Post(ev Event, addr string) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	url := fmt.Sprintf("http://%s/event", addr)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("post to %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("hive returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	return nil
}
