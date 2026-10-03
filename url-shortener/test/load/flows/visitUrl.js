import http from "k6/http";
import { check } from "k6";

export function visitUrl(baseUrl, shortUrl) {
  const res = http.get(`${baseUrl}/v1/url/${shortUrl}`, {
    redirects: 0,
    tags: { name: "GET /v1/url/:id" },
  });
  check(res, {
    "visit shortUrl: 302": (r) => r.status === 302,
  });
  return res;
}
