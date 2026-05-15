#!/usr/bin/env bash
# DEPRECATED 2026-05-14 — replaced by the Go runtime.
# This bash runner bypassed the modelconfig resolver and pinned every role
# to `claude -p`. The Go civilization runtime honors --catalog and per-role policy.
echo "hive/loop/run.sh is deprecated as of 2026-05-14." >&2
echo "Use: go run ./cmd/hive civilization run --human <name> --idea <spec> [--catalog <path>]" >&2
echo "See hive/CLAUDE.md for current runtime usage." >&2
exit 1
