package hive

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

const defaultFactoryTLC51OutputLimit = 4 * 1024 * 1024

type FactoryTLC51ClientConfig struct {
	Executable            string
	ExecutableSHA256      string
	RepositoryRoot        string
	ReleaseManifest       string
	ReleaseManifestSHA256 string
	Timeout               time.Duration
	MaxOutputBytes        int
	Environment           []string
}

type FactoryTLC51ClientIdentity struct {
	ExecutableRealpath    string `json:"executable_realpath"`
	ExecutableSHA256      string `json:"executable_sha256"`
	RepositoryRoot        string `json:"repository_root"`
	ReleaseManifest       string `json:"release_manifest"`
	ReleaseManifestSHA256 string `json:"release_manifest_sha256"`
}

type factoryTLC51Invoke func(context.Context, string, []byte) ([]byte, int, error)

// FactoryTLC51CommandClient invokes only the pure, report-only TLC gate
// commands from a pinned executable and release manifest. It exposes no
// mutation command and grants no authority.
type FactoryTLC51CommandClient struct {
	identity    FactoryTLC51ClientIdentity
	timeout     time.Duration
	maxOutput   int
	environment []string
	invoke      factoryTLC51Invoke
}

func NewFactoryTLC51CommandClient(config FactoryTLC51ClientConfig) (*FactoryTLC51CommandClient, error) {
	if strings.TrimSpace(config.Executable) == "" || strings.TrimSpace(config.RepositoryRoot) == "" || strings.TrimSpace(config.ReleaseManifest) == "" {
		return nil, errors.New("TLC 5.1 client requires executable, repository root, and release manifest")
	}
	executable, err := filepath.Abs(config.Executable)
	if err != nil {
		return nil, fmt.Errorf("TLC executable path: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, fmt.Errorf("TLC executable realpath: %w", err)
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, errors.New("TLC executable must be an executable regular file")
	}
	executableSHA, err := factoryTLC51FileSHA256(executable)
	if err != nil {
		return nil, err
	}
	if !validFactoryTLC51Digest(config.ExecutableSHA256) || executableSHA != config.ExecutableSHA256 {
		return nil, errors.New("TLC executable SHA-256 mismatch")
	}
	root, err := filepath.Abs(config.RepositoryRoot)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("TLC repository root realpath: %w", err)
	}
	if rootInfo, statErr := os.Stat(root); statErr != nil || !rootInfo.IsDir() {
		return nil, errors.New("TLC repository root must be an existing directory")
	}
	manifest := config.ReleaseManifest
	if !filepath.IsAbs(manifest) {
		manifest = filepath.Join(root, manifest)
	}
	manifest, err = filepath.Abs(manifest)
	if err != nil {
		return nil, err
	}
	manifest, err = filepath.EvalSymlinks(manifest)
	if err != nil {
		return nil, fmt.Errorf("TLC release manifest realpath: %w", err)
	}
	manifestSHA, err := factoryTLC51FileSHA256(manifest)
	if err != nil {
		return nil, err
	}
	if !validFactoryTLC51Digest(config.ReleaseManifestSHA256) || manifestSHA != config.ReleaseManifestSHA256 {
		return nil, errors.New("TLC release manifest SHA-256 mismatch")
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Timeout < time.Second || config.Timeout > 5*time.Minute {
		return nil, errors.New("TLC client timeout must be between 1s and 5m")
	}
	if config.MaxOutputBytes == 0 {
		config.MaxOutputBytes = defaultFactoryTLC51OutputLimit
	}
	if config.MaxOutputBytes < 1024 || config.MaxOutputBytes > 32*1024*1024 {
		return nil, errors.New("TLC client output limit must be between 1KiB and 32MiB")
	}
	environment, err := validateFactoryTLC51Environment(config.Environment)
	if err != nil {
		return nil, err
	}
	client := &FactoryTLC51CommandClient{
		identity: FactoryTLC51ClientIdentity{
			ExecutableRealpath: executable, ExecutableSHA256: executableSHA, RepositoryRoot: root,
			ReleaseManifest: manifest, ReleaseManifestSHA256: manifestSHA,
		},
		timeout: config.Timeout, maxOutput: config.MaxOutputBytes, environment: environment,
	}
	client.invoke = client.invokeCommand
	return client, nil
}

func (client *FactoryTLC51CommandClient) Identity() FactoryTLC51ClientIdentity {
	return client.identity
}

func (client *FactoryTLC51CommandClient) Plan(ctx context.Context, facts json.RawMessage) (factoryv1.TLC51GatePlan, error) {
	if !json.Valid(facts) {
		return factoryv1.TLC51GatePlan{}, errors.New("TLC 5.1 facts must be valid JSON")
	}
	output, exitCode, err := client.invoke(ctx, "plan", facts)
	if err != nil {
		return factoryv1.TLC51GatePlan{}, err
	}
	if exitCode != 0 {
		return factoryv1.TLC51GatePlan{}, fmt.Errorf("tlc gate plan exited %d", exitCode)
	}
	return factoryv1.ParseTLC51GatePlan(output)
}

func (client *FactoryTLC51CommandClient) Evaluate(ctx context.Context, evaluation json.RawMessage) (factoryv1.TLC51GateReceipt, error) {
	if !json.Valid(evaluation) {
		return factoryv1.TLC51GateReceipt{}, errors.New("TLC 5.1 evaluation must be valid JSON")
	}
	var identity struct {
		SchemaVersion string          `json:"schema_version"`
		Plan          json.RawMessage `json:"plan"`
	}
	if err := json.Unmarshal(evaluation, &identity); err != nil || identity.SchemaVersion != factoryv1.TLC51EvaluationSchema {
		return factoryv1.TLC51GateReceipt{}, errors.New("TLC 5.1 evaluation schema is invalid")
	}
	planRaw, err := canonicalizeFactoryTLC51Embedded(identity.Plan)
	if err != nil {
		return factoryv1.TLC51GateReceipt{}, fmt.Errorf("evaluation plan: %w", err)
	}
	plan, err := factoryv1.ParseTLC51GatePlan(planRaw)
	if err != nil {
		return factoryv1.TLC51GateReceipt{}, err
	}
	output, exitCode, err := client.invoke(ctx, "evaluate", evaluation)
	if err != nil {
		return factoryv1.TLC51GateReceipt{}, err
	}
	// The TLC CLI returns 1 for fail/unknown receipts. A structurally valid
	// receipt remains durable decision evidence; only other exit codes fail.
	if exitCode != 0 && exitCode != 1 {
		return factoryv1.TLC51GateReceipt{}, fmt.Errorf("tlc gate evaluate exited %d", exitCode)
	}
	return factoryv1.ParseTLC51GateReceipt(output, plan)
}

func canonicalizeFactoryTLC51Embedded(raw json.RawMessage) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func (client *FactoryTLC51CommandClient) invokeCommand(ctx context.Context, operation string, input []byte) ([]byte, int, error) {
	if operation != "plan" && operation != "evaluate" {
		return nil, -1, fmt.Errorf("unsupported TLC gate operation %q", operation)
	}
	callCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	tempDir, err := os.MkdirTemp("", "hive-tlc51-")
	if err != nil {
		return nil, -1, err
	}
	defer os.RemoveAll(tempDir)
	if err := os.Chmod(tempDir, 0o700); err != nil {
		return nil, -1, err
	}
	inputPath := filepath.Join(tempDir, operation+".json")
	file, err := os.OpenFile(inputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, -1, err
	}
	if _, err = file.Write(input); err != nil {
		_ = file.Close()
		return nil, -1, err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return nil, -1, err
	}
	if err = file.Close(); err != nil {
		return nil, -1, err
	}
	flag := "--facts"
	if operation == "evaluate" {
		flag = "--input"
	}
	args := []string{"--root", client.identity.RepositoryRoot, "gate", operation, flag, inputPath, "--release-manifest", client.identity.ReleaseManifest}
	command := exec.CommandContext(callCtx, client.identity.ExecutableRealpath, args...)
	command.Env = append([]string(nil), client.environment...)
	stdout := newFactoryTLC51BoundedBuffer(client.maxOutput)
	stderr := newFactoryTLC51BoundedBuffer(client.maxOutput)
	command.Stdout, command.Stderr = stdout, stderr
	err = command.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, -1, fmt.Errorf("invoke tlc gate %s: %w", operation, err)
		}
	}
	if stdout.exceeded || stderr.exceeded {
		return nil, -1, errors.New("TLC gate output exceeded configured limit")
	}
	if callCtx.Err() != nil {
		return nil, -1, fmt.Errorf("TLC gate %s: %w", operation, callCtx.Err())
	}
	if len(stdout.Bytes()) == 0 {
		return nil, exitCode, fmt.Errorf("TLC gate %s returned empty stdout (stderr_sha256=%s)", operation, stderr.SHA256())
	}
	return stdout.Bytes(), exitCode, nil
}

