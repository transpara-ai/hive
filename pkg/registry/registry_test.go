package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAndResolve(t *testing.T) {
	dir := t.TempDir()

	// Create a mock repo with CLAUDE.md.
	repoDir := filepath.Join(dir, "myrepo")
	os.MkdirAll(repoDir, 0755)
	os.WriteFile(filepath.Join(repoDir, "CLAUDE.md"), []byte("# My Repo\nArchitecture docs."), 0644)

	// Write repos.json with a relative path.
	config := `{"repos": [{"name": "myrepo", "url": "https://github.com/test/myrepo", "local_path": "myrepo", "language": "go", "build_cmd": "go build ./...", "test_cmd": "go test ./...", "deploy_target": "npm"}]}`
	configPath := filepath.Join(dir, "repos.json")
	os.WriteFile(configPath, []byte(config), 0644)

	reg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reg.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(reg.Repos))
	}

	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	repo, ok := reg.Get("myrepo")
	if !ok {
		t.Fatal("Get(myrepo) not found")
	}
	if repo.AbsPath == "" {
		t.Fatal("AbsPath empty after Resolve")
	}
	if repo.ClaudeMD == "" {
		t.Fatal("ClaudeMD empty after Resolve")
	}
	if repo.ClaudeMD != "# My Repo\nArchitecture docs." {
		t.Errorf("ClaudeMD = %q", repo.ClaudeMD)
	}
}

func TestGet(t *testing.T) {
	reg := &Registry{Repos: []Repo{
		{Name: "a", AbsPath: "/a"},
		{Name: "b", AbsPath: "/b"},
	}}
	if _, ok := reg.Get("a"); !ok {
		t.Error("expected to find repo a")
	}
	if _, ok := reg.Get("c"); ok {
		t.Error("expected not to find repo c")
	}
}

func TestForPath(t *testing.T) {
	dir := t.TempDir()
	reg := &Registry{Repos: []Repo{
		{Name: "x", AbsPath: dir},
	}}
	repo, ok := reg.ForPath(dir)
	if !ok {
		t.Fatal("ForPath didn't match")
	}
	if repo.Name != "x" {
		t.Errorf("Name = %q, want x", repo.Name)
	}
}

func TestAvailable(t *testing.T) {
	existing := t.TempDir()
	reg := &Registry{Repos: []Repo{
		{Name: "exists", AbsPath: existing},
		{Name: "missing", AbsPath: "/nonexistent/path/xyz"},
	}}
	avail := reg.Available()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available, got %d", len(avail))
	}
	if avail[0].Name != "exists" {
		t.Errorf("Name = %q, want exists", avail[0].Name)
	}
}

func TestRepoMap(t *testing.T) {
	reg := &Registry{Repos: []Repo{
		{Name: "a", AbsPath: "/path/a"},
		{Name: "b", AbsPath: "/path/b"},
		{Name: "c"}, // no AbsPath — excluded
	}}
	m := reg.RepoMap()
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if m["a"] != "/path/a" {
		t.Errorf("a = %q", m["a"])
	}
}

func TestFromMap(t *testing.T) {
	dir := t.TempDir()
	reg := FromMap(map[string]string{"test": dir})
	if len(reg.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(reg.Repos))
	}
	if reg.Repos[0].Name != "test" {
		t.Errorf("Name = %q", reg.Repos[0].Name)
	}
	if reg.Repos[0].BuildCmd == "" {
		t.Error("BuildCmd empty")
	}
}

func TestSummary(t *testing.T) {
	reg := &Registry{Repos: []Repo{
		{Name: "site", AbsPath: t.TempDir(), Language: "go", URL: "https://github.com/test/site", BuildCmd: "go build ./...", TestCmd: "go test ./...", DeployTarget: "npm"},
	}}
	s := reg.Summary()
	if s == "" {
		t.Fatal("Summary empty")
	}
	if !contains(s, "site") || !contains(s, "npm") || !contains(s, "go build") {
		t.Errorf("Summary missing expected content: %s", s)
	}
}

func TestClaudeMDTruncation(t *testing.T) {
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "big")
	os.MkdirAll(repoDir, 0755)
	big := make([]byte, 5000)
	for i := range big {
		big[i] = 'x'
	}
	os.WriteFile(filepath.Join(repoDir, "CLAUDE.md"), big, 0644)

	reg := &Registry{
		Repos: []Repo{{Name: "big", LocalPath: "big"}},
		dir:   dir,
	}
	reg.Resolve()

	repo, _ := reg.Get("big")
	if len(repo.ClaudeMD) > 4100 {
		t.Errorf("ClaudeMD not truncated: len=%d", len(repo.ClaudeMD))
	}
	if !contains(repo.ClaudeMD, "truncated") {
		t.Error("missing truncation marker")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
