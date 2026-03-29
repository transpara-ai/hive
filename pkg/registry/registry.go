// Package registry provides a queryable repo registry for the hive pipeline.
// Agents discover repos, their build/test commands, and CLAUDE.md context
// through the registry instead of relying on CLI flags.
package registry

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo describes a single git repository the hive can work on.
type Repo struct {
	Name         string `json:"name"`          // short name: "site", "hive"
	URL          string `json:"url"`           // clone URL
	LocalPath    string `json:"local_path"`    // absolute or relative to repos.json dir
	Language     string `json:"language"`      // "go", "typescript", etc.
	BuildCmd     string `json:"build_cmd"`     // e.g. "go build -buildvcs=false ./..."
	TestCmd      string `json:"test_cmd"`      // e.g. "go test ./..."
	DeployTarget string `json:"deploy_target"` // "fly.io", "npm", "" (none)

	// Resolved at load time, not persisted.
	AbsPath  string `json:"-"` // absolute path after Resolve()
	ClaudeMD string `json:"-"` // CLAUDE.md contents, loaded by Resolve()
}

// Registry holds all known repos.
type Registry struct {
	Repos []Repo `json:"repos"`
	dir   string // directory repos.json was loaded from (for relative path resolution)
}

// Load reads a registry from a JSON file.
func Load(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse registry: %w", err)
	}
	r.dir = filepath.Dir(path)
	return &r, nil
}

// FromMap creates a registry from a name→path map (backward compat with --repos flag).
func FromMap(repoMap map[string]string) *Registry {
	r := &Registry{}
	for name, path := range repoMap {
		abs, _ := filepath.Abs(path)
		r.Repos = append(r.Repos, Repo{
			Name:      name,
			LocalPath: path,
			AbsPath:   abs,
			Language:  "go",
			BuildCmd:  "go.exe build -buildvcs=false ./...",
			TestCmd:   "go.exe test -buildvcs=false ./...",
		})
	}
	return r
}

// Resolve resolves relative paths to absolute and loads CLAUDE.md from each repo.
func (r *Registry) Resolve() error {
	for i := range r.Repos {
		repo := &r.Repos[i]

		// Resolve path relative to the registry file's directory.
		p := repo.LocalPath
		if !filepath.IsAbs(p) && r.dir != "" {
			p = filepath.Join(r.dir, p)
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue // skip unresolvable
		}
		repo.AbsPath = abs

		// Load CLAUDE.md if the repo directory exists.
		claudePath := filepath.Join(abs, "CLAUDE.md")
		if data, err := os.ReadFile(claudePath); err == nil {
			s := string(data)
			if len(s) > 4000 {
				s = s[:4000] + "\n... (truncated)"
			}
			repo.ClaudeMD = s
		}
	}
	return nil
}

// Get returns a repo by name.
func (r *Registry) Get(name string) (*Repo, bool) {
	for i := range r.Repos {
		if r.Repos[i].Name == name {
			return &r.Repos[i], true
		}
	}
	return nil, false
}

// ForPath returns the repo whose AbsPath matches the given directory.
func (r *Registry) ForPath(dir string) (*Repo, bool) {
	abs, _ := filepath.Abs(dir)
	for i := range r.Repos {
		if r.Repos[i].AbsPath == abs {
			return &r.Repos[i], true
		}
	}
	return nil, false
}

// Available returns repos that have a valid local path on disk.
func (r *Registry) Available() []Repo {
	var out []Repo
	for _, repo := range r.Repos {
		if repo.AbsPath != "" {
			if _, err := os.Stat(repo.AbsPath); err == nil {
				out = append(out, repo)
			}
		}
	}
	return out
}

// RepoMap returns name→absolute-path for backward compatibility with --repos.
func (r *Registry) RepoMap() map[string]string {
	m := make(map[string]string, len(r.Repos))
	for _, repo := range r.Repos {
		if repo.AbsPath != "" {
			m[repo.Name] = repo.AbsPath
		}
	}
	return m
}

// Summary returns a human-readable list of available repos for prompt injection.
func (r *Registry) Summary() string {
	avail := r.Available()
	if len(avail) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Available Repositories\n\n")
	for _, repo := range avail {
		b.WriteString(fmt.Sprintf("- **%s** (%s)", repo.Name, repo.Language))
		if repo.URL != "" {
			b.WriteString(fmt.Sprintf(" — %s", repo.URL))
		}
		if repo.DeployTarget != "" {
			b.WriteString(fmt.Sprintf(" [deploys to %s]", repo.DeployTarget))
		}
		b.WriteString("\n")
		if repo.BuildCmd != "" {
			b.WriteString(fmt.Sprintf("  Build: `%s`\n", repo.BuildCmd))
		}
		if repo.TestCmd != "" {
			b.WriteString(fmt.Sprintf("  Test: `%s`\n", repo.TestCmd))
		}
	}
	return b.String()
}

// EnsureCloned clones any repos that have a URL but no local directory.
// Uses --depth 1 for speed. After cloning, updates AbsPath and reloads CLAUDE.md.
func (r *Registry) EnsureCloned() int {
	cloned := 0
	for i := range r.Repos {
		repo := &r.Repos[i]
		if repo.URL == "" {
			continue
		}
		if repo.AbsPath != "" {
			if _, err := os.Stat(repo.AbsPath); err == nil {
				continue // already exists
			}
		}
		// Clone into the directory the registry expects.
		target := repo.AbsPath
		if target == "" {
			// Default: clone next to repos.json dir.
			target = filepath.Join(r.dir, repo.Name)
		}
		log.Printf("[registry] cloning %s → %s", repo.URL, target)
		cmd := exec.Command("git", "clone", "--depth", "1", repo.URL, target)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[registry] clone %s failed: %v\n%s", repo.Name, err, out)
			continue
		}
		repo.AbsPath = target
		// Load CLAUDE.md from the freshly cloned repo.
		claudePath := filepath.Join(target, "CLAUDE.md")
		if data, err := os.ReadFile(claudePath); err == nil {
			s := string(data)
			if len(s) > 4000 {
				s = s[:4000] + "\n... (truncated)"
			}
			repo.ClaudeMD = s
		}
		cloned++
		log.Printf("[registry] cloned %s", repo.Name)
	}
	return cloned
}

// PullAll runs git pull --ff-only on all repos that have a local directory.
// Errors are logged but don't stop processing — a stale repo is better than
// no repo.
func (r *Registry) PullAll() {
	for _, repo := range r.Repos {
		if repo.AbsPath == "" {
			continue
		}
		if _, err := os.Stat(repo.AbsPath); err != nil {
			continue
		}
		cmd := exec.Command("git", "pull", "--ff-only")
		cmd.Dir = repo.AbsPath
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[registry] pull %s: %v\n%s", repo.Name, err, out)
		}
	}
}
