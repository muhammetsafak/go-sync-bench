# Shared by verify.sh and run.sh.
# shellcheck shell=bash
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

IMAGE=go-sync-bench
GO_IMAGE=golang:1.27.1-trixie
NODE_IMAGE=node:24-trixie-slim
MATRIX=bench/matrix.json

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }

# cpuset_for <P>: the vCPUs a P container is pinned to (vCPU 0-3 stay with the
# Docker VM kernel and dockerd).
cpuset_for() { jq -r --arg p "$1" '.cpusets[$p]' "$MATRIX"; }

build_image() {
  # The Dockerfile only copies local files; the build needs no network.
  docker build --network none -q -f docker/Dockerfile -t "$IMAGE" . >/dev/null
}

# sandbox_run <P> <results-dir|-> <syncbench args...>
# The measurement sandbox: no network, read-only root, a private /tmp, no
# capabilities, an unprivileged user, P vCPUs, 4 GiB and no swap. The only
# writable bind mount is the result directory of the run.
sandbox_run() {
  local p="$1" out="$2"; shift 2
  local cs; cs="$(cpuset_for "$p")"
  local mount=()
  [ "$out" != "-" ] && mount=(-v "$out:/results")
  docker run --rm --network none --read-only --tmpfs /tmp \
    --cpuset-cpus "$cs" -e GOMAXPROCS="$p" \
    --memory 4g --memory-swap 4g --pids-limit 1024 \
    --user 65534:65534 --cap-drop ALL --security-opt no-new-privileges \
    "${mount[@]}" "$IMAGE" "$@"
}

# tool_run <args...>: a one-shot helper call of the harness (cell lists, shuffle
# orders) in the same sandbox, without CPU pinning. With TOOL_MOUNT set, that
# directory is mounted read-only at /results (a cell rerun reads its list there).
tool_run() {
  docker run --rm --network none --read-only --tmpfs /tmp \
    --user 65534:65534 --cap-drop ALL --security-opt no-new-privileges \
    ${TOOL_MOUNT:+-v "$TOOL_MOUNT:/results:ro"} "$IMAGE" "$@"
}

# host_load: summed %CPU of every host process except the Docker VM and its
# helpers, averaged over three samples one second apart (gate E1). ps is a
# read-only host command; the harness itself never runs on the host.
host_load() {
  local total=0 i s
  for i in 1 2 3; do
    s=$(ps -A -o %cpu=,comm= | awk '
      /com\.apple\.Virtualization|Docker|docker|vpnkit|qemu/ { next }
      { sum += $1 } END { printf "%.1f", sum }')
    total=$(awk -v a="$total" -v b="$s" 'BEGIN { printf "%.1f", a + b }')
    [ "$i" -lt 3 ] && sleep 1
  done
  awk -v t="$total" 'BEGIN { printf "%.1f", t / 3 }'
}

# power_ok: gate E2. On battery or with a recorded thermal/performance warning
# the run does not start. Prints the raw pmset lines for RUN.md.
power_ok() {
  local batt therm
  batt="$(pmset -g batt 2>/dev/null | head -1)"
  therm="$(pmset -g therm 2>/dev/null | tr '\n' ' ')"
  printf 'batt: %s\ntherm: %s\n' "$batt" "$therm"
  case "$batt" in *"AC Power"*) ;; *) return 1 ;; esac
  # "No ... warning level has been recorded" is the healthy output; a recorded
  # level above 0 or a CPU speed limit below 100 is a warning.
  pmset -g therm 2>/dev/null | awk '
    /warning level/ && !/No / { for (i = 1; i <= NF; i++) if ($i ~ /^[0-9]+$/ && $i > 0) bad = 1 }
    /Speed_Limit/ { for (i = 1; i <= NF; i++) if ($i ~ /^[0-9]+$/ && $i < 100) bad = 1 }
    END { exit bad }' || return 1
  return 0
}

# pmset_sleep_value: reads `pmset -g` on stdin and prints the value of the
# system "sleep" setting (minutes of idle before system sleep, 0 = never), or
# "unknown" when the line is missing. displaysleep and disksleep do not count.
pmset_sleep_value() {
  awk '$1 == "sleep" && $2 ~ /^[0-9]+$/ { print $2; found = 1; exit }
       END { if (!found) print "unknown" }'
}

# sleep_ok: the system-sleep part of gate E2. Prints the value for RUN.md and
# fails unless system sleep is disabled (sleep 0). A caffeinate assertion is
# not enough: it ends with the process that holds it, the setting does not.
sleep_ok() {
  local v
  v="$(pmset -g 2>/dev/null | pmset_sleep_value)"
  printf 'sleep: %s\n' "$v"
  [ "$v" = 0 ]
}
