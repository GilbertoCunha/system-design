import { shortenUrl } from "./flows/shortenUrl.js";

// Test configuration
export const options = {
  thresholds: {
    http_req_duration: ["p(99) < 100"],
    http_req_failed: ["rate < 0.01"],
    http_reqs: ["rate > 900"], // 1k rps with 5% slack
  },
  // Create scenario for constant 1000rps throughput
  scenarios: {
    writes: {
      executor: "constant-arrival-rate",
      rate: 1000,
      timeUnit: "1s",
      duration: "1m",
      preAllocatedVUs: 50,
      maxVUs: 2000,
    },
  },
};

// Simulated user behavior
export default function () {
  const baseURL = __ENV.BASE_URL || "http://localhost:8080";
  const query = `search-attempt-${__VU}-${__ITER}`;
  const url = `https://www.google.com/search?q=${encodeURIComponent(query)}`;
  shortenUrl(baseURL, url);
}
