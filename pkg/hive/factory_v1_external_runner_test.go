package hive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/transpara-ai/hive/pkg/hive/factoryv1"
)

func TestFactoryV1RunnerRejectsExecutableHashReplacement(t *testing.T) {
	if _, err := os.Stat("/usr/bin/jq"); err != nil {
		t.Skip("/usr/bin/jq is required for the strict JSON runner fixture")
	}
	root := t.TempDir()
	executable := filepath.Join(root, "runner.sh")
	script := `#!/bin/sh
set -eu
request=$(cat)
provider=$(printf '%s' "$request" | /usr/bin/jq -c '.provider')
operation=$(printf '%s' "$request" | /usr/bin/jq -r '.operation')
if [ "$operation" = reconcile ]; then
  /usr/bin/jq -nc --argjson provider "$provider" '{effect_exists:false,conflict:false,result:{status:"blocked",evidence:[{kind:"reconcile",reference:"fixture:no-effect"}],blocker:"effect absent",usage:{tokens:0,cost_micros:0},provider:$provider}}'
else
  /usr/bin/jq -nc --argjson provider "$provider" '{status:"passed",evidence:[{kind:"fixture",reference:"fixture:passed"}],usage:{tokens:7,cost_micros:9},provider:$provider}'
fi
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatalf("write runner fixture: %v", err)
	}
	digest, err := factoryV1FileSHA256(executable)
	if err != nil {
		t.Fatalf("hash runner fixture: %v", err)
	}
	binding, err := ResolveFactoryV1ProviderBinding("fixture-provider", "Fixture/Independent", executable, digest, "fixture-model-v1", "fixture-credential-source")
	if err != nil {
		t.Fatalf("resolve provider: %v", err)
	}
	runner, err := NewFactoryV1ExternalRunner([]FactoryV1RunnerProvider{{Binding: binding, Timeout: 5 * time.Second}}, 64*1024)
	if err != nil {
		t.Fatalf("new external runner: %v", err)
	}
	request := factoryv1.RunRequest{Operation: "execute", RepositoryRoot: root, Provider: binding}
	result, err := runner.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != factoryv1.RunnerPassed || result.Provider != binding || result.Usage.Tokens != 7 {
		t.Fatalf("execute result = %+v", result)
	}
	request.Operation = "reconcile"
	reconciled, err := runner.Reconcile(context.Background(), request)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled.EffectExists || reconciled.Conflict || reconciled.Result.Provider != binding {
		t.Fatalf("reconcile result = %+v", reconciled)
	}

	if err := os.WriteFile(executable, []byte(script+"\n# replaced after binding\n"), 0o700); err != nil {
		t.Fatalf("replace runner fixture: %v", err)
	}
	request.Operation = "execute"
	if _, err := runner.Execute(context.Background(), request); err == nil || !strings.Contains(err.Error(), "changed after startup") {
		t.Fatalf("replacement error = %v", err)
	}
}

func TestResolveFactoryV1ProviderBindingRejectsWrongDigest(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "runner")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveFactoryV1ProviderBinding("provider", "family", executable, strings.Repeat("0", 64), "model", "credential-source")
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("digest error = %v", err)
	}
}

func TestFactoryV1RunnerEnvironmentAllowlistSeparatesProviders(t *testing.T) {
	if _, err := os.Stat("/usr/bin/jq"); err != nil {
		t.Skip("/usr/bin/jq is required for the strict JSON runner fixture")
	}
	root := t.TempDir()
	executable := filepath.Join(root, "environment-runner.sh")
	script := `#!/bin/sh
set -eu
request=$(cat)
provider=$(printf '%s' "$request" | /usr/bin/jq -r '.provider.provider_id')
provider_json=$(printf '%s' "$request" | /usr/bin/jq -c '.provider')
if [ -z "${PATH+x}" ]; then
  echo 'PATH missing' >&2
  exit 1
fi
if [ -n "${FACTORY_V1_PARENT_SECRET+x}" ]; then
  echo 'parent secret leaked' >&2
  exit 1
fi
case "$provider" in
  author-fixture)
    if [ -z "${FACTORY_V1_AUTHOR_ONLY+x}" ] || [ -n "${FACTORY_V1_REVIEWER_ONLY+x}" ]; then
      echo 'author environment separation failed' >&2
      exit 1
    fi
    reference='env_keys:FACTORY_V1_AUTHOR_ONLY,PATH'
    ;;
  reviewer-fixture)
    if [ -z "${FACTORY_V1_REVIEWER_ONLY+x}" ] || [ -n "${FACTORY_V1_AUTHOR_ONLY+x}" ]; then
      echo 'reviewer environment separation failed' >&2
      exit 1
    fi
    reference='env_keys:FACTORY_V1_REVIEWER_ONLY,PATH'
    ;;
  *)
    echo 'unknown provider' >&2
    exit 1
    ;;
