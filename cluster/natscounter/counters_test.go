package natscounter

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/natsrv/natstest"
)

func newTestCounters(t *testing.T, nodes int) []*Counters {
	t.Helper()
	cluster := natstest.StartCluster(t, nodes)
	counters := make([]*Counters, 0, len(cluster))
	for _, node := range cluster {
		natstest.WaitJetStream(t, node.Conn)
		c, err := New(node.Conn, Options{Replicas: 3, ClusterSize: nodes, Logger: zap.NewNop()})
		if nil != err {
			t.Fatalf("create counters: %v", err)
		}
		counters = append(counters, c)
	}
	return counters
}

func TestCappedReplicas(t *testing.T) {
	cases := []struct{ requested, size, want int }{
		{3, 1, 1},
		{3, 5, 3},
		{0, 3, 1},
		{2, 0, 2},
	}
	for _, tc := range cases {
		if got := cappedReplicas(tc.requested, tc.size); tc.want != got {
			t.Fatalf("cappedReplicas(%d, %d) = %d, want %d", tc.requested, tc.size, got, tc.want)
		}
	}
}

func TestAcquireIsSharedAcrossNodes(t *testing.T) {
	counters := newTestCounters(t, 1)
	ctx := context.Background()
	first := counters[0]

	for i := 0; i < 2; i++ {
		ok, err := first.Acquire(ctx, "ws_user", "alice", 2)
		if nil != err || !ok {
			t.Fatalf("acquire %d: ok=%v err=%v", i, ok, err)
		}
	}
	ok, err := first.Acquire(ctx, "ws_user", "alice", 2)
	if nil != err || ok {
		t.Fatalf("third acquire: ok=%v err=%v, want rejection", ok, err)
	}
	if err := first.Release(ctx, "ws_user", "alice"); nil != err {
		t.Fatalf("release: %v", err)
	}
	ok, err = first.Acquire(ctx, "ws_user", "alice", 2)
	if nil != err || !ok {
		t.Fatalf("acquire after release: ok=%v err=%v", ok, err)
	}
	ok, err = first.Acquire(ctx, "ws_user", "bob", 2)
	if nil != err || !ok {
		t.Fatalf("other user acquire: ok=%v err=%v", ok, err)
	}
}

func TestAcquireAcrossClusterNodes(t *testing.T) {
	counters := newTestCounters(t, 3)
	ctx := context.Background()

	for i, c := range counters[:2] {
		ok, err := c.Acquire(ctx, "ws_total", "all", 2)
		if nil != err || !ok {
			t.Fatalf("node %d acquire: ok=%v err=%v", i, ok, err)
		}
	}
	ok, err := counters[2].Acquire(ctx, "ws_total", "all", 2)
	if nil != err || ok {
		t.Fatalf("third node acquire: ok=%v err=%v, want rejection", ok, err)
	}
}

func TestReleaseNeverLeavesNegativeCounter(t *testing.T) {
	counters := newTestCounters(t, 1)
	ctx := context.Background()
	c := counters[0]

	if err := c.Release(ctx, "ws_user", "carol"); nil != err {
		t.Fatalf("release without acquire: %v", err)
	}
	ok, err := c.Acquire(ctx, "ws_user", "carol", 1)
	if nil != err || !ok {
		t.Fatalf("acquire after stray release: ok=%v err=%v", ok, err)
	}
}

func TestAllowEnforcesWindowAllowance(t *testing.T) {
	counters := newTestCounters(t, 1)
	ctx := context.Background()
	c := counters[0]
	fixed := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return fixed }

	for i := 0; i < 5; i++ {
		ok, err := c.Allow(ctx, "download", "192.0.2.1", 1, 5)
		if nil != err || !ok {
			t.Fatalf("request %d: ok=%v err=%v", i, ok, err)
		}
	}
	ok, err := c.Allow(ctx, "download", "192.0.2.1", 1, 5)
	if nil != err || ok {
		t.Fatalf("sixth request: ok=%v err=%v, want rejection", ok, err)
	}
	ok, err = c.Allow(ctx, "download", "192.0.2.2", 1, 5)
	if nil != err || !ok {
		t.Fatalf("other client: ok=%v err=%v", ok, err)
	}

	c.now = func() time.Time { return fixed.Add(10 * time.Second) }
	ok, err = c.Allow(ctx, "download", "192.0.2.1", 1, 5)
	if nil != err || !ok {
		t.Fatalf("next window: ok=%v err=%v", ok, err)
	}
}

func TestWindowAllowance(t *testing.T) {
	cases := []struct {
		rps     float64
		burst   int
		seconds int64
		allowed int64
	}{
		{100, 1, 1, 100},
		{10, 5, 1, 10},
		{1, 5, 5, 5},
		{0.1, 1, 10, 1},
	}
	for _, tc := range cases {
		seconds := windowSeconds(tc.rps, tc.burst)
		if tc.seconds != seconds {
			t.Fatalf("windowSeconds(%v, %d) = %d, want %d", tc.rps, tc.burst, seconds, tc.seconds)
		}
		if got := windowAllowance(tc.rps, tc.burst, seconds); tc.allowed != got {
			t.Fatalf("windowAllowance(%v, %d) = %d, want %d", tc.rps, tc.burst, got, tc.allowed)
		}
	}
}
