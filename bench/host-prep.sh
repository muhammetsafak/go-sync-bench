#!/usr/bin/env bash
# host-prep.sh — prepares the Linux measurement host for the linux-metal
# profile (Ubuntu 26.04 LTS, Ryzen 9 5950X: CCD0 = CPUs 0-7 for the system,
# CCD1 = CPUs 8-15 for the measurement containers). It changes system
# configuration, so it runs only as root on Linux and only with
# BENCH_HOST_PREP_CONFIRM=1:
#
#   sudo BENCH_HOST_PREP_CONFIRM=1 ./bench/host-prep.sh             prepare, then reboot
#   sudo BENCH_HOST_PREP_CONFIRM=1 ./bench/host-prep.sh --runtime   runtime tuning only
#   sudo BENCH_HOST_PREP_CONFIRM=1 ./bench/host-prep.sh --revert    undo, then reboot
#
# The CPU sets come from profiles["linux-metal"] in bench/matrix.json
# (isolated_cpus, system_cpus). Full preparation, in order:
#   1. docker      the docker CLI is not the snap and docker-ce comes from the
#                  official apt repo (download.docker.com); nothing changes
#                  until this passes
#   1b. topology   the profile CPU sets exist, each is one L3 domain (CCD) and
#                  no two of their CPUs are SMT siblings; nothing is written
#                  when this fails
#   2. GRUB        /etc/default/grub.d/99-go-sync-bench.cfg appends
#                    nosmt isolcpus=managed_irq,<iso> nohz_full=<iso>
#                    rcu_nocbs=<iso> irqaffinity=<sys>
#                  to GRUB_CMDLINE_LINUX_DEFAULT, then update-grub. When
#                  update-grub fails, the drop-in is removed again (and
#                  update-grub rerun) before the script stops.
#                  isolcpus carries managed_irq and no "domain" flag on
#                  purpose: domain isolation turns off load balancing on the
#                  isolated CPUs, and every thread of a P>1 container would
#                  stay on the first CPU of its cpuset. Ordinary tasks are
#                  kept off the isolated CPUs by step 3 instead.
#   3. affinity    /etc/systemd/system.conf.d/90-go-sync-bench.conf sets
#                  CPUAffinity=<sys> for PID 1, so every service, dockerd and
#                  login shells run on the system CPUs; a container's cpuset
#                  (docker --cpuset-cpus) still puts it on the isolated CPUs
#   4. sleep       sleep/suspend/hibernate/hybrid-sleep targets masked
#   5. services    unattended-upgrades, snapd, apt-daily*, fwupd, motd-news
#                  and irqbalance stopped, disabled and masked; the previous
#                  state of every unit of steps 4-5 is kept for --revert,
#                  which unmasks only the units that were not masked before
#   6. persistence this script is installed as /usr/local/sbin/bench-host-prep
#                  with /etc/default/bench-host-tune (the CPU sets), and
#                  bench-host-tune.service runs --runtime at every boot,
#                  before docker
#   7. runtime     the --runtime steps, once now
# --runtime: governor performance (and EPP performance where it exists),
# boost off (cpufreq/boost and policy*/boost), every movable IRQ, the default
# IRQ affinity and the unbound workqueue cpumask to the system CPUs.
#
# Kernel parameters and CPUAffinity take effect after a reboot; afterwards
# ./bench/host-check.sh must pass. Exit codes: 0 done, 1 a step failed,
# 2 usage or a guard (not Linux, no confirmation, not root, missing tool).
# --revert runs every step even when one fails and exits 1 if any did.
set -euo pipefail

PROG=host-prep.sh
log() { echo "$PROG: $*" >&2; }
warn() { echo "$PROG: WARNING: $*" >&2; }
die() { echo "$PROG: $*" >&2; exit 1; }
guard() { echo "$PROG: $*" >&2; exit 2; }

if [ "$(uname -s)" != Linux ]; then
  guard "not Linux ($(uname -s)); host-prep prepares only the Linux measurement host"
