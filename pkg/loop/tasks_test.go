package loop

import (
	"strings"
	"testing"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
)

func TestIsMetaTaskBody(t *testing.T) {
	tests := []struct {
		title       string
		description string
		want        bool
	}{
		// Positive — each meta-task pattern should be detected.
		{"op=complete task-123", "", true},
		{"Close task build feature", "", true},
		{"Mark done: implement auth", "", true},
		{"Close the following tasks", "", true},

		// Pattern in description rather than title.
		{"Follow-up work", "op=complete the previous task", true},
		{"Wrap up", "close task xyz after review", true},
		{"Update status", "mark done all items in backlog", true},
		{"Batch", "close the following: task-1, task-2", true},

		// Case-insensitive matching.
		{"OP=COMPLETE task", "", true},
		{"CLOSE TASK now", "", true},
		{"MARK DONE", "", true},
		{"CLOSE THE FOLLOWING", "", true},

		// Negative — genuine new task descriptions.
		{"Build authentication module", "implement OAuth2 flow", false},
		{"Fix bug in task parser", "the parser drops empty lines", false},
		{"Add tests for loop", "cover edge cases in signal parsing", false},
		{"Deploy to production", "run fly deploy after build", false},
		{"", "", false},
	}

	for _, tt := range tests {
		got := isMetaTaskBody(tt.title, tt.description)
		if got != tt.want {
			t.Errorf("isMetaTaskBody(%q, %q) = %v, want %v", tt.title, tt.description, got, tt.want)
		}
	}
}

func TestParseTaskCommandsMetaTaskNotFiltered(t *testing.T) {
	// parseTaskCommands itself does NOT filter meta-tasks — that happens in
	// execTaskCreate. Verify the command is still parsed so the guard can fire.
	response := `/task create {"title": "close task xyz", "description": "mark done"}`
	commands := parseTaskCommands(response)
	if len(commands) != 1 {
		t.Fatalf("got %d commands, want 1", len(commands))
	}
	if commands[0].Action != "create" {
		t.Errorf("action = %q, want %q", commands[0].Action, "create")
	}
}

func TestExecTaskCreateRejectsMetaTask(t *testing.T) {
	// execTaskCreate must return an error for meta-task payloads before
	// reaching TaskStore.Create. A nil TaskStore is safe because the guard
	// fires before any store call.
	cases := []struct {
		name    string
		payload string
	}{
		{"op=complete in title", `{"title": "op=complete task-123"}`},
		{"close task in title", `{"title": "Close Task xyz"}`},
		{"mark done in description", `{"title": "Wrap up", "description": "mark done all items"}`},
		{"close the following in description", `{"title": "Batch", "description": "close the following: t1, t2"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := execTaskCreate([]byte(tc.payload), nil, types.ActorID{}, nil, types.ConversationID{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "meta-task rejected") {
				t.Errorf("error %q does not contain \"meta-task rejected\"", err.Error())
			}
		})
	}
}


func TestIsMetaTaskBodyTitleDescriptionJoin(t *testing.T) {
	// The join is title + " " + description — a pattern can span the boundary.
	// This tests that documented behaviour rather than treating it as a surprise.
	// "close the" in title + "following tasks" in description → matched.
	got := isMetaTaskBody("close the", "following tasks")
	if !got {
		t.Error("expected match when pattern spans title/description join boundary, got false")
	}
	// Ensure it doesn't false-positive on unrelated fragments.
	got = isMetaTaskBody("close the", "release branch")
	if got {
		t.Error("expected no match for unrelated fragments across boundary, got true")
	}
}
