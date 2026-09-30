// node --test bench/gates.test.mjs — the rerun trigger (gates.mjs) and its
// wiring in report.mjs. Runs offline; report.mjs writes only to a temp dir.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync, spawnSync } from 'node:child_process';
import { trimOutlier, rangeRatio, rerunTrigger, RERUN_BASIS } from './gates.mjs';

const TICK = 41;
const reps = (ops, p99 = ops.map(() => 10000), drift = ops.map(() => 0)) =>
  ops.map((o, i) => ({ round: i + 1, ops: o, p99: p99[i], drift: drift[i], tickNs: TICK }));
const pts = (xs) => xs.map((value, i) => ({ round: i + 1, value }));

test('trimOutlier drops the value farthest from the median, first on a tie', () => {
  assert.equal(trimOutlier(pts([10, 11, 50, 12, 9])).dropped.round, 3);
  assert.equal(trimOutlier(pts([10, 20, 10, 20, 15])).dropped.round, 1);
  assert.equal(trimOutlier(pts([10, 50])).dropped, null, 'below three values nothing is dropped');
  assert.deepEqual(trimOutlier(pts([10, null, 12, NaN])).kept.map((p) => p.value), [10, 12]);
});

test('rangeRatio', () => {
  assert.equal(rangeRatio([90, 100, 110]), 0.2);
  assert.equal(rangeRatio([0, 0]), null);
});

test('throughput: a single outlier repetition does not trigger a rerun', () => {
  const r = rerunTrigger(reps([100, 101, 102, 100, 150]));
  assert.deepEqual(r.gates, []);
  assert.equal(r.spreadTrim.throughput_dropped_round, 5);
  assert.equal(r.spreadTrim.throughput, 0.0199);
});

test('throughput: two outliers do trigger G4', () => {
  const r = rerunTrigger(reps([100, 120, 100, 130, 100]));
  assert.deepEqual(r.gates, ['G4']);
  assert.equal(r.spreadTrim.throughput_trigger, true);
});

test('throughput: the threshold is strict (> 0.10)', () => {
  assert.deepEqual(rerunTrigger(reps([100, 110, 100, 100, 300])).gates, []);
});

test('p99: over 0.50 after the trim triggers G4, 0.40 does not', () => {
  const flat = [100, 100, 100, 100, 100];
  const hi = rerunTrigger(reps(flat, [1000, 1600, 1000, 1700, 1000]));
  assert.deepEqual(hi.gates, ['G4']);
  assert.equal(hi.spreadTrim.p99_dropped_round, 4);
  assert.equal(hi.spreadTrim.p99, 0.6);
  assert.deepEqual(rerunTrigger(reps(flat, [1000, 1400, 1000, 1700, 1000])).gates, []);
});

test('p99: below G8 floor and within one tick are not judged', () => {
  const flat = [100, 100, 100, 100, 100];
  const low = rerunTrigger(reps(flat, [100, 160, 100, 90, 41]));
  assert.equal(low.spreadTrim.p99_skipped, 'below_resolution');
  assert.deepEqual(low.gates, []);
  const tick = rerunTrigger(reps(flat, [200, 241, 200, 241, 200]));
  assert.equal(tick.spreadTrim.p99_skipped, 'within_one_tick');
  assert.deepEqual(tick.gates, []);
  const floor = rerunTrigger(reps(flat, [164, 400, 164, 500, 164]));
  assert.equal(floor.spreadTrim.p99_skipped, null, 'a median exactly at the floor is judged');
  assert.deepEqual(floor.gates, ['G4']);
});

test('G5 is unchanged: two drifted repetitions trigger, one does not', () => {
  const flat = [100, 100, 100, 100, 100];
  assert.deepEqual(rerunTrigger(reps(flat, undefined, [0.06, 0.07, 0, 0, 0])).gates, ['G5']);
  assert.deepEqual(rerunTrigger(reps(flat, undefined, [0.06, 0, 0, 0, 0])).gates, []);
  assert.deepEqual(rerunTrigger(reps([100, 120, 100, 130, 100], undefined, [0.1, 0.1, 0, 0, 0])).gates, ['G4', 'G5']);
});

