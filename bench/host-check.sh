#!/usr/bin/env bash
# host-check.sh — gate E2 of the linux-metal profile. Read-only: it reads
# sysfs, procfs, systemctl and docker state and changes nothing.
#
#   ./bench/host-check.sh                         check, JSON record on stdout
#   ./bench/host-check.sh --out FILE              check, JSON record to FILE
#   ./bench/host-check.sh --phase round-3 --skip-load --out FILE
#
# run.sh (BENCH_PROFILE=linux-metal) calls it before the run (phase start,
# load included), at the start of every round and after the last round
# (--skip-load: the 1-minute load average still carries the previous
# container; E1 is the load gate between containers). Every record lands in
# results/<stamp>/host-check/, the start record also as environment.json.
#
# Checks (each one is a line on stderr, "ok" or "FAIL", and an entry of
# .checks in the record):
#   linux-metal profile   bench/matrix.json has profiles["linux-metal"] with
#                         isolated_cpus, system_cpus and cpusets
#   cpu sets              isolated and system CPUs are online and disjoint;
#                         every measurement cpuset lies in the isolated CPUs
#                         and has P CPUs
#   L3 topology           the isolated CPUs are exactly one L3 domain (one CCD)
#                         and the system CPUs another, read from
#                         /sys/devices/system/cpu/cpu*/cache/index3/shared_cpu_list
#   SMT off               smt/active is 0 and no online CPU has an online sibling
#   isolcpus              /proc/cmdline has isolcpus=managed_irq,<isolated CPUs>
#                         and no domain isolation (/sys/.../cpu/isolated is
#                         empty): domain isolation turns off load balancing, so
#                         every thread of a P>1 container would stay on the
#                         first CPU of its cpuset
#   nohz_full             /sys/devices/system/cpu/nohz_full equals the isolated
#                         CPUs (it also moves the tick, RCU callbacks, unbound
#                         kthreads and workqueues to the system CPUs)
#   process affinity      PID 1 (and so every service, dockerd included) is
#                         affined to the system CPUs (systemd CPUAffinity=)
#   governor              every online CPU runs "performance" (acpi-cpufreq,
#                         amd-pstate and amd-pstate-epp; with EPP also
#                         energy_performance_preference = performance)
#   boost off             cpufreq/boost and every policy*/boost read 0; without
#                         any boost control, cpuinfo_max_freq of the isolated
#                         CPUs must not exceed BENCH_NOMINAL_KHZ
#   IRQ affinity          no movable IRQ has an effective affinity on an
#                         isolated CPU; an IRQ whose mask (its online part:
#                         nosmt takes CPUs offline) holds only isolated CPUs is
#                         tolerated and listed under irq.pinned_to_isolated,
#                         unless debugfs (readable as root) shows it is not
#                         kernel-managed; then it fails like a movable IRQ.
#                         Each record carries managed: true|false|null
#   virtualization        bare metal: systemd-detect-virt says "none" (no VM,
#                         no container; when the tool exists) and /proc/cpuinfo has no
#                         "hypervisor" flag (ADR-0050 condition 1)
#   load                  1-minute load average <= BENCH_LOAD_LIMIT (1.0)
#   sleep                 sleep/suspend/hibernate/hybrid-sleep targets masked
#   background services   unattended-upgrades, snapd, apt-daily*, fwupd,
#                         motd-news and irqbalance are not active
#   docker                the docker CLI is not the snap and no docker snap
#                         is installed
# Recorded, not judged: kernel, CPU model, microcode, cmdline (rcu_nocbs,
# irqaffinity), cpufreq driver, cpuinfo_max_freq, cpuidle, THP, NUMA
# balancing, docker version, cgroup version, k10temp temperatures.
#
# Exit codes: 0 every check passed, 1 a check failed, 2 usage, not Linux, or a
# missing tool (jq).
#
# Environment: BENCH_LOAD_LIMIT (default 1.0), BENCH_NOMINAL_KHZ (only read when
# no boost control exists; 3400000 for a Ryzen 9 5950X), BENCH_MATRIX (default
# bench/matrix.json).
set -euo pipefail

if [ "$(uname -s)" != Linux ]; then
  echo "host-check.sh: not Linux ($(uname -s)); the linux-metal host check runs only on the Linux measurement host" >&2
  exit 2
fi

usage() { echo "usage: $0 [--out FILE] [--phase NAME] [--skip-load]" >&2; exit 2; }

OUT_FILE=""
PHASE=manual
SKIP_LOAD=0
while [ $# -gt 0 ]; do
  case "$1" in
    --out) [ $# -ge 2 ] || usage; OUT_FILE="$2"; shift ;;
    --phase) [ $# -ge 2 ] || usage; PHASE="$2"; shift ;;
    --skip-load) SKIP_LOAD=1 ;;
    *) usage ;;
  esac
  shift
