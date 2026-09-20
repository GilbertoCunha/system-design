import { shortenAndVisit } from "./flows/shortenAndVisit.js";

// Test configuration
export const options = {
  thresholds: {
    http_req_duration: ["p(99) < 500"],
    http_req_failed: ["rate < 0.01"],
    dropped_iterations: ["count < 100"], // k6 couldn't sustain the rate
  },
  // Create constant load of 10k req/s per endpoint
  scenarios: {
    writes: {
      executor: "constant-arrival-rate",
      rate: 10000,
      timeUnit: "1s",
      duration: "1m",
      preAllocatedVUs: 500,
      maxVUs: 5000,
    },
  },
};

// Simulated user behavior
export default function () {
  const baseURL = __ENV.BASE_URL || "http://localhost:8080";
  const query = `search-attempt-${__VU}-${__ITER}`;
  const url = `https://www.google.com/search?q=${encodeURIComponent(query)}`;
  shortenAndVisit(baseURL, url);
}
