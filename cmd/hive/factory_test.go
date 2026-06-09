package main

import (
	"strings"
	"testing"

	"github.com/transpara-ai/hive/pkg/hive"
)

// TestFactoryVerbIsRegistered asserts the router knows the "factory" verb and,
// when invoked with no subcommand, returns a usage error that names it.
func TestFactoryVerbIsRegistered(t *testing.T) {
	err := routeAndDispatch([]string{"factory"})
	if err == nil || !strings.Contains(err.Error(), "factory") {
		t.Fatalf("expected factory usage error, got %v", err)
	}
}

// TestFactoryOrderRequiresHuman asserts the --human guard fires BEFORE any side
// effect: the spec path does not exist, so if validation ran in the wrong order
// we would see a file/store/loop error instead of a missing-human error.
func TestFactoryOrderRequiresHuman(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "order", "--spec", "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "human") {
		t.Fatalf("expected missing --human error, got %v", err)
	}
}

func TestParseFactoryOrderModelOverrideFlags(t *testing.T) {
	flags := factoryOrderModelOverrideFlags{}
	if err := flags.models.Set("guardian=api-sonnet"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := flags.authModes.Set("guardian=api-key"); err != nil {
		t.Fatalf("set auth-mode: %v", err)
	}
	if err := flags.requiredCapabilities.Set("guardian=reasoning,coding"); err != nil {
		t.Fatalf("set required capability: %v", err)
	}
	if err := flags.maxCosts.Set("guardian=0.25"); err != nil {
		t.Fatalf("set max cost: %v", err)
	}

	overrides, err := parseFactoryOrderModelOverrideFlags(flags)
	if err != nil {
		t.Fatalf("parseFactoryOrderModelOverrideFlags: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides = %+v, want one", overrides)
	}
	override := overrides[0]
	if override.Role != "guardian" || override.Model != "api-sonnet" || override.AuthMode != "api-key" {
		t.Fatalf("override = %+v, want guardian api-sonnet api-key", override)
	}
	if got := strings.Join(override.RequiredCapabilities, ","); got != "reasoning,coding" {
		t.Fatalf("required capabilities = %q, want reasoning,coding", got)
	}
	if override.MaxCostPerCallUSD == nil || *override.MaxCostPerCallUSD != 0.25 {
		t.Fatalf("max cost = %v, want 0.25", override.MaxCostPerCallUSD)
	}
}

func TestParseFactoryOrderModelOverrideFlagsRejectsDuplicateScalar(t *testing.T) {
	flags := factoryOrderModelOverrideFlags{}
	_ = flags.models.Set("guardian=sonnet")
	_ = flags.models.Set("guardian=opus")

	_, err := parseFactoryOrderModelOverrideFlags(flags)
	if err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("expected duplicate scalar error, got %v", err)
	}
}

func TestValidateFactoryOrderModelOverridesPreservesCanOperateGuardrail(t *testing.T) {
	_, err := validateFactoryOrderModelOverrides("", []hive.ModelOverrideRequest{
		{Role: "implementer", Model: "api-sonnet", AuthMode: "api-key"},
	})
	if err == nil || !strings.Contains(err.Error(), "CanOperate") {
		t.Fatalf("expected CanOperate guardrail error, got %v", err)
	}
}

// TestFactoryUnknownSubverb asserts an unrecognized subcommand is reported.
func TestFactoryUnknownSubverb(t *testing.T) {
	err := cmdFactory([]string{"frob"})
	if err == nil || !strings.Contains(err.Error(), "frob") {
		t.Fatalf("expected unknown-subverb error, got %v", err)
	}
}

// TestFactoryRequestPRRequiresRepo asserts request-pr validates required flags
// before opening a store or touching GitHub. With no --repo the command must
// fail fast with a flag-name error.
func TestFactoryRequestPRRequiresRepo(t *testing.T) {
	err := cmdFactory([]string{"request-pr"})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("expected missing --repo error, got %v", err)
	}
}

// TestFactoryRequestPRRequiresFlags asserts that request-pr validates all
// required flags before any store open or network access. requireFlags lists
// --repo first, so calling request-pr with NO flags must produce an error
// mentioning "repo" — the very first missing required flag.
func TestFactoryRequestPRRequiresFlags(t *testing.T) {
	err := routeAndDispatch([]string{"factory", "request-pr"})
	if err == nil || !strings.Contains(err.Error(), "repo") {
		t.Fatalf("expected missing-flag error mentioning repo, got %v", err)
	}
}

// TestFactoryCreatePRRequiresRequest asserts create-pr validates --request
// before opening a store or calling GitHub.
func TestFactoryCreatePRRequiresRequest(t *testing.T) {
	err := cmdFactory([]string{"create-pr"})
	if err == nil || !strings.Contains(err.Error(), "request") {
		t.Fatalf("expected missing --request error, got %v", err)
	}
}
