# Database Optimization Notes

Context: under load (`test/load/peak.js`, 10k VUs), `ShortenUrl` requests fail with
500s. The service wraps every DB call in a hard 500ms deadline
(`internal/service.go:44` and `:77`); when Postgres doesn't answer in time the
context expires and the handler returns `Internal server error`.

These are four of the levers worth pulling, and how each one actually works.

# 1. Fast-fail vs slow-fail

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

