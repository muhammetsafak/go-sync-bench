#!/usr/bin/env node
// Two runs, cell by cell: the difference of the headline numbers of a second,
// independent run (another day, a rebooted host) against the first, and the
// verdict of the reproduction rule (ADR-0050, class T, condition 5): every
// compared number within ±5 %.
//
//   node bench/compare-runs.mjs [--headline FILE] [--tolerance 0.05] [--json OUT] <A> <B>
//
// A and B are summary.json files or result directories (a directory with a
// merged/ report is read from there). The numbers compared per cell:
// throughput.ops_per_sec and latency.op_p99 (both medians of the repetitions).
// Difference: (B - A) / A, in percent. A number that cannot be compared (missing
// in a run, A = 0, or a p99 below the clock resolution, "< X ns", in only one
// run) is "incomparable" and fails its cell; a p99 below the resolution in both
// runs counts as the same.
//
// --headline FILE: the headline cells, one per line; a line
//   <cellA> <cellB> <ops|p99>
// is a headline comparison of that metric (report.mjs checks its difference
// against the noise floor); `#` starts a comment. Without it every cell
// present in A is compared.
//
// Exit status: 0 every compared number within the tolerance, 1 not, 2 usage or
// input error. report.mjs imports compareSummaries and readHeadline.
import { readFileSync, existsSync, statSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

export const SECOND_RUN_TOLERANCE = 0.05;

export const METRICS = [
  ['ops_per_sec', (c) => c.throughput?.ops_per_sec],
  ['op_p99', (c) => c.latency?.op_p99?.value],
];

// The metrics a headline comparison can name: ops = throughput.ops_per_sec,
// p99 = latency.op_p99.
export const HEADLINE_METRICS = ['ops', 'p99'];

// readHeadline(path) -> { cells: [...ids], pairs: [{ a, b, metric, line }, ...] }
// A comparison line without a metric (two cell IDs only) is kept with
// metric: null; report.mjs counts it as a missing noise floor condition.
export function readHeadline(path) {
  const cells = [];
  const pairs = [];
  const lines = readFileSync(path, 'utf8').split('\n');
  lines.forEach((raw, i) => {
    const line = raw.replace(/#.*/, '').trim();
    if (!line) return;
    const tokens = line.split(/\s+/);
    if (tokens.length > 3) throw new Error(`${path}:${i + 1}: one cell ID, or "<cellA> <cellB> <${HEADLINE_METRICS.join('|')}>" for a comparison`);
    if (tokens.length === 3 && !HEADLINE_METRICS.includes(tokens[2])) {
      throw new Error(`${path}:${i + 1}: metric "${tokens[2]}" is not one of ${HEADLINE_METRICS.join(', ')}`);
    }
    const ids = tokens.slice(0, 2);
    for (const id of ids) if (!cells.includes(id)) cells.push(id);
    if (ids.length === 2) pairs.push({ a: ids[0], b: ids[1], metric: tokens[2] ?? null, line: i + 1 });
  });
  return { cells, pairs };
}

// summaryPath(p): a summary.json, or the report of a result directory.
export function summaryPath(p) {
  if (existsSync(p) && statSync(p).isDirectory()) {
    for (const c of [join(p, 'merged', 'summary.json'), join(p, 'summary.json')]) if (existsSync(c)) return c;
    throw new Error(`${p}: no summary.json (run bench/report.mjs first)`);
  }
  if (!existsSync(p)) throw new Error(`${p}: not found`);
  return p;
}

export const readSummary = (p) => JSON.parse(readFileSync(summaryPath(p), 'utf8'));

const belowResolution = (v) => typeof v === 'string' && v.startsWith('<');

const diffOf = (a, b) => {
  if (typeof a !== 'number' || typeof b !== 'number' || !Number.isFinite(a) || !Number.isFinite(b)) return null;
  if (a === b) return 0;
  return a === 0 ? null : (b - a) / a;
};

// compareSummaries(a, b, {cells, tolerance}) -> { within, compared, cells, missing, ... }
// a and b are parsed summary.json objects; cells limits the comparison to the
// listed cell IDs (null: every cell of a).
export function compareSummaries(a, b, { cells = null, tolerance = SECOND_RUN_TOLERANCE } = {}) {
  const ia = new Map((a.cells ?? []).map((c) => [c.cell_id, c]));
  const ib = new Map((b.cells ?? []).map((c) => [c.cell_id, c]));
  const ids = cells ?? [...ia.keys()].sort();
  const rows = [];
  const missing = [];
  for (const id of ids) {
    const ca = ia.get(id);
    const cb = ib.get(id);
    if (!ca || !cb) {
      missing.push({ cell_id: id, in_a: Boolean(ca), in_b: Boolean(cb) });
      continue;
    }
    const metrics = {};
    let within = true;
    for (const [name, pick] of METRICS) {
      const va = pick(ca);
      const vb = pick(cb);
      const d = diffOf(va, vb);
      if (d !== null) {
        const ok = Math.abs(d) <= tolerance + 1e-12;
        if (!ok) within = false;
        metrics[name] = { a: va, b: vb, diff_pct: Number((d * 100).toFixed(3)), within: ok };
      } else if (belowResolution(va) && belowResolution(vb)) {
        // both below the clock resolution: no number to compare, the same side of the floor
        metrics[name] = { a: va, b: vb, diff_pct: null, within: true, same: 'below the clock resolution in both runs' };
      } else {
        within = false;
        metrics[name] = {
          a: va ?? null,
          b: vb ?? null,
          diff_pct: null,
          within: false,
          incomparable: 'not a number in both runs (missing, zero in A, or below the clock resolution in one run only)',
        };
      }
    }
    rows.push({ cell_id: id, within, metrics });
  }
  const compared = rows.reduce((n, r) => n + Object.values(r.metrics).filter((m) => m.diff_pct !== null || m.same).length, 0);
  const incomparable = rows.filter((r) => Object.values(r.metrics).some((m) => m.incomparable)).map((r) => r.cell_id);
  const outside = rows.filter((r) => !r.within).map((r) => r.cell_id);
  const absDiffs = rows.flatMap((r) => Object.values(r.metrics).map((m) => m.diff_pct)).filter((d) => d !== null).map(Math.abs);
  return {
    tolerance,
    rule: `every compared number of B within ±${tolerance * 100} % of A ((B - A) / A); a listed cell missing from either run, ` +
      'or a number that cannot be compared (missing, zero in A, below the clock resolution in one run only), fails',
    scope: cells ? 'headline' : 'all_cells',
    cells_compared: rows.length,
    numbers_compared: compared,
    max_abs_diff_pct: absDiffs.length ? Math.max(...absDiffs) : null,
    within: rows.length > 0 && compared > 0 && outside.length === 0 && missing.length === 0,
    outside,
    incomparable,
    missing,
    cells: rows,
  };
}

function main(argv) {
  const usage = () => {
    console.error('usage: node bench/compare-runs.mjs [--headline FILE] [--tolerance 0.05] [--json OUT] <A> <B>');
    process.exit(2);
  };
  let headline = null;
  let tolerance = SECOND_RUN_TOLERANCE;
  let jsonOut = null;
  const pos = [];
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--headline' || a === '--tolerance' || a === '--json') {
      if (i + 1 >= argv.length) usage();
      const v = argv[++i];
      if (a === '--headline') headline = v;
      else if (a === '--json') jsonOut = v;
      else {
        tolerance = Number(v);
        if (!(tolerance > 0 && tolerance < 1)) usage();
      }
    } else if (a.startsWith('--')) usage();
    else pos.push(a);
  }
  if (pos.length !== 2) usage();
  let res;
  try {
    const a = readSummary(pos[0]);
    const b = readSummary(pos[1]);
    const cells = headline ? readHeadline(headline).cells : null;
    res = { a: summaryPath(pos[0]), b: summaryPath(pos[1]), headline, ...compareSummaries(a, b, { cells, tolerance }) };
  } catch (e) {
    console.error(`compare-runs: ${e.message}`);
    process.exit(2);
  }
  const fmt = (m) => (m.diff_pct !== null ? `${m.diff_pct >= 0 ? '+' : ''}${m.diff_pct.toFixed(2)}%` : m.same ? 'same (< res)' : 'INCOMPARABLE');
  for (const r of res.cells) {
    const parts = METRICS.map(([n]) => `${n} ${fmt(r.metrics[n])}`).join('  ');
    console.log(`${r.within ? 'ok     ' : 'OUTSIDE'} ${r.cell_id}  ${parts}`);
  }
  for (const m of res.missing) console.log(`MISSING ${m.cell_id} (in A: ${m.in_a}, in B: ${m.in_b})`);
  console.log(
    `compare-runs: ${res.cells_compared} cells, ${res.numbers_compared} numbers, max |diff| ` +
      `${res.max_abs_diff_pct === null ? 'n/a' : res.max_abs_diff_pct.toFixed(2) + '%'}, ` +
      `${res.outside.length} outside ±${tolerance * 100}% (${res.incomparable.length} incomparable), ${res.missing.length} missing -> ` +
      `${res.within ? 'within' : 'NOT within'}`,
  );
  if (jsonOut) writeFileSync(jsonOut, JSON.stringify(res, null, 2) + '\n');
  process.exit(res.within ? 0 : 1);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) main(process.argv.slice(2));