esac
/usr/bin/jq -nc --argjson provider "$provider_json" --arg reference "$reference" '{status:"passed",evidence:[{kind:"environment_keys",reference:$reference}],usage:{tokens:1,cost_micros:1},provider:$provider}'
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	digest, err := factoryV1FileSHA256(executable)
	if err != nil {
		t.Fatal(err)
	}
	author, err := ResolveFactoryV1ProviderBinding("author-fixture", "OpenAI/Fixture", executable, digest, "author-model", "author-credential-source")
	if err != nil {
		t.Fatal(err)
	}
	reviewer, err := ResolveFactoryV1ProviderBinding("reviewer-fixture", "Anthropic/Fixture", executable, digest, "reviewer-model", "reviewer-credential-source")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FACTORY_V1_AUTHOR_ONLY", "author-sentinel-value")
	t.Setenv("FACTORY_V1_REVIEWER_ONLY", "reviewer-sentinel-value")
	t.Setenv("FACTORY_V1_PARENT_SECRET", "parent-secret-sentinel-value")
	runner, err := NewFactoryV1ExternalRunner([]FactoryV1RunnerProvider{
		{Binding: author, EnvironmentAllowlist: []string{" FACTORY_V1_AUTHOR_ONLY ", "FACTORY_V1_AUTHOR_ONLY"}, Timeout: 5 * time.Second},
		{Binding: reviewer, EnvironmentAllowlist: []string{"FACTORY_V1_REVIEWER_ONLY"}, Timeout: 5 * time.Second},
	}, 64*1024)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		binding factoryv1.ProviderBinding
		want    string
	}{
		{binding: author, want: "env_keys:FACTORY_V1_AUTHOR_ONLY,PATH"},
		{binding: reviewer, want: "env_keys:FACTORY_V1_REVIEWER_ONLY,PATH"},
	} {
		result, err := runner.Execute(context.Background(), factoryv1.RunRequest{Operation: "execute", RepositoryRoot: root, Provider: test.binding})
		if err != nil {
			t.Fatalf("execute %s: %v", test.binding.ProviderID, err)
		}
		var fixtureEvidence, environmentEvidence *factoryv1.Evidence
		for index := range result.Evidence {
			switch result.Evidence[index].Kind {
			case "environment_keys":
				fixtureEvidence = &result.Evidence[index]
			case "runner_environment":
				environmentEvidence = &result.Evidence[index]
			}
		}
		if fixtureEvidence == nil || fixtureEvidence.Reference != test.want || environmentEvidence == nil {
			t.Fatalf("%s evidence = %+v, want fixture %q and runner environment", test.binding.ProviderID, result.Evidence, test.want)
		}
		if strings.Contains(environmentEvidence.Metadata["selected_keys"], "FACTORY_V1_PARENT_SECRET") ||
			!strings.Contains(environmentEvidence.Metadata["selected_keys"], "PATH") {
			t.Fatalf("%s selected environment keys = %q", test.binding.ProviderID, environmentEvidence.Metadata["selected_keys"])
		}
		for _, key := range []string{"stdout_sha256", "stderr_sha256"} {
			if !strings.Contains(environmentEvidence.Metadata[key], "-sha256:") {
				t.Fatalf("%s %s = %q", test.binding.ProviderID, key, environmentEvidence.Metadata[key])
			}
		}
		rendered := fmt.Sprintf("%+v", result)
		for _, secret := range []string{"author-sentinel-value", "reviewer-sentinel-value", "parent-secret-sentinel-value"} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("%s result serialized an environment value", test.binding.ProviderID)
			}
		}
	}
	for _, providerID := range []string{author.ProviderID, reviewer.ProviderID} {
		allowlist := runner.providers[providerID].EnvironmentAllowlist
		for _, forbiddenDefault := range []string{"HOME", "GH_TOKEN", "GITHUB_TOKEN", "SSH_AUTH_SOCK", "HTTPS_PROXY"} {
			for _, name := range allowlist {
				if name == forbiddenDefault {
					t.Fatalf("provider %s inherited credential-bearing default %s", providerID, name)
				}
			}
		}
	}

	for _, malformed := range []string{"", "BAD=KEY", "9BAD", "BAD-NAME"} {
		_, err := NewFactoryV1ExternalRunner([]FactoryV1RunnerProvider{{Binding: author, EnvironmentAllowlist: []string{malformed}}}, 64*1024)
		if err == nil || !strings.Contains(err.Error(), "environment") {
			t.Fatalf("malformed key %q error = %v", malformed, err)
		}
	}
}
