# go-sync-bench — 2026-09-29, merged view (first pass + cell rerun)

This directory holds the published summary of the 2026-09-29 run.

- rule: a rerun replaces the cell entirely (design "Kurallar"; g4-esik "Hata modları"). The 88 rerun cells take their 5 repetitions from `../rerun/raw`; the other 757 cells take theirs from `../raw` unchanged. No repetition is picked across passes. `summary.json` `merged` records the counts (`replaced_cells` 88, `missing_rerun_cells` none, 3785 + 440 raw files); each cell's `source` says which pass it comes from.
- produced by `bench/report.mjs` at `bb3292a`, reading `../raw` and `../rerun` directly (command below). `report.mjs --no-merge results/2026-09-29` reproduces the first pass alone in `../summary.json`.
- rerun trigger: trimmed_range_v2 + G5 (user decision 2026-09-29, option E, limit 84 -> 88); text in `summary.json` `method.rerun_trigger`.
- the design's original trigger (full range, G4 thresholds) chose **294 cells (G4 290, G5 11) on the first pass**; that number is in `../summary.json` `gates.rerun_v1` and is the one the G4 decision keeps in print. `gates.rerun_v1` in this directory's `summary.json` (273, all G4) is the same trigger recomputed on the merged data, not the original result.
- flags keep the original thresholds (spread_throughput 0.10, spread_p99 0.25).
- images: first pass `sha256:640a046f…` (`../RUN.md`), rerun `sha256:eabc1573…` (`../rerun/RUN.md`). Only orchestration changed between them (`cmd/syncbench`, `bench/run.sh`); `internal/` and `docker/` did not. The merged view mixes the two images by cell, as `source` shows.

## Result

- cells: 845, valid repetitions: 4225, invalid: 0
- flagged cells: **518** (first pass 530); gate_failed_share 0.613, confidence suggestion: low
- of the 88 rerun cells: 76 still flagged (84 -> 67 with a spread flag), 12 clean
- `spread_persistent`: **36** cells met the G4 part of the rerun trigger on the first pass and still meet it on the rerun (P1 11, P2 18, P4 1, P8 6). They carry the flag in `summary.json` (`gates.spread_persistent`) and are listed in `rerun-cells.txt`. Cells are rerun once; there is no second rerun.
- `gates.rerun_count` / `gates.rerun_cells` (36) count the cells still over the trigger after the rerun, not the cells that were rerun (88, `merged.replaced_cells`).
- throughput median moved by more than 10 % in 14 of the 88 cells (more than 25 % in 1: s3-cond1024-g1024-p2-w0, -25.7 %); the rerun is published, the first pass stays in `../raw`.

## Commands

```
STAMP=2026-09-29 caffeinate -dimsu ./bench/run.sh --cells results/2026-09-29/rerun-cells.txt
docker run --rm --network none --read-only --tmpfs /tmp --user 65534:65534 --cap-drop ALL \
  --security-opt no-new-privileges -v "$PWD/bench:/bench:ro" \
  -v "$PWD/results/2026-09-29:/results" -v "$PWD/results/2026-09-29/raw:/results/raw:ro" \
  -v "$PWD/results/2026-09-29/rerun:/results/rerun:ro" \
  node:24-trixie-slim node /bench/report.mjs /results
```

`report.mjs` writes only `merged/summary.json`, `summary.csv` and `rerun-cells.txt` here. Earlier, before `96d8e99`, this view was built by copying files into `merged/` with a `merge.sh` script; those copies (`raw/`, `timing/`, `selftest/`, `matrix.json`, `merge.json`, `merge.sh`, `rerun_of.json`) are obsolete, not part of the published data and not committed. Do not run `merge.sh`: it rebuilds `merged/` from scratch and drops `summary.json`.
