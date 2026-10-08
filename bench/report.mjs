#!/usr/bin/env node
// Raw repetition JSON -> summary.json + summary.csv, with the soft gates of the
// design evaluated per cell. Raw files are never modified; the report can be
// re-run at any time.
//
//   node bench/report.mjs [--no-merge] [--headline FILE] [--second-run PATH] results/<stamp>
//
// With a cell rerun in results/<stamp>/rerun/, the report reads the first pass
// merged with the rerun and writes to results/<stamp>/merged/ (see "inputs"
// below); without one, or with --no-merge, it summarises results/<stamp> in
// place.
//
// Estimator: the median of the valid repetitions of a cell for every metric;
// latency percentiles are per repetition first, then the median of the
// percentiles. The best throughput repetition is kept as a separate field.
//
// Rerun trigger: the G4 flags keep the design's thresholds; which cells run
// again is decided by the rule of gates.mjs (G4 v2, trimmed range) plus G5.
// The candidates are written to rerun-cells.txt, the input of
// `./bench/run.sh --cells`. summary.method states all of it in words.
//
// Confidence (ADR-0050, class T): `low` when more than 15 % of the cells failed
// a soft gate, when any cell has fewer than 5 valid repetitions, or when a run
// directory has no environment record (environment.json or RUN.md); `high` only when every condition below holds, `medium`
// otherwise. The conditions, and which of them are missing, are written to
// summary.gates.confidence_conditions / confidence_missing:
//   repetitions   every cell has at least 5 valid repetitions
//   flagged_share at most 5 % of the cells failed a soft gate, and no headline
//                 cell did
//   environment   environment.json (linux-metal profile, run.sh) passed
//                 host-check at the start and at the end, and every record
//                 under host-check/ passed; for a merged report, the rerun's
//                 as well
//   noise_floor   every headline comparison (--headline, a line
//                 "<cellA> <cellB> <ops|p99>") differs in that metric by at
//                 least 3x the repetition spread of the same metric in the
//                 noise floor cell (s1 atomic, W=0, 2 <= G <= P) at its P
//   second_run    --second-run PATH (summary.json or result directory of an
//                 independent run: another day, another boot) repeats the
//                 headline numbers within ±5 % (bench/compare-runs.mjs)
// Open data (condition 6) is a publishing step and is not checked here.
import { readFileSync, readdirSync, writeFileSync, existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { compareSummaries, readHeadline, readSummary, SECOND_RUN_TOLERANCE } from './compare-runs.mjs';
import {
  RERUN_BASIS, RERUN_TPUT, RERUN_P99, RERUN_DRIFT_REPS, RERUN_RESOLUTION_FACTOR, RERUN_RULE, rerunTrigger,
} from './gates.mjs';

const usage = () => {
  console.error('usage: node bench/report.mjs [--no-merge] [--headline FILE] [--second-run PATH] <results-dir>');
  process.exit(2);
};
const args = process.argv.slice(2);
let noMerge = false;
let headlineFile = null;
let secondRunPath = null;
let dir = null;
for (let i = 0; i < args.length; i++) {
  const a = args[i];
  if (a === '--no-merge') noMerge = true;
  else if (a === '--headline' || a === '--second-run') {
    if (i + 1 >= args.length) usage();
    if (a === '--headline') headlineFile = args[++i];
    else secondRunPath = args[++i];
  } else if (a.startsWith('--')) usage();
  else if (dir === null) dir = a;
  else usage();
}
if (!dir) usage();

// Thresholds of the design (gates G4-G9 and the fairness attribution).
const G4_TPUT = 0.10;
const G4_P99 = 0.25;
const G5_DRIFT = 0.05;
const G6_RATIO = 0.90;
const G7_SAMPLES = 10000;
const G8_FACTOR = 4;
const G9_TAIL = 0.01;
const SLOW_SHARE = 0.5;
const REP_BUDGET_S = 5;
const LOW_CONFIDENCE_SHARE = 0.15;
// The `high` branch (ADR-0050, class T).
const HIGH_FLAGGED_SHARE = 0.05;
const HIGH_MIN_REPS = 5;
const HIGH_NOISE_FACTOR = 3;

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
// Quantile of the valid values, linear interpolation between order statistics
// (type 7, the default of R and NumPy).
const quantile = (xs, q) => {
  const v = nums(xs).sort((a, b) => a - b);
  if (v.length === 0) return null;
  const h = (v.length - 1) * q;
  const lo = Math.floor(h);
  return v[lo] + (h - lo) * ((v[Math.min(lo + 1, v.length - 1)]) - v[lo]);
};
// Median and interquartile range; iqr_rel is the IQR over the median.
const iqrOf = (xs) => {
  const m = median(xs);
  const q1 = quantile(xs, 0.25);
  const q3 = quantile(xs, 0.75);
  return { median: m, q1, q3, iqr: q1 === null ? null : q3 - q1, iqr_rel: m ? (q3 - q1) / m : null };
};
const round = (x, d = 4) => (x === null || x === undefined ? null : Number(x.toFixed(d)));
const get = (o, path) => path.split('.').reduce((a, k) => (a == null ? a : a[k]), o);

// ---- inputs: first pass, or first pass merged with a cell rerun ---------------
// A cell rerun (run.sh --cells) lives in <dir>/rerun/. When it is there, the
// report reads the merged set: every rerun cell is replaced entirely by its
// rerun (raw, timing), the rest of the first pass is taken as is, and the
// output goes to <dir>/merged/. Nothing under <dir> or <dir>/rerun is written.
// A listed cell with no rerun record at all (an interrupted rerun) is not
// replaced: it keeps its first pass and is named in summary.merged and on
// stderr. `--no-merge` keeps the first pass on its own.
const rerunDir = join(dir, 'rerun');
const merge = !noMerge && existsSync(join(rerunDir, 'raw')) && existsSync(join(rerunDir, 'cells.txt'));
const outDir = merge ? join(dir, 'merged') : dir;
const fileCell = (f) => f.split('.')[0];
const listedIds = merge
  ? [...new Set(readFileSync(join(rerunDir, 'cells.txt'), 'utf8').split('\n').map((l) => l.trim()).filter((l) => l && !l.startsWith('#')))]
  : [];
const rerunRawIds = merge
  ? new Set(readdirSync(join(rerunDir, 'raw')).filter((f) => f.endsWith('.json')).map(fileCell))
  : new Set();
// The cells actually replaced: listed and present in rerun/raw.
const rerunIds = new Set(listedIds.filter((id) => rerunRawIds.has(id)));
const missingRerun = listedIds.filter((id) => !rerunRawIds.has(id)).sort();
if (missingRerun.length) {
  console.error(`report: ${missingRerun.length} listed rerun cells have no rerun record, first pass kept: ${missingRerun.join(' ')}`);
}

// ---- raw files ---------------------------------------------------------------
const reps = [];
const invalid = [];
const firstPassReps = new Map(); // rerun cell -> its valid first-pass repetitions
const rawCount = { first_pass: 0, rerun: 0 };
function readRaw(rawDir, from) {
  const files = existsSync(rawDir) ? readdirSync(rawDir).sort() : [];
  for (const f of files) {
    if (!f.endsWith('.json')) continue;
    const replaced = from === 'first_pass' && rerunIds.has(fileCell(f));
    const rec = readJSON(join(rawDir, f));
    if (replaced) {
      if (!/\.invalid(-\d+)?\.json$/.test(f) && rec.gates?.valid) {
        if (!firstPassReps.has(rec.cell_id)) firstPassReps.set(rec.cell_id, []);
        firstPassReps.get(rec.cell_id).push(rec);
      }
      continue;
    }
    rawCount[from]++;
    if (/\.invalid(-\d+)?\.json$/.test(f)) {
      // A placeholder (`missing`) stands for an attempt that left no record.
      invalid.push({ file: f, cell_id: rec.cell_id, round: rec.round, gates: rec.gates, missing: rec.missing ?? null });
      continue;
    }
    if (!rec.gates?.valid) {
      invalid.push({ file: f, cell_id: rec.cell_id, round: rec.round, gates: rec.gates });
      continue;
    }
    reps.push(rec);
  }
}
readRaw(join(dir, 'raw'), 'first_pass');
if (merge) readRaw(join(rerunDir, 'raw'), 'rerun');

// ---- side files: timing and self-test -----------------------------------------
const timing = [];
function readTiming(base, skipRerun) {
  if (!existsSync(join(base, 'timing'))) return;
  for (const f of readdirSync(join(base, 'timing')).sort()) {
    for (const line of readFileSync(join(base, 'timing', f), 'utf8').split('\n')) {
      if (!line.trim()) continue;
      const t = JSON.parse(line);
      if (skipRerun && rerunIds.has(t.cell_id)) continue;
      timing.push(t);
    }
  }
}
readTiming(dir, merge);
if (merge) readTiming(rerunDir, false);
const measured = timing.filter((t) => !t.skipped);
const selftests = [];
function readSelftests(base, prefix) {
  if (!existsSync(join(base, 'selftest'))) return;
  for (const f of readdirSync(join(base, 'selftest')).sort()) {
    const s = readJSON(join(base, 'selftest', f));
    selftests.push({ file: prefix + f, result: s.result, ...s });
  }
}
readSelftests(dir, merge ? 'first-' : '');
if (merge) readSelftests(rerunDir, 'rerun-');

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

// The attribution rule has no "scheduler" source (user decision 2026-09-27,
// Faz 02): a cell the primitive does not explain is "unexplained" and carries
// b, the expected quantum Jain and the measured Jain side by side.
function fairnessSource(g, p, b, s) {
  if (g <= p || b >= 1) return 'sync';
  if (s >= SLOW_SHARE) return 'sync';
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
  const qs = iqrOf(vals);
  const iqr = { q1: round(qs.q1, 1), q3: round(qs.q3, 1), iqr: round(qs.iqr, 1), iqr_rel: round(qs.iqr_rel) };
  if (m < floor) return { value: `< ${Math.round(floor)} ns`, below_resolution: true, spread: round(spread(vals)), ...iqr };
  return { value: Math.round(m), spread: round(spread(vals)), ...iqr, dropped_g7: dropped || undefined };
}

function triggerOf(rs) {
  return rerunTrigger(rs.map((r) => ({
    round: r.round,
    ops: r.throughput?.ops_per_sec,
    p99: r.latency?.op_latency_hist?.p99,
    drift: r.calib?.drift,
    tickNs: r.env?.clock_resolution_ns,
  })));
}

const cells = [];
for (const [id, rs] of [...byCell.entries()].sort()) {
  rs.sort((a, b) => a.round - b.round);
  const r0 = rs[0];
  const ops = rs.map((r) => r.throughput?.ops_per_sec);
  const flags = [];
  const rerunV1 = []; // the design's trigger (full range, G4 thresholds), kept for comparison

  // G4: spread of throughput and of op_latency p99.
  const tSpread = spread(ops);
  const tIqr = iqrOf(ops);
  const p99s = rs.map((r) => r.latency?.op_latency_hist?.p99);
  const pSpread = spread(p99s);
  if (rs.length >= 2 && tSpread !== null && tSpread > G4_TPUT) flags.push('spread_throughput');
  if (rs.length >= 2 && pSpread !== null && pSpread > G4_P99) flags.push('spread_p99');
  if (flags.some((f) => f.startsWith('spread'))) rerunV1.push('G4');

  // G5: calibration drift per repetition.
  const drifted = rs.filter((r) => (r.calib?.drift ?? 0) > G5_DRIFT).length;
  if (drifted > 0) flags.push('calib_drift');
  if (drifted >= 2) rerunV1.push('G5');

  // Rerun trigger (G4 v2 + G5, gates.mjs).
  const trig = triggerOf(rs);
  const rerun = trig.gates;

  // spread_persistent: a rerun cell that met the G4 part of the trigger in the
  // first pass and still meets it on its rerun.
  const firstPassGates = merge && rerunIds.has(id)
    ? triggerOf([...(firstPassReps.get(id) ?? [])].sort((a, b) => a.round - b.round)).gates
    : null;
  const persistent = firstPassGates !== null && firstPassGates.includes('G4') && rerun.includes('G4');
  if (persistent) flags.push('spread_persistent');

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
  const source = jain !== null && s !== null && b !== null ? fairnessSource(g, r0.P, b, s) : null;
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
      median: round(tIqr.median, 1),
      q1: round(tIqr.q1, 1),
      q3: round(tIqr.q3, 1),
      iqr: round(tIqr.iqr, 1),
      iqr_rel: round(tIqr.iqr_rel),
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
    spread_trim: trig.spreadTrim,
    rerun_gates: rerun,
    rerun_gates_v1: rerunV1,
  };
  if (merge) {
    cell.source = rerunIds.has(id) ? 'rerun' : 'first_pass';
    cell.first_pass_rerun_gates = firstPassGates;
  }
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
    const want = /unexplained/.test(c.note) ? 'unexplained' : 'sync';
    return {
      cell_id: c.cell_id,
      expected: want,
      got: c.fairness.fairness_source,
      ok: c.fairness.fairness_source === want,
      jain: c.fairness.jain,
      expected_quantum_jain: c.fairness.expected_quantum_jain,
      quantum_bound: c.fairness.quantum_bound,
      quantum_bound_raw: c.fairness.quantum_bound_raw,
      slow_acquire_share: c.fairness.slow_acquire_share,
    };
  });
