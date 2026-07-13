package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const hiveUnitPreflightTimeout = 5 * time.Second

type hiveUnitCredentialPosture string

const (
	hiveUnitCredentialUnknown hiveUnitCredentialPosture = "UNKNOWN"
	hiveUnitCredentialAbsent  hiveUnitCredentialPosture = "ABSENT"
	hiveUnitCredentialEmpty   hiveUnitCredentialPosture = "EMPTY"
	hiveUnitCredentialPresent hiveUnitCredentialPosture = "PRESENT"
)

type hiveUnitProperties struct {
	ActiveState string
	SubState    string
	ExecStart   string
	MainPID     int
}

type hiveUnitPreflightReport struct {
	ActiveState     string
	SubState        string
	MainPID         int
	ApproveRequests bool
	ApproveRoles    bool
	Credential      hiveUnitCredentialPosture
	UnitKnown       bool
	AutonomyKnown   bool
	Unknown         bool
}

type hiveUnitCommandRunner func(context.Context, string, ...string) ([]byte, error)
type hiveUnitReadFile func(string) ([]byte, error)

func cmdFactoryPreflightHiveUnit(args []string) error {
	fs := flag.NewFlagSet("factory preflight-hive-unit", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: hive factory preflight-hive-unit")
		fmt.Fprintln(fs.Output(), "Reports merged hive.service unit, autonomy, and credential postures without mutating the unit.")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%w: factory preflight-hive-unit accepts no positional arguments", errUsage)
	}

	ctx, cancel := context.WithTimeout(context.Background(), hiveUnitPreflightTimeout)
	defer cancel()
	return runHiveUnitPreflight(ctx, os.Stdout, runHiveUnitSystemctl, os.ReadFile)
}

func runHiveUnitSystemctl(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func runHiveUnitPreflight(ctx context.Context, stdout io.Writer, runner hiveUnitCommandRunner, readFile hiveUnitReadFile) error {
	raw, err := runner(ctx, "systemctl", "--user", "show", "hive.service", "-p", "ActiveState", "-p", "SubState", "-p", "ExecStart", "-p", "MainPID")
	if err != nil {
		writeUnknownHiveUnitPreflight(stdout)
		return fmt.Errorf("hive.service merged properties UNKNOWN: %w", err)
	}
	properties, err := parseHiveUnitProperties(raw)
	if err != nil {
		writeUnknownHiveUnitPreflight(stdout)
		return fmt.Errorf("hive.service merged properties UNKNOWN: %w", err)
	}

	var environ []byte
	var environErr error
	if properties.MainPID > 0 {
		environ, environErr = readFile(filepath.Join("/proc", strconv.Itoa(properties.MainPID), "environ"))
	} else {
		environErr = fmt.Errorf("MainPID is not a running process")
	}
	report := evaluateHiveUnitPreflight(properties, environ, environErr)
	writeHiveUnitPreflightReport(stdout, report)
	if report.Unknown {
		return fmt.Errorf("hive.service posture UNKNOWN")
	}
	return nil
}

func parseHiveUnitProperties(raw []byte) (hiveUnitProperties, error) {
	var properties hiveUnitProperties
	if len(bytes.TrimSpace(raw)) == 0 {
		return properties, fmt.Errorf("empty systemctl output")
	}

	values := make(map[string]string, 4)
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return properties, fmt.Errorf("malformed systemctl property")
		}
		if _, duplicate := values[name]; duplicate {
			return properties, fmt.Errorf("duplicate systemctl property %s", name)
		}
		values[name] = value
	}

	for _, name := range []string{"ActiveState", "SubState", "ExecStart", "MainPID"} {
		if strings.TrimSpace(values[name]) == "" {
			return properties, fmt.Errorf("missing or empty systemctl property %s", name)
		}
	}
	mainPID, err := strconv.Atoi(values["MainPID"])
	if err != nil || mainPID < 0 {
		return properties, fmt.Errorf("MainPID is invalid")
	}
	properties = hiveUnitProperties{
		ActiveState: values["ActiveState"],
		SubState:    values["SubState"],
		ExecStart:   values["ExecStart"],
		MainPID:     mainPID,
	}
	return properties, nil
}

