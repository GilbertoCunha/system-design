package internal

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// redisPoolCollector exports the pool statistics go-redis keeps but
// redisprometheus leaves out (it has hits, misses, timeouts and connection
// counts), plus the pool size and query timeout, so dashboards can draw them
// as limits instead of hard-coding the config.
type redisPoolCollector struct {
	client       *redis.Client
	queryTimeout time.Duration

	waits, waitSeconds, pending, unusable, size, timeout *prometheus.Desc
}

func newRedisPoolCollector(client *redis.Client, queryTimeout time.Duration) *redisPoolCollector {
	return &redisPoolCollector{
		client:       client,
		queryTimeout: queryTimeout,
		waits: prometheus.NewDesc("redis_pool_wait_total",
			"Times a command had to wait for a free pool connection", nil, nil),
		// go-redis only adds a wait that ends with a connection: waits cut
		// short by a timeout aren't in here. Those show up as
		// redis_query_duration_seconds{outcome="timeout"}.
		waitSeconds: prometheus.NewDesc("redis_pool_wait_seconds_total",
			"Total time commands waited for a free pool connection (successful waits only)", nil, nil),
		pending: prometheus.NewDesc("redis_pool_pending_requests",
			"Commands waiting for a pool connection right now", nil, nil),
		unusable: prometheus.NewDesc("redis_pool_unusable_total",
			"Times a pool connection was found unusable", nil, nil),
		size: prometheus.NewDesc("redis_pool_size",
			"Maximum number of connections in the pool", nil, nil),
		timeout: prometheus.NewDesc("redis_query_timeout_seconds",
			"Timeout for each Redis command, from the config", nil, nil),
	}
}

func (c *redisPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.waits, c.waitSeconds, c.pending, c.unusable, c.size, c.timeout} {
		ch <- d
	}
}

func (c *redisPoolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.client.PoolStats()
	ch <- prometheus.MustNewConstMetric(c.waits, prometheus.CounterValue, float64(s.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.waitSeconds, prometheus.CounterValue, float64(s.WaitDurationNs)/1e9)
	ch <- prometheus.MustNewConstMetric(c.pending, prometheus.GaugeValue, float64(s.PendingRequests))
	ch <- prometheus.MustNewConstMetric(c.unusable, prometheus.CounterValue, float64(s.Unusable))
	ch <- prometheus.MustNewConstMetric(c.size, prometheus.GaugeValue, float64(c.client.Options().PoolSize))
	ch <- prometheus.MustNewConstMetric(c.timeout, prometheus.GaugeValue, c.queryTimeout.Seconds())
}
