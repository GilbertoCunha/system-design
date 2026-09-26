import { shortenAndVisit } from "./flows/shortenAndVisit.js";
import { rampAndHold, tagPhase } from "./rampAndHold.js";

// Ramp up to 1000 iterations/s (2k req/s: a create and a visit each), then
// hold it. The target is 10k iterations/s (20k req/s).
const load = rampAndHold({ rate: 1000, preAllocatedVUs: 1000, maxVUs: 8000 });

// Test configuration
export const options = {
  thresholds: {
    "http_req_duration{phase:steady}": ["p(99) < 500"],
    "http_req_failed{phase:steady}": ["rate < 0.01"],
    dropped_iterations: [`count < ${load.maxDroppedIterations}`],
  },
  scenarios: {
    writes: load.scenario,
  },
};

// Simulated user behavior
export default function () {
  tagPhase(load.rampSeconds);
  const baseURL = __ENV.BASE_URL || "http://localhost:8080";
  const query = `search-attempt-${__VU}-${__ITER}`;
  const url = `https://www.google.com/search?q=${encodeURIComponent(query)}`;
  shortenAndVisit(baseURL, url);
}
