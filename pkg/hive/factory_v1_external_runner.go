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
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

const defaultFactoryV1RunnerOutputLimit = 2 * 1024 * 1024

var (
	factoryV1PortableEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	factoryV1CommonEnvironment       = []string{
		"PATH", "TMPDIR", "TMP", "TEMP", "USER", "LOGNAME", "LANG", "LC_ALL", "LC_CTYPE", "SSL_CERT_FILE", "SSL_CERT_DIR",
	}
)

// FactoryV1RunnerProvider binds one provider identity to one exact local
// executable. Args are operator configuration; runner output cannot change
// them or select a different executable, model, family, or credential source.
type FactoryV1RunnerProvider struct {
	Binding              factoryv1.ProviderBinding
	Args                 []string
	EnvironmentAllowlist []string
	Timeout              time.Duration
}

// FactoryV1ExternalRunner is the strict JSON process boundary used by the v1
// scheduler. Provider bindings are checked at construction and immediately
// before every invocation so replacing an executable after startup fails
// closed.
type FactoryV1ExternalRunner struct {
	providers map[string]FactoryV1RunnerProvider
	maxOutput int
}

func NewFactoryV1ExternalRunner(providers []FactoryV1RunnerProvider, maxOutput int) (*FactoryV1ExternalRunner, error) {
	if len(providers) == 0 {
		return nil, errors.New("factory v1 external runner requires at least one provider")
	}
	if maxOutput == 0 {
		maxOutput = defaultFactoryV1RunnerOutputLimit
	}
	if maxOutput < 1024 || maxOutput > 64*1024*1024 {
		return nil, errors.New("factory v1 runner output limit must be between 1 KiB and 64 MiB")
	}
	result := &FactoryV1ExternalRunner{providers: make(map[string]FactoryV1RunnerProvider, len(providers)), maxOutput: maxOutput}
	for _, provider := range providers {
		validated, err := validateFactoryV1RunnerProvider(provider)
		if err != nil {
			return nil, err
		}
		if _, exists := result.providers[validated.Binding.ProviderID]; exists {
			return nil, fmt.Errorf("factory v1 provider id %q is duplicated", validated.Binding.ProviderID)
		}
		result.providers[validated.Binding.ProviderID] = validated
	}
	return result, nil
}

