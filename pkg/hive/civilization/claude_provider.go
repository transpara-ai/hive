package civilization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeCLIConfig contains caller-owned, non-secret adapter configuration.
// Sandbox and account qualification are deployment prerequisites, not TLC work.
type ClaudeCLIConfig struct {
	Executable       string
	ExecutableSHA256 string
	SettingsFile     string
	SettingsSHA256   string
	Model            string
	Timeout          time.Duration
	OutputLimitBytes int
	EnvironmentKeys  []string
	ReceiptDirectory string
}

type ClaudeCLI struct {
	config   ClaudeCLIConfig
	receipts *CodexCLI
}

func NewClaudeCLI(config ClaudeCLIConfig) (*ClaudeCLI, error) {
	path, digest, err := resolvePinnedExecutable(config.Executable)
	if err != nil {
		return nil, err
	}
	if digest != config.ExecutableSHA256 {
		return nil, errors.New("Claude executable SHA-256 does not match configured digest")
	}
	config.Executable = path
	if config.Timeout <= 0 || config.OutputLimitBytes <= 0 {
		return nil, errors.New("Claude timeout and output limit must be positive")
	}
	if err := validateSelection(ExecutionSelection{Provider: "claude", Model: config.Model}); err != nil {
		return nil, err
	}
	if err := validateClaudeSettings(config); err != nil {
		return nil, err
	}
	if config.ReceiptDirectory != "" {
		if !filepath.IsAbs(config.ReceiptDirectory) {
			return nil, errors.New("Claude receipt directory must be absolute")
		}
		if err := os.MkdirAll(config.ReceiptDirectory, 0o700); err != nil {
			return nil, err
		}
		if err := os.Chmod(config.ReceiptDirectory, 0o700); err != nil {
			return nil, err
		}
	}
	return &ClaudeCLI{config: config, receipts: &CodexCLI{config: CodexCLIConfig{ReceiptDirectory: config.ReceiptDirectory}}}, nil
}

