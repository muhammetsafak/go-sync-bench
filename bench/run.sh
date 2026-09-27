#!/usr/bin/env bash
# Orchestrator of the go-sync-bench experiment.
#
# Protocol, in one place so the report can quote it:
#   * a round runs every cell of the set once; a set has 5 (main) or 3 (smoke)
#     rounds, never five repetitions back to back
#   * two-level shuffle per round, derived from the run seed: (a) the order of
#     the P groups, (b) the order of the cells inside each P group; both orders
#     and the seed are written to RUN.md
#   * one container per round x P, pinned to that P's cpuset with
#     GOMAXPROCS=P; inside it every repetition is its own process, so rusage,
#     runtime/metrics and the heap start clean
#   * the container has no network, a read-only root, a tmpfs /tmp, no
#     capabilities, an unprivileged user, 4 GiB without swap; the only
#     writable mount is the result directory of the run
#   * gate E2 (AC power, no thermal warning) before every round, gate E1 (a
#     fixed cool-down, then host load < one core) after every container
#   * raw JSON is kept verbatim; report.mjs derives every number
#
#   STAMP=<YYYY-MM-DD> ./bench/run.sh --all      the main matrix (845 cells x 5 rounds)
#   ./bench/run.sh --smoke                       the smoke set (22 cells x 3 rounds)
#   STAMP=<stamp> ./bench/run.sh --all --resume  skip repetitions already written
#
# Environment: SEED (default: cksum of the stamp), COOL (cool-down seconds;
# 60 for --all, 5 for --smoke), E1_ENFORCE (1 for --all, 0 for --smoke: the
# smoke run records the host load but does not wait for it).
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

MODE=""
RESUME=""
for a in "$@"; do
  case "$a" in
    --all) MODE=all ;;
    --smoke) MODE=smoke ;;
    --resume) RESUME=-resume ;;
    *) echo "usage: $0 --all|--smoke [--resume]" >&2; exit 2 ;;
  esac
done
[ -n "$MODE" ] || { echo "usage: $0 --all|--smoke [--resume]" >&2; exit 2; }

if [ "$MODE" = all ]; then
  SET=main
  STAMP="${STAMP:-$(date -u +%Y-%m-%d)}"
  COOL="${COOL:-60}"
  E1_ENFORCE="${E1_ENFORCE:-1}"
else
  SET=smoke
  STAMP="${STAMP:-smoke-$(date -u +%Y%m%dT%H%M%SZ)}"
  COOL="${COOL:-5}"
  E1_ENFORCE="${E1_ENFORCE:-0}"
fi
SEED="${SEED:-$(printf '%s' "$STAMP" | cksum | awk '{print $1}')}"
E1_LIMIT=100      # summed %CPU of host processes outside the Docker VM
E1_MAX_WAIT=600   # seconds

OUT="$ROOT/results/$STAMP"
mkdir -p "$OUT"
# The container runs as nobody (65534); the bind-mounted result directory must
# accept its writes.
chmod 0777 "$OUT"
RUNLOG="$OUT/run.log"
RUNMD="$OUT/RUN.md"

build_image

ROUNDS=$(tool_run cells -set "$SET" -rounds)
CELLS=$(tool_run cells -set "$SET")
PS=$(tool_run cells -set "$SET" -ps)

if [ ! -f "$RUNMD" ]; then
  {
    echo "# Run $STAMP"
    echo
    echo "- set: \`$SET\` ($CELLS cells x $ROUNDS rounds)"
    echo "- seed: \`$SEED\`"
    echo "- image: \`$IMAGE\` ($(docker image inspect -f '{{.Id}}' "$IMAGE"))"
    echo "- go: $(tool_run version)"
    echo "- host: $(sysctl -n machdep.cpu.brand_string 2>/dev/null || uname -m), $(sysctl -n hw.ncpu 2>/dev/null) CPUs; docker $(docker version -f '{{.Server.Version}}' 2>/dev/null)"
    echo "- P groups: $PS; cpusets: $(jq -c .cpusets "$MATRIX")"
    echo "- cool-down: ${COOL}s after every container; E1 enforced: $E1_ENFORCE (limit ${E1_LIMIT}% for up to ${E1_MAX_WAIT}s)"
    echo "- started: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo
  } > "$RUNMD"
fi
cp "$MATRIX" "$OUT/matrix.json"

log "run $STAMP: set=$SET cells=$CELLS rounds=$ROUNDS seed=$SEED" 2>&1 | tee -a "$RUNLOG"

status=0
for ((r = 1; r <= ROUNDS; r++)); do
  # E2: power and thermal state at the start of every round.
  if ! pw="$(power_ok)"; then
    { echo "## Round $r — not started (E2)"; echo; echo '```'; echo "$pw"; echo '```'; } >> "$RUNMD"
    log "E2: on battery or thermal warning; stopping" 2>&1 | tee -a "$RUNLOG"
    exit 7
  fi
  ORDER="$(tool_run order -set "$SET" -seed "$SEED" -round "$r")"
  read -r -a PORDER <<< "$(echo "$ORDER" | head -1)"
  {
    echo "## Round $r"
    echo
    echo '```'
    echo "$pw"
    echo "P order: ${PORDER[*]}"
    echo "$ORDER" | tail -n +2
    echo '```'
    echo
  } >> "$RUNMD"

  for p in "${PORDER[@]}"; do
    cs="$(cpuset_for "$p")"
    log "round $r P=$p cpuset=$cs" 2>&1 | tee -a "$RUNLOG"
    t0=$(date +%s)
    set +e
    sandbox_run "$p" "$OUT" round -set "$SET" -seed "$SEED" -round "$r" -P "$p" \
      -cpuset "$cs" -out /results $RESUME 2>&1 | tee -a "$RUNLOG"
    code=${PIPESTATUS[0]}
    set -e
    t1=$(date +%s)
    echo "- round $r P=$p: exit $code, $((t1 - t0)) s" >> "$RUNMD"
    if [ "$code" -ne 0 ]; then
      log "round $r P=$p exited $code" 2>&1 | tee -a "$RUNLOG"
      status=1
      # G2 (exit 3) is a configuration error: the run stops.
      [ "$code" -eq 3 ] && { echo "- stopped: G2 cpuset gate" >> "$RUNMD"; exit 3; }
    fi

    # E1: fixed cool-down, then wait for a quiet host.
    sleep "$COOL"
    waited=0
    load="$(host_load)"
    while awk -v l="$load" -v m="$E1_LIMIT" 'BEGIN { exit !(l >= m) }'; do
      if [ "$E1_ENFORCE" != 1 ]; then break; fi
      if [ "$waited" -ge "$E1_MAX_WAIT" ]; then
        echo "- E1: host load ${load}% after ${waited}s; round stopped" >> "$RUNMD"
        log "E1: host load ${load}% for ${E1_MAX_WAIT}s; stopping" 2>&1 | tee -a "$RUNLOG"
        exit 8
      fi
      sleep 10; waited=$((waited + 13))
      load="$(host_load)"
    done
    loadavg="$(sysctl -n vm.loadavg 2>/dev/null | tr -d '{}' | xargs)"
    echo "  - E1: host load ${load}% (loadavg $loadavg), extra wait ${waited}s" >> "$RUNMD"
  done
done

echo "- finished: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$RUNMD"
log "run $STAMP done; results in $OUT" 2>&1 | tee -a "$RUNLOG"
echo "$OUT"
exit $status
