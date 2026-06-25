package hive

import (
	"fmt"
	"strings"
)

var readOnlyObserverForbiddenUnitKeys = map[string]struct{}{
	"after":    {},
	"bindsto":  {},
	"partof":   {},
	"requires": {},
	"upholds":  {},
	"wants":    {},
}

// ValidateReadOnlyObserverUnit rejects systemd unit text that lets a monitor,
// observer, or dashboard service start or couple itself to the Hive runtime.
func ValidateReadOnlyObserverUnit(name, unitText string) error {
	unitName := strings.TrimSpace(name)
	if unitName == "" {
		unitName = "read-only observer unit"
	}
	for lineNumber, rawLine := range strings.Split(unitText, "\n") {
		line := strings.TrimSpace(rawLine)
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
				return fmt.Errorf("%s line %d: read-only observers must not declare %s=%s", unitName, lineNumber+1, key, target)
			}
		}
	}
	return nil
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
