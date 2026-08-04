#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "$script_dir/lib.sh"

factory_v1_require_command jq
factory_v1_require_command lsof
factory_v1_require_command realpath

runtime_root=${FACTORY_V1_RUNTIME_ROOT:-/home/transpara/transpara-ai/runtime/civilization-dark-factory-v1}
config_path=${FACTORY_V1_CONFIG_PATH:-$runtime_root/config/runtime.env}
output=''
operation_paths_file=${FACTORY_V1_OPERATION86_PATHS_FILE:-}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --runtime-root) runtime_root=$2; shift 2 ;;
    --config) config_path=$2; shift 2 ;;
    --operation86-paths) operation_paths_file=$2; shift 2 ;;
    --output) output=$2; shift 2 ;;
    *) factory_v1_die "unknown preflight argument: $1" ;;
  esac
done

runtime_root=$(factory_v1_realpath_missing_ok "$runtime_root")
config_path=$(factory_v1_realpath_missing_ok "$config_path")
factory_v1_assert_runtime_root "$runtime_root"
factory_v1_path_is_within "$config_path" "$runtime_root" || factory_v1_die "fresh config is outside runtime root"

receipt_dir=$runtime_root/receipts
install -d -m 700 -- "$receipt_dir"
output=${output:-$receipt_dir/preflight-$(date -u +'%Y%m%dT%H%M%SZ').json}

secret_safe=pass
operation_safe=pass
ports_safe=pass
declare -a secret_notes=()
declare -a operation_notes=()
declare -a port_notes=()

if [ ! -f "$config_path" ] || [ -L "$config_path" ]; then
  secret_safe=fail
  secret_notes+=("config_missing_or_not_regular")
else
  [ "$(factory_v1_mode "$config_path")" = 600 ] || { secret_safe=fail; secret_notes+=("config_mode_not_0600"); }
  [ "$(stat -c '%u' -- "$config_path")" -eq "$(id -u)" ] || { secret_safe=fail; secret_notes+=("config_owner_mismatch"); }
fi

tracked_config=$(factory_v1_realpath_missing_ok "$(cd "$script_dir/../.." && pwd)/loop/config.env")
if [ "$config_path" = "$tracked_config" ]; then
  secret_safe=fail
  secret_notes+=("tracked_loop_config_selected")
fi

for private_dir in "$runtime_root/config" "$runtime_root/run" "$runtime_root/manifests" "$runtime_root/receipts"; do
  if [ -e "$private_dir" ] && [ "$(factory_v1_mode "$private_dir")" != 700 ]; then
    secret_safe=fail
    secret_notes+=("private_directory_mode:$private_dir")
  fi
done

if [ -n "$operation_paths_file" ]; then
  [ -f "$operation_paths_file" ] || factory_v1_die "Operation #86 paths file is unavailable: $operation_paths_file"
  while IFS= read -r protected_path || [ -n "$protected_path" ]; do
    [ -n "$protected_path" ] || continue
    [[ "$protected_path" = \#* ]] && continue
    protected_path=$(factory_v1_realpath_missing_ok "$protected_path")
    if factory_v1_path_is_within "$runtime_root" "$protected_path" || factory_v1_path_is_within "$protected_path" "$runtime_root"; then
      operation_safe=fail
      operation_notes+=("runtime_overlap:$protected_path")
    fi
    [ -e "$protected_path" ] || { operation_safe=fail; operation_notes+=("protected_path_missing:$protected_path"); }
  done <"$operation_paths_file"
else
  operation_safe=not_configured
  operation_notes+=("comparison_hook_not_configured")
fi

for port in 5432 8080 8083 8088; do
  pids=$(factory_v1_listener_pids "$port")
  if [ -n "$pids" ]; then
    ports_safe=fail
    port_notes+=("existing_listener:$port:$(tr '\n' ',' <<<"$pids" | sed 's/,$//')")
  fi
done

overall=pass
[ "$secret_safe" = pass ] && [ "$operation_safe" = pass ] && [ "$ports_safe" = pass ] || overall=fail

jq -n \
  --arg generated_at "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
  --arg overall "$overall" \
  --arg secret_safe "$secret_safe" \
  --arg operation_safe "$operation_safe" \
  --arg ports_safe "$ports_safe" \
  --arg runtime_root "$runtime_root" \
  --arg config_sha256 "$([ -f "$config_path" ] && factory_v1_sha256_file "$config_path" || printf unavailable)" \
  --argjson secret_notes "$(printf '%s\n' "${secret_notes[@]:-}" | jq -Rsc 'split("\n") | map(select(length > 0))')" \
  --argjson operation_notes "$(printf '%s\n' "${operation_notes[@]:-}" | jq -Rsc 'split("\n") | map(select(length > 0))')" \
  --argjson port_notes "$(printf '%s\n' "${port_notes[@]:-}" | jq -Rsc 'split("\n") | map(select(length > 0))')" \
  '{schema:"factory-v1-preflight-receipt/v1",generated_at:$generated_at,status:$overall,runtime_root:$runtime_root,config_sha256:$config_sha256,checks:[{name:"TestFactoryV1SecretSafeIsolation",status:$secret_safe,notes:$secret_notes},{name:"TestFactoryV1Operation86Immutability",status:$operation_safe,notes:$operation_notes},{name:"TestFactoryV1RequiredPortsAvailable",status:$ports_safe,notes:$port_notes}]}' \
  >"${output}.tmp.$$"
chmod 600 "${output}.tmp.$$"
mv -f -- "${output}.tmp.$$" "$output"
cat -- "$output"
[ "$overall" = pass ]