const flaggedCells = cells.filter((c) => c.flagged).length;
// Soft gates that change what is published (missing reps are a run problem).
const gateFailed = cells.filter((c) => c.flags.some((f) => f !== 'missing_reps')).length;
const flagCounts = {};
for (const c of cells) for (const f of c.flags) flagCounts[f] = (flagCounts[f] ?? 0) + 1;
const walls = measured.map((t) => t.wall_s);
const rerunCells = cells.filter((c) => c.rerun_gates.length);
const v1Cells = cells.filter((c) => c.rerun_gates_v1.length);
const byP = (cs) => {
  const n = {};
  for (const c of [...cs].sort((a, b) => a.P - b.P)) n[`P${c.P}`] = (n[`P${c.P}`] ?? 0) + 1;
  return n;
};
// A cell rerun (run.sh --cells) leaves its link to the first pass here.
const rerunOfPath = join(merge ? rerunDir : dir, 'rerun_of.json');
const rerunOf = existsSync(rerunOfPath) ? readJSON(rerunOfPath) : null;
const persistentCells = cells.filter((c) => c.flags.includes('spread_persistent'));

// ---- environment (linux-metal: environment.json + host-check/) ------------------
// One run directory (the first pass, or the rerun): its start record, its end
// record and every host-check record of the run.
function readEnvironment(base, label) {
  const envPath = join(base, 'environment.json');
  if (!existsSync(envPath)) return { run: label, present: false, passed: false, problems: [`${label}: no environment.json`] };
  const env = readJSON(envPath);
  const problems = [];
  if (env.host_check?.passed !== true) problems.push(`${label}: host-check at the start failed (${(env.host_check?.failed ?? []).join(', ')})`);
  if (!env.end) problems.push(`${label}: no host-check record after the last round (environment.json has no .end)`);
  else if (env.end.host_check?.passed !== true) problems.push(`${label}: host-check after the last round failed (${(env.end.host_check?.failed ?? []).join(', ')})`);
  const records = [];
  const hcDir = join(base, 'host-check');
  if (existsSync(hcDir)) {
    for (const f of readdirSync(hcDir).filter((x) => x.endsWith('.json')).sort()) {
      let rec;
      try {
        rec = readJSON(join(hcDir, f));
      } catch {
        rec = null;
      }
      const passed = rec?.host_check?.passed === true;
      records.push({ file: f, phase: rec?.phase ?? null, passed });
      if (!passed) problems.push(`${label}: host-check/${f} failed or unreadable`);
    }
  }
  if (!records.length) problems.push(`${label}: no host-check/ records`);
  return {
    run: label,
    present: true,
    passed: problems.length === 0,
    profile: env.profile ?? null,
    checked_at: env.checked_at ?? null,
    boot_id: env.host?.boot_id ?? null,
    cpu_model: env.host?.cpu_model ?? null,
    kernel: env.host?.kernel ?? null,
    cmdline_params: env.host?.cmdline_params ?? null,
    temperature_start: env.temperature?.celsius ?? null,
    temperature_end: env.end?.temperature?.celsius ?? null,
    records: records.length,
    failed_records: records.filter((r) => !r.passed).map((r) => r.file),
    problems,
  };
}
const environments = [readEnvironment(dir, merge ? 'first pass' : 'run')];
if (merge) environments.push(readEnvironment(rerunDir, 'rerun'));
const environment = {
  passed: environments.every((e) => e.passed),
  runs: environments,
};

