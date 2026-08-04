#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib.sh
source "$script_dir/lib.sh"

factory_v1_require_command jq
factory_v1_require_command lsof
factory_v1_require_command sha256sum
factory_v1_require_command realpath

repo_root=$(cd "$script_dir/../.." && pwd)
runtime_root=${FACTORY_V1_RUNTIME_ROOT:-/home/transpara/transpara-ai/runtime/civilization-dark-factory-v1}
runtime_root=$(factory_v1_realpath_missing_ok "$runtime_root")
factory_v1_assert_runtime_root "$runtime_root"

config_path=${FACTORY_V1_CONFIG_PATH:-$runtime_root/config/runtime.env}
config_path=$(factory_v1_realpath_missing_ok "$config_path")
factory_v1_path_is_within "$config_path" "$runtime_root" || factory_v1_die "config must be inside the fresh runtime root"

tracked_loop_config=$(factory_v1_realpath_missing_ok "$repo_root/loop/config.env")
[ "$config_path" != "$tracked_loop_config" ] || factory_v1_die "tracked loop/config.env is forbidden"

run_dir=$runtime_root/run
log_dir=$runtime_root/logs
manifest_dir=$runtime_root/manifests
receipt_dir=$runtime_root/receipts
postgres_data=$runtime_root/postgres/data
postgres_socket=$runtime_root/postgres/socket

postgres_distribution=${FACTORY_V1_POSTGRES_DISTRIBUTION:-/home/transpara/.local/lib/civilization-tools/postgresql-16.14-0ubuntu0.24.04.1}
postgres_bin=$postgres_distribution/usr/lib/postgresql/16/bin
postgres_library=$postgres_distribution/usr/lib/x86_64-linux-gnu
postgres_ld_library_path=$postgres_library${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}

hive_root=${FACTORY_V1_HIVE_ROOT:-$repo_root}
work_root=${FACTORY_V1_WORK_ROOT:-/home/transpara/transpara-ai/repos/work}
site_root=${FACTORY_V1_SITE_ROOT:-/home/transpara/transpara-ai/repos/site}

hive_daemon_cmd=${FACTORY_V1_HIVE_DAEMON_CMD:-./bin/hive factory-v1 daemon}
hive_ops_cmd=${FACTORY_V1_HIVE_OPS_CMD:-./bin/hive-ops-api}
work_cmd=${FACTORY_V1_WORK_CMD:-env PORT=8080 ./bin/work-server}
site_cmd=${FACTORY_V1_SITE_CMD:-env PORT=8088 ./bin/site}

declare -a service_names=(postgres hive-daemon hive-ops work site)
declare -A service_ports=(
  [postgres]=5432
  [hive-daemon]=0
  [hive-ops]=8083
  [work]=8080
  [site]=8088
)
declare -A service_roots=(
  [postgres]="$runtime_root"
  [hive-daemon]="$hive_root"
  [hive-ops]="$hive_root"
  [work]="$work_root"
  [site]="$site_root"
)
declare -A service_commands=(
  [postgres]=""
  [hive-daemon]="$hive_daemon_cmd"
  [hive-ops]="$hive_ops_cmd"
  [work]="$work_cmd"
  [site]="$site_cmd"
)

now_utc() {
  date -u +'%Y-%m-%dT%H:%M:%SZ'
}

prepare_directories() {
  install -d -m 700 -- "$runtime_root" "$(dirname "$config_path")" "$run_dir" "$log_dir" "$manifest_dir" "$receipt_dir" "$postgres_socket"
}

random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
  fi
}

