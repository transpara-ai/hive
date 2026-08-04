package factoryv1

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Stage string

const (
	StageIngestWork        Stage = "ingest_work"
	StageCraftFactoryOrder Stage = "craft_factory_order"
	StageDesign            Stage = "design"
	StageIADA              Stage = "iada"
	StageCFADA             Stage = "cfada"
	StageHumanDesignReview Stage = "human_design_review"
	StageWriteCode         Stage = "write_code"
	StageCreateDraftPR     Stage = "create_draft_pr"
	StageIAR               Stage = "iar"
	StageCFAR              Stage = "cfar"
	StageMarkPRReady       Stage = "mark_pr_ready"
	StageHumanReview       Stage = "human_review"
)

var TLCStages = []Stage{
	StageIngestWork,
	StageCraftFactoryOrder,
	StageDesign,
	StageIADA,
	StageCFADA,
	StageHumanDesignReview,
	StageWriteCode,
	StageCreateDraftPR,
	StageIAR,
	StageCFAR,
	StageMarkPRReady,
	StageHumanReview,
}

var defaultPeers = map[Stage][]string{
	StageIngestWork:        {"intake", "archivist"},
	StageCraftFactoryOrder: {"planner", "reviewer"},
	StageDesign:            {"planner", "reviewer", "guardian"},
	StageIADA:              {"reviewer"},
	StageCFADA:             {"independent_reviewer", "guardian"},
	StageHumanDesignReview: {"human", "guardian"},
	StageWriteCode:         {"implementer", "tester"},
	StageCreateDraftPR:     {"implementer", "guardian"},
	StageIAR:               {"reviewer"},
	StageCFAR:              {"independent_reviewer", "guardian"},
	StageMarkPRReady:       {"guardian", "reviewer"},
	StageHumanReview:       {"human"},
}

func PeersForStage(stage Stage) []string {
	return append([]string(nil), defaultPeers[stage]...)
}

func StageIndex(stage Stage) int {
	for i, candidate := range TLCStages {
		if stage == candidate {
			return i
		}
	}
	return -1
}

func NextStage(completed []Stage) (Stage, bool) {
	if len(completed) >= len(TLCStages) {
		return "", false
	}
	for i, stage := range completed {
		if stage != TLCStages[i] {
			return "", false
		}
	}
	return TLCStages[len(completed)], true
}

type RunnerStatus string

const (
	RunnerPassed        RunnerStatus = "passed"
	RunnerBlocked       RunnerStatus = "blocked"
	RunnerHumanRequired RunnerStatus = "human_required"
)

func (s RunnerStatus) valid() bool {
	return s == RunnerPassed || s == RunnerBlocked || s == RunnerHumanRequired
}

type TransitionState string

const (
	TransitionRunning       TransitionState = "running"
	TransitionPassed        TransitionState = "passed"
	TransitionBlocked       TransitionState = "blocked"
	TransitionHumanRequired TransitionState = "human_required"
)

type Evidence struct {
	Kind            string                `json:"kind"`
	Reference       string                `json:"reference"`
	SHA256          string                `json:"sha256,omitempty"`
	DesignBlobSHA   string                `json:"design_blob_sha,omitempty"`
	PRHeadSHA       string                `json:"pr_head_sha,omitempty"`
	ReviewedHeadSHA string                `json:"reviewed_head_sha,omitempty"`
	AuthorFamily    string                `json:"author_family,omitempty"`
	ReviewerFamily  string                `json:"reviewer_family,omitempty"`
	BlockerCount    *int                  `json:"blocker_count,omitempty"`
	Provider        *ProviderBinding      `json:"provider,omitempty"`
	PR              *PREvidence           `json:"pr,omitempty"`
	Approval        *HumanApprovalReceipt `json:"approval,omitempty"`
	Metadata        map[string]string     `json:"metadata,omitempty"`
}

