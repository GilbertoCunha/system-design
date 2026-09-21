import http from "k6/http";
import { check } from "k6";

export function shortenUrl(baseUrl, url) {
  const res = http.post(`${baseUrl}/v1/url`, JSON.stringify({ longUrl: url }));
  check(res, {
    "create shortUrl: 201": (r) => r.status === 201,
  });
  return res;
}