done

command -v jq >/dev/null || { echo "host-check.sh: jq is required (apt install jq)" >&2; exit 2; }

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MATRIX="${BENCH_MATRIX:-$ROOT/bench/matrix.json}"
PROFILE=linux-metal
LOAD_LIMIT="${BENCH_LOAD_LIMIT:-1.0}"
NOMINAL_KHZ="${BENCH_NOMINAL_KHZ:-}"
SYS=/sys/devices/system/cpu

CHECKS=()
# check <name> <ok:true|false> <detail>
check() {
  CHECKS+=("$1"$'\t'"$2"$'\t'"$3")
  if [ "$2" = true ]; then
    printf 'ok    %-20s %s\n' "$1" "$3" >&2
  else
    printf 'FAIL  %-20s %s\n' "$1" "$3" >&2
  fi
}

# readf <file>: the first line of a file, or "" when it is missing/unreadable.
readf() { [ -r "$1" ] && head -n 1 "$1" 2>/dev/null || true; }

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

# set operations on space-separated CPU sets
contains() { case " $1 " in *" $2 "*) return 0 ;; esac; return 1; }
subset() { local x; for x in $1; do contains "$2" "$x" || return 1; done; return 0; }
intersect() { local x r=""; for x in $1; do contains "$2" "$x" && r="$r $x"; done; echo "${r# }"; }
count() { set -- $1; echo $#; }
compact() { # "8 9 10 12" -> "8-10,12"
  awk 'BEGIN { RS = " "; s = ""; } NF {
         n = $1 + 0
         if (started && n == prev + 1) { prev = n; next }
         if (started) s = s (s == "" ? "" : ",") (start == prev ? start : start "-" prev)
         start = n; prev = n; started = 1 }
       END { if (started) s = s (s == "" ? "" : ",") (start == prev ? start : start "-" prev); print s }' <<< "$1"
}

# --- profile from the matrix --------------------------------------------------
PROF_JSON="$(jq -c --arg p "$PROFILE" '.profiles[$p] // empty' "$MATRIX" 2>/dev/null || true)"
ISOLATED=""; SYSTEM=""; CPUSETS_JSON='{}'
if [ -n "$PROF_JSON" ] \
  && ISO_RAW="$(jq -er '.isolated_cpus' <<< "$PROF_JSON")" \
  && SYS_RAW="$(jq -er '.system_cpus' <<< "$PROF_JSON")" \
  && CPUSETS_JSON="$(jq -ec '.cpusets | select(type == "object" and length > 0)' <<< "$PROF_JSON")"; then
  ISOLATED="$(expand "$ISO_RAW")"; SYSTEM="$(expand "$SYS_RAW")"
  check "linux-metal profile" true "isolated $ISO_RAW, system $SYS_RAW, cpusets $CPUSETS_JSON"
else
  CPUSETS_JSON='{}'
  check "linux-metal profile" false "$MATRIX has no complete profiles[\"$PROFILE\"] (isolated_cpus, system_cpus, cpusets)"
fi

