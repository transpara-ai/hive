package hive

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"strings"

	hiveagent "github.com/transpara-ai/agent"
)

// AgentIdentityEnvironment identifies the operating environment for an agent
// identity. The zero value is treated as production.
type AgentIdentityEnvironment string

const (
	AgentIdentityEnvironmentProduction  AgentIdentityEnvironment = "production"
	AgentIdentityEnvironmentDevelopment AgentIdentityEnvironment = "development"
	AgentIdentityEnvironmentTest        AgentIdentityEnvironment = "test"
)

// AgentIdentityMode identifies the key provenance path Hive requests.
type AgentIdentityMode string

const (
	AgentIdentityModeGenerated            AgentIdentityMode = "generated"
	AgentIdentityModeExternallyManaged    AgentIdentityMode = "externally_managed"
	AgentIdentityModeDeterministicFixture AgentIdentityMode = "deterministic_fixture"
)

// KeyProvenanceCategory is recorded in EventGraph identity registration facts.
type KeyProvenanceCategory string

const (
	KeyProvenanceGenerated            KeyProvenanceCategory = "generated"
	KeyProvenanceExternallyManaged    KeyProvenanceCategory = "externally_managed"
	KeyProvenanceDeterministicFixture KeyProvenanceCategory = "deterministic_fixture"
)

type preparedAgentIdentity struct {
	Environment AgentIdentityEnvironment
	Mode        AgentIdentityMode
	Provenance  KeyProvenanceCategory
	AgentEnv    hiveagent.IdentityEnvironment
	AgentMode   hiveagent.IdentityMode
	SigningKey  ed25519.PrivateKey
}

func prepareAgentIdentity(def AgentDef) (preparedAgentIdentity, error) {
	env := def.IdentityEnvironment
	if env == "" {
		env = AgentIdentityEnvironmentProduction
	}
	mode := def.IdentityMode
	if mode == "" {
		mode = AgentIdentityModeGenerated
	}

	agentEnv, err := mapAgentIdentityEnvironment(env)
	if err != nil {
		return preparedAgentIdentity{}, err
	}

	switch mode {
	case AgentIdentityModeGenerated:
		if def.SigningKey != nil {
			return preparedAgentIdentity{}, fmt.Errorf("agent identity: generated mode must not supply SigningKey; use externally_managed")
		}
		return preparedAgentIdentity{
			Environment: env,
			Mode:        mode,
			Provenance:  KeyProvenanceGenerated,
			AgentEnv:    agentEnv,
			AgentMode:   hiveagent.IdentityModeGenerated,
		}, nil

	case AgentIdentityModeExternallyManaged:
		if strings.TrimSpace(def.ExternalKeyRef) == "" {
			return preparedAgentIdentity{}, fmt.Errorf("agent identity: externally_managed mode requires ExternalKeyRef")
		}
		if len(def.SigningKey) != ed25519.PrivateKeySize {
			return preparedAgentIdentity{}, fmt.Errorf("agent identity: externally_managed mode requires an Ed25519 SigningKey from an approved key surface")
		}
		if env == AgentIdentityEnvironmentProduction && isPublicNameDerivedSigningKey(def.Name, def.SigningKey) {
			return preparedAgentIdentity{}, fmt.Errorf("agent identity: public-name-derived identity is blocked in production")
		}
		return preparedAgentIdentity{
			Environment: env,
			Mode:        mode,
			Provenance:  KeyProvenanceExternallyManaged,
			AgentEnv:    agentEnv,
			AgentMode:   hiveagent.IdentityModeGenerated,
			SigningKey:  def.SigningKey,
		}, nil

	case AgentIdentityModeDeterministicFixture:
		if env == AgentIdentityEnvironmentProduction {
			return preparedAgentIdentity{}, fmt.Errorf("agent identity: deterministic fixture identity is blocked in production")
		}
		return preparedAgentIdentity{
			Environment: env,
			Mode:        mode,
			Provenance:  KeyProvenanceDeterministicFixture,
			AgentEnv:    agentEnv,
			AgentMode:   hiveagent.IdentityModeDeterministic,
		}, nil

	default:
		return preparedAgentIdentity{}, fmt.Errorf("agent identity: unsupported identity mode %q", mode)
	}
}

func mapAgentIdentityEnvironment(env AgentIdentityEnvironment) (hiveagent.IdentityEnvironment, error) {
	switch env {
	case AgentIdentityEnvironmentProduction:
		return hiveagent.IdentityEnvironmentProduction, nil
	case AgentIdentityEnvironmentDevelopment:
		return hiveagent.IdentityEnvironmentDevelopment, nil
	case AgentIdentityEnvironmentTest:
		return hiveagent.IdentityEnvironmentTest, nil
	default:
		return "", fmt.Errorf("agent identity: unsupported identity environment %q", env)
	}
}

func isPublicNameDerivedSigningKey(name string, key ed25519.PrivateKey) bool {
	if len(key) != ed25519.PrivateKeySize {
		return false
	}
	seed := sha256.Sum256([]byte("agent:" + name))
	deterministic := ed25519.NewKeyFromSeed(seed[:])
	return bytes.Equal(key.Public().(ed25519.PublicKey), deterministic.Public().(ed25519.PublicKey))
}
