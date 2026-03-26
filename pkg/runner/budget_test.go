package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDailyBudgetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "loop"), 0755)

	db := NewDailyBudget(dir)

	if got := db.Spent(); got != 0 {
		t.Errorf("Spent() = %f, want 0 (no file yet)", got)
	}
	if got := db.Remaining(10.0); got != 10.0 {
		t.Errorf("Remaining(10) = %f, want 10 (nothing spent)", got)
	}

	db.Record(3.0)
	db.Record(2.5)

	if got := db.Spent(); got != 5.5 {
		t.Errorf("Spent() = %f, want 5.5", got)
	}
	if got := db.Remaining(10.0); got != 4.5 {
		t.Errorf("Remaining(10) = %f, want 4.5", got)
	}

	// Over-ceiling clamps to 0.
	if got := db.Remaining(4.0); got != 0 {
		t.Errorf("Remaining(4) = %f, want 0 (over ceiling)", got)
	}
}

func TestDailyBudgetPersistence(t *testing.T) {
	dir := t.TempDir()

	// First instance records.
	a := NewDailyBudget(dir)
	a.Record(1.25)

	// Second instance (simulating a new process) reads the same file.
	b := NewDailyBudget(dir)
	if got := b.Spent(); got != 1.25 {
		t.Errorf("Spent() after restart = %f, want 1.25", got)
	}
}
