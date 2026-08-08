#!/usr/bin/env bash
set -euo pipefail

counter=${1:?counter path required}
ready_after=${2:?ready threshold required}
count=0
[ ! -f "$counter" ] || count=$(<"$counter")
count=$((count + 1))
printf '%s\n' "$count" >"$counter"
[ "$count" -ge "$ready_after" ]