initialize_config() {
  prepare_directories
  if [ ! -e "$config_path" ]; then
    local token tmp
    token=$(random_hex)
    tmp="${config_path}.tmp.$$"
    {
      printf 'DATABASE_URL=%q\n' 'postgresql://127.0.0.1:5432/factory_v1?sslmode=disable'
      printf 'PGHOST=%q\n' '127.0.0.1'
      printf 'PGPORT=%q\n' '5432'
      printf 'PGDATABASE=%q\n' 'factory_v1'
      printf 'HIVE_FACTORY_V1_HOST=%q\n' '127.0.0.1'
      printf 'HIVE_FACTORY_V1_PORT=%q\n' '8083'
	  printf 'HIVE_FACTORY_V1_ENABLED=%q\n' 'true'
	  printf 'HIVE_FACTORY_V1_ACTOR_ID=%q\n' 'actor_00000000000000000000000000000086'
	  printf 'HIVE_FACTORY_V1_CREDENTIAL_KEY_ID=%q\n' 'factory-v1-local-operator'
	  printf 'HIVE_FACTORY_V1_INSTANCE_ID=%q\n' 'civilization-dark-factory-v1'
      printf 'WORK_HOST=%q\n' '127.0.0.1'
      printf 'WORK_PORT=%q\n' '8080'
      printf 'WORK_HUMAN=%q\n' 'Civilization v1 local operator'
	  printf 'WORK_BIND_HOST=%q\n' '127.0.0.1'
      printf 'SITE_HOST=%q\n' '127.0.0.1'
      printf 'SITE_PORT=%q\n' '8088'
	  printf 'SITE_BIND_HOST=%q\n' '127.0.0.1'
	  printf 'HIVE_OPS_API_ADDR=%q\n' '127.0.0.1:8083'
	  printf 'HIVE_OPS_API_BASE_URL=%q\n' 'http://127.0.0.1:8083'
	  printf 'HIVE_OPS_HUMAN_ACTOR=%q\n' 'actor_00000000000000000000000000000086'
      printf 'FACTORY_V1_RUNTIME_ROOT=%q\n' "$runtime_root"
      printf 'FACTORY_V1_CONFIG_PATH=%q\n' "$config_path"
      printf 'FACTORY_V1_OPERATOR_TOKEN=%q\n' "$token"
	  printf 'HIVE_OPS_API_KEY=%q\n' "$token"
	  printf 'WORK_API_KEY=%q\n' "$token"
	  printf 'WORK_API_TOKEN=%q\n' "$token"
	  printf 'HIVE_OPS_SIGNING_KEY=%q\n' "$(random_hex)"
      printf 'FACTORY_V1_CREDENTIAL_SOURCE_ID=%q\n' 'fresh-local-runtime-env-v1'
    } >"$tmp"
    chmod 600 "$tmp"
    mv -f -- "$tmp" "$config_path"
  fi
  [ -f "$config_path" ] || factory_v1_die "config is not a regular file: $config_path"
  [ ! -L "$config_path" ] || factory_v1_die "config must not be a symlink"
  [ "$(stat -c '%u' -- "$config_path")" -eq "$(id -u)" ] || factory_v1_die "config owner mismatch"
  [ "$(factory_v1_mode "$config_path")" = 600 ] || factory_v1_die "config must have mode 0600"
}

load_fresh_config() {
  initialize_config
  # This is the supervisor-created, mode-0600 file inside the validated fresh
  # runtime root.  The tracked loop configuration is never sourced.
  set -a
  # shellcheck disable=SC1090
  source "$config_path"
  set +a
}

assert_service_inputs() {
  local service root command
  for service in hive-daemon hive-ops work site; do
    root=${service_roots[$service]}
    command=${service_commands[$service]}
    [ -d "$root" ] || factory_v1_die "$service root is unavailable: $root"
    [ -n "$command" ] || factory_v1_die "$service command is empty: $service"
    bash -n <<<"$command" || factory_v1_die "$service command is not valid shell syntax"
  done
  if [ -z "${FACTORY_V1_POSTGRES_CMD:-}" ]; then
    for command in postgres initdb createdb pg_isready; do
      [ -x "$postgres_bin/$command" ] || factory_v1_die "bundled PostgreSQL 16 tool is unavailable: $postgres_bin/$command"
    done
    env LD_LIBRARY_PATH="$postgres_ld_library_path" "$postgres_bin/postgres" --version | grep -q 'PostgreSQL) 16\.' || factory_v1_die "bundled PostgreSQL is not major version 16"
  fi
}

owner_file() {
  printf '%s/%s.owner.json\n' "$run_dir" "$1"
}

assert_record_pid() {
  local record=$1 field=$2 pid expected_ticks expected_exe expected_hash actual_exe
  pid=$(jq -er ".$field.pid" "$record") || return 1
  expected_ticks=$(jq -er ".$field.start_ticks" "$record") || return 1
  expected_exe=$(jq -er ".$field.executable" "$record") || return 1
  expected_hash=$(jq -er ".$field.executable_sha256" "$record") || return 1
  [ -r "/proc/$pid/stat" ] || return 1
  [ "$(factory_v1_pid_start_ticks "$pid")" = "$expected_ticks" ] || return 1
  actual_exe=$(factory_v1_pid_executable "$pid") || return 1
  [ "$actual_exe" = "$expected_exe" ] || return 1
  [ "$(factory_v1_sha256_file "$actual_exe")" = "$expected_hash" ] || return 1
}

