// Rerun trigger of the report (user decision 2026-09-29, design note
// go-mutex-channel-g4-esik.md, option E). Kept apart from report.mjs so the
// rule can be tested without a result directory.
//
// The G4 flags (spread_throughput > 0.10, spread_p99 > 0.25 over all valid
// repetitions) are not touched by this module: they still describe the cell.
// Only the decision "run this cell again" uses the rule below.

export const RERUN_BASIS = 'trimmed_range_v2';
export const RERUN_TPUT = 0.10;
export const RERUN_P99 = 0.50;
export const RERUN_DRIFT = 0.05; // G5, unchanged
export const RERUN_DRIFT_REPS = 2; // G5, unchanged
export const RERUN_RESOLUTION_FACTOR = 4; // G8's floor, in clock ticks

export const RERUN_RULE =
  'G4 v2 (trimmed_range_v2): drop the one repetition farthest from the median ' +
  '(only with 3 or more values), then rerun when (max-min)/median of the rest ' +
  `exceeds ${RERUN_TPUT} for throughput, or exceeds ${RERUN_P99} for op_latency p99; ` +
  'the p99 test applies only when the median p99 of all repetitions is at or above ' +
  `G8's floor (${RERUN_RESOLUTION_FACTOR} x clock resolution) and the trimmed p99 range is ` +
  `wider than one clock tick. G5 unchanged: rerun when ${RERUN_DRIFT_REPS} or more ` +
  `repetitions drifted more than ${RERUN_DRIFT}.`;

const finite = (x) => typeof x === 'number' && Number.isFinite(x);

export function median(xs) {
  const v = xs.filter(finite).sort((a, b) => a - b);
  if (v.length === 0) return null;
  const m = v.length >> 1;
  return v.length % 2 ? v[m] : (v[m - 1] + v[m]) / 2;
}

// rangeRatio is (max-min)/median, null without a nonzero median.
export function rangeRatio(xs) {
  const v = xs.filter(finite);
  const m = median(v);
  return m ? (Math.max(...v) - Math.min(...v)) / m : null;
}

// trimOutlier drops the single point whose value is farthest from the median
// of all values (the first one on a tie). Points are {round, value}; points
// without a finite value are ignored. Below three values nothing is dropped:
// trimming two values would leave no range to judge.
export function trimOutlier(points) {
  const pts = points.filter((p) => finite(p.value));
  if (pts.length < 3) return { kept: pts, dropped: null };
  const m = median(pts.map((p) => p.value));
  let k = 0;
  let d = -1;
  pts.forEach((p, i) => {
    const e = Math.abs(p.value - m);
    if (e > d) {
      d = e;
      k = i;
    }
  });
  return { kept: pts.filter((_, i) => i !== k), dropped: pts[k] };
}

// rerunTrigger evaluates one cell. reps: [{round, ops, p99, drift, tickNs}].
// Returns the gates that ask for a rerun and the per-cell trimmed spreads.
export function rerunTrigger(reps) {
  const gates = [];

  const t = trimOutlier(reps.map((r) => ({ round: r.round, value: r.ops })));
  const tSpread = t.kept.length >= 2 ? rangeRatio(t.kept.map((p) => p.value)) : null;
  if (tSpread !== null && tSpread > RERUN_TPUT) gates.push('G4');

  const p99Points = reps.map((r) => ({ round: r.round, value: r.p99 }));
  const p99All = p99Points.filter((p) => finite(p.value)).map((p) => p.value);
  const p = trimOutlier(p99Points);
  const pVals = p.kept.map((x) => x.value);
  const tick = median(reps.map((r) => r.tickNs)) ?? 0;
  const floor = RERUN_RESOLUTION_FACTOR * tick;
  let pSpread = null;
  let pSkip = null;
  if (pVals.length < 2) pSkip = 'too_few';
  else if (median(p99All) < floor) pSkip = 'below_resolution';
  else if (Math.max(...pVals) - Math.min(...pVals) <= tick) pSkip = 'within_one_tick';
  else pSpread = rangeRatio(pVals);
  if (pSpread !== null && pSpread > RERUN_P99 && !gates.includes('G4')) gates.push('G4');

  const drifted = reps.filter((r) => (r.drift ?? 0) > RERUN_DRIFT).length;
  if (drifted >= RERUN_DRIFT_REPS) gates.push('G5');

  const r4 = (x) => (x === null ? null : Number(x.toFixed(4)));
  return {
    gates,
    spreadTrim: {
      throughput: r4(tSpread),
      throughput_dropped_round: t.dropped?.round ?? null,
      throughput_trigger: tSpread !== null && tSpread > RERUN_TPUT,
      p99: r4(pSpread),
      p99_dropped_round: p.dropped?.round ?? null,
      p99_skipped: pSkip,
      p99_trigger: pSpread !== null && pSpread > RERUN_P99,
    },
  };
}
