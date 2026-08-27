package hive

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validFactoryTLC51StartupFixture(t *testing.T) FactoryTLC51StartupConfig {
	t.Helper()
	secret := filepath.Join(t.TempDir(), "tlc51-secret")
	if err := os.WriteFile(secret, []byte("test-only\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 14, 0, 0, 0, time.UTC)
	config := FactoryTLC51StartupConfig{
		Mode: FactoryTLC51CanaryMode, BearerToken: "explicit-test-bearer", SigningKeyID: "key-51",
		AuthorCredentialSourceID: "author-source", HumanCredentialSourceID: "human-source", SecretFiles: []string{secret}, WorkerGroup: "tlc51-canary",
		ExpectedDesignGitBlob: strings.Repeat("a", 40), ExpectedImplementationHead: strings.Repeat("b", 40),
		ExpectedEffect: "runtime_activation", ExpectedSubjectDigest: strings.Repeat("c", 64), Now: now,
	}
	config.Authority = FactoryTLC51AuthorityBinding{
		RecordID: "authority-1", RecordDigest: strings.Repeat("d", 64), AuthenticationRef: "provider://authority/1",
		ActorID: "human-1", SigningKeyID: config.SigningKeyID, DesignGitBlob: config.ExpectedDesignGitBlob,
		ImplementationHead: config.ExpectedImplementationHead, Effect: config.ExpectedEffect, SubjectDigest: config.ExpectedSubjectDigest,
		ObservedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
	}
	return config
}

func TestFactoryTLC51StartupAcceptsExactCanaryBinding(t *testing.T) {
	if err := ValidateFactoryTLC51Startup(validFactoryTLC51StartupFixture(t)); err != nil {
		t.Fatalf("exact canary binding: %v", err)
	}
}

func TestFactoryTLC51StartupRejectsCredentialAndAuthorityFallbacks(t *testing.T) {
	tests := []struct {
		name string
		edit func(*FactoryTLC51StartupConfig)
		want string
	}{
		{"development bearer", func(c *FactoryTLC51StartupConfig) { c.BearerToken = "dev" }, "non-development bearer"},
		{"actor-derived key", func(c *FactoryTLC51StartupConfig) { c.SigningKeyActorDerived = true }, "actor-derived"},
		{"absent key id", func(c *FactoryTLC51StartupConfig) { c.SigningKeyID = "" }, "signing key id"},
		{"shared Human credential", func(c *FactoryTLC51StartupConfig) { c.HumanCredentialSourceID = c.AuthorCredentialSourceID }, "must not share"},
		{"wrong authority head", func(c *FactoryTLC51StartupConfig) { c.Authority.ImplementationHead = strings.Repeat("e", 40) }, "exact design, head, effect, and subject"},
		{"expired authority", func(c *FactoryTLC51StartupConfig) { c.Authority.ExpiresAt = c.Now }, "future-observed or expired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := validFactoryTLC51StartupFixture(t)
			test.edit(&config)
			if err := ValidateFactoryTLC51Startup(config); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestFactoryTLC51StartupRejectsNon0600Secret(t *testing.T) {
	config := validFactoryTLC51StartupFixture(t)
	if err := os.Chmod(config.SecretFiles[0], 0o640); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFactoryTLC51Startup(config); err == nil || !strings.Contains(err.Error(), "mode 0600") {
		t.Fatalf("error = %v, want mode 0600 rejection", err)
	}
}

func TestFactoryTLC51DevelopmentCannotCarryLiveCredentialsOrJoinCanary(t *testing.T) {
	config := FactoryTLC51StartupConfig{Mode: FactoryTLC51DevelopmentMode, WorkerGroup: "canary"}
	if err := ValidateFactoryTLC51Startup(config); err == nil || !strings.Contains(err.Error(), "cannot join") {
		t.Fatalf("canary group error = %v", err)
	}
	config.WorkerGroup = "local"
	config.HasEventGraphCredential = true
	if err := ValidateFactoryTLC51Startup(config); err == nil || !strings.Contains(err.Error(), "cannot hold") {
		t.Fatalf("credential error = %v", err)
	}
}