func validateClaudeSettings(config ClaudeCLIConfig) error {
	if !filepath.IsAbs(config.SettingsFile) || !sha256Pattern.MatchString(config.SettingsSHA256) {
		return errors.New("absolute Claude settings file and SHA-256 are required")
	}
	info, err := os.Lstat(config.SettingsFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
		return errors.New("Claude settings must be a regular file not writable by group or other")
	}
	digest, err := digestFile(config.SettingsFile)
	if err != nil || digest != config.SettingsSHA256 {
		return errors.New("Claude settings SHA-256 does not match configured digest")
	}
	raw, err := os.ReadFile(config.SettingsFile)
	if err != nil {
		return err
	}
	var settings struct {
		Sandbox struct {
			Enabled                  bool  `json:"enabled"`
			FailIfUnavailable        bool  `json:"failIfUnavailable"`
			AllowUnsandboxedCommands *bool `json:"allowUnsandboxedCommands"`
		} `json:"sandbox"`
		AdvisorModel string `json:"advisorModel"`
		Model        string `json:"model"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return errors.New("invalid Claude settings JSON")
	}
	if !settings.Sandbox.Enabled || !settings.Sandbox.FailIfUnavailable || settings.Sandbox.AllowUnsandboxedCommands == nil || *settings.Sandbox.AllowUnsandboxedCommands {
		return errors.New("Claude requires sandbox.enabled, failIfUnavailable, and allowUnsandboxedCommands=false")
	}
	if settings.AdvisorModel != "" {
		return errors.New("primary Claude execution must not enable optional advisor collaboration")
	}
	if settings.Model != "" {
		return errors.New("configure the Claude default model in the caller, not in sandbox settings")
	}
	return nil
}

func (c *ClaudeCLI) Run(ctx context.Context, request ProviderRequest) (ProviderResult, error) {
	requested := request.Selection
	if requested.Provider != "" && requested.Provider != "claude" {
		return ProviderResult{}, errors.New("Claude cannot execute another provider selection")
	}
	if err := validateSelection(requested); err != nil {
		return ProviderResult{}, err
	}
	if requested.ReasoningEffort == "none" || requested.ReasoningEffort == "minimal" {
		return ProviderResult{}, errors.New("invalid Claude reasoning effort")
	}
	if request.Operation != OperationRoute && request.Operation != OperationImplement && request.Operation != OperationReview {
		return ProviderResult{}, errors.New("unsupported provider operation")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return ProviderResult{}, errors.New("provider prompt is required")
	}
	if c.config.ReceiptDirectory != "" && !providerAttemptPattern.MatchString(request.AttemptID) {
		return ProviderResult{}, errors.New("provider attempt id must be a 64-character lowercase hex digest")
	}
	root, err := confinedRepositoryRoot(request.RepositoryRoot)
	if err != nil {
		return ProviderResult{}, err
	}
	_, digest, err := resolvePinnedExecutable(c.config.Executable)
	if err != nil || digest != c.config.ExecutableSHA256 {
		return ProviderResult{}, errors.New("Claude executable changed after provider initialization")
	}
	if err := validateClaudeSettings(c.config); err != nil {
		return ProviderResult{}, err
	}
	request.Selection.Provider = "claude"
	source := "invocation"
	if request.Selection.Model == "" {
		request.Selection.Model = c.config.Model
		source = "configured_provider_default"
	}
	if request.Selection.Model == "" {
		source = "provider_default"
	}
	requestDigest := providerRequestDigest(request, root+"\x00"+digest+"\x00"+c.config.SettingsSHA256)
	if c.config.ReceiptDirectory != "" {
		if result, found, err := c.receipts.loadReceipt(request.AttemptID, requestDigest); err != nil {
			return ProviderResult{}, err
		} else if found {
			return result, nil
		}
	}
	args := []string{"--print", "--output-format", "json", "--json-schema", string(providerResultSchema), "--no-session-persistence", "--safe-mode", "--restricted", "--setting-sources", "", "--settings", c.config.SettingsFile, "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--permission-mode", "dontAsk"}
	tools := "Read,Glob,Grep"
	if request.Operation == OperationImplement {
		tools += ",Edit,Write,Bash"
	}
	args = append(args, "--tools", tools)
	if request.Selection.Model != "" {
		args = append(args, "--model", request.Selection.Model)
	}
	if request.Selection.ReasoningEffort != "" {
		args = append(args, "--effort", request.Selection.ReasoningEffort)
	}
	runCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, c.config.Executable, args...)
	cmd.Dir, cmd.Stdin, cmd.Env = root, strings.NewReader(request.Prompt), allowedEnvironment(c.config.EnvironmentKeys)
	stdout, stderr := newBoundedBuffer(c.config.OutputLimitBytes), newBoundedBuffer(c.config.OutputLimitBytes)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return ProviderResult{}, fmt.Errorf("Claude %s timed out after %s", request.Operation, c.config.Timeout)
		}
		return ProviderResult{}, fmt.Errorf("Claude %s failed: %w%s", request.Operation, err, boundedStderr(stderr.String()))
	}
	if stdout.Overflowed() || stderr.Overflowed() {
		return ProviderResult{}, errors.New("Claude output exceeded configured limit")
	}
	var native struct {
		IsError          bool                       `json:"is_error"`
		Result           string                     `json:"result"`
		StructuredOutput json.RawMessage            `json:"structured_output"`
		ModelUsage       map[string]json.RawMessage `json:"modelUsage"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &native); err != nil {
		return ProviderResult{}, errors.New("invalid Claude result JSON")
	}
	if native.IsError {
		return ProviderResult{}, fmt.Errorf("Claude rejected execution: %s", native.Result)
	}
	result, err := decodeProviderResult(native.StructuredOutput)
	if err != nil {
		return ProviderResult{}, err
	}
	if (request.Operation == OperationRoute) != (len(result.TLCEnvelope) > 0) {
		return ProviderResult{}, errors.New("Claude result has an incompatible TLC envelope")
	}
	effective := request.Selection
	if len(native.ModelUsage) == 1 {
		for model := range native.ModelUsage {
			if effective.Model != "" && effective.Model != model {
				return ProviderResult{}, fmt.Errorf("Claude reported model %q instead of requested %q; use an exact available model identifier", model, effective.Model)
			}
			effective.Model = model
		}
	}
	result.Execution = &ExecutionEvidence{Requested: requested, Effective: effective, ModelSource: source}
	if c.config.ReceiptDirectory != "" {
		if err := c.receipts.storeReceipt(providerReceipt{Schema: "civilization-provider-receipt/v1", AttemptID: request.AttemptID, RequestSHA256: requestDigest, Result: result}); err != nil {
			return ProviderResult{}, err
		}
	}
	return result, nil
}
