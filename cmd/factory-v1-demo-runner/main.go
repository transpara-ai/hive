package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

const (
	runnerSchemaVersion = "factory-v1-demo-runner-v1"
	stateDirEnv         = "FACTORY_V1_DEMO_STATE_DIR"
	maxCommandOutput    = 1 << 20
)

type config struct {
	SchemaVersion       string                                    `json:"schema_version"`
	GitExecutable       string                                    `json:"git_executable"`
	GHExecutable        string                                    `json:"gh_executable"`
	GitRemote           string                                    `json:"git_remote"`
	AuthorFamily        string                                    `json:"author_family"`
	CommitUserName      string                                    `json:"commit_user_name"`
	CommitUserEmail     string                                    `json:"commit_user_email"`
	Repositories        map[string]repositoryConfig               `json:"repositories"`
	StandingApprovals   map[string]factoryv1.HumanApprovalReceipt `json:"standing_approvals"`
	ReviewerEvidenceDir string                                    `json:"reviewer_evidence_dir"`
}

type repositoryConfig struct {
	Root         string     `json:"root"`
	Identity     string     `json:"identity"`
	RemoteURL    string     `json:"remote_url"`
	BaseBranch   string     `json:"base_branch"`
	TestCommands [][]string `json:"test_commands"`
}

type commandResult struct {
	Stdout string
	Stderr string
}

type commander interface {
	Run(ctx context.Context, dir, executable string, args ...string) (commandResult, error)
}

type execCommander struct{}

func (execCommander) Run(ctx context.Context, dir, executable string, args ...string) (commandResult, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = dir
	var stdout, stderr limitedBuffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := commandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if stdout.exceeded || stderr.exceeded {
		return result, fmt.Errorf("%s command output exceeded %d bytes", filepath.Base(executable), maxCommandOutput)
	}
	if err != nil {
		return result, fmt.Errorf("%s command failed (stderr-sha256:%s): %w", filepath.Base(executable), factoryv1.HashText(result.Stderr), err)
	}
	return result, nil
}

type limitedBuffer struct {
	bytes.Buffer
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := maxCommandOutput - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			b.exceeded = true
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	} else {
		b.exceeded = true
	}
	return original, nil
}

func main() {
	os.Exit(runMain(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, execCommander{}))
}

func runMain(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, commands commander) int {
	flags := flag.NewFlagSet("factory-v1-demo-runner", flag.ContinueOnError)
	flags.SetOutput(stderr)
	stateDir := flags.String("state-dir", os.Getenv(stateDirEnv), "private mode-0700 runner state directory")
	configPath := flags.String("config", "", "private mode-0600 config JSON (default: STATE_DIR/config.json)")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "factory-v1-demo-runner accepts no positional arguments")
		return 2
	}
	if strings.TrimSpace(*stateDir) == "" {
		fmt.Fprintf(stderr, "--state-dir or %s is required\n", stateDirEnv)
		return 2
	}
	resolvedState, cfg, err := loadConfig(*stateDir, *configPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	request, err := decodeRunRequest(stdin)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	runner, err := newDemoRunner(resolvedState, cfg, commands)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := runner.validateRequest(ctx, request); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	switch request.Operation {
	case "execute":
		result, executeErr := runner.execute(ctx, request)
		if executeErr != nil {
			fmt.Fprintln(stderr, executeErr)
			return 1
		}
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	case "reconcile":
		result, reconcileErr := runner.reconcile(ctx, request)
		if reconcileErr != nil {
			fmt.Fprintln(stderr, reconcileErr)
			return 1
		}
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	default:
		fmt.Fprintf(stderr, "operation must be execute or reconcile, got %q\n", request.Operation)
		return 1
	}
	return 0
}

func decodeRunRequest(reader io.Reader) (factoryv1.RunRequest, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 4<<20))
	decoder.DisallowUnknownFields()
	var request factoryv1.RunRequest
	if err := decoder.Decode(&request); err != nil {
		return factoryv1.RunRequest{}, fmt.Errorf("decode strict RunRequest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return factoryv1.RunRequest{}, errors.New("decode strict RunRequest: trailing JSON value")
		}
		return factoryv1.RunRequest{}, fmt.Errorf("decode strict RunRequest trailing data: %w", err)
	}
	return request, nil
}

