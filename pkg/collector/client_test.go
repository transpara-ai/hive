package collector

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Minimal valid kubeconfig for testing config parsing. Connects to a dummy
// server — we never actually dial, just verify that BuildConfigFromFlags
// produces a *rest.Config without error.
const testKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user:
    token: fake-token
`

func TestResolveKubeconfig_Explicit(t *testing.T) {
	got := resolveKubeconfig("/custom/path/config")
	if got != "/custom/path/config" {
		t.Fatalf("resolveKubeconfig = %q, want /custom/path/config", got)
	}
}

func TestResolveKubeconfig_Default(t *testing.T) {
	got := resolveKubeconfig("")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}
	want := filepath.Join(home, ".kube", "config")
	if got != want {
		t.Fatalf("resolveKubeconfig(\"\") = %q, want %q", got, want)
	}
}

func TestBuildConfig_Kubeconfig(t *testing.T) {
	// Write a temp kubeconfig file.
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(path, []byte(testKubeconfig), 0600); err != nil {
		t.Fatalf("write temp kubeconfig: %v", err)
	}

	cfg, err := buildConfig(path, false)
	if err != nil {
		t.Fatalf("buildConfig: %v", err)
	}
	if cfg.Host != "https://127.0.0.1:6443" {
		t.Fatalf("Host = %q, want https://127.0.0.1:6443", cfg.Host)
	}
	if cfg.BearerToken != "fake-token" {
		t.Fatalf("BearerToken = %q, want fake-token", cfg.BearerToken)
	}
}

func TestBuildConfig_KubeconfigNotFound(t *testing.T) {
	_, err := buildConfig("/nonexistent/path/kubeconfig", false)
	if err == nil {
		t.Fatal("expected error for missing kubeconfig")
	}
	if !strings.Contains(err.Error(), "kubeconfig") {
		t.Fatalf("error should mention kubeconfig, got: %v", err)
	}
}

func TestBuildConfig_InClusterOutsideCluster(t *testing.T) {
	// When not running inside a pod, InClusterConfig should fail.
	_, err := buildConfig("", true)
	if err == nil {
		t.Fatal("expected error for in-cluster config outside a cluster")
	}
	if !strings.Contains(err.Error(), "in-cluster") {
		t.Fatalf("error should mention in-cluster, got: %v", err)
	}
}

func TestNewClients_ValidKubeconfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	if err := os.WriteFile(path, []byte(testKubeconfig), 0600); err != nil {
		t.Fatalf("write temp kubeconfig: %v", err)
	}

	clients, err := NewClients(path, false)
	if err != nil {
		t.Fatalf("NewClients: %v", err)
	}
	if clients.Core == nil {
		t.Fatal("Core client is nil")
	}
	if clients.Metrics == nil {
		t.Fatal("Metrics client is nil")
	}
}

func TestNewClients_InvalidPath(t *testing.T) {
	_, err := NewClients("/does/not/exist", false)
	if err == nil {
		t.Fatal("expected error for invalid kubeconfig path")
	}
}
