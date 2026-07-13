package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestEvaluateHiveUnitPreflightCoversActiveStateDomain(t *testing.T) {
	for _, state := range []string{
		"active",
		"reloading",
		"inactive",
		"failed",
		"activating",
		"deactivating",
		"maintenance",
		"refreshing",
	} {
		t.Run(state, func(t *testing.T) {
			report := evaluateHiveUnitPreflight(hiveUnitProperties{
				ActiveState: state,
				SubState:    "running",
				ExecStart:   "/usr/local/bin/hive civilization daemon",
				MainPID:     42,
			}, []byte("PATH=/usr/bin\x00"), nil)
			if report.Unknown {
				t.Fatalf("state %q classified UNKNOWN: %+v", state, report)
			}
			if report.ActiveState != state {
				t.Fatalf("ActiveState = %q, want %q", report.ActiveState, state)
			}
		})
	}

	report := evaluateHiveUnitPreflight(hiveUnitProperties{
		ActiveState: "future-state",
		SubState:    "running",
		ExecStart:   "/usr/local/bin/hive civilization daemon",
		MainPID:     42,
	}, []byte("PATH=/usr/bin\x00"), nil)
	if !report.Unknown {
		t.Fatalf("unknown ActiveState must fail closed: %+v", report)
	}
}

func TestEvaluateHiveUnitPreflightCoversAutonomyPostures(t *testing.T) {
	tests := []struct {
		name            string
		execStart       string
		approveRequests bool
		approveRoles    bool
	}{
		{name: "neither", execStart: "/usr/local/bin/hive civilization daemon"},
		{name: "requests only", execStart: "/usr/local/bin/hive civilization daemon --approve-requests", approveRequests: true},
		{name: "roles only", execStart: "/usr/local/bin/hive civilization daemon --approve-roles=true", approveRoles: true},
		{name: "both", execStart: "/usr/local/bin/hive civilization daemon --approve-requests --approve-roles", approveRequests: true, approveRoles: true},
		{name: "single dash requests", execStart: "/usr/local/bin/hive civilization daemon -approve-requests", approveRequests: true},
		{name: "single dash roles", execStart: "/usr/local/bin/hive civilization daemon -approve-roles=true", approveRoles: true},
		{name: "explicit requests false", execStart: "/usr/local/bin/hive civilization daemon --approve-requests=false"},
		{name: "explicit roles false", execStart: "/usr/local/bin/hive civilization daemon -approve-roles=0"},
		{name: "invalid value fails cautious", execStart: "/usr/local/bin/hive civilization daemon --approve-requests=unexpected", approveRequests: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := evaluateHiveUnitPreflight(hiveUnitProperties{
				ActiveState: "active",
				SubState:    "running",
				ExecStart:   tt.execStart,
				MainPID:     42,
			}, []byte("PATH=/usr/bin\x00"), nil)
			if report.Unknown {
				t.Fatalf("known posture classified UNKNOWN: %+v", report)
			}
			if report.ApproveRequests != tt.approveRequests || report.ApproveRoles != tt.approveRoles {
				t.Fatalf("posture = requests:%t roles:%t, want requests:%t roles:%t", report.ApproveRequests, report.ApproveRoles, tt.approveRequests, tt.approveRoles)
			}
		})
	}
}

func TestEvaluateHiveUnitPreflightCoversCredentialPostures(t *testing.T) {
	tests := []struct {
		name    string
		environ []byte
		err     error
		want    hiveUnitCredentialPosture
		unknown bool
	}{
		{name: "absent", environ: []byte("PATH=/usr/bin\x00"), want: hiveUnitCredentialAbsent},
		{name: "empty", environ: []byte("TRANSPARA_API_KEY=\x00PATH=/usr/bin\x00"), want: hiveUnitCredentialEmpty},
		{name: "present", environ: []byte("TRANSPARA_API_KEY=super-secret\x00"), want: hiveUnitCredentialPresent},
		{name: "unreadable", err: errors.New("permission denied"), want: hiveUnitCredentialUnknown, unknown: true},
		{name: "empty input", environ: nil, want: hiveUnitCredentialUnknown, unknown: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := evaluateHiveUnitPreflight(hiveUnitProperties{
				ActiveState: "active",
				SubState:    "running",
				ExecStart:   "/usr/local/bin/hive civilization daemon",
				MainPID:     42,
			}, tt.environ, tt.err)
			if report.Credential != tt.want || report.Unknown != tt.unknown {
				t.Fatalf("report = %+v, want credential %q unknown=%t", report, tt.want, tt.unknown)
			}
		})
	}
}

