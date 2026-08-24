package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

type githubCommandRunner func(ctx context.Context, executable string, args ...string) ([]byte, error)

type githubPRObserver struct {
	executable string
	ttl        time.Duration
	failureTTL time.Duration
	timeout    time.Duration
	now        func() time.Time
	run        githubCommandRunner

	mu           sync.Mutex
	cache        map[string]githubPRCacheEntry
	failureUntil time.Time
}

type githubPRCacheEntry struct {
	observation factoryv1.PRObservation
	expiresAt   time.Time
}

type githubPRView struct {
	State            string `json:"state"`
	IsDraft          bool   `json:"isDraft"`
	HeadRefOID       string `json:"headRefOid"`
	MergeStateStatus string `json:"mergeStateStatus"`
	URL              string `json:"url"`
}

func newGitHubPRObserver(executable string) (*githubPRObserver, error) {
	if strings.TrimSpace(executable) == "" {
		return nil, errors.New("GitHub CLI executable is required")
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return nil, fmt.Errorf("resolve GitHub CLI: %w", err)
	}
	return &githubPRObserver{
		executable: resolved,
		// One minute keeps the operator view current while bounding authenticated
		// GitHub reads as the durable order history grows.
		ttl:        time.Minute,
		failureTTL: 15 * time.Second,
		timeout:    5 * time.Second,
		now:        time.Now,
		run: func(ctx context.Context, executable string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, executable, args...).Output()
		},
		cache: make(map[string]githubPRCacheEntry),
	}, nil
}

func (o *githubPRObserver) ObservePR(ctx context.Context, repository string, number int) (factoryv1.PRObservation, error) {
	repository = strings.TrimSpace(repository)
	if repository == "" || strings.Count(repository, "/") != 1 || strings.ContainsAny(repository, " \t\r\n") || number < 1 {
		return factoryv1.PRObservation{}, errors.New("invalid GitHub PR identity")
	}
	key := repository + "#" + strconv.Itoa(number)
	now := o.now().UTC()
	o.mu.Lock()
	if cached, ok := o.cache[key]; ok && now.Before(cached.expiresAt) {
		o.mu.Unlock()
		return cached.observation, nil
	}
	if now.Before(o.failureUntil) {
		o.mu.Unlock()
		return factoryv1.PRObservation{}, errors.New("GitHub PR observation is temporarily unavailable")
	}
	o.mu.Unlock()

	viewContext, cancelView := context.WithTimeout(ctx, o.timeout)
	output, err := o.run(viewContext, o.executable, "pr", "view", strconv.Itoa(number), "-R", repository, "--json", "state,isDraft,headRefOid,mergeStateStatus,url")
	cancelView()
	if err != nil {
		o.recordFailure(o.now().UTC())
		return factoryv1.PRObservation{}, errors.New("GitHub PR query failed")
	}
	var view githubPRView
	if err := json.Unmarshal(output, &view); err != nil {
		o.recordFailure(o.now().UTC())
		return factoryv1.PRObservation{}, errors.New("GitHub PR query returned invalid JSON")
	}
	state := strings.ToLower(strings.TrimSpace(view.State))
	if state != "open" && state != "closed" && state != "merged" {
		o.recordFailure(o.now().UTC())
		return factoryv1.PRObservation{}, errors.New("GitHub PR query returned an unknown state")
	}

	checksPassing := false
	detail := "required GitHub checks are not passing or unavailable"
	checksContext, cancelChecks := context.WithTimeout(ctx, o.timeout)
	_, checksErr := o.run(checksContext, o.executable, "pr", "checks", strconv.Itoa(number), "-R", repository, "--required")
	cancelChecks()
	if checksErr == nil {
		checksPassing = true
		detail = "required GitHub checks pass"
	}
	observation := factoryv1.PRObservation{
		Repository: repository, Number: number, URL: view.URL, State: state,
		HeadSHA: view.HeadRefOID, Draft: view.IsDraft, ChecksPassing: checksPassing, MergeStateStatus: view.MergeStateStatus,
		ObservedAt: now, Source: "github_cli", Detail: detail,
	}
	o.mu.Lock()
	o.cache[key] = githubPRCacheEntry{observation: observation, expiresAt: now.Add(o.ttl)}
	o.mu.Unlock()
	return observation, nil
}

func (o *githubPRObserver) recordFailure(now time.Time) {
	o.mu.Lock()
	o.failureUntil = now.Add(o.failureTTL)
	o.mu.Unlock()
}
