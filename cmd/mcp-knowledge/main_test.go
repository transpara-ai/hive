package main

import (
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
