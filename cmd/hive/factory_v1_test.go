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
