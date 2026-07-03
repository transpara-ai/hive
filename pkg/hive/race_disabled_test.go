//go:build !race

package hive

// raceEnabled is false for normal (non-race) test builds. See
// race_enabled_test.go for the race-build counterpart. The production
// latency-budget claim is the non-race number; -race relaxes the budget
// purely to absorb instrumentation overhead, not to weaken the guarantee.
const raceEnabled = false