// ---- confidence (ADR-0050, class T) ----------------------------------------------
let headline = null;
if (headlineFile) {
  try {
    headline = readHeadline(headlineFile);
  } catch (e) {
    console.error(`report: ${e.message}`);
    process.exit(2);
  }
}
const cellById = new Map(cells.map((c) => [c.cell_id, c]));
const gateFailing = (c) => c.flags.some((f) => f !== 'missing_reps');
const conditions = {};

// repetitions: every cell >= 5 valid repetitions.
{
  const few = cells.filter((c) => c.reps < HIGH_MIN_REPS);
  conditions.repetitions = {
    ok: cells.length > 0 && few.length === 0,
    min_reps: HIGH_MIN_REPS,
    cells_below: few.length,
    detail: few.length ? `${few.length} cells have fewer than ${HIGH_MIN_REPS} valid repetitions` : `every cell has >= ${HIGH_MIN_REPS}`,
  };
}

// flagged_share: <= 5 % of the cells failed a soft gate; no headline cell did.
{
  const share = cells.length ? gateFailed / cells.length : null;
  const flaggedHeadline = headline ? headline.cells.filter((id) => cellById.has(id) && gateFailing(cellById.get(id))) : [];
  const ok = share !== null && share <= HIGH_FLAGGED_SHARE && flaggedHeadline.length === 0;
  conditions.flagged_share = {
    ok,
    value: share === null ? null : round(share),
    limit: HIGH_FLAGGED_SHARE,
    flagged_headline_cells: flaggedHeadline,
    detail: share === null
      ? 'no cells'
      : `${gateFailed}/${cells.length} cells (${(share * 100).toFixed(1)} %) failed a soft gate, limit ${HIGH_FLAGGED_SHARE * 100} %` +
        (flaggedHeadline.length ? `; headline cells flagged: ${flaggedHeadline.join(' ')}` : ''),
  };
}

