#!/usr/bin/env bash
set -euo pipefail
umask 077

test_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script_dir=$(cd "$test_dir/.." && pwd)
fixture=$test_dir/fixtures/listener.sh
ready_fixture=$test_dir/fixtures/postgres-ready.sh
tmp_root=$(mktemp -d /tmp/factory-v1-scripts-test.XXXXXX)
external_pid=''

cleanup() {
  if [ -n "$external_pid" ] && kill -0 "$external_pid" 2>/dev/null; then
    kill -TERM "$external_pid" 2>/dev/null || true
  fi
  FACTORY_V1_RUNTIME_ROOT="$tmp_root/runtime" \
    FACTORY_V1_HIVE_ROOT="$tmp_root/hive" \
    FACTORY_V1_WORK_ROOT="$tmp_root/work" \
    FACTORY_V1_SITE_ROOT="$tmp_root/site" \
    FACTORY_V1_POSTGRES_CMD="$fixture 5432" \
    FACTORY_V1_HIVE_DAEMON_CMD="$fixture 8084" \
    FACTORY_V1_HIVE_OPS_CMD="$fixture 8083" \
    FACTORY_V1_WORK_CMD="$fixture 8080" \
    FACTORY_V1_SITE_CMD="$fixture 8088" \
    "$script_dir/supervisor.sh" stop >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$tmp_root/hive" "$tmp_root/work" "$tmp_root/site" "$tmp_root/protected"
printf 'sealed\n' >"$tmp_root/protected/evidence.txt"
printf '%s\n' "$tmp_root/protected" >"$tmp_root/operation86-paths.txt"

export FACTORY_V1_RUNTIME_ROOT="$tmp_root/runtime"
export FACTORY_V1_HIVE_ROOT="$tmp_root/hive"
export FACTORY_V1_WORK_ROOT="$tmp_root/work"
export FACTORY_V1_SITE_ROOT="$tmp_root/site"
export FACTORY_V1_POSTGRES_CMD="$fixture 5432"
export FACTORY_V1_POSTGRES_READY_CMD="$ready_fixture $tmp_root/postgres-ready.count 3"
export FACTORY_V1_HIVE_DAEMON_CMD="$fixture 8084"
export FACTORY_V1_HIVE_OPS_CMD="$fixture 8083"
export FACTORY_V1_WORK_CMD="$fixture 8080"
export FACTORY_V1_SITE_CMD="$fixture 8088"
export FACTORY_V1_START_TIMEOUT_SECONDS=10
export FACTORY_V1_STOP_TIMEOUT_SECONDS=5

"$script_dir/supervisor.sh" init >/dev/null
[ "$(stat -c '%a' "$tmp_root/runtime/config/runtime.env")" = 600 ]
for key in FACTORY_V1_RUNTIME_ADDR FACTORY_V1_RUNTIME_API_KEY HIVE_FACTORY_V1_RUNTIME_URL HIVE_FACTORY_V1_RUNTIME_API_KEY; do
  grep -q "^${key}=" "$tmp_root/runtime/config/runtime.env"
done

# Legacy configurations migrate only when all four observation keys are absent;
# partial observation configuration fails closed.
sed -i -E '/^(FACTORY_V1_RUNTIME_ADDR|FACTORY_V1_RUNTIME_API_KEY|HIVE_FACTORY_V1_RUNTIME_URL|HIVE_FACTORY_V1_RUNTIME_API_KEY)=/d' "$tmp_root/runtime/config/runtime.env"
"$script_dir/supervisor.sh" status >/dev/null
for key in FACTORY_V1_RUNTIME_ADDR FACTORY_V1_RUNTIME_API_KEY HIVE_FACTORY_V1_RUNTIME_URL HIVE_FACTORY_V1_RUNTIME_API_KEY; do
  grep -q "^${key}=" "$tmp_root/runtime/config/runtime.env"
done
cp -- "$tmp_root/runtime/config/runtime.env" "$tmp_root/runtime/config/runtime.env.complete"
sed -i '/^HIVE_FACTORY_V1_RUNTIME_API_KEY=/d' "$tmp_root/runtime/config/runtime.env"
if "$script_dir/supervisor.sh" status >"$tmp_root/partial-config.out" 2>"$tmp_root/partial-config.err"; then
  printf 'expected partial runtime observation config to fail\n' >&2
  exit 1
fi
grep -q 'runtime observation config is partial' "$tmp_root/partial-config.err"
mv -f -- "$tmp_root/runtime/config/runtime.env.complete" "$tmp_root/runtime/config/runtime.env"

# The standalone receipt must include the daemon's required port 8084.
"$fixture" 8084 &
external_pid=$!
for _ in $(seq 1 50); do
  lsof -nP -t -iTCP:8084 -sTCP:LISTEN >/dev/null 2>&1 && break
  sleep 0.1
done
if "$script_dir/preflight.sh" \
  --runtime-root "$tmp_root/runtime" \
  --config "$tmp_root/runtime/config/runtime.env" \
  --operation86-paths "$tmp_root/operation86-paths.txt" \
  --output "$tmp_root/preflight-8084-occupied.json" >/dev/null; then
  printf 'expected standalone preflight to reject occupied port 8084\n' >&2
  exit 1
fi
jq -e '.status == "fail" and any(.checks[]; .name == "TestFactoryV1RequiredPortsAvailable" and (.notes | any(startswith("existing_listener:8084:"))))' "$tmp_root/preflight-8084-occupied.json" >/dev/null
kill -TERM "$external_pid"
wait "$external_pid" || true
external_pid=''

"$script_dir/preflight.sh" \
  --runtime-root "$tmp_root/runtime" \
  --config "$tmp_root/runtime/config/runtime.env" \
  --operation86-paths "$tmp_root/operation86-paths.txt" \
  --output "$tmp_root/preflight.json" >/dev/null
jq -e '.status == "pass" and ([.checks[].name] | index("TestFactoryV1SecretSafeIsolation") != null) and ([.checks[].name] | index("TestFactoryV1Operation86Immutability") != null)' "$tmp_root/preflight.json" >/dev/null

"$script_dir/operation86-evidence.sh" capture \
  --runtime-root "$tmp_root/runtime" \
  --paths "$tmp_root/operation86-paths.txt" \
  --output "$tmp_root/baseline.json" >/dev/null
"$script_dir/operation86-evidence.sh" compare \
  --runtime-root "$tmp_root/runtime" \
  --paths "$tmp_root/operation86-paths.txt" \
  --baseline "$tmp_root/baseline.json" \
  --output "$tmp_root/final.json" >/dev/null
jq -e '.check == "TestFactoryV1Operation86Immutability" and .status == "pass"' "$tmp_root/final.json" >/dev/null

"$script_dir/supervisor.sh" start >/dev/null
[ "$(<"$tmp_root/postgres-ready.count")" -ge 3 ]
status_json=$("$script_dir/supervisor.sh" status)
jq -e '[.services[].state] | all(. == "running")' <<<"$status_json" >/dev/null
[ -s "$tmp_root/runtime/manifests/process-manifest.json" ]
[ -s "$tmp_root/runtime/manifests/process-manifest.sha256" ]

"$script_dir/supervisor.sh" restart >/dev/null
status_json=$("$script_dir/supervisor.sh" status)
jq -e '[.services[].state] | all(. == "running")' <<<"$status_json" >/dev/null
"$script_dir/supervisor.sh" stop >/dev/null

# A crashed process leaves stale, hashed metadata.  Restart/stop may remove a
# record only after proving every recorded PID is absent; no unrelated PID is
# signalled.
"$script_dir/supervisor.sh" start >/dev/null
hive_daemon_pid=$(jq -r '.main.pid' "$tmp_root/runtime/run/hive-daemon.owner.json")
kill -TERM "$hive_daemon_pid"
for _ in $(seq 1 50); do
  [ ! -r "/proc/$hive_daemon_pid/stat" ] && break
  sleep 0.1
done
"$script_dir/supervisor.sh" restart >/dev/null
"$script_dir/supervisor.sh" stop >/dev/null

# A service that exits during startup must roll back only the already-recorded
# fresh-runtime processes and leave all required ports free.
FACTORY_V1_HIVE_OPS_CMD='exit 17'
export FACTORY_V1_HIVE_OPS_CMD
if "$script_dir/supervisor.sh" start >"$tmp_root/partial-start.out" 2>"$tmp_root/partial-start.err"; then
  printf 'expected partial startup to fail\n' >&2
  exit 1
fi
for port in 5432 8080 8083 8084 8088; do
  if lsof -nP -t -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    printf 'partial startup left listener on port %s\n' "$port" >&2
    exit 1
  fi
done
FACTORY_V1_HIVE_OPS_CMD="$fixture 8083"
export FACTORY_V1_HIVE_OPS_CMD

"$fixture" 8083 &
external_pid=$!
for _ in $(seq 1 50); do
  lsof -nP -t -iTCP:8083 -sTCP:LISTEN >/dev/null 2>&1 && break
  sleep 0.1
done
if "$script_dir/supervisor.sh" start >"$tmp_root/unexpected-start.out" 2>"$tmp_root/preflight-failure.err"; then
  printf 'expected start to fail closed on external listener\n' >&2
  exit 1
fi
kill -TERM "$external_pid"
wait "$external_pid" || true
external_pid=''

# Foreground run mode keeps ownership under an external service manager and
# stops the whole stack when that manager sends TERM.
# A reboot-style stale receipt whose PID has been reused is discarded without
# signalling that unrelated process.
jq -n --argjson pid "$$" \
  '{schema:"factory-v1-process-owner/v1",service:"site",main:{pid:$pid,start_ticks:"0",executable:"/not/the/test/shell",executable_sha256:"0000"},listener:null}' \
  >"$tmp_root/runtime/run/site.owner.json"
"$script_dir/supervisor.sh" run >"$tmp_root/run-mode.out" 2>"$tmp_root/run-mode.err" &
run_pid=$!
for _ in $(seq 1 100); do
  ready=true
  for port in 5432 8080 8083 8084 8088; do
    lsof -nP -t -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1 || ready=false
  done
  grep -q 'factory-v1 stack started' "$tmp_root/run-mode.out" || ready=false
  "$ready" && break
  sleep 0.1
done
grep -q 'discarded stale site receipt after PID reuse' "$tmp_root/run-mode.out"
kill -TERM "$run_pid"
wait "$run_pid"
for port in 5432 8080 8083 8084 8088; do
  if lsof -nP -t -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    printf 'run mode left listener on port %s after TERM\n' "$port" >&2
    exit 1
  fi
done

if FACTORY_V1_MONITOR_INTERVAL_SECONDS=invalid "$script_dir/supervisor.sh" run >"$tmp_root/invalid-monitor.out" 2>"$tmp_root/invalid-monitor.err"; then
  printf 'expected invalid monitor interval to fail\n' >&2
  exit 1
fi
grep -q 'FACTORY_V1_MONITOR_INTERVAL_SECONDS must be a positive integer' "$tmp_root/invalid-monitor.err"
for port in 5432 8080 8083 8084 8088; do
  if lsof -nP -t -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    printf 'invalid monitor interval left listener on port %s\n' "$port" >&2
    exit 1
  fi
done

grep -q '^EnvironmentFile=-' "$script_dir/civilization-factory-v1.service.example"
grep -q '^StartLimitIntervalSec=0' "$script_dir/civilization-factory-v1.service.example"

printf 'factory-v1 shell tests: PASS\n'
