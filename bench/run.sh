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
#   * gate E2 (AC power, no thermal warning, system sleep disabled) before the
#     run and every round, gate E1 (a fixed cool-down, then host load < one
#     core) after every container
#   * raw JSON is kept verbatim; report.mjs derives every number
#
#   STAMP=<YYYY-MM-DD> ./bench/run.sh --all      the main matrix (845 cells x 5 rounds)
#   ./bench/run.sh --smoke                       the smoke set (22 cells x 3 rounds)
#   STAMP=<stamp> ./bench/run.sh --all --resume  skip repetitions already written
#   STAMP=<stamp> ./bench/run.sh --cells FILE    rerun the listed main cells
#
# A cell rerun (--cells) runs every listed cell of the main set from scratch,
# all rounds, with the protocol above, into results/<stamp>/rerun/. The first
# pass under results/<stamp>/ is only read: its raw files must exist and its
# matrix.json must equal bench/matrix.json. FILE (one cell id per line, '#'
# comments; report.mjs writes one as results/<stamp>/rerun-cells.txt) is
# copied to rerun/cells.txt; every rerun record carries rerun_of pointing at
# the first-pass repetition it replaces (../../raw/<id>.r<n>), and
# rerun/rerun_of.json lists the link. --resume continues an interrupted rerun
# of the same list.
#
# Exit codes: 0 done, 1 a container failed, 2 usage or a precondition of
# --cells, 3 G2 cpuset gate, 6 E2 system sleep enabled, 7 E2 power/thermal,
# 8 E1, 9 harness error (a repetition failed G1/G3 twice).
#
# Environment: SEED (default: cksum of the stamp; of "<stamp>/rerun" for
# --cells), COOL (cool-down seconds; 60 for --all/--cells, 5 for --smoke),
# E1_ENFORCE and SLEEP_ENFORCE (1 for --all/--cells, 0 for --smoke: the smoke
# run records the host load and the sleep setting but does not act on them).
set -euo pipefail
CALLER_PWD="$PWD"
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

usage() { echo "usage: $0 --all|--smoke|--cells FILE [--resume]" >&2; exit 2; }
die2() { echo "run.sh: $*" >&2; exit 2; }

MODE=""
RESUME=""
CELLS_FILE=""
while [ $# -gt 0 ]; do
  case "$1" in
    --all) MODE=all ;;
    --smoke) MODE=smoke ;;
    --cells) [ $# -ge 2 ] || usage; MODE=cells; CELLS_FILE="$2"; shift ;;
    --resume) RESUME=-resume ;;
    *) usage ;;
  esac
  shift
done
[ -n "$MODE" ] || usage