assert_owner_record() {
  local service=$1 record port listener_pid
  record=$(owner_file "$service")
  [ -f "$record" ] || return 1
  assert_record_pid "$record" main || return 1
  if jq -e '.listener != null' "$record" >/dev/null; then
    assert_record_pid "$record" listener || return 1
    port=${service_ports[$service]}
    listener_pid=$(jq -er '.listener.pid' "$record") || return 1
    factory_v1_listener_pids "$port" | grep -qx "$listener_pid" || return 1
  fi
}

assert_live_record_pids_owned() {
  local record=$1 field pid
  for field in main listener; do
    pid=$(jq -r ".$field.pid // empty" "$record")
    [ -n "$pid" ] || continue
    [ -r "/proc/$pid/stat" ] || continue
    assert_record_pid "$record" "$field" || return 1
  done
}

preflight_ports() {
  local service port pids
  for service in postgres hive-ops work site; do
    port=${service_ports[$service]}
    pids=$(factory_v1_listener_pids "$port")
    if [ -n "$pids" ]; then
      factory_v1_die "preflight refused existing listener on 127.0.0.1:$port (pid(s): $(tr '\n' ',' <<<"$pids" | sed 's/,$//'))"
    fi
  done
}

write_owner_record() {
  local service=$1 main_pid=$2 listener_pid=${3:-} command=$4 record main_exe listener_exe config_hash
  record=$(owner_file "$service")
  main_exe=$(factory_v1_pid_executable "$main_pid")
  config_hash=$(factory_v1_sha256_file "$config_path")
  if [ -n "$listener_pid" ]; then
    listener_exe=$(factory_v1_pid_executable "$listener_pid")
    jq -n \
      --arg service "$service" \
      --arg started_at "$(now_utc)" \
      --arg command_sha256 "$(printf '%s' "$command" | sha256sum | awk '{print $1}')" \
      --arg config_sha256 "$config_hash" \
      --argjson main_pid "$main_pid" \
      --arg main_ticks "$(factory_v1_pid_start_ticks "$main_pid")" \
      --arg main_exe "$main_exe" \
      --arg main_sha "$(factory_v1_sha256_file "$main_exe")" \
      --argjson listener_pid "$listener_pid" \
      --arg listener_ticks "$(factory_v1_pid_start_ticks "$listener_pid")" \
      --arg listener_exe "$listener_exe" \
      --arg listener_sha "$(factory_v1_sha256_file "$listener_exe")" \
      '{schema:"factory-v1-process-owner/v1",service:$service,started_at:$started_at,command_sha256:$command_sha256,config_sha256:$config_sha256,main:{pid:$main_pid,start_ticks:$main_ticks,executable:$main_exe,executable_sha256:$main_sha},listener:{pid:$listener_pid,start_ticks:$listener_ticks,executable:$listener_exe,executable_sha256:$listener_sha}}' \
      >"${record}.tmp.$$"
  else
    jq -n \
      --arg service "$service" \
      --arg started_at "$(now_utc)" \
      --arg command_sha256 "$(printf '%s' "$command" | sha256sum | awk '{print $1}')" \
      --arg config_sha256 "$config_hash" \
      --argjson main_pid "$main_pid" \
      --arg main_ticks "$(factory_v1_pid_start_ticks "$main_pid")" \
      --arg main_exe "$main_exe" \
      --arg main_sha "$(factory_v1_sha256_file "$main_exe")" \
      '{schema:"factory-v1-process-owner/v1",service:$service,started_at:$started_at,command_sha256:$command_sha256,config_sha256:$config_sha256,main:{pid:$main_pid,start_ticks:$main_ticks,executable:$main_exe,executable_sha256:$main_sha},listener:null}' \
      >"${record}.tmp.$$"
  fi
  chmod 600 "${record}.tmp.$$"
  mv -f -- "${record}.tmp.$$" "$record"
}

wait_for_listener() {
  local service=$1 main_pid=$2 port=$3 deadline pids pid
  deadline=$((SECONDS + ${FACTORY_V1_START_TIMEOUT_SECONDS:-45}))
  while [ "$SECONDS" -lt "$deadline" ]; do
    kill -0 "$main_pid" 2>/dev/null || factory_v1_die "$service exited before binding port $port; inspect $log_dir/$service.log"
    pids=$(factory_v1_listener_pids "$port")
    if [ -n "$pids" ]; then
      while IFS= read -r pid; do
        [ -n "$pid" ] || continue
        if factory_v1_is_descendant_or_self "$pid" "$main_pid"; then
          printf '%s\n' "$pid"
          return 0
        fi
      done <<<"$pids"
      factory_v1_die "$service did not own the process that bound port $port"
    fi
    sleep 0.2
  done
  factory_v1_die "$service timed out waiting for port $port; inspect $log_dir/$service.log"
}

