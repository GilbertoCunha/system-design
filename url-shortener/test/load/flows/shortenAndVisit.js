import { shortenUrl } from "./shortenUrl.js";
import { check } from "k6";
import http from "k6/http";

export function shortenAndVisit(baseUrl, url) {
  const resp1 = shortenUrl(baseUrl, url);

  // The create failed, so there is no shortUrl to visit. shortenUrl has
  // already recorded the failed check; stop here rather than parsing a body
  // that holds an error message instead of JSON.
  if (resp1.status !== 201) return resp1;

  const shortUrl = JSON.parse(resp1.body).shortUrl;
  const resp2 = http.get(`${baseUrl}/api/v1/url/${shortUrl}`, { redirects: 0 });
  check(resp2, {
    "visit shortUrl: 302": (r) => r.status === 302,
  });
  return resp2;
}
