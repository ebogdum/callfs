package localbus

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ebogdum/callfs/v2/cluster"
)

func fastBackoff() []time.Duration {
	return []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}
}

func TestQueueDelivery(t *testing.T) {
	l := newBus(t, Options{Self: cluster.Member{ID: "n1"}})
	got := make(chan *cluster.Task, 2)
	stop := l.Consume("replicate", 2, func(_ context.Context, task *cluster.Task) error {
		got <- task
		return nil
	})
	defer stop()
	if err := l.Enqueue(context.Background(), "replicate", "n1", "t1", echo{Value: "x"}); nil != err {
		t.Fatal(err)
	}
	select {
	case task := <-got:
		var in echo
		if err := task.Decode(&in); nil != err || "x" != in.Value || "t1" != task.ID || 1 != task.Attempt || "replicate" != task.Pool || "n1" != task.Affinity {
			t.Fatalf("unexpected task %+v %v", task, err)
		}
		if err := task.InProgress(); nil != err {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no delivery")
	}
}

func TestQueueEnqueueBeforeConsume(t *testing.T) {
	l := newBus(t, Options{})
	if err := l.Enqueue(context.Background(), "p", "", "t1", nil); nil != err {
		t.Fatal(err)
	}
	done := make(chan struct{})
	stop := l.Consume("p", 1, func(context.Context, *cluster.Task) error {
		close(done)
		return nil
	})
	defer stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("no delivery")
	}
}

func TestQueueDedup(t *testing.T) {
	l := newBus(t, Options{})
	var runs atomic.Int32
	stop := l.Consume("p", 1, func(context.Context, *cluster.Task) error {
		runs.Add(1)
		return nil
	})
	defer stop()
	for i := 0; i < 3; i++ {
		if err := l.Enqueue(context.Background(), "p", "", "same", nil); nil != err {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if 1 != runs.Load() {
		t.Fatalf("ran %d times", runs.Load())
	}
}

func TestQueueRetryThenSuccess(t *testing.T) {
	l := newBus(t, Options{TaskBackoff: fastBackoff()})
	attempts := make(chan int, 8)
	stop := l.Consume("p", 1, func(_ context.Context, task *cluster.Task) error {
		attempts <- task.Attempt
		if 3 > task.Attempt {
			return errors.New("transient")
		}
		return nil
	})
	defer stop()
	if err := l.Enqueue(context.Background(), "p", "", "t", nil); nil != err {
		t.Fatal(err)
	}
	for want := 1; want <= 3; want++ {
		select {
		case got := <-attempts:
			if want != got {
				t.Fatalf("attempt %d, want %d", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("missing attempt %d", want)
		}
	}
}

func TestQueueDeadLetterAfterMaxDeliver(t *testing.T) {
	dead := make(chan cluster.DeadLetter, 1)
	l := newBus(t, Options{
		TaskBackoff:    fastBackoff(),
		TaskMaxDeliver: 3,
		OnDeadLetter:   func(d cluster.DeadLetter) { dead <- d },
	})
	var runs atomic.Int32
	stop := l.Consume("p", 1, func(context.Context, *cluster.Task) error {
		runs.Add(1)
		return errors.New("boom")
	})
	defer stop()
	if err := l.Enqueue(context.Background(), "p", "", "t", echo{Value: "b"}); nil != err {
		t.Fatal(err)
	}
	select {
	case d := <-dead:
		if "p" != d.Pool || "t" != d.ID || 3 != d.Attempts || "boom" != d.Error || 0 == len(d.Body) || d.FailedAt.IsZero() {
			t.Fatalf("unexpected dead letter %+v", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no dead letter")
	}
	if 3 != runs.Load() {
		t.Fatalf("ran %d times", runs.Load())
	}
}

func TestQueuePermanentDeadLettersImmediately(t *testing.T) {
	dead := make(chan cluster.DeadLetter, 1)
	l := newBus(t, Options{OnDeadLetter: func(d cluster.DeadLetter) { dead <- d }})
	stop := l.Consume("p", 1, func(context.Context, *cluster.Task) error {
		return cluster.Permanent(errors.New("bad input"))
	})
	defer stop()
	if err := l.Enqueue(context.Background(), "p", "", "t", nil); nil != err {
		t.Fatal(err)
	}
	select {
	case d := <-dead:
		if 1 != d.Attempts || "bad input" != d.Error {
			t.Fatalf("unexpected dead letter %+v", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no dead letter")
	}
}

func TestQueueFullAndValidation(t *testing.T) {
	l := newBus(t, Options{TaskMaxPending: 1, Self: cluster.Member{ID: "n1"}})
	ctx := context.Background()
	if err := l.Enqueue(ctx, "p", "", "a", nil); nil != err {
		t.Fatal(err)
	}
	if err := l.Enqueue(ctx, "p", "", "b", nil); !errors.Is(err, cluster.ErrQueueFull) {
		t.Fatalf("expected full, got %v", err)
	}
	if err := l.Enqueue(ctx, "bad pool", "", "a", nil); !errors.Is(err, cluster.ErrInvalidName) {
		t.Fatalf("pool: %v", err)
	}
	if err := l.Enqueue(ctx, "q", "", "", nil); !errors.Is(err, cluster.ErrInvalidTask) {
		t.Fatalf("id: %v", err)
	}
	if err := l.Enqueue(ctx, "q", "n2", "x", nil); !errors.Is(err, cluster.ErrNoResponders) {
		t.Fatalf("affinity: %v", err)
	}
}

func TestQueueStopHaltsDelivery(t *testing.T) {
	l := newBus(t, Options{})
	var runs atomic.Int32
	stop := l.Consume("p", 1, func(context.Context, *cluster.Task) error {
		runs.Add(1)
		return nil
	})
	stop()
	if err := l.Enqueue(context.Background(), "p", "", "t", nil); nil != err {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if 0 != runs.Load() {
		t.Fatalf("delivered after stop: %d", runs.Load())
	}
}

func TestQueueHandlerPanicRetried(t *testing.T) {
	done := make(chan struct{})
	l := newBus(t, Options{TaskBackoff: fastBackoff()})
	stop := l.Consume("p", 1, func(_ context.Context, task *cluster.Task) error {
		if 1 == task.Attempt {
			panic("oops")
		}
		close(done)
		return nil
	})
	defer stop()
	if err := l.Enqueue(context.Background(), "p", "", "t", nil); nil != err {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("not retried after panic")
	}
}
