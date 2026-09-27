#!/usr/bin/env node
// Raw repetition JSON -> summary.json + summary.csv, with the soft gates of the
// design evaluated per cell. Raw files are never modified; the report can be
// re-run at any time.
//
//   node bench/report.mjs results/<stamp>
//
// Estimator: the median of the valid repetitions of a cell for every metric;
// latency percentiles are per repetition first, then the median of the
// percentiles. The best throughput repetition is kept as a separate field.
import { readFileSync, readdirSync, writeFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';

const dir = process.argv[2];
if (!dir) {
  console.error('usage: node bench/report.mjs <results-dir>');
  process.exit(2);
}

// Thresholds of the design (gates G4-G9 and the fairness attribution).
const G4_TPUT = 0.10;
const G4_P99 = 0.25;
const G5_DRIFT = 0.05;
const G6_RATIO = 0.90;
const G7_SAMPLES = 10000;
const G8_FACTOR = 4;
const G9_TAIL = 0.01;
const BAND_LOW = 0.8;
const BAND_HIGH = 1.2;
const SLOW_SHARE = 0.5;
const REP_BUDGET_S = 5;
const LOW_CONFIDENCE_SHARE = 0.15;

const readJSON = (p) => JSON.parse(readFileSync(p, 'utf8'));
const matrix = existsSync(join(dir, 'matrix.json')) ? readJSON(join(dir, 'matrix.json')) : null;

const median = (xs) => {
  const v = xs.filter((x) => typeof x === 'number' && Number.isFinite(x)).sort((a, b) => a - b);
  if (v.length === 0) return null;
  const m = v.length >> 1;
  return v.length % 2 ? v[m] : (v[m - 1] + v[m]) / 2;
};
const nums = (xs) => xs.filter((x) => typeof x === 'number' && Number.isFinite(x));
const min = (xs) => (nums(xs).length ? Math.min(...nums(xs)) : null);
const max = (xs) => (nums(xs).length ? Math.max(...nums(xs)) : null);
const spread = (xs) => {
  const m = median(xs);
  return m ? (max(xs) - min(xs)) / m : null;
};
const round = (x, d = 4) => (x === null || x === undefined ? null : Number(x.toFixed(d)));
const get = (o, path) => path.split('.').reduce((a, k) => (a == null ? a : a[k]), o);

// ---- raw files ---------------------------------------------------------------
const rawDir = join(dir, 'raw');
const files = existsSync(rawDir) ? readdirSync(rawDir) : [];
const reps = [];
const invalid = [];
for (const f of files) {
  if (!f.endsWith('.json')) continue;
  const rec = readJSON(join(rawDir, f));
  if (f.endsWith('.invalid.json')) {
    invalid.push({ file: f, cell_id: rec.cell_id, round: rec.round, gates: rec.gates });
    continue;
  }
  if (!rec.gates?.valid) {
    invalid.push({ file: f, cell_id: rec.cell_id, round: rec.round, gates: rec.gates });
    continue;
  }
  reps.push(rec);
}

// ---- side files: timing and self-test -----------------------------------------
const timing = [];
if (existsSync(join(dir, 'timing'))) {
  for (const f of readdirSync(join(dir, 'timing'))) {
    for (const line of readFileSync(join(dir, 'timing', f), 'utf8').split('\n')) {
      if (line.trim()) timing.push(JSON.parse(line));
    }
  }
}
const measured = timing.filter((t) => !t.skipped);
const selftests = [];
if (existsSync(join(dir, 'selftest'))) {
  for (const f of readdirSync(join(dir, 'selftest')).sort()) {
    const s = readJSON(join(dir, 'selftest', f));
    selftests.push({ file: f, result: s.result, ...s });
  }
}

// ---- per cell -----------------------------------------------------------------
const byCell = new Map();
for (const r of reps) {
  if (!byCell.has(r.cell_id)) byCell.set(r.cell_id, []);
  byCell.get(r.cell_id).push(r);
}
const setName = reps[0]?.set ?? 'main';
const expectedRounds = matrix?.sets?.[setName]?.rounds ?? null;
const notes = new Map();
for (const c of matrix?.sets?.[setName]?.cells ?? []) {
  const id = cellID(c);
  if (c.note) notes.set(id, c.note);
}
function cellID(c) {
  let id = `${c.scenario}-${c.impl}`;
  if (c.scenario === 's2') id += `-r${c.read}`;
  return `${id}-g${c.G}-p${c.P}-w${c.W}`;
}

function fairnessSource(g, p, b, s, j) {
  if (g <= p || b >= 1) return 'sync';
  if (s >= SLOW_SHARE) return 'sync';
  if (j >= BAND_LOW * b && j <= BAND_HIGH * b) return 'scheduler';
  return 'unexplained';
}

// A latency percentile: median over the repetitions whose histogram passes G7
// (p99.9 only), rendered as "< X ns" below G8's resolution floor.
function percentile(rs, hist, q) {
  const vals = [];
  let dropped = 0;
  for (const r of rs) {
    const h = r.latency?.[hist];
    if (!h || h[q] == null) continue;
    if (q === 'p999' && h.n < G7_SAMPLES) {
      dropped++;
      continue;
    }
    vals.push(h[q]);
  }
  const m = median(vals);
  const floor = G8_FACTOR * (median(rs.map((r) => r.env?.clock_resolution_ns)) ?? 0);
  if (m === null) return { value: null, dropped_g7: dropped };
  if (m < floor) return { value: `< ${Math.round(floor)} ns`, below_resolution: true, spread: round(spread(vals)) };
  return { value: Math.round(m), spread: round(spread(vals)), dropped_g7: dropped || undefined };
}

const cells = [];
for (const [id, rs] of [...byCell.entries()].sort()) {
  rs.sort((a, b) => a.round - b.round);
  const r0 = rs[0];
  const ops = rs.map((r) => r.throughput?.ops_per_sec);
  const flags = [];
  const rerun = [];

  // G4: spread of throughput and of op_latency p99.
  const tSpread = spread(ops);
  const p99s = rs.map((r) => r.latency?.op_latency_hist?.p99);
  const pSpread = spread(p99s);
  if (rs.length >= 2 && tSpread !== null && tSpread > G4_TPUT) flags.push('spread_throughput');
  if (rs.length >= 2 && pSpread !== null && pSpread > G4_P99) flags.push('spread_p99');
  if (flags.some((f) => f.startsWith('spread'))) rerun.push('G4');

  // G5: calibration drift per repetition.
  const drifted = rs.filter((r) => (r.calib?.drift ?? 0) > G5_DRIFT).length;
  if (drifted > 0) flags.push('calib_drift');
  if (drifted >= 2) rerun.push('G5');

  // G6: observer effect (latency fields only).
  const ratio = median(rs.map((r) => r.latency?.ops_ratio));
  if (ratio !== null && ratio < G6_RATIO) flags.push('observer_effect');

  // G9: operations that finished after the window closed.
  const tail = median(rs.map((r) => r.throughput?.window_tail_frac));
  if (tail !== null && tail >= G9_TAIL) flags.push('window_tail');

  // Missing repetitions.
  const rounds = rs.map((r) => r.round);
  const missing = expectedRounds
    ? Array.from({ length: expectedRounds }, (_, i) => i + 1).filter((x) => !rounds.includes(x))
    : [];
  if (missing.length) flags.push('missing_reps');

  // Fairness attribution with the medians.
  const g = r0.scenario === 's3' ? Math.floor(r0.G / 2) : r0.G;
  const jain = median(rs.map((r) => r.throughput?.fairness_jain));
  const s = median(rs.map((r) => r.fairness?.slow_acquire_share));
  const b = r0.fairness?.quantum_bound ?? null;
  const bRaw = r0.fairness?.quantum_bound_raw ?? null;
  const source = jain !== null && s !== null && b !== null ? fairnessSource(g, r0.P, b, s, jain) : null;
  if (source === 'unexplained') flags.push('fairness_unexplained');

  const slowByThreshold = {};
  for (const k of Object.keys(r0.latency?.slow_share_by_threshold_us ?? {}).sort((a, b) => a - b)) {
    slowByThreshold[k] = round(median(rs.map((r) => r.latency?.slow_share_by_threshold_us?.[k])));
  }
  const starving = rs.map((r) => r.latency?.starving_fraction);
  const expectedJain = r0.fairness?.expected_quantum_jain ?? null;

  const cell = {
    cell_id: id,
    scenario: r0.scenario,
    impl: r0.impl,
    G: r0.G,
    P: r0.P,
    W_nominal_ns: r0.W_nominal_ns,
    W_realized_ns: round(median(rs.map((r) => r.W_realized_ns)), 1),
    read_ratio: r0.read_ratio ?? null,
    note: notes.get(id) ?? null,
    reps: rs.length,
    rounds,
    missing_rounds: missing,
    throughput: {
      ops_per_sec: round(median(ops), 1),
      min: round(min(ops), 1),
      max: round(max(ops), 1),
      spread: round(tSpread),
      best_ops_per_sec: round(max(ops), 1),
      ns_per_op: round(median(rs.map((r) => r.throughput?.ns_per_op)), 2),
      cpu_per_op_ns: round(median(rs.map((r) => r.throughput?.cpu_per_op_ns)), 2),
      cpu_user_s: round(median(rs.map((r) => r.throughput?.cpu_user_s))),
      cpu_sys_s: round(median(rs.map((r) => r.throughput?.cpu_sys_s))),
      rt_mutex_wait_s: round(median(rs.map((r) => r.throughput?.rt_mutex_wait_s))),
      rt_sched_latency_p99_s: round(median(rs.map((r) => r.throughput?.rt_sched_latency_p50_p99?.[1])), 7),
      gc_cycles: median(rs.map((r) => r.throughput?.gc_cycles)),
      window_tail_frac: round(tail, 6),
      drain_frac: round(median(rs.map((r) => r.throughput?.drain_frac)), 6),
    },
    fairness: {
      jain: round(jain),
      jain_min: round(min(rs.map((r) => r.throughput?.fairness_jain))),
      jain_max: round(max(rs.map((r) => r.throughput?.fairness_jain))),
      min_max: round(median(rs.map((r) => r.throughput?.fairness_min_max))),
      zero_op_goroutines: median(rs.map((r) => r.throughput?.zero_op_goroutines)),
      producer_jain: round(median(rs.map((r) => r.throughput?.producer_jain))),
      quantum_bound: b === null ? null : round(b),
      quantum_bound_raw: bRaw === null ? null : round(bRaw),
      expected_quantum_jain: expectedJain === null ? null : round(expectedJain),
      jain_over_expected: expectedJain && jain !== null ? round(jain / expectedJain) : null,
      slow_acquire_share: round(s),
      fairness_source: source,
      fairness_source_per_rep: rs.map((r) => r.fairness?.fairness_source),
    },
    latency: {
      ops_ratio: round(ratio),
      op_p50: percentile(rs, 'op_latency_hist', 'p50'),
      op_p99: percentile(rs, 'op_latency_hist', 'p99'),
      op_p999: percentile(rs, 'op_latency_hist', 'p999'),
      op_max_ns: median(rs.map((r) => r.latency?.op_latency_hist?.max)),
      acquire_p50: percentile(rs, 'acquire_wait_hist', 'p50'),
      acquire_p99: percentile(rs, 'acquire_wait_hist', 'p99'),
      acquire_p999: percentile(rs, 'acquire_wait_hist', 'p999'),
      waits_over_1ms_est: median(rs.map((r) => r.latency?.waits_over_1ms_est)),
      slow_share_by_threshold_us: slowByThreshold,
      starving_fraction: round(median(starving), 6),
      starving_null_reason: r0.latency?.starving_null_reason ?? null,
    },
    calib: {
      drift_max: round(max(rs.map((r) => r.calib?.drift))),
      drifted_reps: drifted,
      start_ns_per_iter: round(median(rs.map((r) => r.calib?.start_ns_per_iter))),
    },
    wall_s_max: round(max(rs.map((r) => r.wall_s)), 2),
    flagged: flags.length > 0,
    flags,
    rerun_gates: rerun,
  };
  cells.push(cell);
}

// ---- run-level ------------------------------------------------------------------
const noiseFloor = {};
for (const c of cells) {
  if (c.scenario === 's1' && c.impl === 'atomic' && c.W_nominal_ns === 0 && c.G >= 2 && c.G <= c.P) {
    noiseFloor[`P${c.P}`] = { cell_id: c.cell_id, jain: c.fairness.jain, min_max: c.fairness.min_max };
  }
}
const labelChecks = cells
  .filter((c) => c.note?.startsWith('label:'))
  .map((c) => {
    const want = /scheduler/.test(c.note) ? 'scheduler' : 'sync';
    return {
      cell_id: c.cell_id,
      expected: want,
      got: c.fairness.fairness_source,
      ok: c.fairness.fairness_source === want,
      jain: c.fairness.jain,
      quantum_bound: c.fairness.quantum_bound,
      slow_acquire_share: c.fairness.slow_acquire_share,
    };
  });
const flaggedCells = cells.filter((c) => c.flagged).length;
// Soft gates that change what is published (missing reps are a run problem).
const gateFailed = cells.filter((c) => c.flags.some((f) => f !== 'missing_reps')).length;
const flagCounts = {};
for (const c of cells) for (const f of c.flags) flagCounts[f] = (flagCounts[f] ?? 0) + 1;
const walls = measured.map((t) => t.wall_s);

const summary = {
  generated: new Date().toISOString(),
  dir,
  set: setName,
  expected_rounds: expectedRounds,
  expected_cells: matrix ? null : undefined,
  cells_measured: cells.length,
  valid_reps: reps.length,
  invalid_reps: invalid,
  thresholds: {
    G4_throughput_spread: G4_TPUT, G4_p99_spread: G4_P99, G5_drift: G5_DRIFT, G6_ops_ratio: G6_RATIO,
    G7_min_samples: G7_SAMPLES, G8_resolution_factor: G8_FACTOR, G9_window_tail: G9_TAIL,
    fairness_band: [BAND_LOW, BAND_HIGH], slow_share: SLOW_SHARE, park_threshold_us: 10,
  },
  gates: {
    flagged_cells: flaggedCells,
    gate_failed_cells: gateFailed,
    gate_failed_share: cells.length ? round(gateFailed / cells.length) : null,
    flag_counts: flagCounts,
    confidence_suggestion: cells.length && gateFailed / cells.length > LOW_CONFIDENCE_SHARE ? 'low' : 'medium',
    rerun_cells: cells.filter((c) => c.rerun_gates.length).map((c) => ({ cell_id: c.cell_id, gates: c.rerun_gates })),
  },
  selftest: {
    runs: selftests.length,
    results: selftests.reduce((a, s) => ((a[s.result] = (a[s.result] ?? 0) + 1), a), {}),
    files: selftests.map((s) => ({ file: s.file, result: s.result })),
  },
  timing: {
    reps: measured.length,
    wall_s_median: round(median(walls), 2),
    wall_s_max: round(max(walls), 2),
    budget_s: REP_BUDGET_S,
    over_budget: measured.filter((t) => t.over_budget).map((t) => ({ cell_id: t.cell_id, round: t.round, attempt: t.attempt, wall_s: round(t.wall_s, 2) })),
    reruns_after_hard_gate: measured.filter((t) => t.attempt === 2).length,
  },
  noise_floor: noiseFloor,
  label_checks: labelChecks,
  cells,
};
delete summary.expected_cells;
writeFileSync(join(dir, 'summary.json'), JSON.stringify(summary, null, 2) + '\n');

// ---- CSV ------------------------------------------------------------------------------
const cols = [
  ['cell_id', (c) => c.cell_id],
  ['scenario', (c) => c.scenario],
  ['impl', (c) => c.impl],
  ['G', (c) => c.G],
  ['P', (c) => c.P],
  ['W_ns', (c) => c.W_nominal_ns],
  ['read', (c) => c.read_ratio ?? ''],
  ['reps', (c) => c.reps],
  ['ops_per_sec', (c) => c.throughput.ops_per_sec],
  ['ops_min', (c) => c.throughput.min],
  ['ops_max', (c) => c.throughput.max],
  ['ops_spread', (c) => c.throughput.spread],
  ['ops_best', (c) => c.throughput.best_ops_per_sec],
  ['cpu_per_op_ns', (c) => c.throughput.cpu_per_op_ns],
  ['jain', (c) => c.fairness.jain],
  ['min_max', (c) => c.fairness.min_max],
  ['zero_op', (c) => c.fairness.zero_op_goroutines],
  ['quantum_bound', (c) => c.fairness.quantum_bound],
  ['slow_share', (c) => c.fairness.slow_acquire_share],
  ['fairness_source', (c) => c.fairness.fairness_source],
  ['op_p50', (c) => c.latency.op_p50.value],
  ['op_p99', (c) => c.latency.op_p99.value],
  ['op_p999', (c) => c.latency.op_p999.value],
  ['acquire_p99', (c) => c.latency.acquire_p99.value],
  ['waits_over_1ms_est', (c) => c.latency.waits_over_1ms_est],
  ['starving_fraction', (c) => c.latency.starving_fraction],
  ['ops_ratio', (c) => c.latency.ops_ratio],
  ['drift_max', (c) => c.calib.drift_max],
  ['flags', (c) => c.flags.join(';')],
];
const esc = (v) => {
  const s = v === null || v === undefined ? '' : String(v);
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
};
const csv = [cols.map(([h]) => h).join(',')]
  .concat(cells.map((c) => cols.map(([, f]) => esc(f(c))).join(',')))
  .join('\n');
writeFileSync(join(dir, 'summary.csv'), csv + '\n');

console.log(
  `report: ${cells.length} cells, ${reps.length} valid reps, ${invalid.length} invalid, ` +
    `${gateFailed} cells failed a soft gate (${summary.gates.confidence_suggestion}), ` +
    `self-test ${JSON.stringify(summary.selftest.results)}, ` +
    `labels ${labelChecks.filter((l) => l.ok).length}/${labelChecks.length}`,
);
