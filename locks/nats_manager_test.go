package locks

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/natsrv/natstest"
	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
)

func newNATSTestManagers(t *testing.T, ttl time.Duration, count int) []*NATSManager {
	t.Helper()
	nodes := natstest.StartCluster(t, 1)
	natstest.WaitJetStream(t, nodes[0].Conn)
	js, err := jetstream.New(nodes[0].Conn)
	if nil != err {
		t.Fatalf("jetstream: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, bucket := range []string{natsLockBucket} {
		if _, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: bucket, History: 1, LimitMarkerTTL: time.Second}); nil != err {
			t.Fatalf("create bucket %s: %v", bucket, err)
		}
	}
	managers := make([]*NATSManager, 0, count)
	for i := 0; i < count; i++ {
		m, err := NewNATSManager(ctx, js, "node", ttl, time.Second, zap.NewNop())
		if nil != err {
			t.Fatalf("new manager: %v", err)
		}
		t.Cleanup(func() { _ = m.Close() })
		managers = append(managers, m)
	}
	return managers
}

func TestNATSMutualExclusion(t *testing.T) {
	ms := newNATSTestManagers(t, 3*time.Second, 2)
	ms[1].SetWaitTimeout(300 * time.Millisecond)
	ctx := context.Background()

	lease, err := ms[0].Acquire(ctx, "k")
	if nil != err {
		t.Fatalf("first acquire: %v", err)
	}
	defer lease.Release(ctx)

	if _, err := ms[1].Acquire(ctx, "k"); !errors.Is(err, ErrLockHeld) {
		t.Fatalf("second acquire err = %v, want ErrLockHeld", err)
	}
}

func TestNATSReleaseLetsOtherAcquire(t *testing.T) {
	ms := newNATSTestManagers(t, 3*time.Second, 2)
	ms[1].SetWaitTimeout(5 * time.Second)
	ctx := context.Background()

	lease, err := ms[0].Acquire(ctx, "k")
	if nil != err {
		t.Fatalf("first acquire: %v", err)
	}
	if token := lease.(*natsLease).Token(); 0 == token {
		t.Fatal("fencing token is zero")
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = lease.Release(ctx)
	}()

	other, err := ms[1].Acquire(ctx, "k")
	if nil != err {
		t.Fatalf("acquire after release: %v", err)
	}
	if err := other.Release(ctx); nil != err {
		t.Fatalf("release: %v", err)
	}
}

func TestNATSTTLExpiryWhenRenewalStops(t *testing.T) {
	ms := newNATSTestManagers(t, time.Second, 2)
	ms[1].SetWaitTimeout(10 * time.Second)
	ctx := context.Background()

	if _, err := ms[0].Acquire(ctx, "k"); nil != err {
		t.Fatalf("first acquire: %v", err)
	}
	if err := ms[0].Close(); nil != err {
		t.Fatalf("close: %v", err)
	}

	other, err := ms[1].Acquire(ctx, "k")
	if nil != err {
		t.Fatalf("acquire after expiry: %v", err)
	}
	_ = other.Release(ctx)
}

func TestNATSSnapshotListsHeldKeys(t *testing.T) {
	ms := newNATSTestManagers(t, 3*time.Second, 1)
	ctx := context.Background()

	a, err := ms[0].Acquire(ctx, "alpha")
	if nil != err {
		t.Fatalf("acquire alpha: %v", err)
	}
	defer a.Release(ctx)
	b, err := ms[0].Acquire(ctx, "beta")
	if nil != err {
		t.Fatalf("acquire beta: %v", err)
	}
	defer b.Release(ctx)

	held, err := ms[0].Snapshot(ctx)
	if nil != err {
		t.Fatalf("snapshot: %v", err)
	}
	if 2 != len(held) || "alpha" != held[0].Key || "beta" != held[1].Key || natsBackendName != held[0].Backend {
		t.Fatalf("unexpected snapshot: %+v", held)
	}

	released, err := ms[0].ForceRelease(ctx, "alpha")
	if nil != err || !released {
		t.Fatalf("force release = %v, %v", released, err)
	}
	held, err = ms[0].Snapshot(ctx)
	if nil != err || 1 != len(held) {
		t.Fatalf("snapshot after force release = %+v, %v", held, err)
	}
}