func validateFactoryTLC51Environment(values []string) ([]string, error) {
	allowed := map[string]struct{}{"PATH": {}, "LANG": {}, "LC_ALL": {}, "PYTHONPATH": {}, "VIRTUAL_ENV": {}}
	result := []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8", "LC_ALL=C.UTF-8"}
	seen := map[string]struct{}{"PATH": {}, "LANG": {}, "LC_ALL": {}}
	for index, value := range values {
		name, _, ok := strings.Cut(value, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("TLC environment[%d] must be NAME=value", index)
		}
		if _, permitted := allowed[name]; !permitted {
			return nil, fmt.Errorf("TLC environment key %q is not allowed", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("TLC environment key %q is duplicated", name)
		}
		seen[name] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func validFactoryTLC51Digest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func factoryTLC51FileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type factoryTLC51BoundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func newFactoryTLC51BoundedBuffer(limit int) *factoryTLC51BoundedBuffer {
	return &factoryTLC51BoundedBuffer{limit: limit}
}

func (buffer *factoryTLC51BoundedBuffer) Write(data []byte) (int, error) {
	written := len(data)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.exceeded = true
		return written, nil
	}
	if len(data) > remaining {
		buffer.exceeded = true
		data = data[:remaining]
	}
	_, _ = buffer.buffer.Write(data)
	return written, nil
}

func (buffer *factoryTLC51BoundedBuffer) Bytes() []byte {
	return append([]byte(nil), buffer.buffer.Bytes()...)
}

func (buffer *factoryTLC51BoundedBuffer) SHA256() string {
	return fmt.Sprintf("%x", sha256.Sum256(buffer.buffer.Bytes()))
}
