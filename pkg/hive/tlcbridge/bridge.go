// Package tlcbridge binds a portable TLC result to Hive-owned source and
// repository-effect identity.
//
// TLC owns routing and brief content. Hive owns source provenance, durable
// state, retries, provider execution, worktrees, and protected effects. This
// package intentionally does not reproduce TLC route policy.
package tlcbridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const EnvelopeSchemaVersion = "tlc-envelope/v1"

// Workflow identifies the external workflow that produced the envelope.
type Workflow struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Brief is the small stable projection Hive and Site need for execution and
// display. Additional TLC-owned data remains intact in CanonicalJSON.
type Brief struct {
	Outcome     string   `json:"outcome"`
	Scope       []string `json:"scope"`
	NonGoals    []string `json:"non_goals"`
	Assumptions []string `json:"assumptions"`
	Constraints []string `json:"constraints"`
	Tests       []string `json:"tests"`
	NextAction  string   `json:"next_action"`
}

// Envelope is the intentionally small transport projection understood by
// Hive. Route-specific policy, evidence, and future fields are not modeled
// here; they are preserved byte-for-byte semantically in CanonicalJSON.
type Envelope struct {
	SchemaVersion string   `json:"schema_version"`
	Workflow      Workflow `json:"workflow"`
	Route         string   `json:"route"`
	Brief         Brief    `json:"brief"`
}

type SourceKind string

const (
	SourceHuman SourceKind = "human"
	SourceIssue SourceKind = "issue"
	SourceOrder SourceKind = "factory_order"
)

// Source is captured by trusted Hive ingress, never accepted from the TLC
// envelope.
type Source struct {
	Kind       SourceKind `json:"kind"`
	Identity   string     `json:"identity"`
	Repository string     `json:"repository"`
}

type RepositoryEffects struct {
	WorktreeRepository    string `json:"worktree_repository"`
	PullRequestRepository string `json:"pull_request_repository"`
}

// BoundRequest is a pure result. Binding performs no persistence, provider
// invocation, repository mutation, pull-request action, or merge.
type BoundRequest struct {
	Source         Source            `json:"source"`
	Envelope       Envelope          `json:"envelope"`
	CanonicalJSON  json.RawMessage   `json:"canonical_json"`
	Effects        RepositoryEffects `json:"effects"`
	IdempotencyKey string            `json:"idempotency_key"`
}

var repositoryComponent = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Bind validates only the stable transport fields needed by Hive, preserves
// all TLC-owned fields in canonical JSON, and derives repository effects from
// independently captured source provenance.
func Bind(source Source, raw []byte) (BoundRequest, error) {
	source.Identity = strings.TrimSpace(source.Identity)
	source.Repository = strings.TrimSpace(source.Repository)
	if err := validateSource(source); err != nil {
		return BoundRequest{}, err
	}

	canonical, err := canonicalJSONObject(raw)
	if err != nil {
		return BoundRequest{}, fmt.Errorf("TLC envelope: %w", err)
	}
	var envelope Envelope
	if err := json.Unmarshal(canonical, &envelope); err != nil {
		return BoundRequest{}, fmt.Errorf("decode TLC envelope projection: %w", err)
	}
	normalizeEnvelope(&envelope)
	if err := validateEnvelope(envelope); err != nil {
		return BoundRequest{}, err
	}

	digestInput := strings.Join([]string{
		string(source.Kind), source.Identity, source.Repository, string(canonical),
	}, "\x00")
	digest := sha256.Sum256([]byte(digestInput))
	return BoundRequest{
		Source:        source,
		Envelope:      envelope,
		CanonicalJSON: append(json.RawMessage(nil), canonical...),
		Effects: RepositoryEffects{
			WorktreeRepository:    source.Repository,
			PullRequestRepository: source.Repository,
		},
		IdempotencyKey: "tlc-envelope-v1-" + hex.EncodeToString(digest[:]),
	}, nil
}

func validateSource(source Source) error {
	switch source.Kind {
	case SourceHuman, SourceIssue, SourceOrder:
	default:
		return fmt.Errorf("unsupported source kind %q", source.Kind)
	}
	if source.Identity == "" {
		return errors.New("source identity is required")
	}
	owner, name, ok := strings.Cut(source.Repository, "/")
	if !ok || strings.Contains(name, "/") || owner != "transpara-ai" ||
		!repositoryComponent.MatchString(owner) || !repositoryComponent.MatchString(name) {
		return fmt.Errorf("source repository %q is not a valid transpara-ai repository", source.Repository)
	}
	return nil
}

func validateEnvelope(envelope Envelope) error {
	if envelope.SchemaVersion != EnvelopeSchemaVersion {
		return fmt.Errorf("unsupported TLC transport schema %q", envelope.SchemaVersion)
	}
	if envelope.Workflow.Name != "transpara-tlc" || envelope.Workflow.Version == "" {
		return errors.New("TLC workflow name and version are required")
	}
	if envelope.Route == "" {
		return errors.New("TLC route is required")
	}
	if envelope.Brief.Outcome == "" || envelope.Brief.NextAction == "" {
		return errors.New("TLC brief outcome and next action are required")
	}
	if envelope.Brief.Tests == nil {
		return errors.New("TLC brief tests must be an array")
	}
	return nil
}

func normalizeEnvelope(envelope *Envelope) {
	envelope.SchemaVersion = strings.TrimSpace(envelope.SchemaVersion)
	envelope.Workflow.Name = strings.TrimSpace(envelope.Workflow.Name)
	envelope.Workflow.Version = strings.TrimSpace(envelope.Workflow.Version)
	envelope.Route = strings.TrimSpace(envelope.Route)
	envelope.Brief.Outcome = strings.TrimSpace(envelope.Brief.Outcome)
	envelope.Brief.NextAction = strings.TrimSpace(envelope.Brief.NextAction)
	for _, values := range [][]string{
		envelope.Brief.Scope,
		envelope.Brief.NonGoals,
		envelope.Brief.Assumptions,
		envelope.Brief.Constraints,
		envelope.Brief.Tests,
	} {
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}
	}
}

func canonicalJSONObject(raw []byte) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("document is empty")
	}
	if err := rejectDuplicateObjectKeys(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, errors.New("document must be one JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("document contains multiple JSON values")
		}
		return nil, err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func rejectDuplicateObjectKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("document contains multiple JSON values")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}