// report.mjs on a synthetic result directory: flags keep the design's
// thresholds, the rerun list follows gates.mjs, the method is stated.
test('report.mjs: flags unchanged, rerun list and method from the E rule', () => {
  const dir = mkdtempSync(join(tmpdir(), 'report-'));
  mkdirSync(join(dir, 'raw'));
  writeFileSync(join(dir, 'matrix.json'), JSON.stringify({ sets: { main: { rounds: 5 } } }));
  const cells = {
    // one outlier: flagged spread_throughput, no rerun
    's1-mutex-g4-p1-w0': { P: 1, ops: [100, 101, 102, 100, 150] },
    // two outliers: flagged and rerun
    's1-mutex-g4-p2-w0': { P: 2, ops: [100, 120, 100, 130, 100] },
    // p99 0.40 after trim: flagged spread_p99 (> 0.25), no rerun
    's1-chan-g4-p2-w0': { P: 2, ops: [100, 100, 100, 100, 100], p99: [1000, 1400, 1000, 1700, 1000] },
  };
  for (const [id, c] of Object.entries(cells)) {
    c.ops.forEach((o, i) => {
      const [scenario, impl] = id.split('-');
      writeFileSync(join(dir, 'raw', `${id}.r${i + 1}.json`), JSON.stringify({
        cell_id: id, set: 'main', round: i + 1, scenario, impl, G: 4, P: c.P, W_nominal_ns: 0,
        gates: { valid: true }, env: { clock_resolution_ns: TICK }, calib: { drift: 0 },
        throughput: { ops_per_sec: o },
        latency: { op_latency_hist: { p99: c.p99?.[i] ?? 10000 } },
      }));
    });
  }
  const report = join(dirname(fileURLToPath(import.meta.url)), 'report.mjs');
  execFileSync(process.execPath, [report, dir], { stdio: 'pipe' });
  const s = JSON.parse(readFileSync(join(dir, 'summary.json'), 'utf8'));
  const flags = Object.fromEntries(s.cells.map((c) => [c.cell_id, c.flags]));
  assert.deepEqual(flags['s1-mutex-g4-p1-w0'], ['spread_throughput']);
  assert.deepEqual(flags['s1-mutex-g4-p2-w0'], ['spread_throughput']);
  assert.deepEqual(flags['s1-chan-g4-p2-w0'], ['spread_p99']);
  assert.deepEqual(s.gates.rerun_cells, [{ cell_id: 's1-mutex-g4-p2-w0', gates: ['G4'] }]);
  assert.equal(s.gates.rerun_basis, RERUN_BASIS);
  assert.deepEqual(s.gates.rerun_by_p, { P2: 1 });
  assert.deepEqual(s.gates.rerun_v1, { basis: 'full_range_v1', cells: 3, G4: 3, G5: 0 });
  assert.match(s.method.rerun_trigger, /trimmed_range_v2/);
  assert.match(s.method.rerun_trigger, /0\.5/);
  assert.equal(s.thresholds.G4_p99_spread, 0.25, 'flag thresholds unchanged');
  assert.equal(s.rerun_of, null);
  const list = readFileSync(join(dir, 'rerun-cells.txt'), 'utf8').split('\n').filter((l) => l && !l.startsWith('#'));
  assert.deepEqual(list, ['s1-mutex-g4-p2-w0']);
  assert.equal(s.merged, undefined, 'no rerun: no merge');
  assert.equal(s.gates.spread_persistent, undefined);
  assert.equal(s.method.spread_persistent, undefined);
  assert.equal(existsSync(join(dir, 'merged')), false);
});

