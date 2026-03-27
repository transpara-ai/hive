package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestServer creates a knowledgeServer pointing at a temp workspace with a
// minimal directory structure so tests don't depend on the real filesystem.
func newTestServer(t *testing.T) (*knowledgeServer, string) {
	t.Helper()
	dir := t.TempDir()

	hiveDir := filepath.Join(dir, "hive")
	loopDir := filepath.Join(hiveDir, "loop")
	agentsDir := filepath.Join(hiveDir, "agents")
	docsDir := filepath.Join(hiveDir, "docs")
	siteDir := filepath.Join(dir, "site")

	for _, d := range []string{loopDir, agentsDir, docsDir, siteDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	s := &knowledgeServer{
		hiveDir:   hiveDir,
		siteDir:   siteDir,
		workspace: dir,
	}
	s.buildTree()
	return s, loopDir
}

// TestBuildHiveLoopIncludesClaimsWhenPresent verifies that claims.md is
// included in the loop knowledge tree when the file exists on disk.
func TestBuildHiveLoopIncludesClaimsWhenPresent(t *testing.T) {
	s, loopDir := newTestServer(t)

	claimsPath := filepath.Join(loopDir, "claims.md")
	if err := os.WriteFile(claimsPath, []byte("# Knowledge Claims\n\n## Absence is invisible to traversal\n\nSome body.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Rebuild tree after writing the file.
	s.buildTree()

	node := s.findTopic("loop/claims")
	if node == nil {
		t.Fatal("loop/claims topic not found in knowledge tree — claims.md was not indexed")
	}
	if node.Path != claimsPath {
		t.Errorf("path = %q, want %q", node.Path, claimsPath)
	}
}

// TestBuildHiveLoopOmitsClaimsWhenAbsent verifies that claims.md does NOT
// appear in the tree when the file has not yet been synced.
func TestBuildHiveLoopOmitsClaimsWhenAbsent(t *testing.T) {
	s, _ := newTestServer(t)
	// claims.md was not written — tree should not include it.
	node := s.findTopic("loop/claims")
	if node != nil {
		t.Error("loop/claims should not appear when claims.md does not exist")
	}
}

// TestHandleSearchFindsClaims verifies that knowledge_search returns results
// from claims.md content, bridging graph store to MCP search.
func TestHandleSearchFindsClaims(t *testing.T) {
	s, loopDir := newTestServer(t)

	claimsContent := "# Knowledge Claims\n\n## Absence is invisible to traversal\n\n**State:** claimed\n\nThe Scout traverses what exists. Tests don't exist, so the Scout never encounters them.\n\n---\n\n## Ship what you build\n\n**State:** verified\n\nEvery build iteration should deploy.\n\n---\n"
	if err := os.WriteFile(filepath.Join(loopDir, "claims.md"), []byte(claimsContent), 0644); err != nil {
		t.Fatal(err)
	}
	s.buildTree()

	result := s.handleSearch(map[string]any{"query": "absence"})
	if strings.Contains(result, "No results") {
		t.Errorf("knowledge_search for 'absence' returned no results; claims.md not indexed\nresult: %s", result)
	}
	if !strings.Contains(result, "claims") {
		t.Errorf("result should reference claims.md item, got: %s", result)
	}
}

// TestHandleTopicsReturnsLoopChildren verifies that handleTopics("loop") returns
// the children of the loop category, including claims.md when it exists.
func TestHandleTopicsReturnsLoopChildren(t *testing.T) {
	s, loopDir := newTestServer(t)

	// Write claims.md so it's indexed.
	if err := os.WriteFile(filepath.Join(loopDir, "claims.md"), []byte("# Knowledge Claims\n\n## Foo\n\nBar.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Write state.md so there's more than one child.
	if err := os.WriteFile(filepath.Join(loopDir, "state.md"), []byte("# State\n\nIteration 10\n"), 0644); err != nil {
		t.Fatal(err)
	}
	s.buildTree()

	result := s.handleTopics(map[string]any{"parent": "loop"})
	if result == "(no children)" {
		t.Fatal("expected children for loop, got none")
	}
	if !strings.Contains(result, "claims.md") {
		t.Errorf("claims.md not listed in loop children\ngot: %s", result)
	}
	if !strings.Contains(result, "state.md") {
		t.Errorf("state.md not listed in loop children\ngot: %s", result)
	}
}

// TestHandleGetClaims verifies that knowledge.get returns the full content of
// the claims file when retrieved by the loop/claims topic ID.
func TestHandleGetClaims(t *testing.T) {
	s, loopDir := newTestServer(t)

	content := "# Knowledge Claims\n\n## Ship what you build\n\nEvery build iteration should deploy.\n"
	if err := os.WriteFile(filepath.Join(loopDir, "claims.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	s.buildTree()

	result := s.handleGet(map[string]any{"id": "loop/claims"})
	if !strings.Contains(result, "Ship what you build") {
		t.Errorf("knowledge.get(loop/claims) missing expected content\ngot: %s", result)
	}
}

// TestHandleSearchFindsDeepClaims verifies that knowledge_search finds claims
// located beyond the 4000-char file-content window. This is the core bug:
// claims.md is 72KB but search previously truncated at 4000 chars, making
// Lesson 109+ invisible.
func TestHandleSearchFindsDeepClaims(t *testing.T) {
	s, loopDir := newTestServer(t)

	// Build a file where the target claim is well past 4000 chars.
	var b strings.Builder
	b.WriteString("# Knowledge Claims\n\n")
	for i := 1; i <= 60; i++ {
		b.WriteString(fmt.Sprintf("## Lesson %d: Filler lesson for padding\n\nFiller body to push content window past 4000 characters.\n\n---\n\n", i))
	}
	// This claim is beyond 4000 chars — previously invisible to search.
	b.WriteString("## Lesson 109: Infrastructure iterations must declare themselves\n\nThis deep claim must be findable.\n\n---\n\n")

	if err := os.WriteFile(filepath.Join(loopDir, "claims.md"), []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	s.buildTree()

	result := s.handleSearch(map[string]any{"query": "lesson 109"})
	if strings.Contains(result, "No results") {
		t.Errorf("knowledge_search for 'lesson 109' returned no results; deep claims not indexed\ngot: %s", result)
	}
	if !strings.Contains(result, "loop/claims/") {
		t.Errorf("result should reference an individual claim ID, got: %s", result)
	}
}

// TestHandleGetIndividualClaim verifies that an individual claim can be
// retrieved by its loop/claims/<slug> ID.
func TestHandleGetIndividualClaim(t *testing.T) {
	s, loopDir := newTestServer(t)

	content := "# Knowledge Claims\n\n## CAUSALITY invariant: every event must declare causes\n\nEvery node posted must include a causes array.\n\n---\n\n"
	if err := os.WriteFile(filepath.Join(loopDir, "claims.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	s.buildTree()

	// Find the claim's ID via search first.
	searchResult := s.handleSearch(map[string]any{"query": "causality"})
	if strings.Contains(searchResult, "No results") {
		t.Fatalf("search for 'causality' found nothing\ngot: %s", searchResult)
	}

	// Extract an ID from the result and fetch it directly.
	node := s.findTopic("loop/claims/causality-invariant-every-event-must-declare-causes")
	if node == nil {
		t.Fatal("individual claim topic not found by expected slug ID")
	}
	result := s.handleGet(map[string]any{"id": node.ID})
	if !strings.Contains(result, "causes array") {
		t.Errorf("handleGet(individual claim) missing body content\ngot: %s", result)
	}
}

// TestParseClaimsDuplicateTitles verifies that duplicate ## headings in
// claims.md produce unique slug IDs (-2, -3 suffixes) rather than colliding.
// The real claims.md has three distinct "Lesson 109" entries.
func TestParseClaimsDuplicateTitles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claims.md")
	content := "# Claims\n\n## Lesson 109: some rule\n\nBody A.\n\n---\n\n## Lesson 109: some rule\n\nBody B.\n\n---\n\n## Lesson 109: some rule\n\nBody C.\n\n---\n\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	claims := parseClaims(path)
	if len(claims) != 3 {
		t.Fatalf("expected 3 claims, got %d", len(claims))
	}

	ids := map[string]bool{}
	for _, c := range claims {
		if ids[c.ID] {
			t.Errorf("duplicate claim ID: %s", c.ID)
		}
		ids[c.ID] = true
	}

	// First occurrence gets the base slug; subsequent get -2, -3.
	base := "loop/claims/lesson-109-some-rule"
	if claims[0].ID != base {
		t.Errorf("first claim ID = %q, want %q", claims[0].ID, base)
	}
	if claims[1].ID != base+"-2" {
		t.Errorf("second claim ID = %q, want %q", claims[1].ID, base+"-2")
	}
	if claims[2].ID != base+"-3" {
		t.Errorf("third claim ID = %q, want %q", claims[2].ID, base+"-3")
	}
}

// TestClaimSlugTruncation verifies that claimSlug truncates at 60 chars and
// does not leave a trailing hyphen after truncation.
func TestClaimSlugTruncation(t *testing.T) {
	// 70-char title — slug must be ≤ 60 chars with no trailing hyphen.
	title := "This is a very long claim title that definitely exceeds sixty characters limit"
	slug := claimSlug(title)
	if len(slug) > 60 {
		t.Errorf("slug len = %d, want ≤ 60; slug = %q", len(slug), slug)
	}
	if strings.HasSuffix(slug, "-") {
		t.Errorf("slug has trailing hyphen: %q", slug)
	}
}

// TestClaimSlugSpecialChars verifies that colons, parens, and other
// non-alphanumeric characters are collapsed to single hyphens.
func TestClaimSlugSpecialChars(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"CAUSALITY: events must declare causes", "causality-events-must-declare-causes"},
		{"Rule (v2.0) — enforce it", "rule-v2-0-enforce-it"},
		{"---leading hyphens---", "leading-hyphens"},
	}
	for _, tc := range cases {
		got := claimSlug(tc.title)
		if got != tc.want {
			t.Errorf("claimSlug(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

// TestClaimSummaryLongLine verifies that a body line exceeding 120 chars is
// truncated with "..." rather than returned in full.
func TestClaimSummaryLongLine(t *testing.T) {
	long := strings.Repeat("x", 130)
	got := claimSummary(long)
	if len(got) > 123 { // 120 + "..."
		t.Errorf("summary not truncated: len=%d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncated summary should end with '...', got: %q", got)
	}
}

// TestClaimSummaryAllMetadata verifies that a body consisting entirely of
// metadata lines returns an empty string rather than metadata noise.
func TestClaimSummaryAllMetadata(t *testing.T) {
	body := "**State:** claimed | **Author:** hive\n---\n\n**State:** verified\n"
	got := claimSummary(body)
	if got != "" {
		t.Errorf("all-metadata body should return empty summary, got: %q", got)
	}
}

// TestParseClaimsEmptyFile verifies that parseClaims returns nil (not a
// panic) when given an empty file.
func TestParseClaimsEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claims.md")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	claims := parseClaims(path)
	if len(claims) != 0 {
		t.Errorf("expected no claims from empty file, got %d", len(claims))
	}
}

// TestParseClaimsNoSections verifies that a file with no ## headings
// (only a # title or body text) produces no claim topics.
func TestParseClaimsNoSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claims.md")
	if err := os.WriteFile(path, []byte("# Knowledge Claims\n\nNo sections here.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	claims := parseClaims(path)
	if len(claims) != 0 {
		t.Errorf("expected no claims when file has no ## sections, got %d", len(claims))
	}
}

// TestHandleSearchResultCap verifies that handleSearch returns at most 10
// results even when more than 10 claims match the query.
func TestHandleSearchResultCap(t *testing.T) {
	s, loopDir := newTestServer(t)

	var b strings.Builder
	b.WriteString("# Claims\n\n")
	for i := 1; i <= 15; i++ {
		b.WriteString(fmt.Sprintf("## Matching lesson %d\n\nThis claim contains the word searchword.\n\n---\n\n", i))
	}
	if err := os.WriteFile(filepath.Join(loopDir, "claims.md"), []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	s.buildTree()

	result := s.handleSearch(map[string]any{"query": "searchword"})
	count := strings.Count(result, "loop/claims/")
	if count > 10 {
		t.Errorf("handleSearch returned %d results, want ≤ 10", count)
	}
	if count == 0 {
		t.Errorf("handleSearch returned no results for 'searchword'")
	}
}

// TestHandleSearchEmptyQuery verifies that an empty query returns an error
// string rather than panicking or returning unrelated results.
func TestHandleSearchEmptyQuery(t *testing.T) {
	s, _ := newTestServer(t)
	result := s.handleSearch(map[string]any{"query": ""})
	if !strings.Contains(result, "Error") && !strings.Contains(result, "required") {
		t.Errorf("empty query should return an error, got: %q", result)
	}
}

// TestHandleGetEmptyID verifies that an empty id returns an error string.
func TestHandleGetEmptyID(t *testing.T) {
	s, _ := newTestServer(t)
	result := s.handleGet(map[string]any{"id": ""})
	if !strings.Contains(result, "Error") && !strings.Contains(result, "required") {
		t.Errorf("empty id should return an error, got: %q", result)
	}
}

// TestClaimChildrenVisibleInTopics verifies that individual claim nodes
// (loop/claims/<slug>) appear as children when listing the loop/claims topic.
func TestClaimChildrenVisibleInTopics(t *testing.T) {
	s, loopDir := newTestServer(t)

	content := "# Claims\n\n## Alpha claim\n\nBody alpha.\n\n---\n\n## Beta claim\n\nBody beta.\n\n---\n\n"
	if err := os.WriteFile(filepath.Join(loopDir, "claims.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	s.buildTree()

	result := s.handleTopics(map[string]any{"parent": "loop/claims"})
	if result == "(no children)" {
		t.Fatal("expected claim children under loop/claims, got none")
	}
	if !strings.Contains(result, "Alpha claim") {
		t.Errorf("Alpha claim not listed in loop/claims children\ngot: %s", result)
	}
	if !strings.Contains(result, "Beta claim") {
		t.Errorf("Beta claim not listed in loop/claims children\ngot: %s", result)
	}
}
