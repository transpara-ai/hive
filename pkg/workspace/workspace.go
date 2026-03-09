// Package workspace manages the file system for generated products.
// Each product gets its own git repo under the workspace root.
package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Workspace manages directories and files for hive-generated products.
type Workspace struct {
	root string // Root directory for all product repos
	org  string // GitHub org for created repos (e.g., "lovyou-ai")
}

// New creates a workspace rooted at the given directory.
func New(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}

	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, fmt.Errorf("create root: %w", err)
	}

	return &Workspace{root: abs, org: "lovyou-ai"}, nil
}

// Root returns the workspace root directory.
func (w *Workspace) Root() string {
	return w.root
}

// SetOrg sets the GitHub org for repo creation.
func (w *Workspace) SetOrg(org string) {
	w.org = org
}

// Product represents a product's working directory backed by a git repo.
type Product struct {
	Name string
	Dir  string
	Repo string // GitHub repo (e.g., "lovyou-ai/my-product")
}

// InitProduct creates a new product with its own git repo.
// If the GitHub repo doesn't exist, it creates one.
func (w *Workspace) InitProduct(name string) (*Product, error) {
	dir := filepath.Join(w.root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	repo := fmt.Sprintf("%s/%s", w.org, name)
	p := &Product{Name: name, Dir: dir, Repo: repo}

	// Initialize git if not already a repo
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		if err := p.git("init", "-b", "main"); err != nil {
			return nil, fmt.Errorf("git init: %w", err)
		}

		// Configure git identity for commits
		_ = p.git("config", "user.name", "hive")
		_ = p.git("config", "user.email", "hive@lovyou.ai")

		// Try to create the GitHub repo (may already exist)
		_ = p.gh("repo", "create", repo, "--public",
			"--description", "Built by hive — lovyou-ai/hive")

		// Set remote
		_ = p.git("remote", "add", "origin",
			fmt.Sprintf("https://github.com/%s.git", repo))
	}

	return p, nil
}

// OpenProduct opens an existing product directory.
func (w *Workspace) OpenProduct(name string) (*Product, error) {
	dir := filepath.Join(w.root, name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, fmt.Errorf("product %q not found", name)
	}
	return &Product{
		Name: name,
		Dir:  dir,
		Repo: fmt.Sprintf("%s/%s", w.org, name),
	}, nil
}

// ProductDir returns the directory for a specific product, creating it if needed.
// Kept for backward compatibility — prefer InitProduct for new products.
func (w *Workspace) ProductDir(name string) string {
	dir := filepath.Join(w.root, name)
	os.MkdirAll(dir, 0755)
	return dir
}

// WriteFile writes content to a file, creating parent directories as needed.
func (w *Workspace) WriteFile(path string, content string) error {
	full := w.resolvePath(path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	return os.WriteFile(full, []byte(content), 0644)
}

// ReadFile reads the contents of a file.
func (w *Workspace) ReadFile(path string) (string, error) {
	full := w.resolvePath(path)
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FileExists checks if a file exists.
func (w *Workspace) FileExists(path string) bool {
	full := w.resolvePath(path)
	_, err := os.Stat(full)
	return err == nil
}

// ListFiles returns all files in a product directory.
func (w *Workspace) ListFiles(productName string) ([]string, error) {
	dir := w.ProductDir(productName)
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

// WriteProductFile writes a file into a product's directory and stages it.
func (p *Product) WriteFile(relPath string, content string) error {
	full := filepath.Join(p.Dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		return err
	}
	return p.git("add", relPath)
}

// Commit commits staged changes in the product repo.
func (p *Product) Commit(message string) error {
	return p.git("commit", "-m", message)
}

// Push pushes to the product's remote.
func (p *Product) Push() error {
	return p.git("push", "-u", "origin", "main")
}

// git runs a git command in the product directory.
func (p *Product) git(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(out), err)
	}
	return nil
}

// gh runs a GitHub CLI command.
func (p *Product) gh(args ...string) error {
	cmd := exec.Command("gh", args...)
	cmd.Dir = p.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh %s: %s: %w", strings.Join(args, " "), string(out), err)
	}
	return nil
}

// resolvePath makes a path absolute relative to the workspace root.
func (w *Workspace) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(w.root, path)
}