case "$MODE" in
  all)
    SET=main
    STAMP="${STAMP:-$(date -u +%Y-%m-%d)}"
    SEED_BASE="$STAMP"
    COOL="${COOL:-60}"
    E1_ENFORCE="${E1_ENFORCE:-1}"
    SLEEP_ENFORCE="${SLEEP_ENFORCE:-1}"
    ;;
  smoke)
    SET=smoke
    STAMP="${STAMP:-smoke-$(date -u +%Y%m%dT%H%M%SZ)}"
    SEED_BASE="$STAMP"
    COOL="${COOL:-5}"
    E1_ENFORCE="${E1_ENFORCE:-0}"
    SLEEP_ENFORCE="${SLEEP_ENFORCE:-0}"
    ;;
  cells)
    SET=main
    [ -n "${STAMP:-}" ] || die2 "--cells needs STAMP=<stamp of the first pass>"
    SEED_BASE="$STAMP/rerun"
    COOL="${COOL:-60}"
    E1_ENFORCE="${E1_ENFORCE:-1}"
    SLEEP_ENFORCE="${SLEEP_ENFORCE:-1}"
    case "$CELLS_FILE" in /*) ;; *) CELLS_FILE="$CALLER_PWD/$CELLS_FILE" ;; esac
    ;;
esac
SEED="${SEED:-$(printf '%s' "$SEED_BASE" | cksum | awk '{print $1}')}"
E1_LIMIT=100      # summed %CPU of host processes outside the Docker VM
E1_MAX_WAIT=600   # seconds

# E2, system sleep: an idle-sleep timer can suspend the Docker VM in the middle
# of a window. Checked before anything is written, and again every round.
check_sleep() {
  if ! SLEEP_STATE="$(sleep_ok)"; then
    [ "$SLEEP_ENFORCE" = 1 ] || return 0
    local msg="E2: system sleep is enabled (pmset sleep=${SLEEP_STATE#sleep: }); the run does not start. Disable it with 'sudo pmset -c sleep 0' and restore it after the run"
    [ -n "${RUNMD:-}" ] && [ -f "$RUNMD" ] && echo "- stopped: $msg" >> "$RUNMD"
    log "$msg"
    exit 6
  fi
}
check_sleep

SETARGS=(-set "$SET")
ROUNDARGS=()
FIRST="$ROOT/results/$STAMP"
if [ "$MODE" = cells ]; then
  # The first pass is only read; everything of the rerun goes under rerun/.
  [ -d "$FIRST/raw" ] || die2 "no first pass at $FIRST/raw"
  [ -f "$CELLS_FILE" ] || die2 "cell list $CELLS_FILE not found"
  cmp -s "$MATRIX" "$FIRST/matrix.json" || die2 "$MATRIX differs from the first pass's $FIRST/matrix.json"
  OUT="$FIRST/rerun"
  if [ -f "$OUT/cells.txt" ]; then
    cmp -s "$CELLS_FILE" "$OUT/cells.txt" || die2 "$OUT/cells.txt exists with another list; a rerun directory holds one list"
  fi
  SETARGS+=(-cells /results/cells.txt)
  ROUNDARGS=(-rerun-of-dir ../../raw)
else
  OUT="$FIRST"
fi
mkdir -p "$OUT"
# The container runs as nobody (65534); the bind-mounted result directory must
# accept its writes.
chmod 0777 "$OUT"
RUNLOG="$OUT/run.log"
RUNMD="$OUT/RUN.md"

build_image

if [ "$MODE" = cells ]; then
  [ -f "$OUT/cells.txt" ] || cp "$CELLS_FILE" "$OUT/cells.txt"
  chmod 0444 "$OUT/cells.txt"
  TOOL_MOUNT="$OUT"
  # Validates the list against the set (unknown or repeated ids fail here).
  IDS="$(tool_run cells "${SETARGS[@]}" -list)" || die2 "invalid cell list $CELLS_FILE"
fi

ROUNDS=$(tool_run cells "${SETARGS[@]}" -rounds)
CELLS=$(tool_run cells "${SETARGS[@]}")
PS=$(tool_run cells "${SETARGS[@]}" -ps)

if [ "$MODE" = cells ]; then
  missing=0
  while read -r id; do
    for ((r = 1; r <= ROUNDS; r++)); do
      [ -f "$FIRST/raw/$id.r$r.json" ] || { log "first pass has no $id.r$r.json"; missing=$((missing + 1)); }
    done
  done <<< "$IDS"
  [ "$missing" = 0 ] || die2 "$missing first-pass repetitions of the list are missing; nothing to rerun against"
  if [ ! -f "$OUT/rerun_of.json" ]; then
    jq -n --arg stamp "$STAMP" --argjson rounds "$ROUNDS" --arg seed "$SEED" --arg ids "$IDS" '{
      rerun_of: $stamp,
      first_pass_raw: "../raw",
      cells_file: "cells.txt",
      rounds: $rounds,
      seed: ($seed | tonumber),
      note: "every repetition is run from scratch; attempt 1 of raw/<id>.r<n>.json has rerun_of = ../../raw/<id>.r<n> (relative to raw/); the first pass is not modified",
      cells: ($ids | split("\n") | map(select(. != "")))
    }' > "$OUT/rerun_of.json"
  fi
fi

if [ ! -f "$RUNMD" ]; then
  {
    if [ "$MODE" = cells ]; then
      echo "# Run $STAMP — cell rerun"
      echo
      echo "- rerun of: \`results/$STAMP\` (first pass, read only); list: \`cells.txt\`; links: \`rerun_of.json\`"
    else
      echo "# Run $STAMP"
      echo
    fi
    echo "- set: \`$SET\` ($CELLS cells x $ROUNDS rounds)"
    echo "- seed: \`$SEED\`"
    echo "- image: \`$IMAGE\` ($(docker image inspect -f '{{.Id}}' "$IMAGE"))"
    echo "- go: $(tool_run version)"
    echo "- host: $(sysctl -n machdep.cpu.brand_string 2>/dev/null || uname -m), $(sysctl -n hw.ncpu 2>/dev/null) CPUs; docker $(docker version -f '{{.Server.Version}}' 2>/dev/null)"
    echo "- P groups: $PS; cpusets: $(jq -c .cpusets "$MATRIX")"
    echo "- cool-down: ${COOL}s after every container; E1 enforced: $E1_ENFORCE (limit ${E1_LIMIT}% for up to ${E1_MAX_WAIT}s)"
    echo "- system sleep (pmset): ${SLEEP_STATE#sleep: }; enforced (sleep must be 0): $SLEEP_ENFORCE"
    echo "- started: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo
  } > "$RUNMD"
fi
cp "$MATRIX" "$OUT/matrix.json"

log "run $STAMP: set=$SET cells=$CELLS rounds=$ROUNDS seed=$SEED" 2>&1 | tee -a "$RUNLOG"

status=0
for ((r = 1; r <= ROUNDS; r++)); do
  # E2: system sleep, power and thermal state at the start of every round.
  check_sleep
  if ! pw="$(power_ok)"; then
    { echo "## Round $r — not started (E2)"; echo; echo '```'; echo "$pw"; echo '```'; } >> "$RUNMD"
    log "E2: on battery or thermal warning; stopping" 2>&1 | tee -a "$RUNLOG"
    exit 7
  fi
  ORDER="$(tool_run order "${SETARGS[@]}" -seed "$SEED" -round "$r")"
  read -r -a PORDER <<< "$(echo "$ORDER" | head -1)"
  {
    echo "## Round $r"
    echo
    echo '```'
    echo "$pw"
    echo "$SLEEP_STATE"
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
    sandbox_run "$p" "$OUT" round "${SETARGS[@]}" -seed "$SEED" -round "$r" -P "$p" \
      -cpuset "$cs" -out /results ${ROUNDARGS[@]+"${ROUNDARGS[@]}"} $RESUME 2>&1 | tee -a "$RUNLOG"
    code=${PIPESTATUS[0]}
    set -e
    t1=$(date +%s)
    echo "- round $r P=$p: exit $code, $((t1 - t0)) s" >> "$RUNMD"
    if [ "$code" -ne 0 ]; then
      log "round $r P=$p exited $code" 2>&1 | tee -a "$RUNLOG"
      status=1
      # G2 (exit 3) is a configuration error; exit 9 is a repetition that
      # failed a hard gate (G1/G3) on both attempts, a harness error. Both
      # stop the run for the user; --resume re-runs the failed repetition.
      [ "$code" -eq 3 ] && { echo "- stopped: G2 cpuset gate" >> "$RUNMD"; exit 3; }
      [ "$code" -eq 9 ] && { echo "- stopped: harness error (a repetition failed a hard gate twice, see run.log)" >> "$RUNMD"; exit 9; }
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
