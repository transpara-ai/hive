package inject

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTitleFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"kanban-board-design.md", "Kanban Board Design"},
		{"my_great_idea.txt", "My Great Idea"},
		{"simple.md", "Simple"},
		{"multi.part.name.md", "Multi Part Name"},
		{"ALREADY-UPPER.md", "Already Upper"},
		{"../path/to/design-doc.md", "Design Doc"},
		{"no-extension", "No Extension"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := TitleFromFilename(tt.filename)
			if got != tt.want {
				t.Errorf("TitleFromFilename(%q) = %q; want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestBuildPayload(t *testing.T) {
	t.Run("file content only", func(t *testing.T) {
		opts := Options{
			FileContent: "# Design\n\nFull document here.",
			SourceFile:  "design.md",
		}
		p := BuildPayload(opts)
		if p.Description != opts.FileContent {
			t.Errorf("Description = %q; want file content", p.Description)
		}
		if p.Body != "" {
			t.Errorf("Body = %q; want empty", p.Body)
		}
		if p.SourceFile != "design.md" {
			t.Errorf("SourceFile = %q; want %q", p.SourceFile, "design.md")
		}
		if p.Priority != "medium" {
			t.Errorf("Priority = %q; want %q", p.Priority, "medium")
		}
	})

	t.Run("with description override", func(t *testing.T) {
		opts := Options{
			FileContent: "# Full doc\n\nLots of detail.",
			SourceFile:  "design.md",
			Description: "Short summary of the design",
		}
		p := BuildPayload(opts)
		if p.Description != "Short summary of the design" {
			t.Errorf("Description = %q; want the override", p.Description)
		}
		if p.Body != opts.FileContent {
			t.Errorf("Body = %q; want file content", p.Body)
		}
	})

	t.Run("with priority", func(t *testing.T) {
		opts := Options{
			FileContent: "content",
			SourceFile:  "x.md",
			Priority:    "high",
		}
		p := BuildPayload(opts)
		if p.Priority != "high" {
			t.Errorf("Priority = %q; want %q", p.Priority, "high")
		}
	})
}

func TestBuildEvent(t *testing.T) {
	opts := Options{
		FileContent: "# Design",
		SourceFile:  "design.md",
		Title:       "Custom Title",
		Actor:       "Alice",
	}
	ev, err := BuildEvent(opts)
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}

	if ev.NodeTitle != "Custom Title" {
		t.Errorf("NodeTitle = %q; want %q", ev.NodeTitle, "Custom Title")
	}
	if ev.Actor != "Alice" {
		t.Errorf("Actor = %q; want %q", ev.Actor, "Alice")
	}
	if ev.ActorKind != "human" {
		t.Errorf("ActorKind = %q; want %q", ev.ActorKind, "human")
	}
	if ev.Op != "intend" {
		t.Errorf("Op = %q; want %q", ev.Op, "intend")
	}
	if !strings.HasPrefix(ev.ID, "idea-file-") {
		t.Errorf("ID = %q; want prefix %q", ev.ID, "idea-file-")
	}

	var p Payload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.Description != "# Design" {
		t.Errorf("payload Description = %q; want file content", p.Description)
	}
}

func TestBuildEvent_EmptyActor(t *testing.T) {
	ev, err := BuildEvent(Options{FileContent: "x", SourceFile: "x.md"})
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}
	if ev.Actor != "" {
		t.Errorf("Actor = %q; want empty string when not set", ev.Actor)
	}
}

func TestBuildEvent_InvalidPriority(t *testing.T) {
	_, err := BuildEvent(Options{FileContent: "x", SourceFile: "x.md", Priority: "urgent"})
	if err == nil {
		t.Error("BuildEvent: expected error for invalid priority, got nil")
	}
	if !strings.Contains(err.Error(), "invalid priority") {
		t.Errorf("error = %q; want to contain %q", err.Error(), "invalid priority")
	}
}

func TestTitleFromFilename_Empty(t *testing.T) {
	// filepath.Base("") returns "." — document expected behaviour.
	got := TitleFromFilename("")
	want := ""
	if got != want {
		t.Errorf("TitleFromFilename(%q) = %q; want %q", "", got, want)
	}
}

func TestPost(t *testing.T) {
	var received Event
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received) //nolint:errcheck
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"received"}`)) //nolint:errcheck
	}))
	defer ts.Close()

	opts := Options{
		FileContent: "test content",
		SourceFile:  "test.md",
	}
	ev, err := BuildEvent(opts)
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}

	addr := strings.TrimPrefix(ts.URL, "http://")

	if err := Post(ev, addr); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if received.Op != "intend" {
		t.Errorf("received Op = %q; want %q", received.Op, "intend")
	}
	if received.NodeTitle != "Test" {
		t.Errorf("received NodeTitle = %q; want %q", received.NodeTitle, "Test")
	}
}

func TestPost_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	ev, err := BuildEvent(Options{FileContent: "x", SourceFile: "x.md"})
	if err != nil {
		t.Fatalf("BuildEvent: %v", err)
	}

	addr := strings.TrimPrefix(ts.URL, "http://")
	if err := Post(ev, addr); err == nil {
		t.Error("Post: expected error for HTTP 500, got nil")
	}
}
