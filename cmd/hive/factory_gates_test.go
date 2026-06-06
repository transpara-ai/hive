package main

import "testing"

// TestParseOrderGateSections extracts the three readiness gate bodies from an
// order spec's markdown sections.
func TestParseOrderGateSections(t *testing.T) {
	spec := "# Order: civic roles\n\nSome intent prose.\n\n" +
		"## Definition of Done\ncivic-roles.md exists and names all roles.\n\n" +
		"## Acceptance Criteria\nEach role has a one-line responsibility.\n\n" +
		"## Test Plan\nReviewer confirms; markdown lints clean.\n"
	dod, ac, tp := parseOrderGateSections(spec)
	if dod != "civic-roles.md exists and names all roles." {
		t.Fatalf("dod = %q", dod)
	}
	if ac != "Each role has a one-line responsibility." {
		t.Fatalf("ac = %q", ac)
	}
	if tp != "Reviewer confirms; markdown lints clean." {
		t.Fatalf("tp = %q", tp)
	}
}

// TestParseOrderGateSectionsMissingReturnsEmpty: a spec with no gate sections
// yields empty strings, so the planner attaches them later (Readiness enforces
// non-empty before the task is assignable).
func TestParseOrderGateSectionsMissingReturnsEmpty(t *testing.T) {
	dod, ac, tp := parseOrderGateSections("# Just intent\n\nNo gate sections here.\n")
	if dod != "" || ac != "" || tp != "" {
		t.Fatalf("expected all empty, got dod=%q ac=%q tp=%q", dod, ac, tp)
	}
}

// TestParseOrderGateSectionsMultilineUntilNextHeading: a section's body is every
// line up to the next heading, trimmed; an absent section stays empty.
func TestParseOrderGateSectionsMultilineUntilNextHeading(t *testing.T) {
	spec := "## Definition of Done\nline one\nline two\n\n## Test Plan\nrun it\n"
	dod, ac, tp := parseOrderGateSections(spec)
	if dod != "line one\nline two" {
		t.Fatalf("dod = %q", dod)
	}
	if ac != "" {
		t.Fatalf("ac = %q, want empty", ac)
	}
	if tp != "run it" {
		t.Fatalf("tp = %q", tp)
	}
}
