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
# Run from the hive repo root. Paths configured in loop/config.env.

set -euo pipefail

LOOP_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${LOOP_DIR}/config.env"
HIVE_DIR="$(dirname "${LOOP_DIR}")"
MAX_REVISE=3

cd "$HIVE_DIR"

run_phase() {
    local phase="$1"
    local prompt_file="$LOOP_DIR/${phase}-prompt.txt"

    if [ ! -f "$prompt_file" ]; then
        echo "ERROR: $prompt_file not found"
        exit 1
    fi

    echo "=== ${phase^^} ==="
    claude --dangerously-skip-permissions -p "$(cat "$prompt_file")"
    echo ""
}

critic_says_revise() {
    local critique_file="$LOOP_DIR/critique.md"
    if [ ! -f "$critique_file" ]; then
        return 1
    fi
    # Check for REVISE verdict (case-insensitive, looks for "Verdict: REVISE" or "REVISE")
    grep -qi 'verdict.*revise\|^##.*revise' "$critique_file"
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

        # REVISE loop: if the critic says REVISE, re-run builder+critic (max 3 rounds).
        round=1
        while critic_says_revise && [ "$round" -lt "$MAX_REVISE" ]; do
            round=$((round + 1))
            echo "=== REVISE (round $round/$MAX_REVISE) ==="
            run_phase builder
            run_phase critic
        done

        if critic_says_revise; then
            echo "WARNING: critic still says REVISE after $MAX_REVISE rounds"
        fi

        run_phase reflector

        # Post iteration summary (only if enabled in config.env).
        if [ "${POST_ENABLED:-false}" = "true" ] && [ -n "${POST_API_KEY:-}" ]; then
            echo "=== POST ==="
            POST_API_KEY="${POST_API_KEY}" POST_API_BASE="${POST_API_BASE}" go run ./cmd/post/
        else
            echo "SKIP: post (POST_ENABLED=${POST_ENABLED:-false})"
        fi
        ;;
    *)
        echo "Usage: $0 [scout|builder|critic|reflector|all]"
        exit 1
        ;;
esac

echo "=== ITERATION COMPLETE ==="
