#!/bin/bash
# close.sh — End-of-iteration: commit loop artifacts locally.
#
# Safety: refuses to run on protected branches, never pushes, never posts
# unless configured. All values from loop/config.env.
#
# Usage:
#   cd <hive-repo> && ./loop/close.sh
set -euo pipefail

LOOP_DIR="$(cd "$(dirname "$0")" && pwd)"
HIVE_DIR="$(dirname "${LOOP_DIR}")"
cd "${HIVE_DIR}"

# Load config.
# shellcheck source=config.env
source "${LOOP_DIR}/config.env"

# Safety: refuse to commit on protected branches.
BRANCH=$(git branch --show-current)
for protected in ${PROTECTED_BRANCHES}; do
    if [ "${BRANCH}" = "${protected}" ]; then
        echo "ERROR: refusing to commit on ${BRANCH} (protected). Switch to a feat/ branch first."
        exit 1
    fi
done

# Safety: verify configured remote exists.
if ! git remote get-url "${GIT_REMOTE}" &>/dev/null; then
    echo "ERROR: '${GIT_REMOTE}' remote not found. Add it before running close.sh."
    echo "  git remote add ${GIT_REMOTE} https://github.com/${GIT_ORG}/${REPO_NAME}.git"
    exit 1
fi

# Extract iteration number from state.md.
ITER=$(grep -o 'Iteration [0-9]*' loop/state.md | head -1 | sed 's/Iteration //')
if [ -z "${ITER}" ]; then
    echo "ERROR: could not find iteration number in loop/state.md"
    exit 1
fi

# Validate that all artifact files were written this iteration.
for f in loop/scout.md loop/build.md loop/critique.md; do
    if [ ! -f "${f}" ]; then
        echo "WARNING: ${f} does not exist"
    fi
done

# Optional: post iteration summary.
if [ "${POST_ENABLED:-false}" = "true" ] && [ -n "${POST_API_KEY:-}" ]; then
    echo "=== POST ==="
    POST_API_KEY="${POST_API_KEY}" POST_API_BASE="${POST_API_BASE}" go run ./cmd/post/
fi

# Commit loop artifacts.
echo "=== COMMIT (iteration ${ITER}, branch ${BRANCH}) ==="
git add loop/
git commit -m "state ${ITER}: loop artifacts

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>"

echo "=== ITERATION ${ITER} COMMITTED ==="
echo "Branch: ${BRANCH}"
echo "Remote: ${GIT_REMOTE} (${GIT_ORG}/${REPO_NAME})"
echo ""
echo "To push and create a PR, run:"
echo "  git push ${GIT_REMOTE} ${BRANCH}"
echo "  gh pr create --repo ${GIT_ORG}/${REPO_NAME} --title 'state ${ITER}' --body 'Loop iteration ${ITER} artifacts'"
