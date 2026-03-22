#!/bin/bash
# run.sh — Run one iteration of the core loop.
#
# Usage:
#   ./loop/run.sh              # Run all four phases
#   ./loop/run.sh scout        # Run just the Scout
#   ./loop/run.sh builder      # Run just the Builder
#   ./loop/run.sh critic       # Run just the Critic
#   ./loop/run.sh reflector    # Run just the Reflector
#
# Requires: claude CLI (Claude Code) on PATH.
# Run from the hive repo root: cd /c/src/matt/lovyou3/hive && ./loop/run.sh

set -euo pipefail

LOOP_DIR="$(cd "$(dirname "$0")" && pwd)"
HIVE_DIR="$(dirname "$LOOP_DIR")"

cd "$HIVE_DIR"

run_phase() {
    local phase="$1"
    local prompt_file="$LOOP_DIR/${phase}-prompt.txt"

    if [ ! -f "$prompt_file" ]; then
        echo "ERROR: $prompt_file not found"
        exit 1
    fi

    echo "=== ${phase^^} ==="
    claude -p "$(cat "$prompt_file")"
    echo ""
}

phase="${1:-all}"

case "$phase" in
    scout)
        run_phase scout
        ;;
    builder)
        run_phase builder
        ;;
    critic)
        run_phase critic
        ;;
    reflector)
        run_phase reflector
        ;;
    all)
        run_phase scout
        run_phase builder
        run_phase critic
        # TODO: if critique says REVISE, run builder+critic again (max 3 rounds)
        run_phase reflector
        # Post iteration summary to lovyou.ai (requires LOVYOU_API_KEY).
        if command -v go &>/dev/null; then
            go run ./cmd/post/
        fi
        ;;
    *)
        echo "Usage: $0 [scout|builder|critic|reflector|all]"
        exit 1
        ;;
esac

echo "=== ITERATION COMPLETE ==="
