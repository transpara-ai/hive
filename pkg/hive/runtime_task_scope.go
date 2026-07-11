package hive

import (
	"strings"

	"github.com/transpara-ai/eventgraph/go/pkg/event"
	"github.com/transpara-ai/eventgraph/go/pkg/types"
	"github.com/transpara-ai/work"
)

func (r *Runtime) oneShotTaskScope() func(types.EventID) bool {
	if r == nil || !r.isolateRunTasks {
		return nil
	}
	return r.taskInCurrentRun
}

func (r *Runtime) oneShotTaskWorkspace() string {
	if r == nil || !r.isolateRunTasks {
		return ""
	}
	return r.repoPath
}

// eventInCurrentRun scopes persistent role-proposal/approval/budget scans for
// one-shot runs. Daemons intentionally retain their global durable behavior.
func (r *Runtime) eventInCurrentRun(ev event.Event) bool {
	return r == nil || !r.isolateRunTasks || ev.ConversationID() == r.convID
}

// taskInCurrentRun proves that taskID is a Work creation event owned by this
// runtime conversation and repository. One-shot runs pass this predicate into
// every agent loop; a read/type/conversation/workspace mismatch fails closed.
func (r *Runtime) taskInCurrentRun(taskID types.EventID) bool {
	if r == nil || r.store == nil || taskID.IsZero() {
		return false
	}
	ev, err := r.store.Get(taskID)
	if err != nil || ev.ConversationID() != r.convID {
		return false
	}
	created, ok := ev.Content().(work.TaskCreatedContent)
	if !ok {
		return false
	}
	want := strings.TrimSpace(r.repoPath)
	got := strings.TrimSpace(created.Workspace)
	if want == "" {
		return got == ""
	}
	return got != "" && cleanSamePath(got, want)
}
