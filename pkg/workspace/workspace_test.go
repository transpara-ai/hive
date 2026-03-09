package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	dir := t.TempDir()
	ws, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if ws.Root() != dir {
		// On Windows, TempDir may not match abs exactly
		abs, _ := filepath.Abs(dir)
		if ws.Root() != abs {
			t.Errorf("Root = %q, want %q", ws.Root(), dir)
		}
	}
}

func TestWriteAndReadFile(t *testing.T) {
	ws, _ := New(t.TempDir())
	content := "package main\n\nfunc main() {}\n"

	err := ws.WriteFile("myproject/main.go", content)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ws.ReadFile("myproject/main.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != content {
		t.Errorf("ReadFile = %q, want %q", got, content)
	}
}

func TestProductDir(t *testing.T) {
	ws, _ := New(t.TempDir())
	dir := ws.ProductDir("alpha")

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("ProductDir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("ProductDir is not a directory")
	}
}

func TestFileExists(t *testing.T) {
	ws, _ := New(t.TempDir())

	if ws.FileExists("nonexistent.go") {
		t.Error("FileExists should return false for missing file")
	}

	ws.WriteFile("exists.go", "package x")
	if !ws.FileExists("exists.go") {
		t.Error("FileExists should return true for existing file")
	}
}

func TestListFiles(t *testing.T) {
	ws, _ := New(t.TempDir())
	ws.WriteFile("proj/main.go", "package main")
	ws.WriteFile("proj/lib/util.go", "package lib")

	files, err := ws.ListFiles("proj")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("ListFiles = %d files, want 2", len(files))
	}
}
