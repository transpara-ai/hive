//go:build race

package hive

// raceEnabled is true when the test binary is built with -race. Race
// instrumentation adds substantial CPU overhead (roughly 4-10x) to every
// goroutine and memory access, so wall-clock latency budgets calibrated
// against production (non-race) behavior have to be relaxed under -race to
// avoid flaking on instrumentation overhead rather than a real regression.
const raceEnabled = true