// ResolveFactoryV1ProviderBinding resolves symlinks and verifies the required
// operator-pinned SHA-256 before constructing the immutable scheduler binding.
func ResolveFactoryV1ProviderBinding(providerID, family, executable, expectedSHA256, modelID, credentialSourceID string) (factoryv1.ProviderBinding, error) {
	if strings.TrimSpace(providerID) == "" || strings.TrimSpace(family) == "" || strings.TrimSpace(executable) == "" || strings.TrimSpace(modelID) == "" || strings.TrimSpace(credentialSourceID) == "" {
		return factoryv1.ProviderBinding{}, errors.New("factory v1 provider id, family, executable, model, and credential source are required")
	}
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if len(expectedSHA256) != sha256.Size*2 {
		return factoryv1.ProviderBinding{}, errors.New("factory v1 provider executable SHA-256 must be 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(expectedSHA256); err != nil {
		return factoryv1.ProviderBinding{}, errors.New("factory v1 provider executable SHA-256 must be lowercase hexadecimal")
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return factoryv1.ProviderBinding{}, fmt.Errorf("resolve factory v1 provider executable: %w", err)
	}
	realpath, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return factoryv1.ProviderBinding{}, fmt.Errorf("resolve factory v1 provider executable realpath: %w", err)
	}
	realpath, err = filepath.Abs(realpath)
	if err != nil {
		return factoryv1.ProviderBinding{}, fmt.Errorf("make factory v1 provider executable realpath absolute: %w", err)
	}
	info, err := os.Stat(realpath)
	if err != nil {
		return factoryv1.ProviderBinding{}, fmt.Errorf("stat factory v1 provider executable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return factoryv1.ProviderBinding{}, errors.New("factory v1 provider executable must be a regular executable file")
	}
	actualSHA256, err := factoryV1FileSHA256(realpath)
	if err != nil {
		return factoryv1.ProviderBinding{}, err
	}
	if actualSHA256 != expectedSHA256 {
		return factoryv1.ProviderBinding{}, fmt.Errorf("factory v1 provider executable SHA-256 mismatch: expected %s, got %s", expectedSHA256, actualSHA256)
	}
	return factoryv1.ProviderBinding{
		ProviderID: providerID, Family: family, ExecutableRealpath: realpath,
		ExecutableSHA256: actualSHA256, ModelID: modelID, CredentialSourceID: credentialSourceID,
	}, nil
}

func validateFactoryV1RunnerProvider(provider FactoryV1RunnerProvider) (FactoryV1RunnerProvider, error) {
	binding := provider.Binding
	resolved, err := ResolveFactoryV1ProviderBinding(binding.ProviderID, binding.Family, binding.ExecutableRealpath, binding.ExecutableSHA256, binding.ModelID, binding.CredentialSourceID)
	if err != nil {
		return FactoryV1RunnerProvider{}, fmt.Errorf("provider %q: %w", binding.ProviderID, err)
	}
	if !reflect.DeepEqual(resolved, binding) {
		return FactoryV1RunnerProvider{}, fmt.Errorf("provider %q binding is not canonical", binding.ProviderID)
	}
	if provider.Timeout <= 0 {
		provider.Timeout = 15 * time.Minute
	}
	if provider.Timeout > 2*time.Hour {
		return FactoryV1RunnerProvider{}, fmt.Errorf("provider %q timeout exceeds two-hour ceiling", binding.ProviderID)
	}
	provider.Binding = resolved
	provider.Args = append([]string(nil), provider.Args...)
	provider.EnvironmentAllowlist, err = normalizeFactoryV1EnvironmentAllowlist(provider.EnvironmentAllowlist)
	if err != nil {
		return FactoryV1RunnerProvider{}, fmt.Errorf("provider %q environment allowlist: %w", binding.ProviderID, err)
	}
	return provider, nil
}

func normalizeFactoryV1EnvironmentAllowlist(extra []string) ([]string, error) {
	names := make(map[string]struct{}, len(factoryV1CommonEnvironment)+len(extra))
	for _, name := range append(append([]string(nil), factoryV1CommonEnvironment...), extra...) {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, errors.New("environment key cannot be empty")
		}
		if !factoryV1PortableEnvironmentName.MatchString(name) {
			return nil, fmt.Errorf("environment key %q is not a portable name", name)
		}
		names[name] = struct{}{}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func factoryV1ProviderEnvironment(names []string, lookup func(string) (string, bool)) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		if value, ok := lookup(name); ok {
			result = append(result, name+"="+value)
		}
	}
	return result
}

func (r *FactoryV1ExternalRunner) Execute(ctx context.Context, request factoryv1.RunRequest) (factoryv1.RunResult, error) {
	if request.Operation != "execute" {
		return factoryv1.RunResult{}, fmt.Errorf("factory v1 Execute requires operation execute, got %q", request.Operation)
	}
	var result factoryv1.RunResult
	if err := r.invoke(ctx, request, &result); err != nil {
		return factoryv1.RunResult{}, err
	}
	if !reflect.DeepEqual(result.Provider, request.Provider) {
		return factoryv1.RunResult{}, errors.New("factory v1 runner response provider does not match the invoked binding")
	}
	return result, nil
}

func (r *FactoryV1ExternalRunner) Reconcile(ctx context.Context, request factoryv1.RunRequest) (factoryv1.ReconcileResult, error) {
	if request.Operation != "reconcile" {
		return factoryv1.ReconcileResult{}, fmt.Errorf("factory v1 Reconcile requires operation reconcile, got %q", request.Operation)
	}
	var result factoryv1.ReconcileResult
	if err := r.invoke(ctx, request, &result); err != nil {
		return factoryv1.ReconcileResult{}, err
	}
	if !reflect.DeepEqual(result.Result.Provider, request.Provider) {
		return factoryv1.ReconcileResult{}, errors.New("factory v1 reconcile response provider does not match the invoked binding")
	}
	return result, nil
}