func TestParseHiveUnitPropertiesFailsClosedOnEmptyUnreadableAndMalformedInputs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty"},
		{name: "missing active", raw: "SubState=running\nExecStart={ path=/usr/local/bin/hive ; argv[]=/usr/local/bin/hive civilization daemon ; }\nMainPID=42\n"},
		{name: "empty active", raw: "ActiveState=\nSubState=running\nExecStart=x\nMainPID=42\n"},
		{name: "missing sub", raw: "ActiveState=active\nExecStart=x\nMainPID=42\n"},
		{name: "empty exec", raw: "ActiveState=active\nSubState=running\nExecStart=\nMainPID=42\n"},
		{name: "missing pid", raw: "ActiveState=active\nSubState=running\nExecStart=x\n"},
		{name: "negative pid", raw: "ActiveState=active\nSubState=running\nExecStart=x\nMainPID=-1\n"},
		{name: "invalid pid", raw: "ActiveState=active\nSubState=running\nExecStart=x\nMainPID=abc\n"},
		{name: "duplicate", raw: "ActiveState=active\nActiveState=inactive\nSubState=running\nExecStart=x\nMainPID=42\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseHiveUnitProperties([]byte(tt.raw)); err == nil {
				t.Fatal("expected fail-closed parse error")
			}
		})
	}
}

func TestRunHiveUnitPreflightUsesOnlyReadOnlyMergedPropertyProbe(t *testing.T) {
	var gotName string
	var gotArgs []string
	runner := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return []byte("ActiveState=active\nSubState=running\nExecStart={ path=/usr/local/bin/hive ; argv[]=/usr/local/bin/hive civilization daemon --approve-requests --approve-roles ; }\nMainPID=42\n"), nil
	}
	readFile := func(path string) ([]byte, error) {
		if path != "/proc/42/environ" {
			return nil, fmt.Errorf("unexpected read path %q", path)
		}
		return []byte("TRANSPARA_API_KEY=\x00PATH=/usr/bin\x00"), nil
	}
	var stdout bytes.Buffer
	if err := runHiveUnitPreflight(context.Background(), &stdout, runner, readFile); err != nil {
		t.Fatalf("run preflight: %v", err)
	}
	if gotName != "systemctl" {
		t.Fatalf("command = %q, want systemctl", gotName)
	}
	wantArgs := []string{"--user", "show", "hive.service", "-p", "ActiveState", "-p", "SubState", "-p", "ExecStart", "-p", "MainPID"}
	if strings.Join(gotArgs, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %q, want %q", gotArgs, wantArgs)
	}
	for _, forbidden := range []string{"start", "stop", "restart", "kill", "signal"} {
		if strings.Contains(strings.Join(gotArgs, " "), forbidden) {
			t.Fatalf("read-only probe contains forbidden mutation %q: %q", forbidden, gotArgs)
		}
	}
	out := stdout.String()
	for _, want := range []string{"unit_posture=KNOWN", "autonomy_posture=FULL", "credential_posture=EMPTY", "overall=PASS"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "TRANSPARA_API_KEY=") {
		t.Fatalf("output disclosed credential material: %s", out)
	}
}

func TestRunHiveUnitPreflightRedactsPresentCredentialValue(t *testing.T) {
	runner := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("ActiveState=active\nSubState=running\nExecStart={ path=/usr/local/bin/hive ; argv[]=/usr/local/bin/hive civilization daemon ; }\nMainPID=42\n"), nil
	}
	readFile := func(string) ([]byte, error) {
		return []byte("TRANSPARA_API_KEY=super-secret\x00PATH=/usr/bin\x00"), nil
	}
	var stdout bytes.Buffer
	if err := runHiveUnitPreflight(context.Background(), &stdout, runner, readFile); err != nil {
		t.Fatalf("run preflight: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "credential_posture=PRESENT") {
		t.Fatalf("output missing PRESENT credential posture: %s", out)
	}
	for _, secretMaterial := range []string{"super-secret", "TRANSPARA_API_KEY="} {
		if strings.Contains(out, secretMaterial) {
			t.Fatalf("output disclosed %q: %s", secretMaterial, out)
		}
	}
}

