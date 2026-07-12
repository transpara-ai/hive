#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
claude_skill="$repo_root/.claude/skills/hive-lifecycle/SKILL.md"
codex_skill="$repo_root/skills/hive-lifecycle/codex/SKILL.md"
runtime_pattern='(^|/)[^ ]*[h]ive[^ ]* (--human|civilization|pipeline|role|council|factory)'

fail() {
  printf 'hive-lifecycle invariant failed: %s\n' "$*" >&2
  exit 1
}

extract_shell_fences() {
  awk '
    {
      line=$0
      sub(/^>[[:space:]]?/, "", line)
      if (line ~ /^```(bash|sh)[[:space:]]*$/) { in_shell=1; next }
      if (line ~ /^```[[:space:]]*$/ && in_shell) { print ""; in_shell=0; next }
      if (in_shell) print line
    }
    END { if (in_shell) exit 2 }
  ' "$1"
}

for skill in "$claude_skill" "$codex_skill"; do
  test -f "$skill" || fail "missing $skill"
  extract_shell_fences "$skill" | bash -n || fail "invalid shell fence in $skill"

  if grep -Fq "pgrep -f '[h]ive (" "$skill"; then
    fail "legacy exact-name runtime predicate remains in $skill"
  fi
  grep -Fq "$runtime_pattern" "$skill" || fail "version-aware runtime predicate missing from $skill"
  grep -Fq 'hive|hive-*|hive_*|go)' "$skill" || fail "version-aware comm allowlist missing from $skill"

  repo_arg_count=$(grep -Fc -- '--repo "$TARGET_REPO"' "$skill" || true)
  [ "$repo_arg_count" -ge 5 ] || fail "expected explicit target on civilization/pipeline/role/council examples in $skill (found $repo_arg_count)"
  grep -Fq 'rev-parse --path-format=absolute --git-common-dir' "$skill" || fail "Hive worktree rejection missing from $skill"
  grep -Fq 'target_status=$(git -C "$TARGET_REPO" status --porcelain 2>/dev/null) ||' "$skill" || fail "fail-closed target-status check missing from $skill"
  if grep -Fq -- '--repo .' "$skill"; then
    fail "current-directory Operate target remains in $skill"
  fi
done

for skill in "$claude_skill" "$codex_skill"; do
  up_fence=$(awk '
    /^## Hive Up[[:space:]]*$/ { in_up=1; next }
    in_up && /^```bash[[:space:]]*$/ { in_fence=1; next }
    in_fence && /^```[[:space:]]*$/ { exit }
    in_fence { print }
  ' "$skill")
  [ -n "$up_fence" ] || fail "Hive Up fence not found in $skill"
  if grep -Eq 'civilization (run|daemon)|systemctl --user (start|restart) hive([[:space:]]|$)' <<<"$up_fence"; then
    fail "Hive Up still launches the runtime in $skill"
  fi
done

for argv in \
  'go run ./cmd/hive civilization run' \
  '/tmp/go-build123/exe/hive civilization daemon' \
  '/tmp/hive-test001-67-b928942 civilization run' \
  '/tmp/hive_debug role reviewer run' \
  '/tmp/hive-test factory scan' \
  '/tmp/hive-test council topic' \
  '/tmp/hive-test pipeline run' \
  '/tmp/hive-test --human Michael'; do
  grep -Eq "$runtime_pattern" <<<"$argv" || fail "runtime predicate missed: $argv"
done

for argv in \
  'codex reviewing text hive civilization run' \
  '/usr/bin/hivemind status' \
  'grep hive civilization' \
  '/usr/bin/hive-ops-api --port 8085'; do
  if grep -Eq "$runtime_pattern" <<<"$argv"; then
    fail "runtime predicate false-positive: $argv"
  fi
done

comm_allowed() {
  case "$1" in
    hive|hive-*|hive_*|go) return 0 ;;
    *) return 1 ;;
  esac
}

for comm in hive hive-test001-67 hive_debug go; do
  comm_allowed "$comm" || fail "comm allowlist missed: $comm"
done
for comm in codex grep hivemind; do
  if comm_allowed "$comm"; then
    fail "comm allowlist false-positive: $comm"
  fi
done

# Council was the only example newly given an explicit target in this repair.
# Prove both the public CLI surface and the source handoff: accepting --repo is
# insufficient if the parsed value is discarded before runCouncilCmd.
council_help=$(go run -buildvcs=false ./cmd/hive council --help 2>&1) || fail "council --help failed"
grep -Eq '^[[:space:]]+-repo string$' <<<"$council_help" || fail "council --help does not expose -repo"
grep -Eq '^[[:space:]]+-space string$' <<<"$council_help" || fail "council --help does not expose -space"
grep -Fq 'repo := fs.Lookup("repo").Value.String()' "$repo_root/cmd/hive/router.go" || fail "council does not read the parsed repo flag"
grep -Fq 'return runCouncilCmd(space, apiBase, repo, budget, topic, catalog)' "$repo_root/cmd/hive/router.go" || fail "council does not pass repo into runCouncilCmd"

printf 'hive-lifecycle skill invariants: PASS\n'
