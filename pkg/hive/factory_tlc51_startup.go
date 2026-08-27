package hive

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const FactoryTLC51DevelopmentBearer = "dev"

type FactoryTLC51StartupMode string

const (
	FactoryTLC51DevelopmentMode FactoryTLC51StartupMode = "development"
	FactoryTLC51EnforcementMode FactoryTLC51StartupMode = "enforcement"
	FactoryTLC51CanaryMode      FactoryTLC51StartupMode = "canary"
)

// FactoryTLC51AuthorityBinding is the authenticated Human authority identity
// admitted at startup. It carries no credential material and is insufficient
// by itself to authorize an effect; the TLC effect boundary still revalidates
// a fresh exact-subject authority record immediately before invocation.
type FactoryTLC51AuthorityBinding struct {
	RecordID           string
	RecordDigest       string
	AuthenticationRef  string
	ActorID            string
	SigningKeyID       string
	DesignGitBlob      string
	ImplementationHead string
	Effect             string
	SubjectDigest      string
	ObservedAt         time.Time
	ExpiresAt          time.Time
}

// FactoryTLC51StartupConfig is a value-only admission contract. Validation is
// deliberately separate from process startup so tests and operators can prove
// fail-closed behavior without activating a worker or protected credential.
type FactoryTLC51StartupConfig struct {
	Mode                       FactoryTLC51StartupMode
	BearerToken                string
	SigningKeyID               string
	SigningKeyActorDerived     bool
	AuthorCredentialSourceID   string
	HumanCredentialSourceID    string
	SecretFiles                []string
	WorkerGroup                string
	HasProductionCredential    bool
	HasGitHubCredential        bool
	HasEventGraphCredential    bool
	HasProtectedCredential     bool
	ExpectedDesignGitBlob      string
	ExpectedImplementationHead string
	ExpectedEffect             string
	ExpectedSubjectDigest      string
	Authority                  FactoryTLC51AuthorityBinding
	Now                        time.Time
}

// ValidateFactoryTLC51Startup refuses unsafe development fallbacks and exact-
// subject authority drift. It performs no startup, credential acquisition, or
// protected effect.
func ValidateFactoryTLC51Startup(config FactoryTLC51StartupConfig) error {
	switch config.Mode {
	case FactoryTLC51DevelopmentMode:
		if strings.EqualFold(strings.TrimSpace(config.WorkerGroup), "canary") {
			return errors.New("TLC 5.1 development mode cannot join the canary worker group")
		}
		if config.HasProductionCredential || config.HasGitHubCredential || config.HasEventGraphCredential || config.HasProtectedCredential {
			return errors.New("TLC 5.1 development mode cannot hold production, GitHub, EventGraph, or protected-effect credentials")
		}
		return nil
	case FactoryTLC51EnforcementMode, FactoryTLC51CanaryMode:
	default:
		return fmt.Errorf("unknown TLC 5.1 startup mode %q", config.Mode)
	}

	if strings.TrimSpace(config.BearerToken) == "" || config.BearerToken == FactoryTLC51DevelopmentBearer {
		return errors.New("TLC 5.1 enforcement/canary requires an explicit non-development bearer")
	}
	if strings.TrimSpace(config.SigningKeyID) == "" {
		return errors.New("TLC 5.1 enforcement/canary requires an explicit signing key id")
	}
	if config.SigningKeyActorDerived {
		return errors.New("TLC 5.1 enforcement/canary rejects actor-derived signing keys")
	}
	if strings.TrimSpace(config.AuthorCredentialSourceID) == "" || strings.TrimSpace(config.HumanCredentialSourceID) == "" {
		return errors.New("TLC 5.1 enforcement/canary requires explicit author and Human credential sources")
	}
	if config.AuthorCredentialSourceID == config.HumanCredentialSourceID {
		return errors.New("TLC 5.1 author and Human credentials must not share a source")
	}
	if len(config.SecretFiles) == 0 {
		return errors.New("TLC 5.1 enforcement/canary requires explicit secret files")
	}
	for _, path := range config.SecretFiles {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("TLC 5.1 secret file %q: %w", path, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
			return fmt.Errorf("TLC 5.1 secret file %q must be a non-symlink regular file with mode 0600", path)
		}
	}
	if err := validateFactoryTLC51AuthorityBinding(config); err != nil {
		return err
	}
	return nil
}

func validateFactoryTLC51AuthorityBinding(config FactoryTLC51StartupConfig) error {
	authority := config.Authority
	if !validFactoryTLC51Digest(authority.RecordDigest) || strings.TrimSpace(authority.RecordID) == "" || strings.TrimSpace(authority.AuthenticationRef) == "" || strings.TrimSpace(authority.ActorID) == "" {
		return errors.New("TLC 5.1 startup authority lacks authenticated record identity")
	}
	if authority.SigningKeyID == "" || authority.SigningKeyID != config.SigningKeyID {
		return errors.New("TLC 5.1 startup authority signing key does not match the configured key id")
	}
	if !validFactoryTLC51GitObject(config.ExpectedDesignGitBlob) || !validFactoryTLC51GitObject(config.ExpectedImplementationHead) || !validFactoryTLC51Digest(config.ExpectedSubjectDigest) || strings.TrimSpace(config.ExpectedEffect) == "" {
		return errors.New("TLC 5.1 expected design/head/effect subject is incomplete")
	}
	if authority.DesignGitBlob != config.ExpectedDesignGitBlob || authority.ImplementationHead != config.ExpectedImplementationHead || authority.Effect != config.ExpectedEffect || authority.SubjectDigest != config.ExpectedSubjectDigest {
		return errors.New("TLC 5.1 startup authority is not bound to the exact design, head, effect, and subject")
	}
	if config.Now.IsZero() || config.Now.Location() != time.UTC || authority.ObservedAt.IsZero() || authority.ObservedAt.Location() != time.UTC || authority.ExpiresAt.IsZero() || authority.ExpiresAt.Location() != time.UTC {
		return errors.New("TLC 5.1 startup authority requires explicit UTC observation, expiry, and validation times")
	}
	if authority.ObservedAt.After(config.Now) || !authority.ExpiresAt.After(config.Now) {
		return errors.New("TLC 5.1 startup authority is future-observed or expired")
	}
	return nil
}

func validFactoryTLC51GitObject(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || (character > '9' && character < 'a') || character > 'f' {
			return false
		}
	}
	return true
}