// environment: host-check passed for every run directory of the report.
conditions.environment = {
  ok: environment.passed,
  detail: environment.passed
    ? 'host-check passed at the start, every round and the end'
    : environments.flatMap((e) => e.problems).join('; '),
};

// noise_floor: each headline comparison differs by >= 3x the noise spread of
// the metric it compares. A headline line names the metric (`ops` or `p99`);
// `ops` is set against the throughput spread of the noise floor cell, `p99`
// against the spread of its op_latency p99. Both spreads are the full range
// over the median ((max-min)/median, the G4 definition), which is stricter than
// the IQR. A p99 below the clock resolution ("< X ns") has no value to compare.
{
  const METRIC = {
    ops: { name: 'throughput', value: (c) => c.throughput.ops_per_sec, spread: (c) => c.throughput.spread },
    p99: { name: 'op_latency p99', value: (c) => c.latency.op_p99.value, spread: (c) => (c.latency.op_p99.below_resolution ? null : c.latency.op_p99.spread ?? null) },
  };
  const noiseCell = (p) => {
    const n = noiseFloor[`P${p}`];
    return n ? cellById.get(n.cell_id) ?? null : null;
  };
  const comparisons = [];
  const problems = [];
  if (!headline) problems.push('no headline given (--headline FILE)');
  else if (!headline.pairs.length) problems.push('the headline file has no comparison (a line "<cellA> <cellB> <ops|p99>")');
  for (const { a, b, metric, line } of headline?.pairs ?? []) {
    const ca = cellById.get(a);
    const cb = cellById.get(b);
    if (!metric) {
      comparisons.push({ a, b, metric: null, ok: false });
      problems.push(`line ${line}: ${a} vs ${b}: metric not given (ops|p99)`);
      continue;
    }
    if (!ca || !cb) {
      comparisons.push({ a, b, metric, ok: false });
      problems.push(`${a} vs ${b} (${metric}): cell not measured`);
      continue;
    }
    const m = METRIC[metric];
    const va = m.value(ca);
    const vb = m.value(cb);
    if (typeof va !== 'number' || typeof vb !== 'number') {
      comparisons.push({ a, b, metric, value_a: va ?? null, value_b: vb ?? null, ok: false });
      problems.push(`${a} vs ${b} (${metric}): ${m.name} not comparable (${va ?? 'none'} vs ${vb ?? 'none'})`);
      continue;
    }
    const diff = va ? Math.abs(vb - va) / va : null;
    const na = noiseCell(ca.P);
    const nb = noiseCell(cb.P);
    const spreads = [na ? m.spread(na) : null, nb ? m.spread(nb) : null];
    const noise = spreads.some((x) => x === null || x === undefined) ? null : Math.max(...spreads);
    const ok = diff !== null && noise !== null && diff >= HIGH_NOISE_FACTOR * noise;
    comparisons.push({ a, b, metric, value_a: va, value_b: vb, diff: round(diff), noise_spread: round(noise), needed: noise === null ? null : round(HIGH_NOISE_FACTOR * noise), ok });
    if (!na || !nb) problems.push(`${a} vs ${b} (${metric}): no noise floor cell (s1 atomic, W=0, 2 <= G <= P) at P${ca.P}/P${cb.P}`);
    else if (noise === null) problems.push(`${a} vs ${b} (${metric}): ${m.name} spread of the noise floor cell unknown (below clock resolution?)`);
    else if (diff === null) problems.push(`${a} vs ${b} (${metric}): ${m.name} of ${a} is 0`);
    else if (!ok) problems.push(`${a} vs ${b} (${metric}): difference ${(diff * 100).toFixed(1)} % < ${HIGH_NOISE_FACTOR} x noise ${m.name} spread ${(noise * 100).toFixed(1)} %`);
  }
  conditions.noise_floor = {
    ok: problems.length === 0,
    factor: HIGH_NOISE_FACTOR,
    comparisons,
    detail: problems.length ? problems.join('; ') : `${comparisons.length} headline comparisons clear ${HIGH_NOISE_FACTOR}x the noise spread of their metric`,
  };
}

