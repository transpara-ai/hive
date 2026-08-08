#!/usr/bin/env bash

# Shared, deliberately small helpers for the Civilization/Dark Factory v1
# local-only operator scripts.  This file must not source deployment config.

factory_v1_die() {
  printf 'factory-v1: %s\n' "$*" >&2
  exit 1
}

factory_v1_require_command() {
  command -v "$1" >/dev/null 2>&1 || factory_v1_die "required command is unavailable: $1"
}

factory_v1_sha256_file() {
  sha256sum -- "$1" | awk '{print $1}'
}

factory_v1_realpath_missing_ok() {
  realpath -m -- "$1"
}

factory_v1_assert_runtime_root() {
  local root
  root=$(factory_v1_realpath_missing_ok "$1")
  case "$root" in
    /|/home|/home/transpara|/home/transpara/transpara-ai|/home/transpara/transpara-ai/repos)
      factory_v1_die "refusing broad runtime root: $root"
      ;;
    /*) ;;
    *) factory_v1_die "runtime root must be absolute: $root" ;;
  esac
}

factory_v1_path_is_within() {
  local child parent
  child=$(factory_v1_realpath_missing_ok "$1")
  parent=$(factory_v1_realpath_missing_ok "$2")
  case "$child" in
    "$parent"|"$parent"/*) return 0 ;;
    *) return 1 ;;
  esac
}

factory_v1_mode() {
  stat -c '%a' -- "$1"
}

factory_v1_listener_pids() {
  local port=$1
  { lsof -nP -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true; } | sort -nu
}

factory_v1_required_ports() {
  printf '%s\n' 5432 8084 8083 8080 8088
}

factory_v1_pid_start_ticks() {
  awk '{print $22}' "/proc/$1/stat"
}

factory_v1_pid_executable() {
  readlink -f -- "/proc/$1/exe"
}

factory_v1_is_descendant_or_self() {
  local candidate=$1 ancestor=$2 parent
  while [[ "$candidate" =~ ^[0-9]+$ ]] && [ "$candidate" -gt 1 ]; do
    [ "$candidate" -eq "$ancestor" ] && return 0
    [ -r "/proc/$candidate/status" ] || return 1
    parent=$(awk '/^PPid:/ {print $2}' "/proc/$candidate/status")
    [ -n "$parent" ] || return 1
    candidate=$parent
  done
  return 1
}

factory_v1_json_receipt() {
  local output=$1
  shift
  local tmp
  tmp="${output}.tmp.$$"
  jq "$@" >"$tmp"
  chmod 600 "$tmp"
  mv -f -- "$tmp" "$output"
}
