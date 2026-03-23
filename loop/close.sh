#!/bin/bash
# close.sh — End-of-iteration: post to lovyou.ai, commit loop artifacts, push.
#
# Usage:
#   cd /c/src/matt/lovyou3/hive && ./loop/close.sh
#
# Requires: LOVYOU_API_KEY set in environment.
set -euo pipefail

LOOP_DIR="$(cd "$(dirname "$0")" && pwd)"
HIVE_DIR="$(dirname "$LOOP_DIR")"
cd "$HIVE_DIR"

# Extract iteration number from state.md.
ITER=$(grep -oP 'Iteration \K\d+' loop/state.md | head -1)
if [ -z "$ITER" ]; then
    echo "ERROR: could not find iteration number in loop/state.md"
    exit 1
fi

# Validate that all artifact files were written this iteration.
for f in loop/scout.md loop/build.md loop/critique.md; do
    if [ ! -f "$f" ]; then
        echo "WARNING: $f does not exist"
    fi
done

# Post to lovyou.ai (feed + board + mind sync).
if [ -n "${LOVYOU_API_KEY:-}" ]; then
    echo "=== POST ==="
    LOVYOU_API_KEY="$LOVYOU_API_KEY" go run ./cmd/post/
else
    echo "SKIP: post (set LOVYOU_API_KEY to enable)"
fi

# Commit and push loop artifacts.
echo "=== COMMIT ==="
git add loop/
git commit -m "state ${ITER}

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"

echo "=== PUSH ==="
git push origin main

echo "=== ITERATION ${ITER} CLOSED ==="