fi
if [ "${BENCH_HOST_PREP_CONFIRM:-}" != 1 ]; then
  guard "changes GRUB, systemd and sysfs settings; run it as: sudo BENCH_HOST_PREP_CONFIRM=1 $0 [--runtime|--revert]"
fi
[ "$(id -u)" = 0 ] || guard "must run as root (sudo BENCH_HOST_PREP_CONFIRM=1 $0)"

MODE=prepare
case "${1:-}" in
  '') ;;
  --runtime) MODE=runtime ;;
  --revert) MODE=revert ;;
  *) guard "usage: $0 [--runtime|--revert]" ;;
esac
[ $# -le 1 ] || guard "usage: $0 [--runtime|--revert]"

SYS=/sys/devices/system/cpu
INSTALLED=/usr/local/sbin/bench-host-prep
ENV_FILE=/etc/default/bench-host-tune
GRUB_DROPIN=/etc/default/grub.d/99-go-sync-bench.cfg
AFFINITY_DROPIN=/etc/systemd/system.conf.d/90-go-sync-bench.conf
UNIT=bench-host-tune.service
UNIT_FILE=/etc/systemd/system/$UNIT
STATE_DIR=/var/lib/go-sync-bench
UNITS_STATE=$STATE_DIR/units-before-host-prep.tsv
SLEEP_UNITS=(sleep.target suspend.target hibernate.target hybrid-sleep.target)
SERVICES=(unattended-upgrades.service snapd.service snapd.socket snapd.seeded.service
  apt-daily.timer apt-daily-upgrade.timer apt-daily.service apt-daily-upgrade.service
  fwupd.service fwupd-refresh.timer fwupd-refresh.service motd-news.timer motd-news.service
  irqbalance.service)
GRUB_PARAMS_RE='^(nosmt|nosmt=.*|isolcpus=.*|nohz_full=.*|rcu_nocbs=.*|irqaffinity=.*)$'

# expand <list>: "0-3,8" -> "0 1 2 3 8" (sorted, unique); "" for an empty list.
expand() {
  local part a b i
  local -a out=()
  local IFS=,
  for part in $1; do
    part="${part//[[:space:]]/}"
    case "$part" in
      '') ;;
      *-*) a=${part%-*}; b=${part#*-}
           [[ "$a" =~ ^[0-9]+$ && "$b" =~ ^[0-9]+$ ]] || continue
           for ((i = a; i <= b; i++)); do out+=("$i"); done ;;
      *) [[ "$part" =~ ^[0-9]+$ ]] && out+=("$part") ;;
    esac
  done
  [ ${#out[@]} -gt 0 ] || { echo ""; return; }
  printf '%s\n' "${out[@]}" | sort -n | uniq | paste -sd ' ' -
}

# cpumask_hex <expanded set>: "0 1 2 3 4 5 6 7" -> "000000ff"; more than 32
# CPUs as comma-separated 32-bit words, the format /proc/irq/*/smp_affinity
# and the workqueue cpumask read.
cpumask_hex() {
  local c w max=0 out=""
  local -a words=()
  for c in $1; do if ((c > max)); then max=$c; fi; done
  for ((w = 0; w <= max / 32; w++)); do words[w]=0; done
  for c in $1; do words[c/32]=$((words[c/32] | (1 << (c % 32)))); done
  for ((w = max / 32; w >= 0; w--)); do out+="${out:+,}$(printf '%08x' "${words[w]}")"; done
  echo "$out"
}

# write_sysfs <file> <value>: 0 when written, 1 otherwise (EIO/EBUSY/EINVAL
# are expected for some files and are judged by the caller).
write_sysfs() { [ -w "$1" ] && { printf '%s\n' "$2" > "$1"; } 2>/dev/null; }

valid_list() { [[ "$1" =~ ^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$ ]]; }

# intersect <expanded set> <expanded set>: the CPUs in both, sorted.
intersect() {
  local c out=""
  for c in $1; do case " $2 " in *" $c "*) out+="${out:+ }$c" ;; esac; done
  echo "$out"
}

