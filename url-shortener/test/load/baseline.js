import { shortenUrl } from "./flows/shortenUrl.js";

// Test configuration
export const options = {
  thresholds: {
    // Assert that 99% of requests finish within 100ms.
    http_req_duration: ["p(99) < 100"],
  },
  // Ramp the number of virtual users up and down
  stages: [
    { duration: "10s", target: 15 },
    { duration: "20s", target: 15 },
    { duration: "5s", target: 0 },
  ],
};

// Simulated user behavior
export default function () {
  const baseURL = __ENV.BASE_URL || "http://localhost:8080";
  const query = `search-attempt-${__VU}-${__ITER}`;
  const url = `https://www.google.com/search?q=${encodeURIComponent(query)}`;
  shortenUrl(baseURL, url);
}
