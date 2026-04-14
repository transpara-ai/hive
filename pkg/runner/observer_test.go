package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lovyou-ai/hive/pkg/api"
)

func TestBuildPart2Instruction(t *testing.T) {
	cases := []struct {
		name           string
		spaceSlug      string
		apiKey         string
		claimsSummary  string
		wantSkip       bool
		wantCurl       bool
		wantKeyInBody  bool
		wantSlugInURL  bool
		wantClaimsURL  bool
		wantGroundTruth bool
	}{
		{
			name:          "empty apiKey returns skip text, no curl",
			spaceSlug:     "hive",
			apiKey:        "",
			wantSkip:      true,
			wantCurl:      false,
			wantClaimsURL: false,
		},
		{
			name:          "set apiKey returns curl with key and slug embedded, including claims URL",
			spaceSlug:     "hive",
			apiKey:        "lv_testkey",
			wantSkip:      false,
			wantCurl:      true,
			wantKeyInBody: true,
			wantSlugInURL: true,
			wantClaimsURL: true,
		},
		{
			name:            "claimsSummary injected as ground truth when apiKey set",
			spaceSlug:       "hive",
			apiKey:          "lv_testkey",
			claimsSummary:   "65 claims exist. Titles: \"Lesson 1\"",
			wantSkip:        false,
			wantCurl:        true,
			wantGroundTruth: true,
			wantClaimsURL:   true,
		},
		{
			name:            "claimsSummary not shown when apiKey empty",
			spaceSlug:       "hive",
			apiKey:          "",
			claimsSummary:   "65 claims exist. Titles: \"Lesson 1\"",
			wantSkip:        true,
			wantCurl:        false,
			wantGroundTruth: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPart2Instruction(tc.spaceSlug, tc.apiKey, tc.claimsSummary, "https://lovyou.ai")

			if tc.wantSkip && !strings.Contains(got, "Skipped") {
				t.Errorf("expected skip message, got: %q", got)
			}
			if !tc.wantSkip && strings.Contains(got, "Skipped") {
				t.Errorf("unexpected skip message, got: %q", got)
			}
			if tc.wantCurl && !strings.Contains(got, "Authorization: Bearer") {
				t.Errorf("expected curl auth command, got: %q", got)
			}
			if !tc.wantCurl && strings.Contains(got, "Authorization: Bearer") {
				t.Errorf("unexpected curl auth command, got: %q", got)
			}
			if tc.wantKeyInBody && !strings.Contains(got, tc.apiKey) {
				t.Errorf("expected API key %q in output, got: %q", tc.apiKey, got)
			}
			if tc.wantSlugInURL && !strings.Contains(got, tc.spaceSlug) {
				t.Errorf("expected slug %q in output, got: %q", tc.spaceSlug, got)
			}
			if tc.wantClaimsURL && !strings.Contains(got, "knowledge?tab=claims") {
				t.Errorf("expected claims URL in output, got: %q", got)
			}
			if !tc.wantClaimsURL && strings.Contains(got, "knowledge?tab=claims") {
				t.Errorf("unexpected claims URL in output, got: %q", got)
			}
			if tc.wantGroundTruth && !strings.Contains(got, "Ground truth") {
				t.Errorf("expected ground truth section in output, got: %q", got)
			}
			if tc.wantGroundTruth && !strings.Contains(got, tc.claimsSummary) {
				t.Errorf("expected claimsSummary %q in output, got: %q", tc.claimsSummary, got)
			}
			if !tc.wantGroundTruth && strings.Contains(got, "Ground truth") {
				t.Errorf("unexpected ground truth section in output, got: %q", got)
			}
		})
	}
}

