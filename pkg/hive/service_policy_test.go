package hive

import (
	"strings"
	"testing"
)

func TestValidateReadOnlyObserverUnitRejectsHiveRuntimeDependencies(t *testing.T) {
	unit := `[Unit]
Description=Civilization live monitor
Wants=hive.service
After=network-online.target hive.service

[Service]
ExecStart=/usr/bin/civilization-live-monitor
`
	err := ValidateReadOnlyObserverUnit("civilization-live-monitor.service", unit)
	if err == nil {
		t.Fatal("ValidateReadOnlyObserverUnit succeeded, want hive runtime dependency rejection")
	}
	if !strings.Contains(err.Error(), "Wants") && !strings.Contains(err.Error(), "wants") {
		t.Fatalf("error = %v, want Wants dependency called out", err)
	}
}

func TestValidateReadOnlyObserverUnitRejectsHiveTemplateRuntimeDependencies(t *testing.T) {
	unit := `[Unit]
Description=Civilization live monitor
Requires=hive@transpara-ai.service
`
	err := ValidateReadOnlyObserverUnit("civilization-live-monitor.service", unit)
	if err == nil {
		t.Fatal("ValidateReadOnlyObserverUnit succeeded, want hive template runtime dependency rejection")
	}
	if !strings.Contains(err.Error(), "hive@transpara-ai.service") {
		t.Fatalf("error = %v, want target unit called out", err)
	}
}

func TestValidateReadOnlyObserverUnitRejectsHiveRuntimeOnFailureWake(t *testing.T) {
	unit := `[Unit]
Description=Civilization live monitor
OnFailure=hive.service
`
	err := ValidateReadOnlyObserverUnit("civilization-live-monitor.service", unit)
	if err == nil {
		t.Fatal("ValidateReadOnlyObserverUnit succeeded, want OnFailure hive runtime rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "onfailure") {
		t.Fatalf("error = %v, want OnFailure called out", err)
	}
}

func TestValidateReadOnlyObserverUnitRejectsHiveRuntimeOnSuccessWake(t *testing.T) {
	unit := `[Unit]
Description=Civilization live monitor
OnSuccess=hive.service
`
	err := ValidateReadOnlyObserverUnit("civilization-live-monitor.service", unit)
	if err == nil {
		t.Fatal("ValidateReadOnlyObserverUnit succeeded, want OnSuccess hive runtime rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "onsuccess") {
		t.Fatalf("error = %v, want OnSuccess called out", err)
	}
}

func TestValidateReadOnlyObserverUnitRejectsContinuedHiveRuntimeDependencies(t *testing.T) {
	unit := `[Unit]
Description=Civilization live monitor
After=network-online.target \
  hive.service
`
	err := ValidateReadOnlyObserverUnit("civilization-live-monitor.service", unit)
	if err == nil {
		t.Fatal("ValidateReadOnlyObserverUnit succeeded, want continued hive runtime dependency rejection")
	}
	if !strings.Contains(err.Error(), "hive.service") {
		t.Fatalf("error = %v, want hive.service target called out", err)
	}
}

func TestValidateReadOnlyObserverUnitRejectsCommentContinuationHiveRuntimeDependencies(t *testing.T) {
	unit := `[Unit]
Description=Civilization live monitor
# disabled \
Wants=hive.service
`
	err := ValidateReadOnlyObserverUnit("civilization-live-monitor.service", unit)
	if err == nil {
		t.Fatal("ValidateReadOnlyObserverUnit succeeded, want hidden hive runtime dependency rejection")
	}
	if !strings.Contains(err.Error(), "hive.service") {
		t.Fatalf("error = %v, want hive.service target called out", err)
	}
}

func TestValidateReadOnlyObserverUnitAllowsReadOnlyMonitorDependencies(t *testing.T) {
	unit := `[Unit]
Description=Civilization live monitor
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/bin/civilization-live-monitor --read-only
`
	if err := ValidateReadOnlyObserverUnit("civilization-live-monitor.service", unit); err != nil {
		t.Fatalf("ValidateReadOnlyObserverUnit: %v", err)
	}
}
