package hive

import (
	"context"
	"os"

	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

// DraftPRArtifact bundles the produced artifact's PR content with its target.
type DraftPRArtifact struct {
	Target         DraftPRTarget
	Title          string
	Body           string
	ChangedFiles   []string
	ActorRole      string
	DeciderActorID string
	DeciderRole    string
}

// CreateDraftPRFromApprovedDecision builds Epic 11 options from an approved
// draft-PR decision and runs the work creator. hive orchestrates; work performs
// the (real) GitHub mutation through the supplied client.
// causes are the causal event IDs from the current chain head; pass nil when
// no prior causes are needed (the task store will reject an empty chain if the
// underlying store requires at least one cause).
func CreateDraftPRFromApprovedDecision(ctx context.Context, ts *work.TaskStore, source types.ActorID, conv types.ConversationID, client work.Epic11PullRequestCreator, art DraftPRArtifact, causes ...types.EventID) (work.Epic11DocsDraftPRRun, error) {
	dir, err := epic11WorkingDir()
	if err != nil {
		return work.Epic11DocsDraftPRRun{}, err
	}
	opts := work.BuildEpic11DocsDraftPROptions(work.Epic11OptionsInput{
		Source:         source,
		ConversationID: conv,
		Causes:         causes,
		WorkingDir:     dir,
		Client:         client,
		Target: work.Epic11DraftPullRequestTarget{
			Repository:             art.Target.Repository,
			BaseRef:                art.Target.BaseRef,
			BaseSHA:                art.Target.BaseSHA,
			HeadRef:                art.Target.HeadRef,
			HeadSHA:                art.Target.HeadSHA,
			HeadExistsOnOrigin:     true,
			Title:                  art.Title,
			Body:                   art.Body,
			ChangedFiles:           art.ChangedFiles,
			ValidationEvidenceRefs: []string{"make verify"},
			Draft:                  true,
			MaintainerCanModify:    true,
			RollbackInstructions:   "Manual rollback only: human may close the draft PR after a separately authorized mutation.",
		},
		ActorRole:      art.ActorRole,
		DeciderActorID: art.DeciderActorID,
		DeciderRole:    art.DeciderRole,
		SingleUseNonce: art.Target.SingleUseNonce,
	})
	return work.RunEpic11DocsDraftPRLiveMutation(ctx, ts, opts)
}

// epic11WorkingDir returns a fresh per-run writable directory for Epic 11
// evidence files. Returns an error if the OS cannot create the directory.
func epic11WorkingDir() (string, error) {
	dir, err := os.MkdirTemp("", "epic11-*")
	if err != nil {
		return "", err
	}
	return dir, nil
}