func TestRunHiveUnitPreflightReportsUnknownAndNonzero(t *testing.T) {
	tests := []struct {
		name      string
		runnerErr error
		raw       []byte
		readErr   error
	}{
		{name: "systemctl unreadable", runnerErr: errors.New("systemctl failed")},
		{name: "properties malformed", raw: []byte("ActiveState=active\n")},
		{name: "environment unreadable", raw: []byte("ActiveState=active\nSubState=running\nExecStart=x\nMainPID=42\n"), readErr: errors.New("permission denied")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := func(context.Context, string, ...string) ([]byte, error) { return tt.raw, tt.runnerErr }
			readFile := func(string) ([]byte, error) { return nil, tt.readErr }
			var stdout bytes.Buffer
			if err := runHiveUnitPreflight(context.Background(), &stdout, runner, readFile); err == nil {
				t.Fatal("UNKNOWN preflight must return nonzero error")
			}
			if !strings.Contains(stdout.String(), "UNKNOWN") || !strings.Contains(stdout.String(), "overall=UNKNOWN") {
				t.Fatalf("UNKNOWN output missing: %s", stdout.String())
			}
		})
	}
}

func TestRunHiveUnitPreflightReportsStoppedUnitWithoutReadingProc(t *testing.T) {
	runner := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("ActiveState=inactive\nSubState=dead\nExecStart={ path=/usr/local/bin/hive ; argv[]=/usr/local/bin/hive civilization daemon ; }\nMainPID=0\n"), nil
	}
	readCalled := false
	readFile := func(string) ([]byte, error) {
		readCalled = true
		return nil, nil
	}
	var stdout bytes.Buffer
	if err := runHiveUnitPreflight(context.Background(), &stdout, runner, readFile); err == nil {
		t.Fatal("stopped unit has no readable process credential posture and must fail closed")
	}
	if readCalled {
		t.Fatal("stopped unit must not read /proc/0/environ")
	}
	out := stdout.String()
	for _, want := range []string{"unit_posture=KNOWN", "active_state=inactive", "autonomy_posture=MANUAL", "credential_posture=UNKNOWN", "overall=UNKNOWN"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stopped-unit output missing %q: %s", want, out)
		}
	}
}

func TestFactoryPreflightHiveUnitIsRegisteredAndDiscoverable(t *testing.T) {
	if !strings.Contains(helpText(), "preflight-hive-unit") {
		t.Fatal("top-level help must list preflight-hive-unit")
	}
	err := cmdFactory([]string{"preflight-hive-unit", "unexpected"})
	if err == nil || !strings.Contains(err.Error(), "no positional arguments") {
		t.Fatalf("registered command should reject positional arguments before probing systemd, got %v", err)
	}
}

// TestPreflightPrefixCollisionSafety proves the credential probe's trailing '='
// in the TRANSPARA_API_KEY= match prefix excludes sibling names: a variable that
// merely shares the prefix (TRANSPARA_API_KEY_BACKUP) must not be read as the
// credential (FO R4, packet D3/AC-4). Absent at the design base bf3f126.
func TestPreflightPrefixCollisionSafety(t *testing.T) {
	report := evaluateHiveUnitPreflight(hiveUnitProperties{
		ActiveState: "active",
		SubState:    "running",
		ExecStart:   "/usr/local/bin/hive civilization daemon",
		MainPID:     42,
	}, []byte("TRANSPARA_API_KEY_BACKUP=super-secret\x00PATH=/usr/bin\x00"), nil)
	if report.Credential != hiveUnitCredentialAbsent {
		t.Fatalf("sibling name TRANSPARA_API_KEY_BACKUP read as credential %q, want ABSENT", report.Credential)
	}
}