func TestBuildOutputInstruction(t *testing.T) {
	cases := []struct {
		name          string
		spaceSlug     string
		apiKey        string
		wantTextFmt   bool
		wantCurl      bool
		wantKeyInBody bool
		wantSlugInURL bool
	}{
		{
			name:        "empty apiKey returns text task format, no curl",
			spaceSlug:   "hive",
			apiKey:      "",
			wantTextFmt: true,
			wantCurl:    false,
		},
		{
			name:          "set apiKey returns curl with key and slug, no text format",
			spaceSlug:     "hive",
			apiKey:        "lv_testkey",
			wantTextFmt:   false,
			wantCurl:      true,
			wantKeyInBody: true,
			wantSlugInURL: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildOutputInstruction(tc.spaceSlug, tc.apiKey, "https://lovyou.ai")

			if tc.wantTextFmt && !strings.Contains(got, "TASK_TITLE:") {
				t.Errorf("expected text task format, got: %q", got)
			}
			if !tc.wantTextFmt && strings.Contains(got, "TASK_TITLE:") {
				t.Errorf("unexpected text task format, got: %q", got)
			}
			if tc.wantCurl && !strings.Contains(got, "Authorization: Bearer") {
				t.Errorf("expected curl auth command, got: %q", got)
			}
			if !tc.wantCurl && strings.Contains(got, "Authorization: Bearer") {
				t.Errorf("unexpected curl auth command, got: %q", got)
			}
			if tc.wantKeyInBody && !strings.Contains(got, tc.apiKey) {
				t.Errorf("expected API key %q in output, got: %q", tc.apiKey, got)
			}
			if tc.wantSlugInURL && !strings.Contains(got, tc.spaceSlug) {
				t.Errorf("expected slug %q in output, got: %q", tc.spaceSlug, got)
			}
		})
	}
}

func TestBuildPart2InstructionBoardAndClaims(t *testing.T) {
	// When apiKey is set, both the /board and /knowledge?tab=claims URLs must appear.
	got := buildPart2Instruction("hive", "lv_key", "", "https://lovyou.ai")

	if !strings.Contains(got, "/board") {
		t.Errorf("expected /board URL in part2 instruction, got: %q", got)
	}
	if !strings.Contains(got, "knowledge?tab=claims") {
		t.Errorf("expected knowledge?tab=claims URL in part2 instruction, got: %q", got)
	}
	if !strings.Contains(got, "limit=50") {
		t.Errorf("expected limit=50 on claims URL in part2 instruction, got: %q", got)
	}
	// There should be exactly 2 Authorization headers: one for board, one for claims.
	authCount := strings.Count(got, "Authorization: Bearer")
	if authCount != 2 {
		t.Errorf("expected 2 curl auth headers (board + claims), got %d in: %q", authCount, got)
	}
}

func TestParseObserverTasks(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []observerTask
	}{
		{
			name:  "empty input returns empty slice",
			input: "",
			want:  nil,
		},
		{
			name:  "no issue found text returns empty slice",
			input: "No issues found.",
			want:  nil,
		},
		{
			name: "single complete task",
			input: `TASK_TITLE: Fix the bug
TASK_PRIORITY: high
TASK_DESCRIPTION: The login button is broken`,
			want: []observerTask{
				{title: "Fix the bug", priority: "high", desc: "The login button is broken"},
			},
		},
		{
			name: "two tasks",
			input: `TASK_TITLE: First task
TASK_PRIORITY: urgent
TASK_DESCRIPTION: First description
TASK_TITLE: Second task
TASK_PRIORITY: low
TASK_DESCRIPTION: Second description`,
			want: []observerTask{
				{title: "First task", priority: "urgent", desc: "First description"},
				{title: "Second task", priority: "low", desc: "Second description"},
			},
		},
		{
			name: "invalid priority defaults to medium",
			input: `TASK_TITLE: Bad priority task
TASK_PRIORITY: critical
TASK_DESCRIPTION: Something urgent`,
			want: []observerTask{
				{title: "Bad priority task", priority: "medium", desc: "Something urgent"},
			},
		},
		{
			name: "missing priority defaults to medium",
			input: `TASK_TITLE: No priority task
TASK_DESCRIPTION: Missing priority`,
			want: []observerTask{
				{title: "No priority task", priority: "medium", desc: "Missing priority"},
			},
		},
		{
			name: "all valid priorities are accepted",
			input: `TASK_TITLE: Urgent task
TASK_PRIORITY: urgent
TASK_DESCRIPTION: u
TASK_TITLE: High task
TASK_PRIORITY: high
TASK_DESCRIPTION: h
TASK_TITLE: Medium task
TASK_PRIORITY: medium
TASK_DESCRIPTION: m
TASK_TITLE: Low task
TASK_PRIORITY: low
TASK_DESCRIPTION: l`,
			want: []observerTask{
				{title: "Urgent task", priority: "urgent", desc: "u"},
				{title: "High task", priority: "high", desc: "h"},
				{title: "Medium task", priority: "medium", desc: "m"},
				{title: "Low task", priority: "low", desc: "l"},
			},
		},
		{
			name: "title with surrounding whitespace is trimmed",
			input: `TASK_TITLE:   Padded title
TASK_PRIORITY:   high
TASK_DESCRIPTION:   Padded desc   `,
			want: []observerTask{
				{title: "Padded title", priority: "high", desc: "Padded desc"},
			},
		},
		{
			name: "task with title only, no other fields",
			input: `TASK_TITLE: Title only`,
			want: []observerTask{
				{title: "Title only", priority: "medium", desc: ""},
			},
		},
		{
			name: "unrecognised lines are ignored",
			input: `Some preamble text.
TASK_TITLE: Real task
Random middle line
TASK_PRIORITY: low
More noise
TASK_DESCRIPTION: The real description`,
			want: []observerTask{
				{title: "Real task", priority: "low", desc: "The real description"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseObserverTasks(tc.input)

			if len(got) != len(tc.want) {
				t.Fatalf("got %d tasks, want %d: %+v", len(got), len(tc.want), got)
			}
			for i, w := range tc.want {
				g := got[i]
				if g.title != w.title {
					t.Errorf("task[%d].title = %q, want %q", i, g.title, w.title)
				}
				if g.priority != w.priority {
					t.Errorf("task[%d].priority = %q, want %q", i, g.priority, w.priority)
				}
				if g.desc != w.desc {
					t.Errorf("task[%d].desc = %q, want %q", i, g.desc, w.desc)
				}
			}
		})
	}
}

