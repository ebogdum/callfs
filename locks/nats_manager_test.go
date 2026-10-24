package locks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ebogdum/callfs/natsrv/natstest"
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
		m, err := NewNATSManager(ctx, js, "node", ttl, zap.NewNop())
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