# --- CPU sets ------------------------------------------------------------------
# --runtime at boot reads $ENV_FILE (the repo may not be mounted yet); every
# other run reads the matrix next to this script.
load_cpu_sets() {
  local iso="" sys=""
  if [ "$MODE" = runtime ] && [ -r "$ENV_FILE" ]; then
    iso="$(awk -F= '$1 == "ISOLATED_CPUS" { print $2 }' "$ENV_FILE" | tr -d '"')"
    sys="$(awk -F= '$1 == "SYSTEM_CPUS" { print $2 }' "$ENV_FILE" | tr -d '"')"
  else
    local root matrix
    root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
    matrix="${BENCH_MATRIX:-$root/bench/matrix.json}"
    [ -r "$matrix" ] || guard "no $matrix (and no $ENV_FILE); run from the go-sync-bench checkout"
    command -v jq >/dev/null || guard "jq is required (apt install jq)"
    iso="$(jq -r '.profiles["linux-metal"].isolated_cpus // empty' "$matrix")"
    sys="$(jq -r '.profiles["linux-metal"].system_cpus // empty' "$matrix")"
  fi
  valid_list "$iso" || die "isolated_cpus '$iso' is not a CPU list"
  valid_list "$sys" || die "system_cpus '$sys' is not a CPU list"
  ISO_LIST="$iso"; SYS_LIST="$sys"
  ISOLATED="$(expand "$iso")"; SYSTEM="$(expand "$sys")"
  local c
  for c in $ISOLATED; do
    case " $SYSTEM " in *" $c "*) die "CPU $c is both isolated ($iso) and system ($sys)" ;; esac
  done
}