// A synthetic first pass plus a cell rerun under rerun/.
const REPORT = join(dirname(fileURLToPath(import.meta.url)), 'report.mjs');
function writeCell(rawDir, id, P, ops, p99) {
  const [scenario, impl] = id.split('-');
  ops.forEach((o, i) => {
    writeFileSync(join(rawDir, `${id}.r${i + 1}.json`), JSON.stringify({
      cell_id: id, set: 'main', round: i + 1, scenario, impl, G: 4, P, W_nominal_ns: 0,
      gates: { valid: true }, env: { clock_resolution_ns: TICK }, calib: { drift: 0 },
      throughput: { ops_per_sec: o },
      latency: { op_latency_hist: { p99: p99?.[i] ?? 10000 } },
    }));
  });
}
function timingLines(ids) {
  return ids.flatMap((id) => [1, 2, 3, 4, 5].map((round) => JSON.stringify({ cell_id: id, round, attempt: 1, wall_s: 1 })))
    .join('\n') + '\n';
}
const STILL = 's1-mutex-g4-p2-w0'; // candidate in both passes
const CLEAN = 's1-mutex-g4-p1-w0'; // candidate in the first pass only
const KEPT = 's1-chan-g4-p2-w0'; // not rerun, flagged by the full-range spread only
const G5ONLY = 's1-atomic-g4-p4-w0'; // candidate first, rerun trips only G5
function mergedFixture() {
  const dir = mkdtempSync(join(tmpdir(), 'report-merge-'));
  for (const d of ['raw', 'timing', 'selftest', 'rerun', 'rerun/raw', 'rerun/timing', 'rerun/selftest']) {
    mkdirSync(join(dir, d));
  }
  writeFileSync(join(dir, 'matrix.json'), JSON.stringify({ sets: { main: { rounds: 5 } } }));
  writeCell(join(dir, 'raw'), STILL, 2, [100, 120, 100, 130, 100]);
  writeCell(join(dir, 'raw'), CLEAN, 1, [100, 125, 100, 135, 100]);
  writeCell(join(dir, 'raw'), KEPT, 2, [100, 101, 102, 100, 150]);
  writeCell(join(dir, 'raw'), G5ONLY, 4, [100, 125, 100, 135, 100]);
  writeCell(join(dir, 'rerun/raw'), STILL, 2, [100, 115, 100, 125, 100]);
  writeCell(join(dir, 'rerun/raw'), CLEAN, 1, [100, 101, 102, 100, 100]);
  writeCell(join(dir, 'rerun/raw'), G5ONLY, 4, [100, 101, 102, 100, 100]);
  // G5 on the rerun: two drifted repetitions, no spread.
  for (const r of [1, 2]) {
    const f = join(dir, 'rerun/raw', `${G5ONLY}.r${r}.json`);
    const rec = JSON.parse(readFileSync(f, 'utf8'));
    rec.calib.drift = 0.08;
    writeFileSync(f, JSON.stringify(rec));
  }
  writeFileSync(join(dir, 'timing', 'r1-P1.jsonl'), timingLines([STILL, CLEAN, KEPT, G5ONLY]));
  writeFileSync(join(dir, 'rerun/timing', 'r1-P1.jsonl'), timingLines([STILL, CLEAN, G5ONLY]));
  writeFileSync(join(dir, 'selftest', 'r1-P2.json'), JSON.stringify({ result: 'pass' }));
  writeFileSync(join(dir, 'rerun/selftest', 'r1-P2.json'), JSON.stringify({ result: 'pass' }));
  writeFileSync(join(dir, 'rerun/cells.txt'), `# rerun candidates\n${STILL}\n${CLEAN}\n${G5ONLY}\n`);
  writeFileSync(join(dir, 'rerun/rerun_of.json'), JSON.stringify({ rerun_of: 'first', cells: [STILL, CLEAN, G5ONLY] }));
  return dir;
}