// second_run: an independent run repeats the headline numbers within ±5 %.
let secondRun = null;
{
  const problems = [];
  if (!secondRunPath) problems.push('no second run given (--second-run PATH)');
  else {
    let other;
    try {
      other = readSummary(secondRunPath);
    } catch (e) {
      console.error(`report: ${e.message}`);
      process.exit(2);
    }
    const cmp = compareSummaries({ cells }, other, { cells: headline ? headline.cells : null });
    const otherEnv = other.environment?.runs?.[0] ?? null;
    const thisEnv = environments[0];
    const day = (t) => (t ? String(t).slice(0, 10) : null);
    secondRun = {
      path: secondRunPath,
      scope: cmp.scope,
      cells_compared: cmp.cells_compared,
      numbers_compared: cmp.numbers_compared,
      max_abs_diff_pct: cmp.max_abs_diff_pct,
      within: cmp.within,
      outside: cmp.outside,
      incomparable: cmp.incomparable,
      missing: cmp.missing.map((m) => m.cell_id),
      other_boot_id: otherEnv?.boot_id ?? null,
      other_checked_at: otherEnv?.checked_at ?? null,
    };
    if (!cmp.within) {
      problems.push(`second run not within ±${SECOND_RUN_TOLERANCE * 100} %: ${cmp.outside.length} cells outside, ` +
        `${cmp.incomparable.length} incomparable, ${cmp.missing.length} missing` +
        (cmp.numbers_compared ? `, max |diff| ${cmp.max_abs_diff_pct} %` : ', nothing compared'));
    }
    if (!thisEnv.boot_id || !otherEnv?.boot_id) problems.push('second run: boot ID of a run unknown (environment.json)');
    else if (thisEnv.boot_id === otherEnv.boot_id) problems.push('second run: same boot as this run (needs a rebooted host)');
    if (!day(thisEnv.checked_at) || !day(otherEnv?.checked_at)) problems.push('second run: date of a run unknown (environment.json)');
    else if (day(thisEnv.checked_at) === day(otherEnv.checked_at)) problems.push('second run: same day as this run (needs another day)');
    if (other.environment && other.environment.passed !== true) problems.push('second run: its own host-check did not pass');
    if (!other.environment) problems.push('second run: its summary has no environment record');
  }
  conditions.second_run = {
    ok: problems.length === 0,
    tolerance: SECOND_RUN_TOLERANCE,
    ...(secondRun ? { result: secondRun } : {}),
    detail: problems.length ? problems.join('; ') : `headline numbers within ±${SECOND_RUN_TOLERANCE * 100} % on another day and boot`,
  };
}

