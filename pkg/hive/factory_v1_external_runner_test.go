package hive

import (
	"context"
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
