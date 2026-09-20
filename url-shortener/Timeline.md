# How the codebase got to this point (short summary)

1. Created the initial golang codebase
2. Wanted to test performance, created `k6` load tests
3. Noticed some issues with connection timeouts to the db. Query timeouts happened often, because a go context with query timeout of 500ms was defined. Root cause unknown, so created a small go routine to log connection pool statistics.
4. Improved the load tests, found out that under a load of around 16k RPS, connection acquisition wait times jumped to ~300ms!
5. Added `max_conns` and `min_conns` to the pooling configuration of the app. Using `50` and `10`, respectively, pool wait times reduced to almost 0ms and p99 request duration under 1m of 10k req/s load on both the GET and POST endpoints reduced to just ~6ms! Simple optimization that packs a punch!