type ProviderBinding struct {
	ProviderID         string `json:"provider_id"`
	Family             string `json:"family"`
	ExecutableRealpath string `json:"executable_realpath"`
	ExecutableSHA256   string `json:"executable_sha256"`
	ModelID            string `json:"model_id"`
	CredentialSourceID string `json:"credential_source_id"`
}

type PREvidence struct {
	Repository      string `json:"repository"`
	Number          int    `json:"number"`
	URL             string `json:"url"`
	HeadSHA         string `json:"head_sha"`
	ReviewedHeadSHA string `json:"reviewed_head_sha"`
	Open            bool   `json:"open"`
	Draft           bool   `json:"draft"`
	ChecksPassing   bool   `json:"checks_passing"`
}

type Usage struct {
	Tokens     int64 `json:"tokens"`
	CostMicros int64 `json:"cost_micros"`
}

type StageTransitionPayload struct {
	TLCVersion     string          `json:"tlc_version"`
	Stage          Stage           `json:"stage"`
	StageIndex     int             `json:"stage_index"`
	State          TransitionState `json:"state"`
	AttemptID      string          `json:"attempt_id"`
	Ordinal        int             `json:"ordinal"`
	Peers          []string        `json:"peers"`
	Evidence       []Evidence      `json:"evidence"`
	Blocker        string          `json:"blocker,omitempty"`
	NextAction     string          `json:"next_action,omitempty"`
	Usage          Usage           `json:"usage"`
	Runner         ProviderBinding `json:"runner"`
	Recovered      bool            `json:"recovered"`
	WorkArtifactID string          `json:"work_artifact_id,omitempty"`
}

func AttemptID(documentSHA256 string, stage Stage, ordinal int) (string, error) {
	if !hexPattern.MatchString(documentSHA256) {
		return "", errors.New("attempt document hash must be 64 lowercase hexadecimal characters")
	}
	if StageIndex(stage) < 0 {
		return "", fmt.Errorf("unknown TLC stage %q", stage)
	}
	if ordinal <= 0 {
		return "", errors.New("attempt ordinal must be positive")
	}
	input := TLCVersion + "\x00" + documentSHA256 + "\x00" + string(stage) + "\x00" + strconv.Itoa(ordinal)
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:]), nil
}

// ValidateTransitionForDocument is the strict public validator used by durable
// adapters; it validates the attempt hash as well as stage ordering.
func ValidateTransitionForDocument(documentSHA256 string, previous []StageTransitionPayload, transition StageTransitionPayload) error {
	if !hexPattern.MatchString(documentSHA256) {
		return errors.New("document hash is invalid")
	}
	expected, err := AttemptID(documentSHA256, transition.Stage, transition.Ordinal)
	if err != nil {
		return err
	}
	if expected != transition.AttemptID {
		return errors.New("transition attempt identity does not match delimited tlc-v1 identity")
	}
	// Avoid the hash inference fallback in ValidateTransition.
	copy := append([]StageTransitionPayload(nil), previous...)
	if err := validateTransitionOrder(copy, transition); err != nil {
		return err
	}
	return nil
}

func validateTransitionOrder(previous []StageTransitionPayload, transition StageTransitionPayload) error {
	index := StageIndex(transition.Stage)
	if index < 0 {
		return fmt.Errorf("unknown TLC stage %q", transition.Stage)
	}
	if transition.TLCVersion != TLCVersion || transition.StageIndex != index {
		return errors.New("transition TLC version or stage index mismatch")
	}
	if transition.Ordinal <= 0 || strings.TrimSpace(transition.AttemptID) == "" {
		return errors.New("transition attempt identity and ordinal are required")
	}
	passed := passedStages(previous)
	expected, hasNext := NextStage(passed)
	if !hasNext || expected != transition.Stage {
		return fmt.Errorf("out-of-order TLC transition: expected %q, got %q", expected, transition.Stage)
	}
	if transition.State != TransitionRunning && transition.State != TransitionPassed && transition.State != TransitionBlocked && transition.State != TransitionHumanRequired {
		return fmt.Errorf("invalid transition state %q", transition.State)
	}
	if transition.State != TransitionRunning && len(transition.Evidence) == 0 {
		return errors.New("terminal transition must cite durable evidence")
	}
	if transition.State == TransitionPassed {
		return validateStageEvidence(transition.Stage, transition.Evidence)
	}
	return nil
}

