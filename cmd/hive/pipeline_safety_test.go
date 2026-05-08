package main

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/transpara-ai/hive/pkg/safety"
)

func TestAuthorizeFinalPipelineSweepBlocksRepoMapByDefault(t *testing.T) {
	var logs bytes.Buffer
	restoreLogs := captureMainLogs(&logs)
	defer restoreLogs()

	err := authorizeFinalPipelineSweep(map[string]string{
		"hive": "/tmp/hive",
		"site": "/tmp/site",
	}, "/tmp/hive", nil)
	if err == nil {
		t.Fatal("expected cross-repo mutation authority error")
	}
	authErr, ok := err.(safety.AuthorityError)
	if !ok {
		t.Fatalf("error type = %T, want safety.AuthorityError", err)
	}
	if authErr.Action != safety.ActionRepoMutateCrossRepo {
		t.Fatalf("action = %s, want %s", authErr.Action, safety.ActionRepoMutateCrossRepo)
	}

	for _, want := range []string{
		"repo.mutate.cross_repo.blocked",
		string(safety.ActionRepoMutateCrossRepo),
		string(safety.ApprovalRequired),
		"repos=2",
		"active_repo=/tmp/hive",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("log missing %q:\n%s", want, logs.String())
		}
	}
}

func TestAuthorizeFinalPipelineSweepAllowsSingleRepoDirectPushPath(t *testing.T) {
	var logs bytes.Buffer
	restoreLogs := captureMainLogs(&logs)
	defer restoreLogs()

	if err := authorizeFinalPipelineSweep(nil, "/tmp/hive", nil); err != nil {
		t.Fatalf("authorizeFinalPipelineSweep with no repo map: %v", err)
	}
	if strings.Contains(logs.String(), "repo.mutate.cross_repo.blocked") {
		t.Fatalf("single-repo path logged cross-repo block unexpectedly:\n%s", logs.String())
	}
}

func captureMainLogs(buf *bytes.Buffer) func() {
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(buf)
	log.SetFlags(0)
	return func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}
}
