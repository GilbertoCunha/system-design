package internal

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// A repo whose client never connects: only the connection slots are used
func newSlotsRepo(t *testing.T, poolSize int) (*RedisUrlRepo, *prometheus.Registry) {
	t.Helper()
	c := &AppConfig{}
	c.Redis.Uri = "redis://127.0.0.1:1"
	c.Redis.Pool.Size = poolSize
	c.Redis.Timeouts.QueryTimeoutMs = 500
	reg := prometheus.NewRegistry()
	repo, err := NewRedisUrlRepo(c, slog.New(slog.NewTextHandler(io.Discard, nil)), reg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo, reg
}

func acquireSum(t *testing.T, reg *prometheus.Registry) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.0
	for _, f := range families {
		if f.GetName() != "redis_pool_acquire_duration_seconds" {
			continue
		}
		for _, m := range f.GetMetric() {
			sum += m.GetHistogram().GetSampleSum()
		}
	}
	return sum
}

// Waiting and holding must add up to all the time commands spend on the pool,
// and the pool can't be more than 100% busy: the dashboard's shares rely on it.
func TestSlotsAccountForAllTimeOnThePool(t *testing.T) {
	const poolSize, workers, hold = 2, 50, 5 * time.Millisecond
	repo, reg := newSlotsRepo(t, poolSize)

	var mu sync.Mutex
	onPool := 0.0 // wall-clock time from asking for a slot to giving it back
	var wg sync.WaitGroup
	start := time.Now()
	for range workers {
		wg.Go(func() {
			asked := time.Now()
			acquired, err := repo.acquireConn(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			time.Sleep(hold)
			repo.releaseConn(acquired)
			mu.Lock()
			onPool += time.Since(asked).Seconds()
			mu.Unlock()
		})
	}
	wg.Wait()
	elapsed := time.Since(start).Seconds()

	wait := acquireSum(t, reg)
	held := testutil.ToFloat64(repo.metrics.redisPoolConnHeldSecondsTotal)

	if diff := math.Abs(wait + held - onPool); diff > 0.01*onPool {
		t.Errorf("wait (%.3fs) + held (%.3fs) = %.3fs, want the time on the pool: %.3fs", wait, held, wait+held, onPool)
	}
	if busy := held / (poolSize * elapsed); busy > 1 {
		t.Errorf("pool busy %.0f%%, can't exceed 100%%", 100*busy)
	}
	if wait <= held {
		t.Errorf("with %d workers on %d connections most time should be waiting: wait %.3fs, held %.3fs", workers, poolSize, wait, held)
	}
}

// A command that times out waiting is still counted, as waiting, and
// reported as a Redis acquire timeout
func TestSlotsTimeoutCountsAsWaiting(t *testing.T) {
	repo, reg := newSlotsRepo(t, 1)
	acquired, err := repo.acquireConn(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = repo.acquireConn(ctx)
	o, ok := errors.AsType[*Overloaded](err)
	if !ok || o.Dependency != "redis" || o.Operation != "acquire_conn" {
		t.Fatalf("got %v, want an Overloaded redis acquire_conn", err)
	}
	if wait := acquireSum(t, reg); wait < 0.02 {
		t.Errorf("timed-out wait not counted: waiting sum %.3fs, want at least 0.020s", wait)
	}

	repo.releaseConn(acquired)
	if _, err := repo.acquireConn(context.Background()); err != nil {
		t.Errorf("slot not given back: %v", err)
	}
}
