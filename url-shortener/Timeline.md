# How the codebase got to this point (short summary)

1. Created the initial golang codebase - no cache, just pg db and some questionable choices
2. Wanted to test performance, created `k6` load tests
3. Noticed some issues with connection timeouts to the db. Query timeouts happened often, because a go context with query timeout of 500ms was defined. Root cause unknown, so created a small go routine to log connection pool statistics. Also added debug query time logging.
4. Improved the load tests, found out that under a load of around 16k RPS, connection acquisition wait times jumped to ~300ms!
5. Added `max_conns` and `min_conns` to the pooling configuration of the app. Using `50` and `10`, respectively, pool wait times reduced to almost 0ms and p99 request duration under 1m of 10k req/s load on both the GET and POST endpoints reduced to just ~6ms! Simple optimization that packs a punch!
6. For further optimization - and just for the sake of tinkering with postgres settings - turned of `synchronous-commit` so that postgres replies before flushing WAL into disk - reduced the p99 for the same conditions as in point 5 to ~4ms.
7. Performed query optimization on inserting the short and long url pairs. Previously, it was doing an `ON CONFLICT UPDATE SET long_url = urls.long_url`, which meant a row + index update on every duplicate write. Now, uses `ON CONFLICT DO NOTHING` together with another `SELECT` to return the `long_url` in case of collisions.
8. Added separate contexts in the database for connection acquisition from the pool (50ms) and for actual query completion. If it fails to acquire a connection in that time limit, a 503 Service Unavailable is returned, together with a `Retry-After` header.
9. (09-21) Added a Redis cache in front of Postgres for reads.

## On the homelab cluster

Deployed on 09-21/23: Talos guests on one Proxmox host (i7-8700, 6 cores / 12 threads), CloudNativePG, Redis, kgateway (Envoy). `k6` runs from a laptop over the mesh VPN from here on, which adds ~47ms to every request it measures; server-side metrics are the reference.

10. (09-26) Made bottlenecks visible before tuning: Prometheus metrics for HTTP, the pg pool and the Redis pool, a Grafana dashboard with a bottlenecks tab, `k6` results sent to VictoriaMetrics. Scrape interval 15s -> 5s, and counters/histograms instead of gauges, because gauges missed spikes between scrapes.
11. (09-26/27) CPU throttling and OOM kills: `GOMAXPROCS` taken from the CPU limit (Go was running more threads than the quota), `GOMEMLIMIT` from the memory limit (200Mi -> 500Mi), Redis `maxmemory 768mb` + `allkeys-lru` (keys have no TTL, so it grew until killed).
12. (09-26) Starting at full rate opened ~1000 TLS connections at once: p99 381ms, against ~40ms once warm. Load tests now ramp, and thresholds judge only the hold (`rampAndHold.js`). Also h2c between Envoy and the API, to share connections.
13. (09-27) To get past 5k req/s, raised whatever the dashboard showed saturated, in turn:
    - API CPU limit 500m -> 3 (request 1)
    - Postgres CPU limit 500m -> 2, memory 500Mi -> 1Gi, `shared_buffers` 250MB
    - Redis CPU limit 500m -> 1
    - pg pool 20 -> 90 conns, with `min_conns` = `max_conns` so no connection (TLS + auth) is opened at peak. Postgres allows 100.
    - Redis pool 50 -> 600
    - pool acquire timeout 100ms -> 500ms
14. (09-27) Errors at the gateway before reaching the API: Envoy's circuit breaker defaults to 1024 requests in flight per backend. Raised to 10000 (`BackendConfigPolicy`).
15. (09-27) Added an in-flight limiter middleware (`max_in_flight` 800 -> 1200): past it the API answers 503 at once instead of queueing. Added metrics for what causes each 503.
16. (09-27) Took Istio (ambient, ztunnel) out of the request path, then removed it.
17. (09-27) `synchronous_commit: off` on the cluster's Postgres. It was misspelled `synchronous_commits` until 09-30, so it did nothing for three days. Added pprof, later scraped continuously by Pyroscope.
18. (09-30) Node pools: cluster components, Envoy included, moved to two 2-vCPU system nodes so application load could not starve them. Workers 4 -> 6 vCPUs. Proxmox CPU weights: worker 200, control plane 150, system 100.
19. (09-30) A load test put half of each system node's CPU in the kernel handling packets. Cilium: VXLAN tunnel -> native routing, eBPF masquerading (host routing), direct server return.
20. (10-01) Workers boot with `mitigations=off` (`pti=on` stays, Talos needs it): cheaper system calls for Envoy, Postgres, Redis. One NIC queue per vCPU was tried and reverted, the host was already oversubscribed.
21. (10-01) Ceiling, 8 guests / 28 vCPUs on 12 threads: the host saturated at ~9.5k req/s (9 busy cores + ~6 of steal), and Envoy on the 2-vCPU system nodes capped at ~6.5k. Every request crossed between guests at each hop. Collapsed to one node per pool (2/2/8 vCPUs), with Envoy on the worker beside the apps (`--concurrency 4`, 2 CPU request, `externalTrafficPolicy: Local`). Same 10k req/s test:
    - p99 493ms -> 96ms
    - `k6` iterations dropped 8,425 -> 47
    - vCPUs busy 8.3 -> 5.8, steal 3.8 -> 0.3
    - host threads idle 1.5 -> 4.0
