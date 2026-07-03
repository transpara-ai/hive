package hive

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/transpara-ai/work"
)

func TestTC5_AC6CivilizationAssemblyProjectedWorkTaskStatusGolden(t *testing.T) {
	const wantSHA256 = "515869eeff1fa54c2cd828c07f96fd31caad64e9a3b08a3a08625c4900c5bde2"

	got := civilizationAssemblyProjectedWorkTaskStatusGoldenBytes()
	if gotSHA256 := fmt.Sprintf("%x", sha256.Sum256(got)); gotSHA256 != wantSHA256 {
		t.Fatalf("civilizationAssemblyProjectedWorkTaskStatus golden SHA256 = %s, want %s", gotSHA256, wantSHA256)
	}
}

func TestTC5_AC6CivilizationAssemblyProjectedWorkTaskStatusUnknownsPreserveBarePrefix(t *testing.T) {
	for _, status := range []work.TaskStatus{"paused", "sp1b_junk_status"} {
		got := civilizationAssemblyProjectedWorkTaskStatus(work.TaskProjection{Status: status}, work.LegacyTaskProjection{})
		want := "work_task_" + string(status)
		if got != want {
			t.Fatalf("status %q projected as %q, want %q", status, got, want)
		}
	}
}

func civilizationAssemblyProjectedWorkTaskStatusGoldenBytes() []byte {
	var b strings.Builder
	for _, taskStatus := range civilizationAssemblyProjectedWorkTaskStatusGoldenTaskStatuses() {
		for _, legacyStatus := range civilizationAssemblyProjectedWorkTaskStatusGoldenLegacyStatuses() {
			for _, taskBlocked := range []bool{false, true} {
				for _, taskReady := range []bool{false, true} {
					for _, legacyBlocked := range []bool{false, true} {
						for _, legacyReady := range []bool{false, true} {
							taskProjection := work.TaskProjection{
								Status:  taskStatus,
								Blocked: taskBlocked,
								Ready:   taskReady,
							}
							legacyProjection := work.LegacyTaskProjection{
								Status:  legacyStatus,
								Blocked: legacyBlocked,
								Ready:   legacyReady,
							}
							fmt.Fprintf(
								&b,
								"task_status=%q legacy_status=%q task_blocked=%t task_ready=%t legacy_blocked=%t legacy_ready=%t output=%q\n",
								taskStatus,
								legacyStatus,
								taskBlocked,
								taskReady,
								legacyBlocked,
								legacyReady,
								civilizationAssemblyProjectedWorkTaskStatus(taskProjection, legacyProjection),
							)
						}
					}
				}
			}
		}
	}
	return []byte(b.String())
}

func civilizationAssemblyProjectedWorkTaskStatusGoldenTaskStatuses() []work.TaskStatus {
	return []work.TaskStatus{
		"",
		work.StatusCreated,
		work.StatusReady,
		work.StatusRunning,
		work.StatusBlocked,
		work.StatusFailed,
		work.StatusRepairRequired,
		work.StatusRepairRunning,
		work.StatusRepaired,
		work.StatusVerificationRunning,
		work.StatusVerified,
		work.StatusCertified,
		work.StatusRejected,
		work.StatusSuperseded,
		work.StatusPolicyBlocked,
		"paused",
		"sp1b_junk_status",
	}
}

func civilizationAssemblyProjectedWorkTaskStatusGoldenLegacyStatuses() []work.LegacyTaskStatus {
	return []work.LegacyTaskStatus{
		"",
		work.LegacyStatusPending,
		work.LegacyStatusAssigned,
		work.LegacyStatusReady,
		work.LegacyStatusBlocked,
		work.LegacyStatusCompleted,
	}
}
