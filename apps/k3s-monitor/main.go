// Package main is the CLI entrypoint for the k3s monitoring app.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/lovyou-ai/k3s-monitor/pkg/config"
)

func main() {
	cfg, err := parseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	log.Printf("k3s-monitor starting: addr=%s scrape=%s retention=%s in-cluster=%t",
		cfg.Addr, cfg.ScrapeInterval, cfg.Retention, cfg.InCluster)

	// TODO: wire up collector, store, anomaly detector, and API server.
	_ = cfg
}

func parseFlags() (*config.Config, error) {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig file (default: in-cluster)")
	inCluster := flag.Bool("in-cluster", false, "use in-cluster kubernetes config")
	scrapeInterval := flag.Duration("scrape-interval", 30*time.Second, "metrics scrape interval")
	retention := flag.Duration("retention", 24*time.Hour, "metrics retention duration")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	configPath := flag.String("config", "", "path to alert config YAML file")

	flag.Parse()

	cfg := &config.Config{
		Kubeconfig:     *kubeconfig,
		InCluster:      *inCluster,
		ScrapeInterval: *scrapeInterval,
		Retention:      *retention,
		Addr:           *addr,
	}

	if *configPath != "" {
		alerts, err := config.LoadAlertConfig(*configPath)
		if err != nil {
			return nil, fmt.Errorf("loading alert config: %w", err)
		}
		cfg.Alerts = alerts
	} else {
		cfg.Alerts = config.DefaultAlertThreshold()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}