assert_loopback_listener() {
  local service=$1 pid=$2 port=$3 endpoints
  endpoints=$(lsof -Pan -p "$pid" -iTCP:"$port" -sTCP:LISTEN -Fn 2>/dev/null | sed -n 's/^n//p')
  [ -n "$endpoints" ] || factory_v1_die "$service listener endpoint could not be inspected"
  while IFS= read -r endpoint; do
    case "$endpoint" in
      127.0.0.1:"$port"|'[::1]':"$port") ;;
      *) factory_v1_die "$service must be loopback-only; found listener endpoint $endpoint" ;;
    esac
  done <<<"$endpoints"
}

start_command_service() {
  local service=$1 command=${service_commands[$1]} root=${service_roots[$1]} port=${service_ports[$1]} main_pid listener_pid=''
  (
    cd "$root"
    exec bash -c "exec $command"
  ) >>"$log_dir/$service.log" 2>&1 &
  main_pid=$!
  write_owner_record "$service" "$main_pid" "" "$command"
  if [ "$port" -gt 0 ]; then
    listener_pid=$(wait_for_listener "$service" "$main_pid" "$port")
    assert_loopback_listener "$service" "$listener_pid" "$port"
  else
    sleep 0.25
    kill -0 "$main_pid" 2>/dev/null || factory_v1_die "$service exited during startup; inspect $log_dir/$service.log"
  fi
  write_owner_record "$service" "$main_pid" "$listener_pid" "$command"
}

initialize_postgres_data() {
  [ -f "$postgres_data/PG_VERSION" ] && return 0
  install -d -m 700 -- "$postgres_data"
  env LD_LIBRARY_PATH="$postgres_ld_library_path" "$postgres_bin/initdb" --pgdata="$postgres_data" --encoding=UTF8 --no-locale --auth-local=trust --auth-host=trust >"$log_dir/postgres-init.log" 2>&1
  [ "$(<"$postgres_data/PG_VERSION")" = 16 ] || factory_v1_die "fresh PostgreSQL data root is not version 16"
}

start_postgres() {
  local command main_pid listener_pid
  if [ -n "${FACTORY_V1_POSTGRES_CMD:-}" ]; then
    command=$FACTORY_V1_POSTGRES_CMD
    service_commands[postgres]=$command
    service_roots[postgres]=${FACTORY_V1_POSTGRES_ROOT:-$runtime_root}
    start_command_service postgres
    return
  fi

  initialize_postgres_data
  command="$postgres_bin/postgres -D $postgres_data -k $postgres_socket -c listen_addresses=127.0.0.1 -c port=5432"
  env LD_LIBRARY_PATH="$postgres_ld_library_path" "$postgres_bin/postgres" -D "$postgres_data" -k "$postgres_socket" -c listen_addresses=127.0.0.1 -c port=5432 >>"$log_dir/postgres.log" 2>&1 &
  main_pid=$!
  write_owner_record postgres "$main_pid" "" "$command"
  listener_pid=$(wait_for_listener postgres "$main_pid" 5432)
  assert_loopback_listener postgres "$listener_pid" 5432
  write_owner_record postgres "$main_pid" "$listener_pid" "$command"

  if ! env LD_LIBRARY_PATH="$postgres_ld_library_path" "$postgres_bin/psql" -h "$postgres_socket" -p 5432 -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname='factory_v1'" | grep -qx 1; then
    env LD_LIBRARY_PATH="$postgres_ld_library_path" "$postgres_bin/createdb" -h "$postgres_socket" -p 5432 factory_v1
  fi
}

