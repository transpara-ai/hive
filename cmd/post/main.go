// Command post publishes a hive iteration summary to lovyou.ai.
//
// It reads loop/state.md and loop/build.md, then posts the build report
// as a feed entry in the "hive" space on lovyou.ai. If the space doesn't
// exist yet, it creates it.
//
// Configuration via environment variables:
//
//	LOVYOU_API_KEY   — required. Bearer token for lovyou.ai API.
//	LOVYOU_BASE_URL  — optional. Defaults to https://lovyou.ai.
//
// Usage:
//
//	cd /c/src/matt/lovyou3/hive
//	LOVYOU_API_KEY=lv_... go run ./cmd/post/
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func main() {
	apiKey := os.Getenv("LOVYOU_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "LOVYOU_API_KEY not set, skipping post")
		return
	}

	baseURL := os.Getenv("LOVYOU_BASE_URL")
	if baseURL == "" {
		baseURL = "https://lovyou.ai"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Read iteration number from state.md.
	state, err := os.ReadFile("loop/state.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read state.md: %v\n", err)
		os.Exit(1)
	}
	re := regexp.MustCompile(`Iteration (\d+)`)
	m := re.FindSubmatch(state)
	if m == nil {
		fmt.Fprintln(os.Stderr, "could not find iteration number in state.md")
		os.Exit(1)
	}
	iteration := string(m[1])

	// Read build report.
	build, err := os.ReadFile("loop/build.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "read build.md: %v\n", err)
		os.Exit(1)
	}

	// Ensure the "hive" space exists.
	if err := ensureSpace(apiKey, baseURL); err != nil {
		fmt.Fprintf(os.Stderr, "ensure space: %v\n", err)
		os.Exit(1)
	}

	// Post iteration summary.
	title := fmt.Sprintf("Iteration %s", iteration)
	if err := post(apiKey, baseURL, title, string(build)); err != nil {
		fmt.Fprintf(os.Stderr, "post: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("posted iteration %s to %s/app/hive/feed\n", iteration, baseURL)
}

func ensureSpace(apiKey, baseURL string) error {
	req, _ := http.NewRequest("GET", baseURL+"/app/hive", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("check space: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // space exists
	}

	// Create the space.
	body, _ := json.Marshal(map[string]string{
		"name":        "Hive",
		"description": "The hive loop — iteration summaries, agent activity, system state.",
		"kind":        "community",
		"visibility":  "public",
	})
	req, _ = http.NewRequest("POST", baseURL+"/app/new", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("create space: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create space: HTTP %d: %s", resp.StatusCode, b)
	}

	fmt.Println("created hive space")
	return nil
}

func post(apiKey, baseURL, title, body string) error {
	payload, _ := json.Marshal(map[string]string{
		"op":    "express",
		"title": title,
		"body":  body,
	})
	req, _ := http.NewRequest("POST", baseURL+"/app/hive/op", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}
	return nil
}
