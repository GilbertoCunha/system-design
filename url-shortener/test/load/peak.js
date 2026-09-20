import { shortenAndVisit } from "./flows/shortenAndVisit.js";

// Test configuration
export const options = {
  thresholds: {
    http_req_duration: ["p(99) < 500"],
    http_req_failed: ["rate < 0.01"],
    http_reqs: ["rate > 19000"], // 10k/endpoint × 2 = 20k, with 5% slack
  },
  // Create constant load of 10k req/s per endpoint
  scenarios: {
    writes: {
      executor: "constant-arrival-rate",
      rate: 10000,
      timeUnit: "1s",
      duration: "1m",
      preAllocatedVUs: 1000,
      maxVUs: 8000,
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