write_manifest() {
  local tmp=$manifest_dir/process-manifest.json.tmp.$$ final=$manifest_dir/process-manifest.json
  jq -s --arg generated_at "$(now_utc)" --arg runtime_root "$runtime_root" --arg config_sha256 "$(factory_v1_sha256_file "$config_path")" \
    '{schema:"factory-v1-supervisor-manifest/v1",generated_at:$generated_at,runtime_root:$runtime_root,config_sha256:$config_sha256,processes:.}' \
    "$run_dir"/*.owner.json >"$tmp"
  chmod 600 "$tmp"
  mv -f -- "$tmp" "$final"
  factory_v1_sha256_file "$final" >"$manifest_dir/process-manifest.sha256"
  chmod 600 "$manifest_dir/process-manifest.sha256"
}

emit_status() {
  local output=${1:-$receipt_dir/status-$(date -u +'%Y%m%dT%H%M%SZ').json} service state port owner listener_pid
  local rows=$runtime_root/status-rows.$$.jsonl
  : >"$rows"
  chmod 600 "$rows"
  for service in "${service_names[@]}"; do
    state=stopped
    owner=$(owner_file "$service")
    if [ -f "$owner" ]; then
      if assert_owner_record "$service"; then state=running; else state=ownership_mismatch; fi
    fi
    port=${service_ports[$service]}
    listener_pid=null
    if [ "$port" -gt 0 ] && [ -f "$owner" ]; then
      listener_pid=$(jq '.listener.pid // null' "$owner")
    fi
    jq -n --arg service "$service" --arg state "$state" --argjson port "$port" --argjson listener_pid "$listener_pid" \
      '{service:$service,state:$state,port:$port,listener_pid:$listener_pid}' >>"$rows"
  done
  jq -s --arg generated_at "$(now_utc)" --arg runtime_root "$runtime_root" --arg config_sha256 "$(factory_v1_sha256_file "$config_path")" \
    '{schema:"factory-v1-supervisor-status/v1",generated_at:$generated_at,runtime_root:$runtime_root,config_sha256:$config_sha256,services:.}' \
    "$rows" >"${output}.tmp.$$"
  chmod 600 "${output}.tmp.$$"
  mv -f -- "${output}.tmp.$$" "$output"
  rm -f -- "$rows"
  cat -- "$output"
}

start_stack() {
  trap 'start_rc=$?; trap - EXIT ERR; stop_stack >/dev/null 2>&1 || true; exit "$start_rc"' EXIT
  load_fresh_config
  assert_service_inputs
  local service
  for service in "${service_names[@]}"; do
    [ ! -e "$(owner_file "$service")" ] || factory_v1_die "owner record already exists for $service; use status, stop, or restart"
  done
  preflight_ports
  start_postgres
  start_command_service hive-daemon
  start_command_service hive-ops
  start_command_service work
  start_command_service site
  write_manifest
  emit_status >/dev/null
  trap - EXIT ERR
  printf 'factory-v1 stack started; status receipt: %s\n' "$receipt_dir"
}

stop_stack() {
  load_fresh_config
  local service record pid listener_pid
  for service in "${service_names[@]}"; do
    record=$(owner_file "$service")
    [ ! -e "$record" ] || assert_live_record_pids_owned "$record" || factory_v1_die "refusing to stop $service: PID/executable ownership mismatch"
  done
  for service in site work hive-ops hive-daemon postgres; do
    record=$(owner_file "$service")
    [ -f "$record" ] || continue
    listener_pid=$(jq -r '.listener.pid // empty' "$record")
    pid=$(jq -r '.main.pid' "$record")
    if [ -n "$listener_pid" ] && [ "$listener_pid" != "$pid" ] && [ -r "/proc/$listener_pid/stat" ]; then kill -TERM "$listener_pid" 2>/dev/null || true; fi
    if [ -r "/proc/$pid/stat" ]; then kill -TERM "$pid" 2>/dev/null || true; fi
    local deadline=$((SECONDS + ${FACTORY_V1_STOP_TIMEOUT_SECONDS:-20}))
    while { [ -r "/proc/$pid/stat" ] || { [ -n "$listener_pid" ] && [ -r "/proc/$listener_pid/stat" ]; }; } && [ "$SECONDS" -lt "$deadline" ]; do sleep 0.2; done
    if [ -r "/proc/$pid/stat" ] || { [ -n "$listener_pid" ] && [ -r "/proc/$listener_pid/stat" ]; }; then
      factory_v1_die "$service did not stop after TERM; no KILL was sent"
    fi
    rm -f -- "$record"
  done
  printf 'factory-v1 stack stopped\n'
}

preflight() {
  load_fresh_config
  assert_service_inputs
  "$script_dir/preflight.sh" --runtime-root "$runtime_root" --config "$config_path"
}

usage() {
  printf 'usage: %s {init|preflight|start|status|restart|stop}\n' "$0" >&2
  exit 2
}

action=${1:-}
case "$action" in
  init) initialize_config; printf 'initialized fresh factory-v1 runtime at %s\n' "$runtime_root" ;;
  preflight) preflight ;;
  start) start_stack ;;
  status) load_fresh_config; emit_status ;;
  restart) stop_stack; start_stack ;;
  stop) stop_stack ;;
  *) usage ;;
esac
