// Package workspace manages the file system for generated products.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// Workspace manages directories and files for hive-generated products.
type Workspace struct {
	root string // Root directory for all products
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

	return &Workspace{root: abs}, nil
}

// Root returns the workspace root directory.
func (w *Workspace) Root() string {
	return w.root
}

// ProductDir returns the directory for a specific product, creating it if needed.
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

// resolvePath makes a path absolute relative to the workspace root.
func (w *Workspace) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(w.root, path)
}