func TestBuildClaimsSummary(t *testing.T) {
	cases := []struct {
		name   string
		claims []api.Node
		want   string
	}{
		{
			name:   "empty returns empty string",
			claims: nil,
			want:   "",
		},
		{
			name:   "single claim",
			claims: []api.Node{{Title: "Lesson 1"}},
			want:   `1 claims exist. Titles: "Lesson 1"`,
		},
		{
			name: "five claims shows all",
			claims: []api.Node{
				{Title: "A"}, {Title: "B"}, {Title: "C"}, {Title: "D"}, {Title: "E"},
			},
			want: `5 claims exist. Titles: "A", "B", "C", "D", "E"`,
		},
		{
			name: "six claims shows five and remainder count",
			claims: []api.Node{
				{Title: "A"}, {Title: "B"}, {Title: "C"}, {Title: "D"}, {Title: "E"}, {Title: "F"},
			},
			want: `6 claims exist (and 1 more). Titles: "A", "B", "C", "D", "E"`,
		},
		{
			name: "ten claims shows five and remainder count",
			claims: func() []api.Node {
				nodes := make([]api.Node, 10)
				for i := range nodes {
					nodes[i] = api.Node{Title: string(rune('A' + i))}
				}
				return nodes
			}(),
			want: `10 claims exist (and 5 more). Titles: "A", "B", "C", "D", "E"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildClaimsSummary(tc.claims)
			if got != tc.want {
				t.Errorf("buildClaimsSummary() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBuildOutputInstructionCategoryModel verifies the two-category anti-meta-task model
// added in iteration 369. Category A must direct inline action; the hard rule must forbid
// creating a task to close a task.
func TestBuildOutputInstructionCategoryModel(t *testing.T) {
	got := buildOutputInstruction("hive", "lv_testkey", "https://lovyou.ai")

	// Category A: must instruct inline action via op=complete and op=edit.
	if !strings.Contains(got, "Category A") {
		t.Errorf("expected Category A section in output, got: %q", got)
	}
	if !strings.Contains(got, `"op":"complete"`) {
		t.Errorf("expected op=complete curl example in Category A, got: %q", got)
	}
	if !strings.Contains(got, `"op":"edit"`) {
		t.Errorf("expected op=edit curl example in Category A, got: %q", got)
	}

	// Category B: must exist and limit to code-required tasks.
	if !strings.Contains(got, "Category B") {
		t.Errorf("expected Category B section in output, got: %q", got)
	}

	// Hard rule: must explicitly forbid the meta-task anti-pattern.
	if !strings.Contains(got, "Creating a task to close a task is always wrong") {
		t.Errorf("expected anti-meta-task rule in output, got: %q", got)
	}
}

// TestBuildOutputInstructionNoAntiPatternWhenNoKey verifies the fallback (no apiKey) path
// does not contain the category model — it's only relevant when direct API access is possible.
func TestBuildOutputInstructionNoAntiPatternWhenNoKey(t *testing.T) {
	got := buildOutputInstruction("hive", "", "https://lovyou.ai")

	if strings.Contains(got, "Category A") {
		t.Errorf("Category A should not appear in no-key output, got: %q", got)
	}
	if strings.Contains(got, "Category B") {
		t.Errorf("Category B should not appear in no-key output, got: %q", got)
	}
}

// TestBuildPart2InstructionMetaTaskItem verifies item 7 (meta-tasks) was added to the
// Part 2 audit checklist and instructs inline closure — not task creation.
func TestBuildPart2InstructionMetaTaskItem(t *testing.T) {
	got := buildPart2Instruction("hive", "lv_testkey", "", "https://lovyou.ai")

	// Item 7 must be present.
	if !strings.Contains(got, "Meta-tasks") {
		t.Errorf("expected Meta-tasks item in part2 checklist, got: %q", got)
	}

	// Must instruct to close inline with op=complete.
	if !strings.Contains(got, "op=complete") {
		t.Errorf("expected op=complete inline instruction for meta-tasks, got: %q", got)
	}

	// Must explicitly say not to create a new task.
	if !strings.Contains(got, "Do not create a new task for this") {
		t.Errorf("expected 'Do not create a new task for this' in meta-task instruction, got: %q", got)
	}

	// Board hygiene rule must mention meta-task pattern recognition.
	if !strings.Contains(got, "Board hygiene rule") {
		t.Errorf("expected board hygiene rule section, got: %q", got)
	}
}

// TestBuildPart2InstructionMetaTaskItemSkippedWhenNoKey verifies item 7 is irrelevant
// without an API key (the whole section is skipped).
func TestBuildPart2InstructionMetaTaskItemSkippedWhenNoKey(t *testing.T) {
	got := buildPart2Instruction("hive", "", "", "https://lovyou.ai")

	// Whole Part 2 is skipped — meta-task instructions shouldn't appear.
	if strings.Contains(got, "Meta-tasks") {
		t.Errorf("Meta-tasks item should not appear when Part 2 is skipped, got: %q", got)
	}
}

// TestParseObserverTasksCauseID verifies that the TASK_CAUSE field is parsed
// correctly. Nodes with a valid ID are threaded through; sentinel values ("none",
// "N/A", empty) are filtered out to avoid creating causality noise.
func TestParseObserverTasksCauseID(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		wantID string
	}{
		{
			name:   "valid node ID is preserved",
			input:  "TASK_TITLE: Fix\nTASK_PRIORITY: high\nTASK_DESCRIPTION: desc\nTASK_CAUSE: node-abc-123",
			wantID: "node-abc-123",
		},
		{
			name:   "TASK_CAUSE: none is filtered",
			input:  "TASK_TITLE: Fix\nTASK_PRIORITY: high\nTASK_DESCRIPTION: desc\nTASK_CAUSE: none",
			wantID: "",
		},
		{
			name:   "TASK_CAUSE: N/A is filtered",
			input:  "TASK_TITLE: Fix\nTASK_PRIORITY: high\nTASK_DESCRIPTION: desc\nTASK_CAUSE: N/A",
			wantID: "",
		},
		{
			name:   "TASK_CAUSE empty after trim is filtered",
			input:  "TASK_TITLE: Fix\nTASK_PRIORITY: high\nTASK_DESCRIPTION: desc\nTASK_CAUSE:    ",
			wantID: "",
		},
		{
			name:   "missing TASK_CAUSE leaves causeID empty",
			input:  "TASK_TITLE: Fix\nTASK_PRIORITY: high\nTASK_DESCRIPTION: desc",
			wantID: "",
		},
		{
			name:   "TASK_CAUSE with surrounding whitespace is trimmed",
			input:  "TASK_TITLE: Fix\nTASK_PRIORITY: high\nTASK_DESCRIPTION: desc\nTASK_CAUSE:   node-456   ",
			wantID: "node-456",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tasks := parseObserverTasks(tc.input)
			if len(tasks) != 1 {
				t.Fatalf("expected 1 task, got %d", len(tasks))
			}
			if tasks[0].causeID != tc.wantID {
				t.Errorf("causeID = %q, want %q", tasks[0].causeID, tc.wantID)
			}
		})
	}
}

// TestParseObserverTasksTwoCauseIDs verifies that each task independently captures
// its own TASK_CAUSE when multiple tasks are parsed from a single LLM response.
func TestParseObserverTasksTwoCauseIDs(t *testing.T) {
	input := `TASK_TITLE: Task one
TASK_PRIORITY: high
TASK_DESCRIPTION: First description
TASK_CAUSE: cause-node-1
TASK_TITLE: Task two
TASK_PRIORITY: medium
TASK_DESCRIPTION: Second description
TASK_CAUSE: cause-node-2`

	tasks := parseObserverTasks(input)
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].causeID != "cause-node-1" {
		t.Errorf("tasks[0].causeID = %q, want %q", tasks[0].causeID, "cause-node-1")
	}
	if tasks[1].causeID != "cause-node-2" {
		t.Errorf("tasks[1].causeID = %q, want %q", tasks[1].causeID, "cause-node-2")
	}
}

// TestBuildOutputInstructionCausesFieldPresent verifies that the curl template
// includes the "causes" field when an API key is set. Observer-created tasks
// must declare a cause to satisfy Invariant 2 (CAUSALITY).
func TestBuildOutputInstructionCausesFieldPresent(t *testing.T) {
	got := buildOutputInstruction("hive", "lv_testkey", "https://lovyou.ai")
	if !strings.Contains(got, `"causes"`) {
		t.Errorf("expected 'causes' field in curl template (Invariant 2: CAUSALITY), got: %q", got)
	}
}

// TestBuildOutputInstructionNoCausesWhenNoKey verifies the text-only fallback
// (no API key) does not contain the causes field — it's only meaningful when
// the Observer can make direct API calls.
func TestBuildOutputInstructionNoCausesWhenNoKey(t *testing.T) {
	got := buildOutputInstruction("hive", "", "https://lovyou.ai")
	if strings.Contains(got, `"causes"`) {
		t.Errorf("causes field should not appear in no-key output, got: %q", got)
	}
}

// TestRunObserverReason_FallbackCause verifies that when the LLM returns
// TASK_CAUSE: none (empty causeID after filtering), runObserverReason applies
// the fallbackCauseID so the CreateTask request still includes a cause.
// Invariant 2: CAUSALITY — every created node must declare its cause.
func TestRunObserverReason_FallbackCause(t *testing.T) {
	var createBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			if op, _ := m["op"].(string); op == "intend" {
				createBody = b
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"new-1","kind":"task","title":"t","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	// LLM returns a task with TASK_CAUSE: none — causeID is empty after parsing.
	provider := &mockProvider{response: "TASK_TITLE: Test gap\nTASK_PRIORITY: medium\nTASK_DESCRIPTION: some gap\nTASK_CAUSE: none"}

	r := &Runner{
		cfg: Config{
			Provider:  provider,
			APIClient: api.New(srv.URL, "test-key"),
			SpaceSlug: "hive",
		},
	}

	r.runObserverReason(context.Background(), "1 claims exist", "claim-fallback-123")

	if createBody == nil {
		t.Fatal("no CreateTask request captured — runObserverReason did not create a task")
	}

	var fields map[string]any
	if err := json.Unmarshal(createBody, &fields); err != nil {
		t.Fatalf("unmarshal CreateTask body: %v", err)
	}
	causeList, ok := fields["causes"].([]any)
	if !ok || len(causeList) == 0 {
		t.Errorf("CAUSALITY violated: CreateTask missing fallback cause when TASK_CAUSE:none, body=%s", createBody)
	}
	if len(causeList) > 0 && causeList[0] != "claim-fallback-123" {
		t.Errorf("causes[0] = %v, want %q", causeList[0], "claim-fallback-123")
	}
}

// TestRunObserverReason_FallbackCause_WhenFallbackEmpty verifies that when both
// the LLM returns TASK_CAUSE:none AND the fallbackCauseID is empty (no claims exist),
// runObserverReason still creates the task without panicking. The task is created
// with zero causes — this is the accepted behaviour when the graph is empty.
func TestRunObserverReason_FallbackCause_WhenFallbackEmpty(t *testing.T) {
	var created bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			if op, _ := m["op"].(string); op == "intend" {
				created = true
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"new-1","kind":"task","title":"t","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	provider := &mockProvider{response: "TASK_TITLE: Orphaned gap\nTASK_PRIORITY: low\nTASK_DESCRIPTION: no claims yet\nTASK_CAUSE: none"}

	r := &Runner{
		cfg: Config{
			Provider:  provider,
			APIClient: api.New(srv.URL, "test-key"),
			SpaceSlug: "hive",
		},
	}

	// fallbackCauseID="" — no claims exist in the graph yet.
	r.runObserverReason(context.Background(), "", "")

	if !created {
		t.Fatal("expected CreateTask to be called even when fallbackCauseID is empty")
	}
}

// TestRunObserverReason_OwnCauseTakesPrecedence verifies that when the LLM returns
// a valid TASK_CAUSE node ID, the fallbackCauseID is not used — the task's own
// cause takes precedence. The fallback must only apply when causeID is empty.
func TestRunObserverReason_OwnCauseTakesPrecedence(t *testing.T) {
	var createBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			if op, _ := m["op"].(string); op == "intend" {
				createBody = b
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"new-1","kind":"task","title":"t","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	// LLM provides a specific cause node ID — should NOT be replaced by the fallback.
	provider := &mockProvider{response: "TASK_TITLE: Specific gap\nTASK_PRIORITY: high\nTASK_DESCRIPTION: details\nTASK_CAUSE: specific-node-789"}

	r := &Runner{
		cfg: Config{
			Provider:  provider,
			APIClient: api.New(srv.URL, "test-key"),
			SpaceSlug: "hive",
		},
	}

	r.runObserverReason(context.Background(), "2 claims exist", "fallback-should-not-appear")

	if createBody == nil {
		t.Fatal("no CreateTask request captured")
	}

	var fields map[string]any
	if err := json.Unmarshal(createBody, &fields); err != nil {
		t.Fatalf("unmarshal CreateTask body: %v", err)
	}
	causeList, ok := fields["causes"].([]any)
	if !ok || len(causeList) == 0 {
		t.Fatalf("expected causes in CreateTask body, got: %s", createBody)
	}
	if causeList[0] != "specific-node-789" {
		t.Errorf("causes[0] = %v, want %q (fallback must not overwrite task's own causeID)", causeList[0], "specific-node-789")
	}
	for _, c := range causeList {
		if c == "fallback-should-not-appear" {
			t.Errorf("fallbackCauseID leaked into causes when task had its own causeID, body=%s", createBody)
		}
	}
}

// TestRunObserverReason_HallucinatedCauseIDGetsReplaced verifies that when the LLM
// returns a TASK_CAUSE node ID that does not exist on the graph (HTTP 404), the
// fallbackCauseID is used instead. This prevents dangling cause IDs (Lesson 170).
func TestRunObserverReason_HallucinatedCauseIDGetsReplaced(t *testing.T) {
	const ghostID = "ghost-node-does-not-exist"
	const fallback = "real-fallback-claim-id"

	var createBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Node existence check — return 404 for the ghost ID.
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, ghostID) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		// Task creation — capture the body.
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		if json.Unmarshal(b, &m) == nil {
			if op, _ := m["op"].(string); op == "intend" {
				createBody = b
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"op":"intend","node":{"id":"new-task","kind":"task","title":"t","created_at":"","updated_at":""}}`))
	}))
	defer srv.Close()

	// LLM returns a hallucinated cause node ID that won't exist on the graph.
	provider := &mockProvider{response: "TASK_TITLE: Ghost cause gap\nTASK_PRIORITY: high\nTASK_DESCRIPTION: details\nTASK_CAUSE: " + ghostID}

	r := &Runner{
		cfg: Config{
			Provider:  provider,
			APIClient: api.New(srv.URL, "test-key"),
			SpaceSlug: "hive",
		},
	}

	r.runObserverReason(context.Background(), "1 claims exist", fallback)

	if createBody == nil {
		t.Fatal("no CreateTask request captured")
	}
	var fields map[string]any
	if err := json.Unmarshal(createBody, &fields); err != nil {
		t.Fatalf("unmarshal CreateTask body: %v", err)
	}
	causeList, ok := fields["causes"].([]any)
	if !ok || len(causeList) == 0 {
		t.Errorf("CAUSALITY violated: CreateTask missing cause after ghost ID replaced, body=%s", createBody)
	}
	if len(causeList) > 0 && causeList[0] != fallback {
		t.Errorf("causes[0] = %v, want fallback %q — ghost ID must be replaced", causeList[0], fallback)
	}
	for _, c := range causeList {
		if c == ghostID {
			t.Errorf("ghost cause ID %q leaked into CreateTask — must be replaced by fallback", ghostID)
		}
	}
}

