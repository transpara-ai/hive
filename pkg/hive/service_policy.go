package hive

import (
	"fmt"
	"strings"
)

var readOnlyObserverForbiddenUnitKeys = map[string]struct{}{
	"after":     {},
	"bindsto":   {},
	"onfailure": {},
	"onsuccess": {},
	"partof":    {},
	"requires":  {},
	"upholds":   {},
	"wants":     {},
}

type systemdLogicalLine struct {
	number int
	text   string
}

// ValidateReadOnlyObserverUnit rejects systemd unit text that lets a monitor,
// observer, or dashboard service start or couple itself to the Hive runtime.
func ValidateReadOnlyObserverUnit(name, unitText string) error {
	unitName := strings.TrimSpace(name)
	if unitName == "" {
		unitName = "read-only observer unit"
	}
	for _, logical := range systemdLogicalLines(unitText) {
		line := strings.TrimSpace(logical.text)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
			continue
		}
		key, rawValue, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if _, forbidden := readOnlyObserverForbiddenUnitKeys[key]; !forbidden {
			continue
		}
		for _, target := range systemdUnitValues(rawValue) {
			if hiveRuntimeUnitTarget(target) {
				return fmt.Errorf("%s line %d: read-only observers must not declare %s=%s", unitName, logical.number, key, target)
			}
		}
	}
	return nil
}

func systemdLogicalLines(unitText string) []systemdLogicalLine {
	rawLines := strings.Split(unitText, "\n")
	out := make([]systemdLogicalLine, 0, len(rawLines))
	var current strings.Builder
	startLine := 0
	for i, rawLine := range rawLines {
		lineNumber := i + 1
		line := strings.TrimRight(rawLine, "\r")
		if current.Len() == 0 {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
				out = append(out, systemdLogicalLine{number: lineNumber, text: line})
				continue
			}
		}
		lineWithoutTrailingSpace := strings.TrimRight(line, " \t")
		continued := strings.HasSuffix(lineWithoutTrailingSpace, `\`)
		if continued {
			line = strings.TrimSuffix(lineWithoutTrailingSpace, `\`)
		}
		if startLine == 0 {
			startLine = lineNumber
		}
		current.WriteString(line)
		if continued {
			continue
		}
		out = append(out, systemdLogicalLine{number: startLine, text: current.String()})
		current.Reset()
		startLine = 0
	}
	if current.Len() > 0 {
		out = append(out, systemdLogicalLine{number: startLine, text: current.String()})
	}
	return out
}

func systemdUnitValues(raw string) []string {
	fields := strings.Fields(raw)
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.HasPrefix(field, "#") || strings.HasPrefix(field, ";") {
			break
		}
		out = append(out, strings.Trim(field, `"'`))
	}
	return out
}

func hiveRuntimeUnitTarget(target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	switch target {
	case "hive.service", "hive@.service", "hive-runtime.service", "civilization-hive.service":
		return true
	default:
		return strings.HasPrefix(target, "hive@") && strings.HasSuffix(target, ".service")
	}
}