func (r *FactoryV1ExternalRunner) invoke(ctx context.Context, request factoryv1.RunRequest, output any) error {
	provider, ok := r.providers[request.Provider.ProviderID]
	if !ok {
		return fmt.Errorf("factory v1 request selected unknown provider %q", request.Provider.ProviderID)
	}
	if !reflect.DeepEqual(provider.Binding, request.Provider) {
		return errors.New("factory v1 request provider does not match the configured executable/model/family binding")
	}
	if _, err := validateFactoryV1RunnerProvider(provider); err != nil {
		return fmt.Errorf("factory v1 provider changed after startup: %w", err)
	}
	if request.RepositoryRoot == "" {
		return errors.New("factory v1 runner repository root is required")
	}
	repositoryRoot, err := filepath.Abs(request.RepositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve factory v1 repository root: %w", err)
	}
	info, err := os.Stat(repositoryRoot)
	if err != nil {
		return fmt.Errorf("stat factory v1 repository root: %w", err)
	}
	if !info.IsDir() {
		return errors.New("factory v1 repository root must be a directory")
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal factory v1 runner request: %w", err)
	}
	callContext, cancel := context.WithTimeout(ctx, provider.Timeout)
	defer cancel()
	command := exec.CommandContext(callContext, provider.Binding.ExecutableRealpath, provider.Args...)
	command.Dir = repositoryRoot
	commandEnvironment := factoryV1ProviderEnvironment(provider.EnvironmentAllowlist, os.LookupEnv)
	command.Env = commandEnvironment
	command.Stdin = bytes.NewReader(encoded)
	stdout := newFactoryV1BoundedBuffer(r.maxOutput)
	stderr := newFactoryV1BoundedBuffer(r.maxOutput)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if errors.Is(callContext.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("factory v1 provider %q timed out after %s", provider.Binding.ProviderID, provider.Timeout)
		}
		return fmt.Errorf("factory v1 provider %q failed (%s): %w", provider.Binding.ProviderID, stderr.sha256(), err)
	}
	if stdout.exceeded {
		return fmt.Errorf("factory v1 provider %q stdout exceeded %d bytes", provider.Binding.ProviderID, r.maxOutput)
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode factory v1 provider %q strict JSON: %w", provider.Binding.ProviderID, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode factory v1 provider %q: trailing JSON value", provider.Binding.ProviderID)
		}
		return fmt.Errorf("decode factory v1 provider %q trailing data: %w", provider.Binding.ProviderID, err)
	}
	appendFactoryV1EnvironmentEvidence(output, provider, commandEnvironment, stdout, stderr)
	return nil
}

func appendFactoryV1EnvironmentEvidence(output any, provider FactoryV1RunnerProvider, environment []string, stdout, stderr *factoryV1BoundedBuffer) {
	selected := make([]string, 0, len(environment))
	for _, entry := range environment {
		if index := strings.IndexByte(entry, '='); index > 0 {
			selected = append(selected, entry[:index])
		}
	}
	evidence := factoryv1.Evidence{
		Kind: "runner_environment", Reference: "provider:" + provider.Binding.ProviderID,
		Metadata: map[string]string{
			"allowlisted_keys": strings.Join(provider.EnvironmentAllowlist, ","),
			"selected_keys":    strings.Join(selected, ","),
			"stdout_sha256":    stdout.sha256For("stdout"),
			"stderr_sha256":    stderr.sha256For("stderr"),
		},
	}
	switch result := output.(type) {
	case *factoryv1.RunResult:
		result.Evidence = append(result.Evidence, evidence)
	case *factoryv1.ReconcileResult:
		result.Result.Evidence = append(result.Result.Evidence, evidence)
	}
}

func factoryV1FileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open factory v1 executable for hashing: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash factory v1 executable: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type factoryV1BoundedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func newFactoryV1BoundedBuffer(limit int) *factoryV1BoundedBuffer {
	return &factoryV1BoundedBuffer{limit: limit}
}

func (b *factoryV1BoundedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.exceeded = true
		return original, nil
	}
	if len(data) > remaining {
		b.exceeded = true
		data = data[:remaining]
	}
	_, _ = b.Buffer.Write(data)
	return original, nil
}

func (b *factoryV1BoundedBuffer) sha256() string {
	return b.sha256For("stderr")
}

func (b *factoryV1BoundedBuffer) sha256For(label string) string {
	sum := sha256.Sum256(b.Bytes())
	return label + "-sha256:" + hex.EncodeToString(sum[:])
}