func TestBuildObserverInstruction(t *testing.T) {
	cases := []struct {
		name      string
		repoPath  string
		spaceSlug string
		apiKey    string
		wantParts []string
		// Part 2 skip or curl
		wantSkipInPart2 bool
		wantCurlInPart2 bool
		// Output section: text format or curl
		wantTextFmtInOutput bool
		wantCurlInOutput    bool
	}{
		{
			name:      "empty apiKey: skip in part2, text format in output",
			repoPath:  "/repo",
			spaceSlug: "hive",
			apiKey:    "",
			wantParts: []string{
				"You are the Observer",
				"Part 1: Product Audit",
				"Part 2: Graph Integrity Audit",
				"/repo",
				"hive",
			},
			wantSkipInPart2:     true,
			wantCurlInPart2:     false,
			wantTextFmtInOutput: true,
			wantCurlInOutput:    false,
		},
		{
			name:      "set apiKey: curl in part2 and output with key+slug",
			repoPath:  "/repo",
			spaceSlug: "hive",
			apiKey:    "lv_testkey",
			wantParts: []string{
				"You are the Observer",
				"Part 1: Product Audit",
				"Part 2: Graph Integrity Audit",
				"/repo",
				"hive",
				"lv_testkey",
			},
			wantSkipInPart2:     false,
			wantCurlInPart2:     true,
			wantTextFmtInOutput: false,
			wantCurlInOutput:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildObserverInstruction(tc.repoPath, tc.spaceSlug, tc.apiKey, "", "https://lovyou.ai")

			for _, part := range tc.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("expected %q in output, got: %q", part, got)
				}
			}

			// Part 2 integrity section appears once; check skip vs curl.
			if tc.wantSkipInPart2 && !strings.Contains(got, "Skipped") {
				t.Errorf("expected skip message in part 2, got: %q", got)
			}
			if !tc.wantSkipInPart2 && strings.Contains(got, "Skipped") {
				t.Errorf("unexpected skip message, got: %q", got)
			}

			// Count Authorization occurrences: part2 curl + output curl.
			authCount := strings.Count(got, "Authorization: Bearer")
			if tc.wantCurlInPart2 && tc.wantCurlInOutput && authCount < 2 {
				t.Errorf("expected at least 2 curl auth headers (part2 + output), got %d in: %q", authCount, got)
			}
			if !tc.wantCurlInPart2 && !tc.wantCurlInOutput && authCount > 0 {
				t.Errorf("expected no curl auth headers, got %d in: %q", authCount, got)
			}

			if tc.wantTextFmtInOutput && !strings.Contains(got, "TASK_TITLE:") {
				t.Errorf("expected TASK_TITLE text format in output section, got: %q", got)
			}
			if !tc.wantTextFmtInOutput && strings.Contains(got, "TASK_TITLE:") {
				t.Errorf("unexpected TASK_TITLE text format, got: %q", got)
			}
		})
	}
}
