#!/usr/bin/env bash
# Acceptance check of the harness. Nothing here runs Go code on the host: every
# build, vet, test, self-test and measurement runs in a container without
# network, with a read-only root and only the work directory mounted.
#
#   ./bench/verify.sh
#
# Steps: go.mod has no dependencies -> image build -> gofmt + go vet +
# go test -race -> report rule tests (node --test) and the pmset sleep parser
# -> self-test x5 at GOMAXPROCS=2 -> matrix size -> park threshold calibration
# (P=1, P=2) -> smoke run -> report.mjs.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

fail() { log "FAIL: $*"; exit 1; }

# 1. Standard library only.
n=$(grep -c require go.mod || true)
[ "$n" = 0 ] || fail "go.mod has $n require lines"
log "go.mod: no require"

# 2. Image (offline build).
build_image
log "image $IMAGE built"

# 3. Format, vet and race tests in the Go toolchain image; the source is
#    mounted read-only and copied to the private /tmp (results/ is dropped from
#    the copy; read-only result directories are made writable there first).
docker run --rm --network none --read-only --tmpfs /tmp:exec \
  --user "$(id -u):$(id -g)" --cap-drop ALL --security-opt no-new-privileges \
  -v "$ROOT:/src:ro" -w /tmp -e GOCACHE=/tmp/cache -e GOTOOLCHAIN=local \
  -e GOPROXY=off -e CGO_ENABLED=1 -e HOME=/tmp \
  "$GO_IMAGE" sh -ec '
    cp -r /src /tmp/w && cd /tmp/w && chmod -R u+w results 2>/dev/null; rm -rf results
    bad=$(gofmt -l .); [ -z "$bad" ] || { echo "gofmt: $bad"; exit 1; }
    go vet ./...
    go test -race -count=1 ./...
  ' || fail "gofmt/vet/test"
log "gofmt, go vet, go test -race: ok"

# 3b. The rerun trigger of the report (gates.mjs) and its wiring, offline in
#     the Node image; the tests write only to the container's /tmp.
docker run --rm --network none --read-only --tmpfs /tmp \
  --user 65534:65534 --cap-drop ALL --security-opt no-new-privileges \
  -v "$ROOT/bench:/bench:ro" -w /tmp \
  "$NODE_IMAGE" node --test /bench/gates.test.mjs >/dev/null || fail "node --test bench/gates.test.mjs"
log "report rule tests: ok"

# 3c. The system-sleep gate reads `pmset -g`; its parser on fixed inputs.
[ "$(printf ' sleep                1 (sleep prevented by powerd)\n displaysleep 0\n' | pmset_sleep_value)" = 1 ] \
  || fail "pmset parser: sleep 1"
[ "$(printf ' displaysleep 10\n sleep                0\n' | pmset_sleep_value)" = 0 ] || fail "pmset parser: sleep 0"
[ "$(printf ' displaysleep 0\n disksleep 0\n' | pmset_sleep_value)" = unknown ] || fail "pmset parser: missing"
log "pmset sleep parser: ok (host: $(pmset -g 2>/dev/null | pmset_sleep_value))"

# 4. Self-test of the starving bit, five attempts at GOMAXPROCS=2.
STAMP="${STAMP:-smoke-$(date -u +%Y%m%dT%H%M%SZ)}"
export STAMP
OUT="$ROOT/results/$STAMP"
mkdir -p "$OUT/selftest-verify"
chmod -R 0777 "$OUT"
pass=0
for i in 1 2 3 4 5; do
  set +e
  sandbox_run 2 "$OUT" selftest -out "/results/selftest-verify/$i.json" >/dev/null
  code=$?
  set -e
  res=$(jq -r .result "$OUT/selftest-verify/$i.json" 2>/dev/null || echo error)
  log "self-test $i: $res (exit $code)"
  [ "$res" = pass ] && pass=$((pass + 1))
done
echo "$pass" > "$OUT/selftest-verify/passes"
log "self-test: $pass/5 pass"

# 5. Matrix size.
main=$(tool_run cells -set main)
[ "$main" = 845 ] || fail "main set has $main cells, expected 845"
log "main set: $main cells"

# 6. Park threshold calibration (the 10 us threshold, design section 6).
mkdir -p "$OUT/calibrate"; chmod 0777 "$OUT/calibrate"
for p in 1 2; do
  sandbox_run "$p" "$OUT" calibrate -out "/results/calibrate/P$p.json" >/dev/null \
    || fail "calibrate P=$p"
done
log "calibration written"

# 7. Smoke run.
./bench/run.sh --smoke >/dev/null || fail "smoke run"
log "smoke run done: $OUT"

# 8. Report, in the Node image, offline; code read-only, results writable.
docker run --rm --network none --read-only --tmpfs /tmp \
  --user 65534:65534 --cap-drop ALL --security-opt no-new-privileges \
  -v "$ROOT/bench:/bench:ro" -v "$OUT:/results" \
  "$NODE_IMAGE" node /bench/report.mjs /results || fail "report.mjs"

# 9. The report exists and covers the smoke set.
want=$(tool_run cells -set smoke)
got=$(jq '.cells_measured' "$OUT/summary.json")
[ "$got" = "$want" ] || fail "summary covers $got of $want smoke cells"
[ -s "$OUT/summary.csv" ] || fail "summary.csv missing"
[ -s "$OUT/rerun-cells.txt" ] || fail "rerun-cells.txt missing"
jq -e '.method.rerun_trigger and .gates.rerun_basis == "trimmed_range_v2"' "$OUT/summary.json" >/dev/null \
  || fail "summary.json does not state the rerun rule"

# 10. A cell rerun needs the stamp of its first pass (usage error, exit 2).
set +e
STAMP= ./bench/run.sh --cells "$OUT/rerun-cells.txt" 2>/dev/null
code=$?
set -e
[ "$code" = 2 ] || fail "run.sh --cells without STAMP exited $code, want 2"
[ "$pass" = 5 ] || fail "self-test passed $pass/5"
log "verify: ok ($OUT)"
