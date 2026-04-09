package checkpoint_test

import (
	"testing"
	"time"

	"github.com/lovyou-ai/hive/pkg/checkpoint"
)

// TestStubThoughtStore_CaptureAndSearch captures a thought and verifies it is
// returned when searched with a matching query within maxAge.
func TestStubThoughtStore_CaptureAndSearch(t *testing.T) {
	store := checkpoint.NewStubThoughtStore()

	if err := store.Capture("implementer completed task-42: add error handling"); err != nil {
		t.Fatalf("Capture returned error: %v", err)
	}

	results, err := store.SearchRecent("task-42", 1*time.Hour)
	if err != nil {
		t.Fatalf("SearchRecent returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchRecent: got %d results, want 1", len(results))
	}
	if results[0].Content != "implementer completed task-42: add error handling" {
		t.Errorf("unexpected content: %q", results[0].Content)
	}
	if results[0].CapturedAt.IsZero() {
		t.Error("CapturedAt should not be zero")
	}
}

// TestStubThoughtStore_MaxAgeFilter inserts one thought 3 hours ago and one
// just now, then searches with a 1-hour maxAge — only the recent one should appear.
func TestStubThoughtStore_MaxAgeFilter(t *testing.T) {
	store := checkpoint.NewStubThoughtStore()

	old := checkpoint.Thought{
		Content:    "old thought from 3 hours ago",
		CapturedAt: time.Now().Add(-3 * time.Hour),
	}
	recent := checkpoint.Thought{
		Content:    "recent thought just now",
		CapturedAt: time.Now(),
	}
	store.Thoughts = append(store.Thoughts, old, recent)

	results, err := store.SearchRecent("thought", 1*time.Hour)
	if err != nil {
		t.Fatalf("SearchRecent returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchRecent: got %d results, want 1 (recent only)", len(results))
	}
	if results[0].Content != "recent thought just now" {
		t.Errorf("unexpected content: %q", results[0].Content)
	}
}

// TestStubThoughtStore_EmptyResults verifies that searching an empty store
// returns an empty (non-nil if we want, but at minimum len==0) slice with no error.
func TestStubThoughtStore_EmptyResults(t *testing.T) {
	store := checkpoint.NewStubThoughtStore()

	results, err := store.SearchRecent("anything", 1*time.Hour)
	if err != nil {
		t.Fatalf("SearchRecent returned error on empty store: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("SearchRecent: got %d results, want 0", len(results))
	}
}