test('report.mjs: a cell rerun is merged into merged/, spread_persistent marks what stays', () => {
  const dir = mergedFixture();
  const out = execFileSync(process.execPath, [REPORT, dir], { encoding: 'utf8' });
  assert.match(out, /merged 3 rerun cells, 1 spread_persistent/);
  assert.equal(existsSync(join(dir, 'summary.json')), false, 'the first pass directory is not written');
  const s = JSON.parse(readFileSync(join(dir, 'merged', 'summary.json'), 'utf8'));
  const by = Object.fromEntries(s.cells.map((c) => [c.cell_id, c]));

  // The rerun replaces the cell entirely; the rest of the first pass stays.
  assert.equal(s.cells_measured, 4);
  assert.equal(s.valid_reps, 20);
  assert.deepEqual(s.merged && {
    replaced: s.merged.replaced_cells, first: s.merged.raw_files_from_first_pass, rerun: s.merged.raw_files_from_rerun,
  }, { replaced: 3, first: 5, rerun: 15 });
  assert.equal(by[STILL].source, 'rerun');
  assert.equal(by[KEPT].source, 'first_pass');
  assert.equal(by[STILL].throughput.max, 125, 'rerun values, not first-pass ones');
  assert.equal(s.rerun_of.rerun_of, 'first');
  assert.equal(s.timing.reps, 20, 'first-pass timing of rerun cells left out');
  assert.deepEqual(s.selftest.files.map((f) => f.file), ['first-r1-P2.json', 'rerun-r1-P2.json']);

  // spread_persistent: candidate in the first pass, G4 trigger still met on the rerun.
  assert.deepEqual(by[STILL].first_pass_rerun_gates, ['G4']);
  assert.deepEqual(by[STILL].rerun_gates, ['G4']);
  assert.deepEqual(by[STILL].flags, ['spread_throughput', 'spread_persistent']);
  assert.deepEqual(by[CLEAN].first_pass_rerun_gates, ['G4']);
  assert.deepEqual(by[CLEAN].flags, []);
  assert.deepEqual(by[G5ONLY].rerun_gates, ['G5']);
  assert.equal(by[G5ONLY].flags.includes('spread_persistent'), false, 'G5 alone is not a spread');
  assert.equal(by[KEPT].first_pass_rerun_gates, null);
  assert.deepEqual(by[KEPT].flags, ['spread_throughput'], 'not rerun: never persistent');
  assert.deepEqual(s.gates.spread_persistent, { count: 1, by_p: { P2: 1 }, cells: [STILL] });
  assert.equal(s.gates.flag_counts.spread_persistent, 1);
  assert.match(s.method.spread_persistent, /G4 part .* first-pass data/);
  assert.match(s.method.spread_persistent, /thresholds are unchanged/);
  assert.equal(s.thresholds.G4_throughput_spread, 0.10, 'flag thresholds unchanged');
  assert.equal(s.thresholds.G4_p99_spread, 0.25, 'flag thresholds unchanged');
  const list = readFileSync(join(dir, 'merged', 'rerun-cells.txt'), 'utf8').split('\n').filter((l) => l && !l.startsWith('#'));
  assert.deepEqual(list, [G5ONLY, STILL]);
  assert.match(readFileSync(join(dir, 'merged', 'summary.csv'), 'utf8'), /spread_throughput;spread_persistent/);
});

const P99ONLY = 's2-mutex-g4-p8-w0'; // p99 spread only, in both passes
const G5TOG4 = 's2-chan-g4-p8-w0'; // G5 in the first pass, G4 only on the rerun
const MISSING = 's2-atomic-g4-p8-w0'; // listed for the rerun, no rerun record
function setDrift(rawDir, id, rounds, drift) {
  for (const r of rounds) {
    const f = join(rawDir, `${id}.r${r}.json`);
    const rec = JSON.parse(readFileSync(f, 'utf8'));
    rec.calib.drift = drift;
    writeFileSync(f, JSON.stringify(rec));
  }
}

