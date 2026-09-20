import { shortenAndVisit } from "./flows/shortenAndVisit.js";

// Test configuration
export const options = {
  thresholds: {
    // Assert that 99% of requests finish within 500ms.
    http_req_duration: ["p(99) < 500"],
  },
  // Ramp the number of virtual users up and down
  stages: [{ duration: "30s", target: 10000 }],
};

// Simulated user behavior
export default function () {
  const baseURL = __ENV.BASE_URL || "http://localhost:8080";
  const query = `search-attempt-${__VU}-${__ITER}`;
  const url = `https://www.google.com/search?q=${encodeURIComponent(query)}`;
  shortenAndVisit(baseURL, url);
}