// low (ADR-0050, class T): more than 15 % of the cells flagged, fewer than 5
// valid repetitions in any cell (K2), or no environment record of a run
// directory. The record is environment.json (linux-metal) or RUN.md (written
// by run.sh for every profile); a directory with neither has no record at all.
const lowReasons = [];
if (cells.length > 0 && gateFailed / cells.length > LOW_CONFIDENCE_SHARE) {
  lowReasons.push(`${gateFailed}/${cells.length} cells failed a soft gate, more than ${LOW_CONFIDENCE_SHARE * 100} %`);
}
{
  const few = cells.filter((c) => c.reps < HIGH_MIN_REPS);
  if (few.length) lowReasons.push(`${few.length} cells have fewer than ${HIGH_MIN_REPS} valid repetitions`);
}
{
  const runDirs = [[dir, merge ? 'first pass' : 'run'], ...(merge ? [[rerunDir, 'rerun']] : [])];
  const bare = runDirs.filter(([d]) => !existsSync(join(d, 'environment.json')) && !existsSync(join(d, 'RUN.md')));
  if (bare.length) lowReasons.push(`no environment record (environment.json or RUN.md) for: ${bare.map(([, l]) => l).join(', ')}`);
}
const confidenceMissing = Object.entries(conditions).filter(([, c]) => !c.ok).map(([k, c]) => `${k}: ${c.detail}`);
const confidence = lowReasons.length ? 'low' : confidenceMissing.length === 0 ? 'high' : 'medium';
const confidenceReason = lowReasons.length
  ? `low: ${lowReasons.join('; ')}`
  : confidence === 'high'
    ? 'high: every condition of ADR-0050 class T checked by the tool holds (open data is a publishing step)'
    : `medium: high needs ${confidenceMissing.length} more condition(s)`;