test('report.mjs: persistence needs G4 in both passes; a listed cell without a rerun keeps its first pass', () => {
  const dir = mergedFixture();
  const flat = [100, 101, 102, 100, 100];
  const p99 = [1000, 1600, 1000, 1700, 1000];
  writeCell(join(dir, 'raw'), P99ONLY, 8, flat, p99);
  writeCell(join(dir, 'rerun/raw'), P99ONLY, 8, flat, p99);
  writeCell(join(dir, 'raw'), G5TOG4, 8, flat);
  setDrift(join(dir, 'raw'), G5TOG4, [1, 2], 0.08);
  writeCell(join(dir, 'rerun/raw'), G5TOG4, 8, [100, 125, 100, 135, 100]);
  writeCell(join(dir, 'raw'), MISSING, 8, [100, 125, 100, 135, 100]);
  writeFileSync(join(dir, 'rerun/cells.txt'), [STILL, CLEAN, G5ONLY, P99ONLY, G5TOG4, MISSING].join('\n') + '\n');

  const run = spawnSync(process.execPath, [REPORT, dir], { encoding: 'utf8' });
  assert.equal(run.status, 0, run.stderr);
  assert.match(run.stdout, /merged 5 rerun cells, 2 spread_persistent/);
  assert.match(run.stderr, new RegExp(`1 listed rerun cells have no rerun record.*${MISSING}`));
  const s = JSON.parse(readFileSync(join(dir, 'merged', 'summary.json'), 'utf8'));
  const by = Object.fromEntries(s.cells.map((c) => [c.cell_id, c]));

  assert.equal(s.cells_measured, 7);
  assert.deepEqual({
    listed: s.merged.listed_cells, replaced: s.merged.replaced_cells, missing: s.merged.missing_rerun_cells,
    first: s.merged.raw_files_from_first_pass, rerun: s.merged.raw_files_from_rerun,
  }, { listed: 6, replaced: 5, missing: [MISSING], first: 10, rerun: 25 });

  // p99-only spread persists through the G4 p99 part of the trigger.
  assert.deepEqual(by[P99ONLY].first_pass_rerun_gates, ['G4']);
  assert.deepEqual(by[P99ONLY].rerun_gates, ['G4']);
  assert.ok(by[P99ONLY].flags.includes('spread_persistent'));
  assert.equal(by[P99ONLY].flags.includes('spread_throughput'), false);

  // G5 in the first pass, G4 only on the rerun: not a persisting spread.
  assert.deepEqual(by[G5TOG4].first_pass_rerun_gates, ['G5']);
  assert.deepEqual(by[G5TOG4].rerun_gates, ['G4']);
  assert.equal(by[G5TOG4].flags.includes('spread_persistent'), false);

  // Listed but never rerun: first-pass data, not counted as replaced.
  assert.equal(by[MISSING].source, 'first_pass');
  assert.equal(by[MISSING].first_pass_rerun_gates, null);
  assert.equal(by[MISSING].throughput.max, 135);
  assert.equal(by[MISSING].flags.includes('spread_persistent'), false);

  assert.deepEqual(s.gates.spread_persistent.cells, [P99ONLY, STILL].sort());
});

test('report.mjs --no-merge: the first pass alone, as before the rerun', () => {
  const dir = mergedFixture();
  execFileSync(process.execPath, [REPORT, '--no-merge', dir], { stdio: 'pipe' });
  assert.equal(existsSync(join(dir, 'merged')), false);
  const s = JSON.parse(readFileSync(join(dir, 'summary.json'), 'utf8'));
  assert.equal(s.cells_measured, 4);
  assert.equal(s.merged, undefined);
  assert.equal(s.gates.spread_persistent, undefined);
  assert.equal(s.rerun_of, null);
  assert.equal(s.timing.reps, 20);
  assert.deepEqual(s.selftest.files.map((f) => f.file), ['r1-P2.json']);
  assert.equal(s.cells.some((c) => 'source' in c || c.flags.includes('spread_persistent')), false);
  assert.deepEqual(s.gates.rerun_cells.map((c) => c.cell_id), [G5ONLY, CLEAN, STILL]);
});