# --- runtime tuning ----------------------------------------------------------------
runtime_tune() {
  local f ok
  local -a fail=()
  log "runtime: isolated $ISO_LIST, system $SYS_LIST"

  # governor (and EPP: amd-pstate-epp pins EPP to performance under the
  # performance governor and may refuse the write with EBUSY; read back)
  fail=()
  for f in $SYS/cpu[0-9]*/cpufreq/scaling_governor; do
    [ -e "$f" ] || continue
    write_sysfs "$f" performance || true
    [ "$(cat "$f" 2>/dev/null)" = performance ] || fail+=("${f#$SYS/}")
  done
  for f in $SYS/cpu[0-9]*/cpufreq/energy_performance_preference; do
    [ -e "$f" ] || continue
    write_sysfs "$f" performance || true
    [ "$(cat "$f" 2>/dev/null)" = performance ] || fail+=("${f#$SYS/}")
  done
  if [ ${#fail[@]} -eq 0 ]; then log "governor: performance"; else warn "governor/EPP not performance: ${fail[*]}"; fi

  # boost
  ok=0; fail=()
  for f in $SYS/cpufreq/boost $SYS/cpufreq/policy*/boost; do
    [ -e "$f" ] || continue
    write_sysfs "$f" 0 || true
    if [ "$(cat "$f" 2>/dev/null)" = 0 ]; then ok=$((ok + 1)); else fail+=("${f#$SYS/}"); fi
  done
  if [ ${#fail[@]} -gt 0 ]; then
    warn "boost still on: ${fail[*]}; disable Core Performance Boost in the BIOS"
  elif [ "$ok" -eq 0 ]; then
    warn "no boost control in sysfs; disable Core Performance Boost in the BIOS and run host-check with BENCH_NOMINAL_KHZ=3400000"
  else
    log "boost: off ($ok control(s))"
  fi

  # IRQs: managed per-queue IRQs refuse the write (EIO); isolcpus=managed_irq
  # keeps their effective affinity on the system CPUs where possible
  local moved=0 refused=0
  for f in /proc/irq/[0-9]*/smp_affinity_list; do
    [ -e "$f" ] || continue
    if write_sysfs "$f" "$SYS_LIST"; then moved=$((moved + 1)); else refused=$((refused + 1)); fi
  done
  log "IRQ affinity: $moved IRQ(s) to $SYS_LIST, $refused refused (kernel-managed)"
  local mask
  mask="$(cpumask_hex "$SYSTEM")"
  write_sysfs /proc/irq/default_smp_affinity "$mask" && log "default IRQ affinity: $mask" \
    || warn "could not write /proc/irq/default_smp_affinity"
  if [ -e /sys/devices/virtual/workqueue/cpumask ]; then
    write_sysfs /sys/devices/virtual/workqueue/cpumask "$mask" && log "unbound workqueue cpumask: $mask" \
      || warn "could not write the workqueue cpumask"
  fi
}

# --- docker --------------------------------------------------------------------
check_docker() {
  local path real status policy
  path="$(command -v docker 2>/dev/null || true)"
  [ -n "$path" ] || die "docker not found; install docker-ce from the official apt repo (https://docs.docker.com/engine/install/ubuntu/)"
  real="$(readlink -f "$path")"
  [[ "$real" != /snap/* ]] || die "docker is the snap ($real); snap remove docker, then install docker-ce from download.docker.com"
  if command -v snap >/dev/null && snap list docker >/dev/null 2>&1; then
    die "the docker snap is installed; snap remove docker"
  fi
  status="$(dpkg-query -W -f='${Status}' docker-ce 2>/dev/null || true)"
  [ "$status" = "install ok installed" ] || die "package docker-ce is not installed ($real is not from the official apt repo)"
  policy="$(apt-cache policy docker-ce 2>/dev/null || true)"
  grep -q 'download\.docker\.com' <<< "$policy" \
    || die "docker-ce has no download.docker.com origin in apt-cache policy; add the official Docker apt repo"
  log "docker: $real, docker-ce $(dpkg-query -W -f='${Version}' docker-ce) from download.docker.com"
}

# --- topology ------------------------------------------------------------------------
# check_topology: the profile CPU sets must match this machine before GRUB is
# written, or the kernel parameters would isolate the wrong cores. It runs
# before the reboot, so SMT may still be on: only the CPUs of the two sets are
# compared, and SMT siblings outside them are ignored. Requirements: every CPU
# exists; the isolated CPUs share one L3 and the system CPUs another (one CCD
# each); no two CPUs of the sets are SMT siblings (nosmt would take one away).
check_topology() {
  local all c l3 sib first_iso first_sys want
  local -a problems=()
  all="$(expand "$ISO_LIST,$SYS_LIST")"
  for c in $all; do
    [ -d "$SYS/cpu$c" ] || problems+=("cpu$c does not exist")
  done
  if [ ${#problems[@]} -eq 0 ]; then
    first_iso=${ISOLATED%% *}; first_sys=${SYSTEM%% *}
    for c in $all; do
      l3="$(expand "$(cat "$SYS/cpu$c/cache/index3/shared_cpu_list" 2>/dev/null || true)")"
      if [ -z "$l3" ]; then problems+=("cpu$c has no cache/index3/shared_cpu_list"); continue; fi
      case " $ISOLATED " in *" $c "*) want="$ISOLATED" ;; *) want="$SYSTEM" ;; esac
      [ "$(intersect "$l3" "$all")" = "$want" ] \
        || problems+=("L3 of cpu$c holds $(intersect "$l3" "$all") of the profile CPUs, want $want")
      sib="$(expand "$(cat "$SYS/cpu$c/topology/thread_siblings_list" 2>/dev/null || true)")"
      [ -z "$sib" ] || [ "$(intersect "$sib" "$all")" = "$c" ] \
        || problems+=("cpu$c has SMT siblings $(intersect "$sib" "$all") inside the profile CPU sets")
    done
    [ -n "$first_iso" ] && [ -n "$first_sys" ] || problems+=("empty CPU set")
  fi
  [ ${#problems[@]} -eq 0 ] || die "profile CPU sets (isolated $ISO_LIST, system $SYS_LIST) do not match this machine, nothing written: $(IFS=';'; echo "${problems[*]}")"
  log "topology: isolated $ISO_LIST = one L3, system $SYS_LIST = another, no SMT siblings inside the sets"
}

# --- GRUB ------------------------------------------------------------------------
grub_params() { echo "nosmt isolcpus=managed_irq,$ISO_LIST nohz_full=$ISO_LIST rcu_nocbs=$ISO_LIST irqaffinity=$SYS_LIST"; }

prepare_grub() {
  command -v update-grub >/dev/null || die "update-grub not found; this host does not boot through GRUB"
  # the same parameters set elsewhere would be passed twice; refuse instead of guessing
  local f tok conflicts=()
  for f in /etc/default/grub /etc/default/grub.d/*.cfg; do
    [ -r "$f" ] && [ "$f" != "$GRUB_DROPIN" ] || continue
    for tok in $(grep -E '^[[:space:]]*GRUB_CMDLINE_LINUX(_DEFAULT)?=' "$f" | sed -E 's/^[^=]*=//; s/["'\'']//g' || true); do
      if [[ "$tok" =~ $GRUB_PARAMS_RE ]]; then conflicts+=("$f: $tok"); fi
    done
  done
  [ ${#conflicts[@]} -eq 0 ] || die "remove these kernel parameters first, $GRUB_DROPIN sets them: ${conflicts[*]}"
  mkdir -p "$(dirname "$GRUB_DROPIN")"
  cat > "$GRUB_DROPIN" <<EOF
# go-sync-bench linux-metal profile (bench/host-prep.sh; --revert removes it)
GRUB_CMDLINE_LINUX_DEFAULT="\${GRUB_CMDLINE_LINUX_DEFAULT} $(grub_params)"
EOF
  log "GRUB: $GRUB_DROPIN adds: $(grub_params)"
  if ! update-grub; then
    grub_rollback "update-grub failed"
  fi
  grep -q -- "isolcpus=managed_irq,$ISO_LIST" /boot/grub/grub.cfg \
    || grub_rollback "/boot/grub/grub.cfg does not carry isolcpus=managed_irq,$ISO_LIST after update-grub"
}

# grub_rollback <reason>: remove the drop-in this run wrote, regenerate
# grub.cfg without it and stop; nothing after GRUB has run yet.
grub_rollback() {
  rm -f "$GRUB_DROPIN"
  if update-grub; then
    die "$1; $GRUB_DROPIN removed and grub.cfg regenerated without it"
  fi
  die "$1; $GRUB_DROPIN removed, but update-grub fails: fix it and run update-grub before the next reboot"
}

# --- systemd -------------------------------------------------------------------------
unit_known() { [ -n "$(systemctl list-unit-files --no-legend "$1" 2>/dev/null)" ]; }

prepare_systemd() {
  mkdir -p "$(dirname "$AFFINITY_DROPIN")" "$STATE_DIR"
  cat > "$AFFINITY_DROPIN" <<EOF
# go-sync-bench linux-metal profile (bench/host-prep.sh; --revert removes it)
[Manager]
CPUAffinity=$SYS_LIST
EOF
  log "systemd: CPUAffinity=$SYS_LIST ($AFFINITY_DROPIN, after reboot)"

  # keep the first recorded state: a second run must not overwrite it with
  # "masked"; --revert unmasks only the units recorded here as not masked
  local u state
  [ -e "$UNITS_STATE" ] || : > "$UNITS_STATE"
  for u in "${SLEEP_UNITS[@]}"; do
    if ! grep -q "^$u"$'\t' "$UNITS_STATE"; then
      state="$(systemctl is-enabled "$u" 2>/dev/null || true)"
      printf '%s\t%s\n' "$u" "${state:-unknown}" >> "$UNITS_STATE"
    fi
  done
  systemctl mask "${SLEEP_UNITS[@]}" >/dev/null
  log "sleep targets masked"

  for u in "${SERVICES[@]}"; do
    unit_known "$u" || { log "service $u: not installed"; continue; }
    if ! grep -q "^$u"$'\t' "$UNITS_STATE"; then
      state="$(systemctl is-enabled "$u" 2>/dev/null || true)"
      printf '%s\t%s\n' "$u" "${state:-unknown}" >> "$UNITS_STATE"
    fi
    systemctl disable --now "$u" >/dev/null 2>&1 || systemctl stop "$u" >/dev/null 2>&1 || true
    systemctl mask "$u" >/dev/null
    log "service $u: stopped, masked"
  done
}

prepare_persistence() {
  install -m 0755 "${BASH_SOURCE[0]}" "$INSTALLED"
  cat > "$ENV_FILE" <<EOF
# go-sync-bench linux-metal CPU sets, read by $INSTALLED --runtime at boot
ISOLATED_CPUS=$ISO_LIST
SYSTEM_CPUS=$SYS_LIST
EOF
  cat > "$UNIT_FILE" <<EOF
# go-sync-bench linux-metal profile (bench/host-prep.sh; --revert removes it)
[Unit]
Description=go-sync-bench linux-metal runtime tuning (governor, boost, IRQ affinity)
After=systemd-modules-load.service local-fs.target
Before=containerd.service docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
Environment=BENCH_HOST_PREP_CONFIRM=1
ExecStart=$INSTALLED --runtime

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable "$UNIT" >/dev/null
  log "persistence: $UNIT runs $INSTALLED --runtime at boot ($ENV_FILE)"
}

# --- revert --------------------------------------------------------------------------
# Every step runs even when an earlier one fails; the exit code is 1 when any
# step failed (each failure is a WARNING line), 0 otherwise.
revert() {
  local u state rc=0
  if [ -e "$GRUB_DROPIN" ]; then
    rm -f "$GRUB_DROPIN"
    log "GRUB: $GRUB_DROPIN removed"
    if ! command -v update-grub >/dev/null; then
      warn "update-grub not found; regenerate grub.cfg before the next reboot"; rc=1
    elif ! update-grub; then
      warn "update-grub failed; fix it and run update-grub before the next reboot, or the kernel parameters stay"; rc=1
    fi
  fi
  rm -f "$AFFINITY_DROPIN" || { warn "could not remove $AFFINITY_DROPIN"; rc=1; }
  if [ -e "$UNIT_FILE" ]; then
    systemctl disable "$UNIT" >/dev/null 2>&1 || true
    rm -f "$UNIT_FILE" || { warn "could not remove $UNIT_FILE"; rc=1; }
  fi
  rm -f "$INSTALLED" "$ENV_FILE" || { warn "could not remove $INSTALLED / $ENV_FILE"; rc=1; }
  # only the units host-prep masked: one already masked before it stays masked
  if [ -r "$UNITS_STATE" ]; then
    while IFS=$'\t' read -r u state; do
      [ -n "$u" ] || continue
      case "$state" in
        masked|masked-runtime) log "unit $u: was masked before host-prep, left masked"; continue ;;
      esac
      if ! systemctl unmask "$u" >/dev/null 2>&1; then warn "could not unmask $u"; rc=1; continue; fi
      if [ "$state" = enabled ]; then
        if systemctl enable "$u" >/dev/null 2>&1; then log "unit $u: unmasked, enabled again"
        else warn "unit $u: unmasked, could not enable it again"; rc=1; fi
      else
        log "unit $u: unmasked (was $state)"
      fi
    done < "$UNITS_STATE"
    if [ "$rc" -eq 0 ]; then rm -f "$UNITS_STATE"; else warn "kept $UNITS_STATE for another --revert"; fi
  else
    log "no $UNITS_STATE; no unit masked by host-prep to restore"
  fi
  systemctl daemon-reload || { warn "systemctl daemon-reload failed"; rc=1; }
  if [ "$rc" -eq 0 ]; then
    log "reverted; reboot to restore the kernel parameters, CPU affinity, governor and boost"
  else
    warn "revert finished with errors (see the WARNING lines); fix them and run --revert again, then reboot"
  fi
  return "$rc"
}

# --- main ----------------------------------------------------------------------------
case "$MODE" in
  runtime)
    load_cpu_sets
    runtime_tune
    ;;
  revert)
    rc=0; revert || rc=$?
    exit "$rc"
    ;;
  prepare)
    command -v systemctl >/dev/null || guard "systemctl not found; host-prep expects systemd"
    load_cpu_sets
    check_docker
    check_topology
    prepare_grub
    prepare_systemd
    prepare_persistence
    runtime_tune
    log "done. Reboot, then run ./bench/host-check.sh: every check must pass before BENCH_PROFILE=linux-metal ./bench/run.sh"
    ;;
esac