22. (10-01) Reserved host core 0 for the server itself (guests pinned to threads 1-5, 7-11): the mesh client alone takes a full thread at 10k req/s. Worker 8 -> 10 vCPUs.
23. (10-01) Ceiling on dev (mesh, 20k req/s target): clean up to ~11.6k req/s, then the API's own in-flight limiter answers 503. API at 2.7 of its 3 CPUs, Envoy 3.3 of 4 threads, worker 6.9 of 8 vCPUs. The limit is now the app's settings, not the gateway.

## Through the Cloudflare tunnel (prod)

Public path: Cloudflare edge -> `cloudflared` -> Envoy (`gw-public`) -> API. `peak.js` ramps to 10k req/s over 180s, then holds 60s.

24. (10-02) First run: clean up to ~2.2-2.6k req/s, then 31% of ~410k requests failed (49% in the hold). Not rate limiting, no 429s:
    - `cloudflared` ran on the 2-vCPU system node, which sat at 2.0 of 2 cores
    - 126k 503s came from Cloudflare's edge and never reached `cloudflared`, which dropped 73k QUIC packets (receive queue full)
    - 1.2k 502s: `cloudflared` opened ~800 connections/s to Envoy and ran out of local ports (`cannot assign requested address`)
    - the API peaked at 0.55 cores, with no 503s of its own
25. (10-02) Moved `cloudflared` to the worker (`GOMAXPROCS=4`, 1 CPU request), `gw-public` Envoy `--concurrency` 2 -> 4 (1 CPU request), `keepAliveConnections` 100 -> 1000. Second run: clean up to ~9.2k req/s, zero failures in the ramp (895k requests).
26. (10-02) Ceiling at ~9.2k req/s: worker at ~8.5 of 10 vCPUs + 0.6 steal, pg pool 90 of 90. Then a collapse to ~2.5k req/s: requests in flight passed the 1000 kept-alive connections, `cloudflared` went back to opening 570-1100 connections/s, ran out of ports again, and spent 4-5 CPUs in the kernel (from 0.44). `k6` p99 ~200ms -> 5.6s. The API dashboard showed POST p99 at 1-1.7s with p50 at 10ms and GET p99 at 47ms: the handler's timer includes reading the body, which `cloudflared` delivered late.
27. (10-02) `keepAliveConnections` 1000 -> 8000, above the 5,000 VUs the test allows. Third run: no collapse. It held ~8.8k req/s for the whole hold, with no HTTP errors (89 client timeouts in 526k requests), no connection churn and no port errors. It still missed the 10k target: `k6` ran out of VUs, p50 ~500ms, p99 1.1-1.5s, 41.7k iterations dropped.
28. (10-02) Ceiling at ~8.8k req/s. Nothing in the cluster was at its limit: `cloudflared` 2.0 of 4 CPUs, Envoy 1.6 of 4 threads, API 1.5 of 3 CPUs, worker ~7.5 of 10 vCPUs + 0.6 steal. The queue sat before Envoy (100-300 requests in flight there, against 5,000 at `k6`), and `cloudflared` dropped 235k QUIC packets on the one tunnel connection that carried all the traffic. The pg pool also ran dry in bursts (acquire p99 up to ~100ms, POST handler p99 ~250-340ms).

## Ceilings so far

| When | Path | Clean throughput | What stopped it |
| --- | --- | --- | --- |
| 10-01, 8 guests | mesh -> Envoy (system nodes) -> API | ~6.5k req/s | Envoy starved on 2-vCPU nodes; host saturated at ~9.5k |
| 10-01, 3 guests | mesh -> Envoy (worker) -> API | ~11.6k req/s | API in-flight limiter (`max_in_flight` 1200, 3 CPUs) |
| 10-02, first run | Cloudflare -> `cloudflared` (system node) -> Envoy -> API | ~2.2-2.6k req/s | `cloudflared` out of CPU on 2 vCPUs |
| 10-02, second run | Cloudflare -> `cloudflared` (worker) -> Envoy -> API | ~9.2k req/s | Host CPU (10 guest threads) and the pg pool; then connection churn |
| 10-02, third run | Same, `keepAliveConnections` 8000 | ~8.8k req/s, held | The tunnel's single QUIC connection (likely); pg pool in bursts |
