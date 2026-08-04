#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "$script_dir/lib.sh"

factory_v1_require_command jq
factory_v1_require_command sha256sum
factory_v1_require_command realpath

action=${1:-}
shift || true
paths_file=''
baseline=''
output=''
runtime_root=${FACTORY_V1_RUNTIME_ROOT:-/home/transpara/transpara-ai/runtime/civilization-dark-factory-v1}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --paths) paths_file=$2; shift 2 ;;
    --baseline) baseline=$2; shift 2 ;;
    --output) output=$2; shift 2 ;;
    --runtime-root) runtime_root=$2; shift 2 ;;
    *) factory_v1_die "unknown evidence argument: $1" ;;
  esac
done

runtime_root=$(factory_v1_realpath_missing_ok "$runtime_root")
factory_v1_assert_runtime_root "$runtime_root"
install -d -m 700 -- "$runtime_root/receipts"

snapshot() {
  local source_file=$1 target=$2 rows path resolved entry type digest target_text
  [ -f "$source_file" ] || factory_v1_die "paths file is unavailable: $source_file"
  rows=$runtime_root/operation86-rows.$$.jsonl
  : >"$rows"
  chmod 600 "$rows"
  while IFS= read -r path || [ -n "$path" ]; do
    [ -n "$path" ] || continue
    [[ "$path" = \#* ]] && continue
    resolved=$(factory_v1_realpath_missing_ok "$path")
    case "$resolved" in /|/home|/home/transpara) factory_v1_die "refusing broad protected path: $resolved" ;; esac
    [ -e "$resolved" ] || [ -L "$resolved" ] || factory_v1_die "protected path is unavailable: $resolved"
    while IFS= read -r -d '' entry; do
      if [ -L "$entry" ]; then
        type=symlink
        target_text=$(readlink -- "$entry")
        digest=$(printf '%s' "$target_text" | sha256sum | awk '{print $1}')
      elif [ -f "$entry" ]; then
        type=file
        digest=$(factory_v1_sha256_file "$entry")
      elif [ -d "$entry" ]; then
        type=directory
        digest=null
      else
        type=other
        digest=null
      fi
      jq -n --arg path "$entry" --arg type "$type" --arg mode "$(factory_v1_mode "$entry")" --arg uid "$(stat -c '%u' -- "$entry")" --arg size "$(stat -c '%s' -- "$entry")" --arg digest "$digest" \
        '{path:$path,type:$type,mode:$mode,uid:$uid,size:$size,sha256:(if $digest == "null" then null else $digest end)}' >>"$rows"
    done < <(find -P "$resolved" -xdev -print0 | sort -z)
  done <"$source_file"
  jq -s --arg captured_at "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" '{schema:"factory-v1-operation86-snapshot/v1",captured_at:$captured_at,check:"TestFactoryV1Operation86Immutability",status:"pass",entries:(sort_by(.path))}' "$rows" >"${target}.tmp.$$"
  chmod 600 "${target}.tmp.$$"
  mv -f -- "${target}.tmp.$$" "$target"
  rm -f -- "$rows"
}

case "$action" in
  capture)
    [ -n "$paths_file" ] || factory_v1_die "capture requires --paths"
    output=${output:-$runtime_root/receipts/operation86-baseline.json}
    snapshot "$paths_file" "$output"
    cat -- "$output"
    ;;
  compare)
    [ -n "$paths_file" ] || factory_v1_die "compare requires --paths"
    [ -n "$baseline" ] || factory_v1_die "compare requires --baseline"
    [ -f "$baseline" ] || factory_v1_die "baseline is unavailable: $baseline"
    output=${output:-$runtime_root/receipts/operation86-final.json}
    current=$runtime_root/receipts/operation86-current.$$.json
    snapshot "$paths_file" "$current"
    baseline_entries=$(jq -cS '.entries' "$baseline")
    current_entries=$(jq -cS '.entries' "$current")
    status=pass
    [ "$baseline_entries" = "$current_entries" ] || status=fail
    jq -n \
      --arg compared_at "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
      --arg status "$status" \
      --arg baseline "$baseline" \
      --arg baseline_sha256 "$(factory_v1_sha256_file "$baseline")" \
      --arg current_sha256 "$(factory_v1_sha256_file "$current")" \
      '{schema:"factory-v1-operation86-comparison/v1",compared_at:$compared_at,check:"TestFactoryV1Operation86Immutability",status:$status,baseline:$baseline,baseline_sha256:$baseline_sha256,current_snapshot_sha256:$current_sha256}' \
      >"${output}.tmp.$$"
    chmod 600 "${output}.tmp.$$"
    mv -f -- "${output}.tmp.$$" "$output"
    rm -f -- "$current"
    cat -- "$output"
    [ "$status" = pass ]
    ;;
  *)
    printf 'usage: %s {capture|compare} --paths FILE [--baseline FILE] [--output FILE] [--runtime-root DIR]\n' "$0" >&2
    exit 2
    ;;
esac
