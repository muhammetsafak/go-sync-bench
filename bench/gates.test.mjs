// node --test bench/gates.test.mjs — the rerun trigger (gates.mjs) and its
// wiring in report.mjs. Runs offline; report.mjs writes only to a temp dir.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync } from 'node:child_process';
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
});