func passedStages(transitions []StageTransitionPayload) []Stage {
	passed := make([]Stage, 0, len(TLCStages))
	for _, stage := range TLCStages {
		for _, transition := range transitions {
			if transition.Stage == stage && transition.State == TransitionPassed {
				passed = append(passed, stage)
				break
			}
		}
		if len(passed) == 0 || passed[len(passed)-1] != stage {
			break
		}
	}
	return passed
}

func validateStageEvidence(stage Stage, evidence []Evidence) error {
	for _, item := range evidence {
		if strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(item.Reference) == "" {
			return errors.New("evidence kind and reference are required")
		}
	}
	if stage == StageIADA || stage == StageCFADA || stage == StageIAR || stage == StageCFAR {
		for _, item := range evidence {
			if item.BlockerCount != nil && *item.BlockerCount == 0 {
				if (stage == StageIADA || stage == StageCFADA) && !gitHashPattern.MatchString(item.DesignBlobSHA) {
					continue
				}
				if (stage == StageIAR || stage == StageCFAR) && (!gitHashPattern.MatchString(item.PRHeadSHA) || item.PRHeadSHA != item.ReviewedHeadSHA) {
					continue
				}
				if (stage == StageCFADA || stage == StageCFAR) && (item.AuthorFamily == "" || item.ReviewerFamily == "" || item.AuthorFamily == item.ReviewerFamily) {
					return errors.New("cross-family gate must bind distinct author and reviewer families")
				}
				return nil
			}
		}
		return errors.New("review gate evidence must report blocker count zero and bind the exact design blob or reviewed PR head")
	}
	if stage == StageDesign {
		for _, item := range evidence {
			if gitHashPattern.MatchString(item.DesignBlobSHA) {
				return nil
			}
		}
		return errors.New("design stage must bind an exact design blob")
	}
	if stage == StageWriteCode {
		for _, item := range evidence {
			if item.Kind == "code" && gitHashPattern.MatchString(item.PRHeadSHA) && item.Metadata["branch"] != "" && item.Metadata["tests_passing"] == "true" {
				return nil
			}
		}
		return errors.New("write_code must bind deterministic branch/head and passing named validation")
	}
	if stage == StageCreateDraftPR {
		for _, item := range evidence {
			if item.PR != nil && item.PR.Repository != "" && item.PR.Number > 0 && item.PR.URL != "" && item.PR.Open && item.PR.Draft && gitHashPattern.MatchString(item.PR.HeadSHA) {
				return nil
			}
		}
		return errors.New("create_draft_pr requires one open draft PR at the exact implementation head")
	}
	if stage == StageMarkPRReady {
		for _, item := range evidence {
			if item.PR != nil {
				return ValidateReadyPR(*item.PR)
			}
		}
		return errors.New("mark_pr_ready requires PR evidence")
	}
	return nil
}

func ValidateReadyPR(pr PREvidence) error {
	if pr.Repository == "" || pr.Number <= 0 || pr.URL == "" {
		return errors.New("PR repository, number, and URL are required")
	}
	if !pr.Open || pr.Draft || !pr.ChecksPassing {
		return errors.New("PR must be open, non-draft, and have passing required checks")
	}
	if !gitHashPattern.MatchString(pr.HeadSHA) || pr.HeadSHA != pr.ReviewedHeadSHA {
		return errors.New("PR head must equal the exact reviewed head")
	}
	return nil
}
