import exec from "k6/execution";

// Load that ramps from 0 up to `rate` iterations per second, then holds it.
//
// Starting at full rate opens a connection per VU all at once. Against the
// cluster that meant ~1000 TLS handshakes and a CPU-starved API in the first
// seconds, which alone set the whole test's p99 (381ms, against ~40ms once
// warm), plus a few dial timeouts. Ramping lets connections and pools warm
// up; the thresholds only judge the hold (requests tagged `phase:steady`).
//
// RATE, RAMP and HOLD (seconds) override the defaults, for quick runs:
//   RATE=50 RAMP=5 HOLD=10 k6 run test/load/baseline.js
export function rampAndHold({ rate, ramp = 30, hold = 60, preAllocatedVUs, maxVUs }) {
  rate = Number(__ENV.RATE || rate);
  ramp = Number(__ENV.RAMP || ramp);
  hold = Number(__ENV.HOLD || hold);
  return {
    rate,
    rampSeconds: ramp,
    scenario: {
      executor: "ramping-arrival-rate",
      startRate: 0,
      timeUnit: "1s",
      stages: [
        { duration: `${ramp}s`, target: rate },
        { duration: `${hold}s`, target: rate },
      ],
      preAllocatedVUs,
      maxVUs,
    },
    // Iterations k6 couldn't start on time because every VU was busy: the
    // target rate wasn't reached. Allows 1% of the hold's iterations.
    maxDroppedIterations: Math.ceil(rate * hold * 0.01),
  };
}

// Tags every metric the VU emits from here on with the test's phase. Call it
// at the start of each iteration.
export function tagPhase(rampSeconds) {
  const elapsed = exec.instance.currentTestRunDuration / 1000;
  exec.vu.metrics.tags.phase = elapsed < rampSeconds ? "ramp" : "steady";
}
