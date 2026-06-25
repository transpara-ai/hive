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
