package registry

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		{Name: "scan-only", AbsPath: "/path/scan-only", IssueScanOnly: true},
	}}
	m := reg.RepoMap()
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if m["a"] != "/path/a" {
		t.Errorf("a = %q", m["a"])
	}
}

func TestIssueScanOnlyReposAreExcludedFromWorkspaceAvailability(t *testing.T) {
	available := t.TempDir()
	scanOnly := t.TempDir()
	if err := os.WriteFile(filepath.Join(scanOnly, "CLAUDE.md"), []byte("# Private context"), 0o600); err != nil {
		t.Fatalf("write private CLAUDE.md: %v", err)
	}
	reg := &Registry{Repos: []Repo{
		{Name: "available", LocalPath: available, AbsPath: available},
		{Name: "scan-only", LocalPath: scanOnly, AbsPath: scanOnly, IssueScanOnly: true},
	}}
	if err := reg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	avail := reg.Available()
	if len(avail) != 1 || avail[0].Name != "available" {
		t.Fatalf("available repos = %#v, want only available", avail)
	}
	if _, ok := reg.RepoMap()["scan-only"]; ok {
		t.Fatal("issue-scan-only repo appeared in workspace repo map")
	}
	if _, ok := reg.ForPath(scanOnly); ok {
		t.Fatal("issue-scan-only repo matched workspace path")
	}
	repo, ok := reg.Get("scan-only")
	if !ok {
		t.Fatal("scan-only repo missing")
	}
	if repo.AbsPath != "" {
		t.Fatalf("issue-scan-only repo AbsPath = %q, want empty", repo.AbsPath)
	}
	if repo.ClaudeMD != "" {
		t.Fatalf("issue-scan-only repo loaded ClaudeMD: %q", repo.ClaudeMD)
	}
}

func TestEnsureClonedSkipsIssueScanOnlyRepos(t *testing.T) {
	marker := installGitRecorderForTest(t)

	reg := &Registry{Repos: []Repo{{
		Name:          "scan-only",
		URL:           "https://github.com/transpara-ai/private-scan-only",
		LocalPath:     filepath.Join(t.TempDir(), "scan-only"),
		IssueScanOnly: true,
	}}}

	if cloned := reg.EnsureCloned(); cloned != 0 {
		t.Fatalf("cloned = %d, want 0", cloned)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("git recorder marker exists after EnsureCloned skip: %v", err)
	}
}

func TestPullAllSkipsIssueScanOnlyRepos(t *testing.T) {
	marker := installGitRecorderForTest(t)
	reg := &Registry{Repos: []Repo{{
		Name:          "scan-only",
		AbsPath:       t.TempDir(),
		IssueScanOnly: true,
	}}}

	reg.PullAll()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("git recorder marker exists after PullAll skip: %v", err)
	}
}

func installGitRecorderForTest(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "git-called")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + strconv.Quote(marker) + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
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

func TestCanonicalReposJSONEntriesAreValid(t *testing.T) {
	reg, err := Load(filepath.Join("..", "..", "repos.json"))
	if err != nil {
		t.Fatalf("Load repos.json: %v", err)
	}
	if len(reg.Repos) == 0 {
		t.Fatal("repos.json has no repos")
	}

	knownLanguages := map[string]bool{
		"go":     true,
		"matlab": true,
	}
	seen := map[string]bool{}

	for _, repo := range reg.Repos {
		if strings.TrimSpace(repo.Name) == "" {
			t.Fatal("repo entry has empty name")
		}
		if seen[repo.Name] {
			t.Fatalf("duplicate repo name %q", repo.Name)
		}
		seen[repo.Name] = true

		if !strings.HasPrefix(repo.URL, "https://github.com/transpara-ai/") {
			t.Fatalf("%s URL = %q, want transpara-ai GitHub URL", repo.Name, repo.URL)
		}
		if strings.TrimSpace(repo.LocalPath) == "" {
			t.Fatalf("%s local_path is empty", repo.Name)
		}
		if !knownLanguages[repo.Language] {
			t.Fatalf("%s language = %q, want known language", repo.Name, repo.Language)
		}
		if strings.TrimSpace(repo.BuildCmd) == "" {
			t.Fatalf("%s build_cmd is empty", repo.Name)
		}
		if strings.TrimSpace(repo.TestCmd) == "" {
			t.Fatalf("%s test_cmd is empty", repo.Name)
		}
	}

	matlabClient, ok := reg.Get("matlab-client")
	if !ok {
		t.Fatal("matlab-client registry entry missing")
	}
	if matlabClient.Language != "matlab" {
		t.Fatalf("matlab-client language = %q, want matlab", matlabClient.Language)
	}
	if !matlabClient.IssueScanOnly {
		t.Fatal("matlab-client must remain issue_scan_only")
	}
	if matlabClient.BuildCmd != "true" {
		t.Fatalf("matlab-client build_cmd = %q, want true", matlabClient.BuildCmd)
	}
	if !strings.Contains(matlabClient.TestCmd, "runtests('tests')") {
		t.Fatalf("matlab-client test_cmd does not run tests/: %q", matlabClient.TestCmd)
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