func queueLen(m *NATSManager, key string) int {
	hash := natsLockKey(key)[len(natsLockPrefix):]
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queues[hash])
}

func waitQueueLen(t *testing.T, m *NATSManager, key string, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if want == queueLen(m, key) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("queue length = %d, want %d", queueLen(m, key), want)
}

func TestNATSFairQueueFIFO(t *testing.T) {
	ms := newNATSTestManagers(t, 5*time.Second, 4)
	ctx := context.Background()

	holder, err := ms[0].Acquire(ctx, "k")
	if nil != err {
		t.Fatalf("holder acquire: %v", err)
	}

	var mu sync.Mutex
	var order []int
	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		ms[i].SetWaitTimeout(15 * time.Second)
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			lease, err := ms[idx].Acquire(ctx, "k")
			if nil != err {
				t.Errorf("waiter %d acquire: %v", idx, err)
				return
			}
			mu.Lock()
			order = append(order, idx)
			mu.Unlock()
			if err := lease.Release(ctx); nil != err {
				t.Errorf("waiter %d release: %v", idx, err)
			}
		}(i)
		waitQueueLen(t, ms[0], "k", i)
	}

	if err := holder.Release(ctx); nil != err {
		t.Fatalf("holder release: %v", err)
	}
	wg.Wait()

	if 3 != len(order) || 1 != order[0] || 2 != order[1] || 3 != order[2] {
		t.Fatalf("acquire order = %v, want [1 2 3]", order)
	}
}

func TestNATSStaleQueueEntryLetsNextWaiterProceed(t *testing.T) {
	ms := newNATSTestManagers(t, 5*time.Second, 2)
	ms[1].SetWaitTimeout(10 * time.Second)
	ctx := context.Background()

	holder, err := ms[0].Acquire(ctx, "k")
	if nil != err {
		t.Fatalf("holder acquire: %v", err)
	}
	hash := natsLockKey("k")[len(natsLockPrefix):]
	if _, err := ms[0].locks.Create(ctx, natsQueuePrefix+hash+".crashed", nil, jetstream.KeyTTL(time.Second)); nil != err {
		t.Fatalf("create stale entry: %v", err)
	}
	waitQueueLen(t, ms[0], "k", 1)

	result := make(chan error, 1)
	go func() {
		lease, err := ms[1].Acquire(ctx, "k")
		if nil == err {
			err = lease.Release(ctx)
		}
		result <- err
	}()
	waitQueueLen(t, ms[0], "k", 2)

	started := time.Now()
	if err := holder.Release(ctx); nil != err {
		t.Fatalf("holder release: %v", err)
	}
	select {
	case err := <-result:
		t.Fatalf("waiter finished before stale entry expired: %v after %s", err, time.Since(started))
	case <-time.After(300 * time.Millisecond):
	}
	if err := <-result; nil != err {
		t.Fatalf("waiter acquire after stale expiry: %v", err)
	}
}

func TestNATSUncontendedAcquireMakesNoQueueEntry(t *testing.T) {
	ms := newNATSTestManagers(t, 3*time.Second, 1)
	ctx := context.Background()

	lease, err := ms[0].Acquire(ctx, "k")
	if nil != err {
		t.Fatalf("acquire: %v", err)
	}
	defer lease.Release(ctx)

	if 0 != queueLen(ms[0], "k") {
		t.Fatalf("queue length = %d, want 0", queueLen(ms[0], "k"))
	}
	lister, err := ms[0].locks.ListKeysFiltered(ctx, natsQueuePrefix+"*.*")
	if nil != err {
		t.Fatalf("list queue keys: %v", err)
	}
	count := 0
	for range lister.Keys() {
		count++
	}
	if 0 != count {
		t.Fatalf("queue keys = %d, want 0", count)
	}
}
