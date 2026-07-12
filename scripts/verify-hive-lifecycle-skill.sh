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

  repo_arg_count=$(grep -Fc -- '--repo "$TARGET_REPO"' "$skill")
  [ "$repo_arg_count" -ge 5 ] || fail "expected explicit target on civilization/pipeline/role/council examples in $skill (found $repo_arg_count)"
  grep -Fq 'rev-parse --path-format=absolute --git-common-dir' "$skill" || fail "Hive worktree rejection missing from $skill"
  grep -Fq 'status --porcelain' "$skill" || fail "clean-target check missing from $skill"
  if grep -Fq -- '--repo .' "$skill"; then
    fail "current-directory Operate target remains in $skill"
  fi
done

claude_up_fence=$(awk '
  /^## Hive Up[[:space:]]*$/ { in_up=1; next }
  in_up && /^```bash[[:space:]]*$/ { in_fence=1; next }
  in_fence && /^```[[:space:]]*$/ { exit }
  in_fence { print }
' "$claude_skill")
[ -n "$claude_up_fence" ] || fail "Claude Hive Up fence not found"
if grep -Eq 'civilization (run|daemon)|systemctl --user (start|restart) hive([[:space:]]|$)' <<<"$claude_up_fence"; then
  fail "Claude Hive Up still launches the runtime"
fi

for argv in \
  'go run ./cmd/hive civilization run' \
  '/tmp/go-build123/exe/hive civilization daemon' \
  '/tmp/hive-test001-67-b928942 civilization run' \
  '/tmp/hive_debug role reviewer run'; do
  grep -Eq "$runtime_pattern" <<<"$argv" || fail "runtime predicate missed: $argv"
done

for argv in \
  'codex reviewing text hive civilization run' \
  '/usr/bin/hivemind status' \
  'grep hive civilization'; do
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

printf 'hive-lifecycle skill invariants: PASS\n'
