import { shortenUrl } from "./flows/shortenUrl.js";
import { rampAndHold, tagPhase } from "./rampAndHold.js";

// Ramp up to 1000 req/s, then hold it
const load = rampAndHold({ rate: 1000, preAllocatedVUs: 50, maxVUs: 2000 });

// Test configuration
export const options = {
  thresholds: {
    "http_req_duration{phase:steady}": ["p(99) < 100"],
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
  shortenUrl(baseURL, url);
}
