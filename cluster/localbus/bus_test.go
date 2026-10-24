package localbus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebogdum/callfs/cluster"
)

type echo struct {
	Value string `json:"value"`
}

func newBus(t *testing.T, opts Options) *Local {
	t.Helper()
	l := New(opts)
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func echoHandler(_ context.Context, req *cluster.Request) (any, error) {
	var in echo
	if err := req.Decode(&in); nil != err {
		return nil, err
	}
	return echo{Value: in.Value + ":" + req.From + ":" + req.OnBehalfOf}, nil
}

func TestCallSelfAndAny(t *testing.T) {
	l := newBus(t, Options{Self: cluster.Member{ID: "n1"}})
	if err := l.Serve("fs", echoHandler); nil != err {
		t.Fatal(err)
	}
	var out echo
	if err := l.Call(context.Background(), "n1", "fs", "stat", echo{Value: "a"}, &out, cluster.OnBehalfOf("alice")); nil != err {
		t.Fatal(err)
	}
	if "a:n1:alice" != out.Value {
		t.Fatalf("unexpected %q", out.Value)
	}
	if err := l.CallAny(context.Background(), "fs", "stat", echo{Value: "b"}, &out); nil != err {
		t.Fatal(err)
	}
	if "b:n1:" != out.Value {
		t.Fatalf("unexpected %q", out.Value)
	}
}

func TestCallErrors(t *testing.T) {
	l := newBus(t, Options{Self: cluster.Member{ID: "n1"}})
	if err := l.Serve("fs", func(context.Context, *cluster.Request) (any, error) {
		return nil, cluster.NewError(404, "not_found", "missing")
	}); nil != err {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := l.Call(ctx, "n2", "fs", "stat", nil, nil); !errors.Is(err, cluster.ErrNoResponders) {
		t.Fatalf("other node: %v", err)
	}
	if err := l.Call(ctx, "n1", "none", "stat", nil, nil); !errors.Is(err, cluster.ErrNoResponders) {
		t.Fatalf("unserved: %v", err)
	}
	if err := l.Call(ctx, "n1", "bad.name", "stat", nil, nil); !errors.Is(err, cluster.ErrInvalidName) {
		t.Fatalf("invalid name: %v", err)
	}
	if err := l.Call(ctx, "n1", "fs", "stat", nil, nil, cluster.OnBehalfOf("internal-proxy")); !errors.Is(err, cluster.ErrInvalidOnBehalf) {
		t.Fatalf("on behalf: %v", err)
	}
	err := l.Call(ctx, "n1", "fs", "stat", nil, nil)
	if 404 != cluster.StatusOf(err) || !errors.Is(err, cluster.NewError(404, "not_found", "")) {
		t.Fatalf("handler error: %v", err)
	}
	if err := l.Serve("fs", echoHandler); !errors.Is(err, cluster.ErrAlreadyServing) {
		t.Fatalf("duplicate serve: %v", err)
	}
}

func TestCallTimeout(t *testing.T) {
	l := newBus(t, Options{})
	release := make(chan struct{})
	defer close(release)
	if err := l.Serve("slow", func(ctx context.Context, _ *cluster.Request) (any, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, nil
	}); nil != err {
		t.Fatal(err)
	}
	err := l.Call(context.Background(), "", "slow", "x", nil, nil, cluster.WithTimeout(50*time.Millisecond))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestServeBusy(t *testing.T) {
	l := newBus(t, Options{})
	started := make(chan struct{})
	release := make(chan struct{})
	if err := l.Serve("svc", func(context.Context, *cluster.Request) (any, error) {
		close(started)
		<-release
		return nil, nil
	}, cluster.WithConcurrency(1)); nil != err {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() { first <- l.Call(context.Background(), "", "svc", "m", nil, nil) }()
	<-started
	if err := l.Call(context.Background(), "", "svc", "m", nil, nil); !errors.Is(err, cluster.ErrBusy) {
		t.Fatalf("expected busy, got %v", err)
	}
	close(release)
	if err := <-first; nil != err {
		t.Fatal(err)
	}
}

func TestIdempotentRetryOnBusy(t *testing.T) {
	l := newBus(t, Options{})
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	if err := l.Serve("svc", func(context.Context, *cluster.Request) (any, error) {
		once.Do(func() { close(started) })
		<-release
		return nil, nil
	}, cluster.WithConcurrency(1)); nil != err {
		t.Fatal(err)
	}
	go func() { _ = l.Call(context.Background(), "", "svc", "m", nil, nil) }()
	<-started
	go func() {
		time.Sleep(30 * time.Millisecond)
		close(release)
	}()
	if err := l.Call(context.Background(), "", "svc", "m", nil, nil, cluster.Idempotent()); nil != err {
		t.Fatalf("retry should succeed: %v", err)
	}
}

func TestIdempotencyKeyDedup(t *testing.T) {
	l := newBus(t, Options{})
	var calls atomic.Int32
	if err := l.Serve("cnt", func(context.Context, *cluster.Request) (any, error) {
		calls.Add(1)
		return nil, nil
	}); nil != err {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := l.Call(context.Background(), "", "cnt", "m", nil, nil, cluster.WithIdempotencyKey("k1")); nil != err {
			t.Fatal(err)
		}
	}
	if 1 != calls.Load() {
		t.Fatalf("handler ran %d times", calls.Load())
	}
}

func TestGather(t *testing.T) {
	l := newBus(t, Options{Self: cluster.Member{ID: "n1"}})
	if err := l.Gather(context.Background(), "none", "m", nil, func(string, []byte, error) {
		t.Fatal("unexpected callback")
	}); nil != err {
		t.Fatal(err)
	}
	if err := l.Serve("svc", echoHandler); nil != err {
		t.Fatal(err)
	}
	var nodes []string
	var got echo
	err := l.Gather(context.Background(), "svc", "m", echo{Value: "g"}, func(node string, raw []byte, err error) {
		nodes = append(nodes, node)
		if nil != err {
			t.Error(err)
			return
		}
		if decodeErr := cluster.Decode(raw, &got); nil != decodeErr {
			t.Error(decodeErr)
		}
	})
	if nil != err {
		t.Fatal(err)
	}
	if 1 != len(nodes) || "n1" != nodes[0] || "g:n1:" != got.Value {
		t.Fatalf("unexpected gather %v %q", nodes, got.Value)
	}
}

func TestBroadcastSubscribe(t *testing.T) {
	l := newBus(t, Options{Self: cluster.Member{ID: "n1"}})
	type received struct {
		from string
		raw  []byte
	}
	ch := make(chan received, 4)
	cancel, err := l.Subscribe("cache.inval", func(_ context.Context, from string, raw []byte) {
		ch <- received{from: from, raw: raw}
	})
	if nil != err {
		t.Fatal(err)
	}
	if err := l.Broadcast(context.Background(), "cache.inval", echo{Value: "p"}); nil != err {
		t.Fatal(err)
	}
	select {
	case r := <-ch:
		var in echo
		if decodeErr := cluster.Decode(r.raw, &in); nil != decodeErr || "p" != in.Value || "n1" != r.from {
			t.Fatalf("unexpected %v %v", r, decodeErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no delivery")
	}
	cancel()
	cancel()
	if err := l.Broadcast(context.Background(), "cache.inval", echo{Value: "q"}); nil != err {
		t.Fatal(err)
	}
	select {
	case r := <-ch:
		t.Fatalf("delivered after cancel: %v", r)
	case <-time.After(100 * time.Millisecond):
	}
	if err := l.Broadcast(context.Background(), "bad topic", nil); !errors.Is(err, cluster.ErrInvalidName) {
		t.Fatalf("invalid topic: %v", err)
	}
	if _, err := l.Subscribe("a..b", func(context.Context, string, []byte) {}); !errors.Is(err, cluster.ErrInvalidName) {
		t.Fatalf("invalid topic: %v", err)
	}
}

func TestClose(t *testing.T) {
	l := New(Options{})
	if err := l.Close(); nil != err {
		t.Fatal(err)
	}
	if err := l.Close(); nil != err {
		t.Fatal(err)
	}
	if err := l.Serve("svc", echoHandler); !errors.Is(err, cluster.ErrClosed) {
		t.Fatalf("serve after close: %v", err)
	}
	if err := l.Broadcast(context.Background(), "t", nil); !errors.Is(err, cluster.ErrClosed) {
		t.Fatalf("broadcast after close: %v", err)
	}
}

func TestPresence(t *testing.T) {
	l := newBus(t, Options{Self: cluster.Member{ID: "n1", APIEndpoint: "https://n1"}})
	self := l.Self()
	if "n1" != self.ID || "n1" != self.ServerName || self.StartedAt.IsZero() || self.Seen.IsZero() {
		t.Fatalf("unexpected self %+v", self)
	}
	members := l.Members()
	if 1 != len(members) || "n1" != members[0].ID {
		t.Fatalf("unexpected members %+v", members)
	}
	if !l.Alive("n1") || l.Alive("n2") {
		t.Fatal("alive mismatch")
	}
	stop := l.Watch(func(cluster.Event) {})
	stop()
}
