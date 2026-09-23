# Monitoring plan

Goal: when running load tests (`test/load/`), be able to see **which part
becomes the bottleneck first**: the API, Redis, Postgres, or the pods' own
CPU/memory limits.

Stack (from `../homelab`):

- **VictoriaMetrics** (single): stores metrics. Scrapes any pod or Service
  that has the `prometheus.io/scrape: "true"` annotation. It already scrapes
  cAdvisor and kube-state-metrics.
- **VictoriaLogs**: stores all pod logs. The collector ships them
  automatically, and JSON logs are split into fields.
- **Grafana**: has `victoriametrics` (PromQL/MetricsQL) and `victorialogs`
  (LogsQL) as datasources. It loads dashboards from any ConfigMap labelled
  `grafana_dashboard`, in any namespace.
- No tracing yet.

## 1. Pick a method so you know what to look for

- **API: use RED.** Track Rate (requests/sec), Errors (error rate) and
  Duration (p50/p95/p99 latency).
- **Postgres, Redis and pods: use USE.** Track Utilization, Saturation and
  Errors.

During a load test, the question is: "When latency goes up, which part is
saturated first?"

## 2. Monitors for each part

There is no Prometheus Operator, so `PodMonitor`/`ServiceMonitor` do nothing
here. Each part opts in with pod annotations instead:

```yaml
prometheus.io/scrape: "true"
prometheus.io/port: "<metrics port>"
prometheus.io/path: "/metrics" # optional, this is the default
```

### Postgres (CNPG)

- CNPG already exposes metrics on port `9187`. They are **not scraped yet**,
  because `enablePodMonitor: true` in `gitops/base/database.yaml` needs the
  PodMonitor CRD.
- Replace it with annotations on the pods through the Cluster spec:
  `spec.inheritedMetadata.annotations` (port `9187`).
- Import the CNPG Grafana dashboard (from the CNPG project) as a ConfigMap
  in this repo.
- Later: enable `pg_stat_statements` to find slow queries.

### Redis

- Add `oliver006/redis_exporter` as a sidecar in `gitops/base/cache.yaml`
  (metrics on port `9121`) and annotate the pod.
- Key metrics: memory used, connected clients, commands/sec, keyspace
  hits/misses, evictions.

### API

This matters most. The Go code has no metrics yet.

- Add `prometheus/client_golang` and expose `/metrics`, then annotate the
  API pods.
- Metrics to add:
  - A request duration histogram, labelled by route and status.
  - **Duration histograms around each DB call and each Redis call**, in
    `internal/pg_repo.go` and `internal/redis_repo.go`. These show where the
    time inside a request goes.
  - A cache hit/miss counter.
  - DB connection pool stats: in-use, idle and wait time. A pool that runs
    out is a very common hidden bottleneck.
- Go runtime metrics (goroutines, GC, memory) come for free with
  `client_golang`.

### Pods

Already collected through cAdvisor. Watch:

- CPU throttling: `container_cpu_cfs_throttled_periods_total`. The pods
  have CPU limits of 500m, so this is a likely bottleneck.
- Memory usage against limits, and restarts/OOM kills (kube-state-metrics).

### Logs (VictoriaLogs)

The API already logs JSON with `slog`, so VictoriaLogs can filter on fields.
Useful additions:

- Log slow requests or slow queries with their duration as a field.
- Add a logs panel to the dashboard (errors from the `url-shortener` pods)
  so errors show up next to the metrics.

## 3. Dashboard as code

- Keep dashboards as JSON inside ConfigMaps in `gitops/` (for example
  `gitops/base/dashboards/`), with the label `grafana_dashboard: "1"`.
  Grafana's sidecar loads them from any namespace, and ArgoCD deploys them
  like everything else.
- Simple workflow: build the dashboard in the Grafana UI, export the JSON
  and commit it. Set the datasource to `victoriametrics`.
- Suggested layout: one row per part (Load, API, Redis, Postgres, Pods,
  Logs), with shared time ranges so spikes line up.

## 4. Connect load tests to the dashboard

- VictoriaMetrics accepts Prometheus remote write on `/api/v1/write`, so k6
  can send results to it directly:

  ```sh
  K6_PROMETHEUS_RW_SERVER_URL=http://<victoria-metrics>:8428/api/v1/write \
  K6_PROMETHEUS_RW_TREND_STATS="p(50),p(95),p(99)" \
  k6 run -o experimental-prometheus-rw test/load/baseline.js
  ```

- VictoriaMetrics has no route outside the cluster, so use
  `kubectl port-forward -n victoria-metrics svc/victoria-metrics 8428` when
  running k6 from a laptop. A Taskfile task can wrap this.
- This puts the load (virtual users, RPS) on the same graphs as the system
  metrics, so you can see things like "at 300 RPS, p99 went up and the DB
  pool was full."

## 5. Later, once metrics are working

- **pprof** on the API finds CPU and memory hotspots inside the Go code
  itself.
- **Tracing** (OpenTelemetry) shows one slow request broken down step by
  step. VictoriaMetrics already accepts OTLP metrics, but traces need a new
  store (for example VictoriaTraces or Tempo).

## Suggested order

1. Scrape the CNPG metrics (annotations) and check they show up in Grafana.
2. Add the Redis exporter.
3. Add API metrics.
4. Put the dashboard in git.
5. Hook up k6.
6. Add pprof, then tracing.