const summary = {
  generated: new Date().toISOString(),
  dir,
  set: setName,
  expected_rounds: expectedRounds,
  expected_cells: matrix ? null : undefined,
  cells_measured: cells.length,
  valid_reps: reps.length,
  invalid_reps: invalid,
  rerun_of: rerunOf,
  merged: merge
    ? {
      rule: 'a rerun replaces the cell entirely; first-pass files of rerun cells are left out, the rest of ' +
        'the first pass is taken as is',
      first_pass_raw: '../raw',
      rerun_raw: '../rerun/raw',
      listed_cells: listedIds.length,
      replaced_cells: rerunIds.size,
      missing_rerun_cells: missingRerun,
      raw_files_from_first_pass: rawCount.first_pass,
      raw_files_from_rerun: rawCount.rerun,
    }
    : undefined,
  method: {
    estimator: 'per metric, the median of the valid repetitions of a cell; latency percentiles are taken per ' +
      'repetition first, then the median; below G8\'s floor a percentile is reported as "< X ns"; the best ' +
      'throughput repetition is kept as a separate field',
    flags: `G4 flags use all valid repetitions: spread_throughput when (max-min)/median > ${G4_TPUT}, ` +
      `spread_p99 when the same ratio of op_latency p99 > ${G4_P99}; calib_drift when any repetition ` +
      `drifted more than ${G5_DRIFT}. Flags do not decide reruns.`,
    rerun_trigger: RERUN_RULE,
    rerun_basis: RERUN_BASIS,
    confidence: `low when more than ${LOW_CONFIDENCE_SHARE * 100} % of the cells failed a soft gate (any flag but ` +
      `missing_reps), when any cell has fewer than ${HIGH_MIN_REPS} valid repetitions, or when a run directory has ` +
      `no environment record (environment.json or RUN.md); high when all of: every cell >= ${HIGH_MIN_REPS} valid repetitions; <= ${HIGH_FLAGGED_SHARE * 100} % ` +
      'of the cells failed a soft gate and no headline cell did; environment.json and every host-check/ record ' +
      `passed (linux-metal profile); each headline comparison differs in its metric (throughput or op_latency p99, ` +
      `named on the headline line) by >= ${HIGH_NOISE_FACTOR}x the repetition spread ((max-min)/median) of the same ` +
      'metric in the noise floor cell at its P; a second run on another day and ' +
      `boot repeats the headline numbers within ±${SECOND_RUN_TOLERANCE * 100} % (compare-runs.mjs); medium ` +
      'otherwise. Open data (ADR-0050 condition 6) is not checked by the tool.',
    spread_persistent: merge
      ? 'spread_persistent marks a rerun cell (listed in rerun/cells.txt and replaced by its rerun) that met ' +
        'the G4 part (trimmed range) of the rerun trigger on the first-pass data and still meets it on the ' +
        'rerun data alone. It is added to the cell\'s flags; the G4 flag thresholds are unchanged. A rerun ' +
        'cell that stays flagged only by the spread_* flags, that meets G5 only, or whose G4 appears only in ' +
        'the rerun, is not marked.'
      : undefined,
  },
  thresholds: {
    G4_throughput_spread: G4_TPUT, G4_p99_spread: G4_P99, G5_drift: G5_DRIFT, G6_ops_ratio: G6_RATIO,
    G7_min_samples: G7_SAMPLES, G8_resolution_factor: G8_FACTOR, G9_window_tail: G9_TAIL,
    slow_share: SLOW_SHARE, park_threshold_us: 10,
    rerun_throughput_trimmed_range: RERUN_TPUT, rerun_p99_trimmed_range: RERUN_P99,
    rerun_p99_resolution_factor: RERUN_RESOLUTION_FACTOR, rerun_drift_reps: RERUN_DRIFT_REPS,
  },
  gates: {
    flagged_cells: flaggedCells,
    gate_failed_cells: gateFailed,
    gate_failed_share: cells.length ? round(gateFailed / cells.length) : null,
    flag_counts: flagCounts,
    confidence_suggestion: confidence,
    confidence_reason: confidenceReason,
    confidence_missing: confidenceMissing,
    confidence_conditions: conditions,
    rerun_basis: RERUN_BASIS,
    rerun_count: rerunCells.length,
    rerun_by_p: byP(rerunCells),
    rerun_cells: rerunCells.map((c) => ({ cell_id: c.cell_id, gates: c.rerun_gates })),
    rerun_v1: {
      basis: 'full_range_v1',
      cells: v1Cells.length,
      G4: v1Cells.filter((c) => c.rerun_gates_v1.includes('G4')).length,
      G5: v1Cells.filter((c) => c.rerun_gates_v1.includes('G5')).length,
    },
    spread_persistent: merge
      ? {
        count: persistentCells.length,
        by_p: byP(persistentCells),
        cells: persistentCells.map((c) => c.cell_id),
      }
      : undefined,
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
  environment,
  headline: headline ? { file: headlineFile, ...headline } : null,
  label_checks: labelChecks,
  cells,
};
delete summary.expected_cells;
mkdirSync(outDir, { recursive: true });
writeFileSync(join(outDir, 'summary.json'), JSON.stringify(summary, null, 2) + '\n');

// The rerun candidates, the input of `./bench/run.sh --cells`. Deterministic
// (no timestamp): rewriting it from the same raw data yields the same file.
writeFileSync(
  join(outDir, 'rerun-cells.txt'),
  [
    `# rerun candidates, ${RERUN_BASIS} + G5 (see summary.json method.rerun_trigger)`,
    `# ${rerunCells.length} cells: ${Object.entries(byP(rerunCells)).map(([k, v]) => `${k}=${v}`).join(' ')}`,
    ...rerunCells.map((c) => c.cell_id),
  ].join('\n') + '\n',
);

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
  ['quantum_bound_raw', (c) => c.fairness.quantum_bound_raw],
  ['expected_quantum_jain', (c) => c.fairness.expected_quantum_jain],
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
  // added with the high branch; appended so existing column positions hold
  ['ops_median', (c) => c.throughput.median],
  ['ops_q1', (c) => c.throughput.q1],
  ['ops_q3', (c) => c.throughput.q3],
  ['ops_iqr', (c) => c.throughput.iqr],
  ['op_p99_iqr', (c) => c.latency.op_p99.iqr ?? ''],
];
const esc = (v) => {
  const s = v === null || v === undefined ? '' : String(v);
  return /[",\n]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
};
const csv = [cols.map(([h]) => h).join(',')]
  .concat(cells.map((c) => cols.map(([, f]) => esc(f(c))).join(',')))
  .join('\n');
writeFileSync(join(outDir, 'summary.csv'), csv + '\n');

console.log(
  `report: ${cells.length} cells, ${reps.length} valid reps, ${invalid.length} invalid, ` +
    `${gateFailed} cells failed a soft gate (${summary.gates.confidence_suggestion}), ` +
    `${rerunCells.length} rerun candidates (${RERUN_BASIS}; v1 ${v1Cells.length}), ` +
    `self-test ${JSON.stringify(summary.selftest.results)}, ` +
    `labels ${labelChecks.filter((l) => l.ok).length}/${labelChecks.length}` +
    (merge ? `, merged ${rerunIds.size} rerun cells, ${persistentCells.length} spread_persistent -> ${outDir}` : ''),
);
console.log(`report: confidence ${confidenceReason}`);
for (const m of confidenceMissing) console.log(`report:   missing for high: ${m}`);