func evaluateHiveUnitPreflight(properties hiveUnitProperties, environ []byte, environErr error) hiveUnitPreflightReport {
	report := hiveUnitPreflightReport{
		ActiveState: properties.ActiveState,
		SubState:    properties.SubState,
		MainPID:     properties.MainPID,
		Credential:  hiveUnitCredentialUnknown,
	}
	report.UnitKnown = knownHiveUnitActiveState(properties.ActiveState) && strings.TrimSpace(properties.SubState) != "" && strings.TrimSpace(properties.ExecStart) != "" && properties.MainPID >= 0
	report.AutonomyKnown = strings.TrimSpace(properties.ExecStart) != ""
	report.Unknown = !report.UnitKnown || !report.AutonomyKnown
	report.ApproveRequests = hiveUnitExecStartHasFlag(properties.ExecStart, "--approve-requests")
	report.ApproveRoles = hiveUnitExecStartHasFlag(properties.ExecStart, "--approve-roles")

	if environErr != nil || len(environ) == 0 {
		report.Unknown = true
		return report
	}
	report.Credential = hiveUnitCredentialAbsent
	for _, entry := range bytes.Split(environ, []byte{0}) {
		if !bytes.HasPrefix(entry, []byte("TRANSPARA_API_KEY=")) {
			continue
		}
		if len(entry) == len("TRANSPARA_API_KEY=") {
			report.Credential = hiveUnitCredentialEmpty
		} else {
			report.Credential = hiveUnitCredentialPresent
		}
		break
	}
	return report
}

func knownHiveUnitActiveState(state string) bool {
	switch state {
	case "active", "reloading", "inactive", "failed", "activating", "deactivating", "maintenance", "refreshing":
		return true
	default:
		return false
	}
}

func hiveUnitExecStartHasFlag(execStart, flagName string) bool {
	target := strings.TrimLeft(flagName, "-")
	for _, field := range strings.Fields(execStart) {
		field = strings.Trim(field, "{};\"")
		name, value, hasValue := strings.Cut(field, "=")
		if !strings.HasPrefix(name, "-") || strings.TrimLeft(name, "-") != target {
			continue
		}
		if !hasValue {
			return true
		}
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return true
		}
		return enabled
	}
	return false
}

func writeUnknownHiveUnitPreflight(stdout io.Writer) {
	fmt.Fprintln(stdout, "unit_posture=UNKNOWN active_state=UNKNOWN sub_state=UNKNOWN exec_start=UNKNOWN main_pid=UNKNOWN")
	fmt.Fprintln(stdout, "autonomy_posture=UNKNOWN approve_requests=UNKNOWN approve_roles=UNKNOWN")
	fmt.Fprintln(stdout, "credential_posture=UNKNOWN lovyou_api_key=UNKNOWN")
	fmt.Fprintln(stdout, "overall=UNKNOWN")
}

func writeHiveUnitPreflightReport(stdout io.Writer, report hiveUnitPreflightReport) {
	if !report.UnitKnown {
		fmt.Fprintf(stdout, "unit_posture=UNKNOWN active_state=%s sub_state=%s exec_start=present main_pid=%d\n", report.ActiveState, report.SubState, report.MainPID)
	} else {
		fmt.Fprintf(stdout, "unit_posture=KNOWN active_state=%s sub_state=%s exec_start=present main_pid=%d\n", report.ActiveState, report.SubState, report.MainPID)
	}
	fmt.Fprintf(stdout, "autonomy_posture=%s approve_requests=%t approve_roles=%t\n", hiveUnitAutonomyPosture(report), report.ApproveRequests, report.ApproveRoles)
	fmt.Fprintf(stdout, "credential_posture=%s lovyou_api_key=%s\n", report.Credential, strings.ToLower(string(report.Credential)))
	if report.Unknown {
		fmt.Fprintln(stdout, "overall=UNKNOWN")
	} else {
		fmt.Fprintln(stdout, "overall=PASS")
	}
}

func hiveUnitAutonomyPosture(report hiveUnitPreflightReport) string {
	if !report.AutonomyKnown {
		return "UNKNOWN"
	}
	switch {
	case report.ApproveRequests && report.ApproveRoles:
		return "FULL"
	case report.ApproveRequests:
		return "REQUESTS_ONLY"
	case report.ApproveRoles:
		return "ROLES_ONLY"
	default:
		return "MANUAL"
	}
}