func loadConfig(stateDir, explicitConfig string) (string, config, error) {
	stateAbs, err := filepath.Abs(stateDir)
	if err != nil {
		return "", config{}, err
	}
	stateInfo, err := os.Stat(stateAbs)
	if err != nil {
		return "", config{}, fmt.Errorf("private state directory: %w", err)
	}
	if !stateInfo.IsDir() || stateInfo.Mode().Perm()&0o077 != 0 {
		return "", config{}, errors.New("private state directory must be a directory with no group/other permissions")
	}
	stateAbs, err = filepath.EvalSymlinks(stateAbs)
	if err != nil {
		return "", config{}, fmt.Errorf("resolve private state directory: %w", err)
	}
	path := explicitConfig
	if path == "" {
		path = filepath.Join(stateAbs, "config.json")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", config{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", config{}, fmt.Errorf("private runner config: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", config{}, errors.New("private runner config must be a regular file with no group/other permissions")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", config{}, err
	}
	var cfg config
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return "", config{}, fmt.Errorf("decode strict runner config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", config{}, errors.New("decode strict runner config: trailing JSON value")
		}
		return "", config{}, fmt.Errorf("decode strict runner config trailing data: %w", err)
	}
	if err := validateConfig(stateAbs, &cfg); err != nil {
		return "", config{}, err
	}
	return stateAbs, cfg, nil
}

func validateConfig(stateDir string, cfg *config) error {
	if cfg.SchemaVersion != runnerSchemaVersion {
		return fmt.Errorf("runner config schema must be %q", runnerSchemaVersion)
	}
	var err error
	cfg.GitExecutable, err = resolveExecutable(cfg.GitExecutable)
	if err != nil {
		return fmt.Errorf("git executable: %w", err)
	}
	cfg.GHExecutable, err = resolveExecutable(cfg.GHExecutable)
	if err != nil {
		return fmt.Errorf("gh executable: %w", err)
	}
	if cfg.GitRemote == "" || cfg.AuthorFamily == "" || cfg.CommitUserName == "" || cfg.CommitUserEmail == "" || len(cfg.Repositories) == 0 {
		return errors.New("runner config requires remote, author family, commit identity, and repositories")
	}
	if strings.ContainsAny(cfg.GitRemote, "/\\") {
		return errors.New("git remote must be an alias, not a path")
	}
	for key, repository := range cfg.Repositories {
		if key != repository.Identity || repository.Identity == "" || repository.Root == "" || repository.BaseBranch == "" || repository.RemoteURL == "" || len(repository.TestCommands) == 0 {
			return fmt.Errorf("repository %q lacks exact identity, root, remote URL, base branch, or named test commands", key)
		}
		if repository.BaseBranch != "main" && repository.BaseBranch != "master" {
			return fmt.Errorf("repository %s base branch is not an allowed protected branch name", key)
		}
		root, resolveErr := filepath.Abs(repository.Root)
		if resolveErr != nil {
			return resolveErr
		}
		root, resolveErr = filepath.EvalSymlinks(root)
		if resolveErr != nil {
			return fmt.Errorf("repository %s root: %w", key, resolveErr)
		}
		repository.Root = root
		if normalizeGitHubURL(repository.RemoteURL) != "https://github.com/"+repository.Identity {
			return fmt.Errorf("repository %s remote URL does not bind its GitHub identity", key)
		}
		for _, command := range repository.TestCommands {
			if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
				return fmt.Errorf("repository %s has an empty named test command", key)
			}
		}
		cfg.Repositories[key] = repository
	}
	evidenceDir := cfg.ReviewerEvidenceDir
	if evidenceDir == "" {
		evidenceDir = filepath.Join(stateDir, "reviewer-artifacts")
	} else if !filepath.IsAbs(evidenceDir) {
		evidenceDir = filepath.Join(stateDir, evidenceDir)
	}
	evidenceDir = filepath.Clean(evidenceDir)
	if !pathWithin(stateDir, evidenceDir) {
		return errors.New("reviewer evidence directory must stay within the private state directory")
	}
	resolvedEvidence, err := ensurePrivateContainedDir(stateDir, evidenceDir)
	if err != nil {
		return err
	}
	cfg.ReviewerEvidenceDir = resolvedEvidence
	for key, receipt := range cfg.StandingApprovals {
		if key != approvalKey(receipt.OrderID, receipt.OrderVersion, receipt.DocumentSHA256) {
			return fmt.Errorf("standing approval map key %q does not bind its exact receipt tuple", key)
		}
	}
	return nil
}

func resolveExecutable(value string) (string, error) {
	if value == "" {
		return "", errors.New("path is required")
	}
	path, err := exec.LookPath(value)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("directory %s is not private", path)
	}
	return nil
}

func ensurePrivateContainedDir(root, path string) (string, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("private directory %s may not be a symlink", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := ensurePrivateDir(path); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !pathWithin(root, resolved) {
		return "", fmt.Errorf("private directory %s escapes state root", path)
	}
	return resolved, nil
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func approvalKey(orderID, version, documentSHA string) string {
	return orderID + "@" + version + "@" + documentSHA
}

func normalizeGitHubURL(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, ".git"))
	if strings.HasPrefix(value, "git@github.com:") {
		value = "https://github.com/" + strings.TrimPrefix(value, "git@github.com:")
	}
	if strings.HasPrefix(value, "ssh://git@github.com/") {
		value = "https://github.com/" + strings.TrimPrefix(value, "ssh://git@github.com/")
	}
	return strings.TrimRight(value, "/")
}

var nowUTC = func() time.Time { return time.Now().UTC() }
