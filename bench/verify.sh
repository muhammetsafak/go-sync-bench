#!/usr/bin/env bash
# Acceptance check of the harness. Nothing here runs Go code on the host: every
# build, vet, test, self-test and measurement runs in a container without
# network, with a read-only root and only the work directory mounted.
#
#   ./bench/verify.sh
#
# Steps: go.mod has no dependencies -> image build -> gofmt + go vet +
# go test -race -> self-test x5 at GOMAXPROCS=2 -> matrix size -> park
# threshold calibration (P=1, P=2) -> smoke run -> report.mjs.
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
#    mounted read-only and copied to the private /tmp.
docker run --rm --network none --read-only --tmpfs /tmp:exec \
  --user "$(id -u):$(id -g)" --cap-drop ALL --security-opt no-new-privileges \
  -v "$ROOT:/src:ro" -w /tmp -e GOCACHE=/tmp/cache -e GOTOOLCHAIN=local \
  -e GOPROXY=off -e CGO_ENABLED=1 -e HOME=/tmp \
  "$GO_IMAGE" sh -ec '
    cp -r /src /tmp/w && cd /tmp/w && rm -rf results
    bad=$(gofmt -l .); [ -z "$bad" ] || { echo "gofmt: $bad"; exit 1; }
    go vet ./...
    go test -race -count=1 ./...
  ' || fail "gofmt/vet/test"
log "gofmt, go vet, go test -race: ok"

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
[ "$pass" = 5 ] || fail "self-test passed $pass/5"
log "verify: ok ($OUT)"
