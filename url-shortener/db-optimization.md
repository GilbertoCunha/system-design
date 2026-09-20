# Database Optimization Notes

Context: under load (`test/load/peak.js`, 10k VUs), `ShortenUrl` requests fail with
500s. The service wraps every DB call in a hard 500ms deadline
(`internal/service.go:44` and `:77`); when Postgres doesn't answer in time the
context expires and the handler returns `Internal server error`.

These are four of the levers worth pulling, and how each one actually works.

## 1. Query optimization: the `ON CONFLICT` rewrite

Postgres never edits a row in place. An `UPDATE` writes a **new copy** of the row
and marks the old one dead (to be cleaned up later by autovacuum). So this, in
`internal/database/query.sql`:

```sql
ON CONFLICT (short_url) DO UPDATE SET long_url = urls.long_url
```

sets the column to the value it already has — but Postgres can't tell that's
pointless. Every duplicate insert still costs a full row write, an index update,
WAL, and a dead tuple. We pay write cost for a no-op.

`DO NOTHING` skips the write, but then `RETURNING` gives nothing back, and
`PutShortUrl` needs the stored `long_url` to detect hash collisions. A CTE gets
both in one round trip:

```sql
-- name: PutShortUrl :one
WITH inserted AS (
  INSERT INTO urls (short_url, long_url) VALUES ($1, $2)
  ON CONFLICT (short_url) DO NOTHING
  RETURNING long_url
)
SELECT long_url FROM inserted
UNION ALL
SELECT long_url FROM urls WHERE short_url = $1
LIMIT 1;
```

Insert succeeds → returns the new `long_url`. Conflict → the insert writes
nothing, and the second branch reads the existing one. The Go code in
`internal/pg_repo.go` and its collision check work unchanged; re-run
`sqlc generate`.

One caveat: if a concurrent transaction inserts the same key between the two
branches, both can come back empty and we get `pgx.ErrNoRows`. Rare, but it
should be treated as retryable rather than a 500.

### Related: the schema

`id BIGSERIAL PRIMARY KEY` adds a second index and a sequence to maintain on
every insert, and nothing reads `id`. `short_url` could be the primary key
directly.

## 2. `synchronous_commit`

Every `COMMIT` normally waits for Postgres to physically flush the write-ahead
log to disk before replying. That's an fsync — milliseconds, and it doesn't get
faster under load, it gets slower as the queue grows.

`synchronous_commit=off` lets the commit return as soon as the WAL is in memory;
the flush happens a moment later in the background.

```yaml
  database:
    image: postgres:18.6
    command:
      - postgres
      - -c
      - synchronous_commit=off
      - -c
      - shared_buffers=1GB
```

**The tradeoff:** if the server loses power, we lose the last ~200ms of
transactions that were reported as committed. The database is *not* corrupted,
and it recovers cleanly — we just lose the tail.

This is very different from `fsync=off`, which *can* corrupt. Don't use that one.

For a URL shortener, losing a few hundred milliseconds of shortened links on a
hard crash is usually acceptable. To scope it more narrowly, set it
per-transaction on just the insert instead of globally.

# 3. Fast-fail vs slow-fail

Today one 500ms budget covers *both* waiting for a connection and running the
query. Under overload almost all of it is waiting. So a doomed request sits there
for a full 500ms — holding a goroutine, a TCP connection, and a queue slot — and
then fails anyway. It consumed resources for half a second and produced nothing,
while making every request behind it slower.

Fast-fail caps the *wait* separately and gives up early:

```go
func (r *PgUrlRepo) GetLongUrl(ctx context.Context, shortUrl string) (string, error) {
	acqCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	conn, err := r.pool.Acquire(acqCtx)
	cancel()
	if err != nil {
		return "", &Overloaded{} // no connection free — don't queue
	}
	defer conn.Release()

	longUrl, err := database.New(conn).GetLongUrl(ctx, shortUrl)
	// ... same error handling as before
}
```

Same number of failures under genuine overload — we can't serve traffic we don't
have capacity for. But each failure now costs 50ms instead of 500ms, so the
server stays responsive and the requests that *do* get a connection aren't stuck
behind a queue of zombies.

Then surface it honestly in the handler: a `503` with a `Retry-After` header, not
a `500`. "I'm busy, come back" is a different thing from "I'm broken," and it
tells the load test the difference too.

## A note on the timeout itself

Raising the 500ms deadline is tempting and counterproductive. A timeout cannot
create capacity; it only chooses who fails. Raising it under overload means more
requests in flight simultaneously, deeper queues, more memory, and a longer wait
before a doomed request gives up — more failures, not fewer. The deadline isn't
the bug; it's load-shedding doing its job, just badly placed. See §4.