# --- CPU sets ----------------------------------------------------------------
ONLINE_RAW="$(readf $SYS/online)"
ONLINE="$(expand "$ONLINE_RAW")"
if [ -n "$ISOLATED" ] && [ -n "$SYSTEM" ]; then
  problems=()
  subset "$ISOLATED" "$ONLINE" || problems+=("isolated CPUs not all online (online $ONLINE_RAW)")
  subset "$SYSTEM" "$ONLINE" || problems+=("system CPUs not all online (online $ONLINE_RAW)")
  [ -z "$(intersect "$ISOLATED" "$SYSTEM")" ] || problems+=("isolated and system CPUs overlap")
  while IFS=$'\t' read -r p cs; do
    set_p="$(expand "$cs")"
    subset "$set_p" "$ISOLATED" || problems+=("P$p cpuset $cs is not inside the isolated CPUs")
    [ "$(count "$set_p")" = "$p" ] || problems+=("P$p cpuset $cs has $(count "$set_p") CPUs")
  done < <(jq -r 'to_entries[] | "\(.key)\t\(.value)"' <<< "$CPUSETS_JSON")
  if [ ${#problems[@]} -eq 0 ]; then
    check "cpu sets" true "online $ONLINE_RAW; measurement cpusets inside isolated $(compact "$ISOLATED")"
  else
    check "cpu sets" false "$(IFS=';'; echo "${problems[*]}")"
  fi
else
  check "cpu sets" false "no profile CPU sets to check"
fi

# --- L3 topology (CCDs) --------------------------------------------------------
l3_of() { expand "$(readf "$SYS/cpu$1/cache/index3/shared_cpu_list")"; }
L3_ISO=""; L3_SYS=""
if [ -n "$ISOLATED" ] && [ -n "$SYSTEM" ]; then
  set -- $ISOLATED; first_iso=$1
  set -- $SYSTEM; first_sys=$1
  L3_ISO="$(intersect "$(l3_of "$first_iso")" "$ONLINE")"
  L3_SYS="$(intersect "$(l3_of "$first_sys")" "$ONLINE")"
  problems=()
  [ -n "$L3_ISO" ] || problems+=("cpu$first_iso has no cache/index3/shared_cpu_list")
  [ "$L3_ISO" = "$ISOLATED" ] || problems+=("L3 of cpu$first_iso is $(compact "$L3_ISO"), want the isolated CPUs $(compact "$ISOLATED")")
  for c in $ISOLATED; do
    [ "$(intersect "$(l3_of "$c")" "$ONLINE")" = "$L3_ISO" ] || { problems+=("cpu$c is in another L3 domain"); break; }
  done
  [ "$L3_SYS" = "$SYSTEM" ] || problems+=("L3 of cpu$first_sys is $(compact "$L3_SYS"), want the system CPUs $(compact "$SYSTEM")")
  if [ ${#problems[@]} -eq 0 ]; then
    check "L3 topology" true "isolated $(compact "$L3_ISO") = one L3 domain; system $(compact "$L3_SYS") = another"
  else
    check "L3 topology" false "$(IFS=';'; echo "${problems[*]}")"
  fi
else
  check "L3 topology" false "no profile CPU sets to check"
fi

# --- virtualization (ADR-0050 condition 1: bare metal) ------------------------
VIRT_DETECT=""
if command -v systemd-detect-virt >/dev/null; then
  VIRT_DETECT="$(systemd-detect-virt 2>/dev/null || true)"
fi
HYPERVISOR_FLAG="$(awk '/^flags/ { print (/(^| )hypervisor( |$)/ ? "true" : "false"); exit }' /proc/cpuinfo 2>/dev/null || true)"
[ -n "$HYPERVISOR_FLAG" ] || HYPERVISOR_FLAG=false
if [ "$HYPERVISOR_FLAG" = false ] && { [ -z "$VIRT_DETECT" ] || [ "$VIRT_DETECT" = none ]; }; then
  check "virtualization" true "bare metal (systemd-detect-virt: ${VIRT_DETECT:-not installed}; no hypervisor flag)"
else
  check "virtualization" false "systemd-detect-virt: ${VIRT_DETECT:-not installed}; hypervisor flag: $HYPERVISOR_FLAG"
fi

# --- SMT ---------------------------------------------------------------------
SMT_CONTROL="$(readf $SYS/smt/control)"
SMT_ACTIVE="$(readf $SYS/smt/active)"
siblings=""
for c in $ONLINE; do
  sib="$(intersect "$(expand "$(readf "$SYS/cpu$c/topology/thread_siblings_list")")" "$ONLINE")"
  [ -z "$sib" ] || [ "$sib" = "$c" ] || { siblings="cpu$c has online siblings $(compact "$sib")"; break; }
done
if [ "$SMT_ACTIVE" = 0 ] && [ -z "$siblings" ]; then
  check "SMT off" true "smt/control=$SMT_CONTROL smt/active=0"
else
  check "SMT off" false "smt/control=${SMT_CONTROL:-?} smt/active=${SMT_ACTIVE:-?}${siblings:+; $siblings}"
fi

# --- isolcpus / nohz_full / process affinity ------------------------------------
CMDLINE="$(readf /proc/cmdline)"
ISOLCPUS_PARAM="$(tr ' ' '\n' <<< "$CMDLINE" | awk -F= '$1 == "isolcpus" { v = substr($0, 10) } END { print v }')"
iso_flags=(); iso_list=()
IFS=, read -r -a iso_tokens <<< "$ISOLCPUS_PARAM"
for tok in ${iso_tokens[@]+"${iso_tokens[@]}"}; do
  if [[ "$tok" =~ ^[a-z_]+$ ]]; then iso_flags+=("$tok"); else iso_list+=("$tok"); fi
done
ISO_PARAM_CPUS="$(expand "$(IFS=,; echo "${iso_list[*]:-}")")"
ISO_KERNEL_RAW="$(readf $SYS/isolated)"
problems=()
if [ -z "$ISOLCPUS_PARAM" ]; then
  problems+=("no isolcpus= on the kernel cmdline")
else
  [ -n "$ISOLATED" ] && [ "$ISO_PARAM_CPUS" = "$ISOLATED" ] \
    || problems+=("isolcpus CPUs $(compact "$ISO_PARAM_CPUS"), want $(compact "$ISOLATED")")
  contains "${iso_flags[*]:-}" managed_irq || problems+=("isolcpus without managed_irq")
  if [ ${#iso_flags[@]} -eq 0 ] || contains "${iso_flags[*]}" domain; then
    problems+=("isolcpus does domain isolation (no flags or 'domain'): P>1 cpusets would not be load balanced")
  fi
fi
[ -z "$(expand "$ISO_KERNEL_RAW")" ] || problems+=("/sys/devices/system/cpu/isolated=$ISO_KERNEL_RAW: domain isolation is on")
if [ ${#problems[@]} -eq 0 ]; then
  check "isolcpus" true "isolcpus=$ISOLCPUS_PARAM, no domain isolation"
else
  check "isolcpus" false "$(IFS=';'; echo "${problems[*]}") (GRUB, see host-prep.sh)"
fi
NOHZ_RAW="$(readf $SYS/nohz_full)"
[ "$NOHZ_RAW" = "(null)" ] && NOHZ_RAW=""
if [ -n "$ISOLATED" ] && [ "$(expand "$NOHZ_RAW")" = "$ISOLATED" ]; then
  check "nohz_full" true "nohz_full=$NOHZ_RAW"
else
  check "nohz_full" false "nohz_full='${NOHZ_RAW}', want $(compact "$ISOLATED") (GRUB nohz_full=, see host-prep.sh)"
fi
INIT_AFFINITY="$(awk '/^Cpus_allowed_list:/ { print $2 }' /proc/1/status 2>/dev/null || true)"
if [ -n "$SYSTEM" ] && [ "$(expand "$INIT_AFFINITY")" = "$SYSTEM" ]; then
  check "process affinity" true "PID 1 Cpus_allowed_list=$INIT_AFFINITY"
else
  check "process affinity" false "PID 1 Cpus_allowed_list='${INIT_AFFINITY}', want $(compact "$SYSTEM") (systemd CPUAffinity=, see host-prep.sh)"
fi

# --- governor / EPP -------------------------------------------------------------
DRIVER="$(readf $SYS/cpu0/cpufreq/scaling_driver)"
AMD_PSTATE_STATUS="$(readf $SYS/amd_pstate/status)"
bad_gov=(); bad_epp=(); GOVS=(); EPPS=()
for c in $ONLINE; do
  g="$(readf "$SYS/cpu$c/cpufreq/scaling_governor")"
  GOVS+=("${g:-none}")
  [ "$g" = performance ] || bad_gov+=("cpu$c=${g:-none}")
  if [ -r "$SYS/cpu$c/cpufreq/energy_performance_preference" ]; then
    e="$(readf "$SYS/cpu$c/cpufreq/energy_performance_preference")"
    EPPS+=("$e")
    [ "$e" = performance ] || bad_epp+=("cpu$c=$e")
  fi
done
GOV_SUMMARY="$(printf '%s\n' "${GOVS[@]}" | sort | uniq -c | awk '{ printf "%s%s x%s", (NR > 1 ? ", " : ""), $2, $1 }')"
if [ -z "$DRIVER" ]; then
  check "governor" false "no cpufreq driver (cpu0/cpufreq/scaling_driver missing)"
elif [ ${#bad_gov[@]} -eq 0 ] && [ ${#bad_epp[@]} -eq 0 ]; then
  check "governor" true "driver $DRIVER${AMD_PSTATE_STATUS:+ ($AMD_PSTATE_STATUS)}: $GOV_SUMMARY${EPPS[0]:+; EPP performance}"
else
  check "governor" false "driver $DRIVER: ${bad_gov[*]:-}${bad_epp[0]:+ EPP ${bad_epp[*]}}"
fi

# --- boost --------------------------------------------------------------------
BOOST_GLOBAL="$(readf $SYS/cpufreq/boost)"
boost_on=(); boost_knobs=0
[ -n "$BOOST_GLOBAL" ] && { boost_knobs=$((boost_knobs + 1)); [ "$BOOST_GLOBAL" = 0 ] || boost_on+=("cpufreq/boost=$BOOST_GLOBAL"); }
for f in $SYS/cpufreq/policy*/boost; do
  [ -r "$f" ] || continue
  boost_knobs=$((boost_knobs + 1))
  v="$(readf "$f")"
  [ "$v" = 0 ] || boost_on+=("${f#$SYS/}=$v")
done
MAXF_JSON="$(for c in $ISOLATED; do printf '%s\t%s\n' "$c" "$(readf "$SYS/cpu$c/cpufreq/cpuinfo_max_freq")"; done \
  | jq -Rn '[inputs | split("\t") | {key: ("cpu" + .[0]), value: (.[1] | tonumber? // null)}] | from_entries')"
if [ ${#boost_on[@]} -gt 0 ]; then
  check "boost off" false "${boost_on[*]} (BIOS Core Performance Boost, or host-prep.sh)"
elif [ "$boost_knobs" -gt 0 ]; then
  check "boost off" true "$boost_knobs boost control(s) read 0"
elif [ -n "$NOMINAL_KHZ" ]; then
  over="$(jq -r --argjson n "$NOMINAL_KHZ" '[to_entries[] | select(.value == null or .value > $n) | "\(.key)=\(.value)"] | join(" ")' <<< "$MAXF_JSON")"
  if [ -z "$over" ] && [ -n "$ISOLATED" ]; then
    check "boost off" true "no boost control; cpuinfo_max_freq <= BENCH_NOMINAL_KHZ=$NOMINAL_KHZ"
  else
    check "boost off" false "no boost control; above BENCH_NOMINAL_KHZ=$NOMINAL_KHZ: ${over:-no isolated CPUs}"
  fi
else
  check "boost off" false "no boost control in sysfs; set BENCH_NOMINAL_KHZ (base clock in kHz, 3400000 for a 5950X) to judge cpuinfo_max_freq"
fi

# --- IRQ affinity --------------------------------------------------------------
IRQ_TOTAL=0; IRQ_UNREADABLE=0
IRQ_BAD=(); IRQ_PINNED=()
# irq_managed <irq>: "true"/"false" from the debugfs IRQ state
# (IRQD_AFFINITY_MANAGED), "unknown" when debugfs is not readable (it needs
# root; host-check normally runs unprivileged).
IRQ_DEBUGFS=/sys/kernel/debug/irq/irqs
irq_managed() {
  local f="$IRQ_DEBUGFS/$1"
  if [ -r "$f" ]; then
    if grep -q 'AFFINITY_MANAGED' "$f" 2>/dev/null; then echo true; else echo false; fi
  else
    echo unknown
  fi
}
IRQ_MANAGED_UNKNOWN=0
if [ -n "$ISOLATED" ]; then
  while read -r irq name; do
    [ -d "/proc/irq/$irq" ] || continue
    IRQ_TOTAL=$((IRQ_TOTAL + 1))
    req="$(readf "/proc/irq/$irq/smp_affinity_list")"
    eff="$(readf "/proc/irq/$irq/effective_affinity_list")"
    # the requested mask may name offline CPUs (nosmt): only the online part counts
    req_on="$(intersect "$(expand "$req")" "$ONLINE")"
    [ -n "$eff" ] || eff="$(compact "$req_on")"
    [ -n "$eff" ] || { IRQ_UNREADABLE=$((IRQ_UNREADABLE + 1)); continue; }
    [ -n "$(intersect "$(expand "$eff")" "$ISOLATED")" ] || continue
    # tolerated only when the online mask holds nothing but isolated CPUs (the
    # kernel cannot move it) and the IRQ is not known to be unmanaged: an
    # unmanaged IRQ on an isolated CPU is movable and fails the check
    managed="$(irq_managed "$irq")"
    if [ -n "$req_on" ] && subset "$req_on" "$ISOLATED" && [ "$managed" != false ]; then
      [ "$managed" = true ] || IRQ_MANAGED_UNKNOWN=$((IRQ_MANAGED_UNKNOWN + 1))
      IRQ_PINNED+=("$irq"$'\t'"$req"$'\t'"$eff"$'\t'"$name"$'\t'"$managed")
    else
      IRQ_BAD+=("$irq"$'\t'"$req"$'\t'"$eff"$'\t'"$name"$'\t'"$managed")
    fi
  done < <(awk 'NR == 1 { ncpu = NF; next }
                $1 ~ /^[0-9]+:$/ { n = $1; sub(":", "", n); name = "";
                  for (i = ncpu + 2; i <= NF; i++) name = name (name == "" ? "" : " ") $i
                  print n, name }' /proc/interrupts)
fi
irq_json() { if [ $# -eq 0 ]; then echo '[]'; else printf '%s\n' "$@" | jq -Rn '[inputs | split("\t") | {irq: (.[0] | tonumber), affinity: .[1], effective: .[2], name: .[3], managed: (if .[4] == "true" then true elif .[4] == "false" then false else null end)}]'; fi; }
IRQ_BAD_JSON="$(irq_json ${IRQ_BAD[@]+"${IRQ_BAD[@]}"})"
IRQ_PINNED_JSON="$(irq_json ${IRQ_PINNED[@]+"${IRQ_PINNED[@]}"})"
if [ -z "$ISOLATED" ]; then
  check "IRQ affinity" false "no isolated CPUs to check against"
elif [ ${#IRQ_BAD[@]} -eq 0 ]; then
  pinned_note="${#IRQ_PINNED[@]} pinned by their mask (irq.pinned_to_isolated)"
  [ "$IRQ_MANAGED_UNKNOWN" -eq 0 ] || pinned_note="$pinned_note, $IRQ_MANAGED_UNKNOWN of them not verified as kernel-managed ($IRQ_DEBUGFS unreadable; run with sudo to verify)"
  check "IRQ affinity" true "$IRQ_TOTAL IRQs, none movable on isolated CPUs; $pinned_note"
else
  check "IRQ affinity" false "$(printf '%s\n' "${IRQ_BAD[@]}" | awk -F'\t' '{ printf "%sirq %s eff %s (%s)", (NR > 1 ? ", " : ""), $1, $3, $4 }')"
fi

# --- load ----------------------------------------------------------------------
LOADAVG="$(readf /proc/loadavg)"
LOAD1="$(awk '{ print $1 }' <<< "$LOADAVG")"
if [ "$SKIP_LOAD" = 1 ]; then
  check "load" true "skipped in phase $PHASE (E1 gates the load); loadavg $LOADAVG"
elif awk -v l="$LOAD1" -v m="$LOAD_LIMIT" 'BEGIN { exit !(l <= m) }'; then
  check "load" true "1-min load $LOAD1 <= $LOAD_LIMIT"
else
  check "load" false "1-min load $LOAD1 > $LOAD_LIMIT; wait for a quiet host (top -o %CPU)"
fi

# --- sleep targets / background services -----------------------------------------
have_systemctl=0; command -v systemctl >/dev/null && have_systemctl=1
unit_state() { systemctl "$1" "$2" 2>/dev/null | head -n 1 || true; }
SLEEP_UNITS=(sleep.target suspend.target hibernate.target hybrid-sleep.target)
SERVICES=(unattended-upgrades.service snapd.service snapd.socket apt-daily.timer apt-daily-upgrade.timer
  apt-daily.service apt-daily-upgrade.service fwupd.service fwupd-refresh.timer motd-news.timer irqbalance.service)
SLEEP_STATES=(); SERVICE_STATES=()
if [ "$have_systemctl" = 1 ]; then
  not_masked=()
  for u in "${SLEEP_UNITS[@]}"; do
    s="$(unit_state is-enabled "$u")"; SLEEP_STATES+=("$u"$'\t'"${s:-unknown}")
    [ "$s" = masked ] || not_masked+=("$u=${s:-unknown}")
  done
  if [ ${#not_masked[@]} -eq 0 ]; then
    check "sleep" true "sleep/suspend/hibernate/hybrid-sleep targets masked"
  else
    check "sleep" false "${not_masked[*]} (host-prep.sh masks them)"
  fi
  active=()
  for u in "${SERVICES[@]}"; do
    s="$(unit_state is-active "$u")"; SERVICE_STATES+=("$u"$'\t'"${s:-unknown}")
    case "$s" in active|activating|reloading) active+=("$u=$s") ;; esac
  done
  if [ ${#active[@]} -eq 0 ]; then
    check "background services" true "none of ${#SERVICES[@]} units active"
  else
    check "background services" false "${active[*]} (host-prep.sh stops and masks them)"
  fi
else
  check "sleep" false "systemctl not found"
  check "background services" false "systemctl not found"
fi
pairs_json() { if [ $# -eq 0 ]; then echo '{}'; else printf '%s\n' "$@" | jq -Rn '[inputs | split("\t") | {key: .[0], value: .[1]}] | from_entries'; fi; }
SLEEP_JSON="$(pairs_json ${SLEEP_STATES[@]+"${SLEEP_STATES[@]}"})"
SERVICES_JSON="$(pairs_json ${SERVICE_STATES[@]+"${SERVICE_STATES[@]}"})"

# --- docker --------------------------------------------------------------------
DOCKER_PATH="$(command -v docker 2>/dev/null || true)"
DOCKER_REAL=""; [ -n "$DOCKER_PATH" ] && DOCKER_REAL="$(readlink -f "$DOCKER_PATH" 2>/dev/null || echo "$DOCKER_PATH")"
DOCKER_SNAP=false
compgen -G '/var/lib/snapd/snaps/docker_*.snap' >/dev/null 2>&1 && DOCKER_SNAP=true
[ -e /snap/bin/docker ] && DOCKER_SNAP=true
DOCKER_SERVER="$( [ -n "$DOCKER_PATH" ] && docker version -f '{{.Server.Version}}' 2>/dev/null || true)"
DOCKER_CGROUP="$( [ -n "$DOCKER_PATH" ] && docker info -f '{{.CgroupDriver}} cgroup v{{.CgroupVersion}}' 2>/dev/null || true)"
if [ -z "$DOCKER_PATH" ]; then
  check "docker" false "docker CLI not found (official apt repo: docker-ce)"
elif [ "$DOCKER_SNAP" = true ] || [[ "$DOCKER_REAL" == /snap/* ]]; then
  check "docker" false "docker snap present ($DOCKER_REAL); use the official apt repo (download.docker.com)"
elif [ -z "$DOCKER_SERVER" ]; then
  check "docker" false "$DOCKER_REAL: daemon not reachable (is the user in the docker group?)"
else
  check "docker" true "$DOCKER_REAL, server $DOCKER_SERVER${DOCKER_CGROUP:+, $DOCKER_CGROUP}"
fi

# --- recorded, not judged ----------------------------------------------------------
TEMPS=()
TEMP_SOURCE=""
for h in /sys/class/hwmon/hwmon*; do
  [ "$(readf "$h/name")" = k10temp ] || continue
  TEMP_SOURCE="${h##*/} k10temp"
  for t in "$h"/temp*_input; do
    [ -r "$t" ] || continue
    label="$(readf "${t%_input}_label")"; [ -n "$label" ] || label="$(basename "${t%_input}")"
    TEMPS+=("$label"$'\t'"$(readf "$t")")
  done
  break
done
TEMPS_JSON="$(if [ ${#TEMPS[@]} -eq 0 ]; then echo '{}'; else printf '%s\n' "${TEMPS[@]}" \
  | jq -Rn '[inputs | split("\t") | {key: .[0], value: ((.[1] | tonumber? // null) | if . == null then null else . / 1000 end)}] | from_entries'; fi)"
[ -n "$TEMP_SOURCE" ] || echo "note  k10temp not found; temperature not recorded" >&2

CPU_MODEL="$(awk -F': ' '/^model name/ { print $2; exit }' /proc/cpuinfo)"
MICROCODE="$(awk -F': ' '/^microcode/ { print $2; exit }' /proc/cpuinfo)"
CPB_FLAG="$(awk '/^flags/ { print (/(^| )cpb( |$)/ ? "true" : "false"); exit }' /proc/cpuinfo)"
[ -n "$CPB_FLAG" ] || CPB_FLAG=false
FIRST_ISO="${ISOLATED%% *}"
CPUIDLE_DRIVER="$(readf $SYS/cpuidle/current_driver)"
CPUIDLE_STATES="$( [ -n "$FIRST_ISO" ] && for s in "$SYS/cpu$FIRST_ISO"/cpuidle/state*; do [ -d "$s" ] && printf '%s%s' "$(readf "$s/name")" "$( [ "$(readf "$s/disable")" = 1 ] && echo '(disabled)')"; printf ' '; done | sed 's/ $//' || true)"
CGROUP_FS="$(stat -fc %T /sys/fs/cgroup 2>/dev/null || true)"

PASSED=true
for c in "${CHECKS[@]}"; do
  IFS=$'\t' read -r _ ok _ <<< "$c"
  [ "$ok" = true ] || PASSED=false
done
CHECKS_JSON="$(printf '%s\n' "${CHECKS[@]}" | jq -Rn '[inputs | split("\t") | {name: .[0], ok: (.[1] == "true"), detail: .[2]}]')"

RECORD="$(jq -n \
  --arg profile "$PROFILE" --arg phase "$PHASE" --argjson passed "$PASSED" \
  --arg checked_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --argjson checks "$CHECKS_JSON" \
  --arg boot_id "$(readf /proc/sys/kernel/random/boot_id)" --arg kernel "$(uname -r)" --arg kernel_version "$(uname -v)" --arg arch "$(uname -m)" \
  --arg cpu_model "$CPU_MODEL" --arg microcode "$MICROCODE" --arg cmdline "$CMDLINE" \
  --arg virt "$VIRT_DETECT" --argjson hypervisor_flag "$HYPERVISOR_FLAG" \
  --arg online "$ONLINE_RAW" --arg isolated "$(compact "$ISOLATED")" --arg system "$(compact "$SYSTEM")" \
  --argjson cpusets "$CPUSETS_JSON" \
  --arg l3_isolated "$(compact "$L3_ISO")" --arg l3_system "$(compact "$L3_SYS")" \
  --arg smt_control "$SMT_CONTROL" --arg smt_active "$SMT_ACTIVE" \
  --arg kernel_isolated "$ISO_KERNEL_RAW" --arg init_affinity "$INIT_AFFINITY" --arg kernel_nohz "$NOHZ_RAW" \
  --arg driver "$DRIVER" --arg amd_pstate "$AMD_PSTATE_STATUS" --arg governors "$GOV_SUMMARY" \
  --arg epp "$(printf '%s\n' ${EPPS[@]+"${EPPS[@]}"} | sort -u | paste -sd ' ' -)" \
  --arg boost_global "$BOOST_GLOBAL" --argjson boost_knobs "$boost_knobs" --argjson cpb_flag "$CPB_FLAG" \
  --arg nominal "$NOMINAL_KHZ" --argjson max_freq "$MAXF_JSON" \
  --argjson irq_total "$IRQ_TOTAL" --argjson irq_unreadable "$IRQ_UNREADABLE" \
  --argjson irq_bad "$IRQ_BAD_JSON" --argjson irq_pinned "$IRQ_PINNED_JSON" \
  --arg loadavg "$LOADAVG" --arg load_limit "$LOAD_LIMIT" --argjson load_skipped "$( [ "$SKIP_LOAD" = 1 ] && echo true || echo false)" \
  --argjson sleep_units "$SLEEP_JSON" --argjson services "$SERVICES_JSON" \
  --arg docker_path "$DOCKER_REAL" --arg docker_server "$DOCKER_SERVER" --arg docker_cgroup "$DOCKER_CGROUP" \
  --argjson docker_snap "$DOCKER_SNAP" --arg cgroup_fs "$CGROUP_FS" \
  --arg cpuidle_driver "$CPUIDLE_DRIVER" --arg cpuidle_states "$CPUIDLE_STATES" \
  --arg thp "$(readf /sys/kernel/mm/transparent_hugepage/enabled)" \
  --arg numa_balancing "$(readf /proc/sys/kernel/numa_balancing)" \
  --arg aslr "$(readf /proc/sys/kernel/randomize_va_space)" \
  --arg temp_source "$TEMP_SOURCE" --argjson temps "$TEMPS_JSON" '
  def nz: if . == "" then null else . end;
  def param($k): ($cmdline | split(" ") | map(select(startswith($k + "="))) | last // null | if . then sub("^[^=]*="; "") else null end);
  {
    schema: "host-check/1",
    profile: $profile,
    phase: $phase,
    checked_at: $checked_at,
    host_check: { passed: $passed, failed: [$checks[] | select(.ok | not) | .name], checks: $checks },
    host: {
      boot_id: ($boot_id | nz), kernel: $kernel, kernel_version: $kernel_version, arch: $arch,
      cpu_model: ($cpu_model | nz), microcode: ($microcode | nz), cmdline: $cmdline,
      virtualization: { systemd_detect_virt: ($virt | nz), hypervisor_flag: $hypervisor_flag },
      cmdline_params: { isolcpus: param("isolcpus"), nohz_full: param("nohz_full"), rcu_nocbs: param("rcu_nocbs"),
                        irqaffinity: param("irqaffinity"), nosmt: ($cmdline | split(" ") | any(. == "nosmt")) }
    },
    cpu: {
      online: $online, isolated: $isolated, system: $system, cpusets: $cpusets,
      l3_domains: { isolated: ($l3_isolated | nz), system: ($l3_system | nz) },
      smt: { control: ($smt_control | nz), active: ($smt_active | nz) },
      kernel_isolated: ($kernel_isolated | nz), init_affinity: ($init_affinity | nz), kernel_nohz_full: ($kernel_nohz | nz),
      cpufreq: { driver: ($driver | nz), amd_pstate_status: ($amd_pstate | nz), governors: ($governors | nz), epp: ($epp | nz) },
      boost: { global: ($boost_global | nz), controls: $boost_knobs, cpb_flag: $cpb_flag, nominal_khz: ($nominal | nz),
               cpuinfo_max_freq_khz: $max_freq },
      cpuidle: { driver: ($cpuidle_driver | nz), states: ($cpuidle_states | nz) }
    },
    irq: { checked: $irq_total, unreadable: $irq_unreadable, on_isolated: $irq_bad, pinned_to_isolated: $irq_pinned },
    load: { loadavg: $loadavg, limit: ($load_limit | tonumber), skipped: $load_skipped },
    systemd: { sleep_units: $sleep_units, services: $services },
    docker: { path: ($docker_path | nz), server_version: ($docker_server | nz), cgroup: ($docker_cgroup | nz),
              snap: $docker_snap, cgroup_fs: ($cgroup_fs | nz) },
    tunables: { transparent_hugepage: ($thp | nz), numa_balancing: ($numa_balancing | nz), randomize_va_space: ($aslr | nz) },
    temperature: { source: ($temp_source | nz), celsius: $temps }
  }')"

if [ -n "$OUT_FILE" ]; then
  printf '%s\n' "$RECORD" > "$OUT_FILE"
else
  printf '%s\n' "$RECORD"
fi

if [ "$PASSED" = true ]; then
  echo "host-check: passed ($PHASE)" >&2
  exit 0
fi
echo "host-check: FAILED ($PHASE): $(jq -r '.host_check.failed | join(", ")' <<< "$RECORD")" >&2
exit 1
