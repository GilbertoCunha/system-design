import http from "k6/http";
import { shortenUrl } from "./flows/shortenUrl.js";
import { visitUrl } from "./flows/visitUrl.js";
import { rampAndHold, tagPhase } from "./rampAndHold.js";

// A URL shortener is read far more than it is written: 10 visits for every
// URL created. RATE is the total requests per second; writes get 1/11 of it
// and reads 10/11, each as its own scenario so the ratio holds whatever the
// API's latency.
const rate = Number(__ENV.RATE || 10000);
const writes = rampAndHold({
  rate: Math.round(rate / 11),
  ramp: 180,
  hold: 60,
  preAllocatedVUs: 100,
  maxVUs: 1000,
  rateFromEnv: false,
});
const reads = rampAndHold({
  rate: Math.round((rate * 10) / 11),
  ramp: 180,
  hold: 60,
  preAllocatedVUs: 900,
  maxVUs: 4000,
  rateFromEnv: false,
});

// Short URLs the reads pick from, created before the load starts. Created
// URLs go into the cache too, so reads are mostly cache hits either way, as
// they would be for a real shortener's popular links.
const POOL_SIZE = Number(__ENV.POOL_SIZE || 1000);

export const options = {
  thresholds: {
    "http_req_duration{phase:steady}": ["p(99) < 500"],
    "http_req_failed{phase:steady}": ["rate < 0.01"],
    dropped_iterations: [
      `count < ${writes.maxDroppedIterations + reads.maxDroppedIterations}`,
    ],
  },
  scenarios: {
    writes: { ...writes.scenario, exec: "write" },
    reads: { ...reads.scenario, exec: "read" },
  },
};

const baseURL = __ENV.BASE_URL || "http://localhost:8080";

export function setup() {
  // A prefix per run, so this run's writes are new rows rather than repeats
  // of the last run's URLs (which the API answers without inserting)
  const run = Date.now();
  const shortUrls = [];
  for (let start = 0; start < POOL_SIZE; start += 100) {
    const batch = [];
    for (let i = start; i < Math.min(start + 100, POOL_SIZE); i++) {
      const longUrl = `https://www.google.com/search?q=pool-${run}-${i}`;
      batch.push(["POST", `${baseURL}/v1/url`, JSON.stringify({ longUrl })]);
    }
    for (const res of http.batch(batch)) {
      if (res.status === 201) shortUrls.push(res.json("shortUrl"));
    }
  }
  if (shortUrls.length === 0) throw new Error("setup created no short URLs");
  return { run, shortUrls };
}

export function write(data) {
  tagPhase(writes.rampSeconds);
  const query = `search-${data.run}-${__VU}-${__ITER}`;
  shortenUrl(baseURL, `https://www.google.com/search?q=${encodeURIComponent(query)}`);
}

export function read(data) {
  tagPhase(reads.rampSeconds);
  const i = Math.floor(Math.random() * data.shortUrls.length);
  visitUrl(baseURL, data.shortUrls[i]);
}
