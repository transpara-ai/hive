package collector

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	versioned "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Clients holds the Kubernetes API clients needed for cluster metrics collection.
type Clients struct {
	Core    kubernetes.Interface // core API (pods, nodes, etc.)
	Metrics versioned.Interface  // metrics API (k8s.io/metrics)
}

// NewClients builds Kubernetes clients from either in-cluster config (pod SA
// token) or a kubeconfig file (dev/external). If kubeconfig is empty and
// inCluster is false, it defaults to ~/.kube/config.
func NewClients(kubeconfig string, inCluster bool) (*Clients, error) {
	cfg, err := buildConfig(kubeconfig, inCluster)
	if err != nil {
		return nil, err
	}

	core, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("collector: core client: %w", err)
	}

	metrics, err := versioned.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("collector: metrics client: %w", err)
	}

	return &Clients{Core: core, Metrics: metrics}, nil
}

// buildConfig resolves a *rest.Config from either in-cluster credentials or a
// kubeconfig file path. Exported only for testing via the package.
func buildConfig(kubeconfig string, inCluster bool) (*rest.Config, error) {
	if inCluster {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("collector: in-cluster config: %w", err)
		}
		return cfg, nil
	}

	path := resolveKubeconfig(kubeconfig)
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("collector: kubeconfig %q: %w", path, err)
	}
	return cfg, nil
}

// resolveKubeconfig returns the given path, or ~/.kube/config when empty.
func resolveKubeconfig(path string) string {
	if path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".kube", "config")
	}
	return filepath.Join(home, ".kube", "config")
}
