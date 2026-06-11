package budget

import "testing"

// v14-F3(c): wall-clock budget bounds for allocator duration renewals,
// following the package's existing env-override pattern.

func TestDurationBoundsDefaults(t *testing.T) {
	t.Setenv("ALLOCATOR_DURATION_FLOOR_MIN", "")
	t.Setenv("ALLOCATOR_DURATION_CEILING_MIN", "")
	cfg := LoadConfig()
	if cfg.DurationFloorMin != 10 {
		t.Fatalf("DurationFloorMin default = %d; want 10", cfg.DurationFloorMin)
	}
	if cfg.DurationCeilingMin != 720 {
		t.Fatalf("DurationCeilingMin default = %d; want 720 (12h)", cfg.DurationCeilingMin)
	}
}

func TestDurationBoundsEnvOverride(t *testing.T) {
	t.Setenv("ALLOCATOR_DURATION_FLOOR_MIN", "5")
	t.Setenv("ALLOCATOR_DURATION_CEILING_MIN", "1440")
	cfg := LoadConfig()
	if cfg.DurationFloorMin != 5 || cfg.DurationCeilingMin != 1440 {
		t.Fatalf("env override = (%d, %d); want (5, 1440)", cfg.DurationFloorMin, cfg.DurationCeilingMin)
	}
}
