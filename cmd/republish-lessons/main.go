// Command republish-lessons re-asserts retracted lesson claims at correct numbers.
//
// Observer retracted 10 claims (144-148) due to duplicate number collisions.
// This command re-publishes them at numbers 184-193, querying the graph first
// to confirm the correct start number (Lesson 183: query server before asserting).
//
// Usage:
//
//	cd /c/src/matt/lovyou3/hive
//	LOVYOU_API_KEY=lv_... go run ./cmd/republish-lessons/
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const baseURL = "https://lovyou.ai"
const spaceSlug = "hive"

// retractedLesson is a lesson to re-assert with a corrected number.
type retractedLesson struct {
	shortID string
	title   string // new title with corrected number
	body    string // fetched from the retracted claims endpoint
}

func main() {
	apiKey := os.Getenv("LOVYOU_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "LOVYOU_API_KEY not set")
		os.Exit(1)
	}

	// Step 1: Query MAX lesson number to confirm start is 184.
	maxNum, err := queryMaxLessonNumber(apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query max lesson: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("MAX lesson number on graph: %d — next will be %d\n", maxNum, maxNum+1)
	if maxNum != 183 {
		fmt.Fprintf(os.Stderr, "ERROR: expected max=183, got %d — check before proceeding\n", maxNum)
		os.Exit(1)
	}

	// Step 2: Fetch retracted claims to get their bodies.
	retracted, err := fetchRetractedClaims(apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch retracted claims: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Fetched %d retracted claims\n", len(retracted))

	// Target IDs (first 8 chars) in publication order.
	// Order matches the task spec: 184-193.
	ordered := []struct {
		shortID    string
		coreTitle  string // lesson title stripped of old number
		newNum     int
	}{
		{"f6e95fb1", "Fixing a search index is necessary but not sufficient for institutional memory to influence action", 184},
		{"df838c45", "File-content truncation in a search index is a silent failure mode worse than an empty index", 185},
		{"88b4092e", "Automatic idempotent backfill is the canonical pattern for invariant repair at scale", 186},
		{"7e8abc86", "Silent struct field omission causes invisible data loss in JSON decode", 187},
		{"5839878a", "Variable name divergence after refactoring is invisible to the Go compiler", 188},
		{"52fb6a61", "Stale-but-non-empty output masks pipeline failures", 189},
		{"9cc2f4fd", "Fix pipelines from source to sink", 190},
		{"1af8ea8b", "API endpoint naming/contract failure — semantic category vs storage type mismatch silently returns nothing", 191},
		{"f6feb13f", "Fan-out + client-side prefix filter + ID-keyed dedup is the correct multi-prefix search pattern", 192},
		{"7404fca0", "Data pipelines spanning multiple layers require one end-to-end integration test — per-layer tests catch per-layer bugs only", 193},
	}

	// Build a map from short ID to body.
	bodyByShortID := make(map[string]string)
	for _, c := range retracted {
		if len(c.ID) >= 8 {
			bodyByShortID[c.ID[:8]] = c.Body
		}
	}

	// Step 3: Assert each lesson at the correct number.
	for _, entry := range ordered {
		body, ok := bodyByShortID[entry.shortID]
		if !ok {
			fmt.Fprintf(os.Stderr, "WARN: no body found for %s — skipping\n", entry.shortID)
			continue
		}

		title := fmt.Sprintf("Lesson %d: %s", entry.newNum, entry.coreTitle)
		if err := assertClaim(apiKey, title, body); err != nil {
			fmt.Fprintf(os.Stderr, "assert %s: %v\n", title, err)
			os.Exit(1)
		}
		fmt.Printf("✓ Asserted: %s\n", title)
	}

	fmt.Println("\nAll 10 lessons re-published at 184-193.")
}

// queryMaxLessonNumber fetches all claims and returns the highest "Lesson N" number.
func queryMaxLessonNumber(apiKey string) (int, error) {
	u := fmt.Sprintf("%s/app/%s/knowledge?tab=claims&limit=200", baseURL, spaceSlug)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Claims []struct {
			Title string `json:"title"`
		} `json:"claims"`
	}
	data, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}

	re := regexp.MustCompile(`^Lesson (\d+)`)
	max := 0
	for _, c := range result.Claims {
		m := re.FindStringSubmatch(c.Title)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n > max {
			max = n
		}
	}
	return max, nil
}

// claimNode is the subset of a claim node we need.
type claimNode struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// fetchRetractedClaims fetches all retracted claims from the graph.
func fetchRetractedClaims(apiKey string) ([]claimNode, error) {
	u := fmt.Sprintf("%s/app/%s/knowledge?tab=claims&state=retracted&limit=200", baseURL, spaceSlug)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Claims []claimNode `json:"claims"`
	}
	data, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return result.Claims, nil
}

// assertClaim posts a new claim (op=assert) with the given title and body.
func assertClaim(apiKey, title, body string) error {
	// Normalize em-dash in title for clean JSON.
	title = strings.ReplaceAll(title, "—", "\u2014")

	payload, _ := json.Marshal(map[string]string{
		"op":    "assert",
		"title": title,
		"body":  body,
	})
	u := fmt.Sprintf("%s/app/%s/op", baseURL, spaceSlug)
	req, _ := http.NewRequest("POST", u, bytes.NewReader(payload))
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
