package localbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ebogdum/callfs/cluster"
)

func TestLeaseHeldAndRelease(t *testing.T) {
	l := newBus(t, Options{})
	held, release, err := l.Hold(context.Background(), "meta_snapshot", time.Second)
	if nil != err {
		t.Fatal(err)
	}
	select {
	case <-held:
		t.Fatal("held closed early")
	default:
	}
	release()
	release()
	select {
	case <-held:
	case <-time.After(time.Second):
		t.Fatal("held not closed after release")
	}
	held2, release2, err := l.Hold(context.Background(), "meta_snapshot", time.Second)
	if nil != err {
		t.Fatal(err)
	}
	defer release2()
	select {
	case <-held2:
		t.Fatal("second lease closed")
	default:
	}
}

func TestLeaseExclusive(t *testing.T) {
	l := newBus(t, Options{})
	_, release, err := l.Hold(context.Background(), "w", time.Second)
	if nil != err {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, err := l.Hold(ctx, "w", time.Second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	acquired := make(chan error, 1)
	go func() {
		_, rel, holdErr := l.Hold(context.Background(), "w", time.Second)
		if nil == holdErr {
			rel()
		}
		acquired <- holdErr
	}()
	release()
	select {
	case err := <-acquired:
		if nil != err {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter never acquired")
	}
}

func TestLeaseContextCancelReleases(t *testing.T) {
	l := newBus(t, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	held, _, err := l.Hold(ctx, "w", time.Second)
	if nil != err {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-held:
	case <-time.After(time.Second):
		t.Fatal("held not closed after cancel")
	}
}

func TestLeaseValidation(t *testing.T) {
	l := newBus(t, Options{})
	if _, _, err := l.Hold(context.Background(), "bad name", time.Second); !errors.Is(err, cluster.ErrInvalidName) {
		t.Fatalf("name: %v", err)
	}
	if _, _, err := l.Hold(context.Background(), "w", 0); !errors.Is(err, cluster.ErrInvalidTTL) {
		t.Fatalf("ttl: %v", err)
	}
}

func TestLeaseClosedBus(t *testing.T) {
	l := New(Options{})
	held, _, err := l.Hold(context.Background(), "w", time.Second)
	if nil != err {
		t.Fatal(err)
	}
	_ = l.Close()
	select {
	case <-held:
	case <-time.After(time.Second):
		t.Fatal("held not closed after bus close")
	}
	if _, _, err := l.Hold(context.Background(), "w", time.Second); !errors.Is(err, cluster.ErrClosed) {
		t.Fatalf("hold after close: %v", err)
	}
}
