package main

import (
	"strings"
	"testing"
)

func TestFactoryV1DaemonRequiresHumanBeforeRuntimeMutation(t *testing.T) {
	err := cmdFactoryV1([]string{"daemon"})
	if err == nil || !strings.Contains(err.Error(), "--human") {
		t.Fatalf("error = %v, want --human requirement", err)
	}
}

func TestFactoryV1RouteExposesDaemonHelp(t *testing.T) {
	if err := routeAndDispatch([]string{"factory-v1", "--help"}); err != nil {
		t.Fatalf("factory-v1 help: %v", err)
	}
}

func TestFactoryV1DaemonIssueScanRequiresBoundedReviewQueueThreshold(t *testing.T) {
	for _, threshold := range []string{"0", "101"} {
		err := cmdFactoryV1([]string{
			"daemon", "--human", "Michael", "--repo-workspace-root", ".",
			"--issue-scan-interval", "1s", "--issue-scan-review-queue-threshold", threshold,
		})
		if err == nil || !strings.Contains(err.Error(), "--issue-scan-review-queue-threshold") {
			t.Fatalf("threshold %s error = %v, want bounded threshold requirement", threshold, err)
		}
	}
}
