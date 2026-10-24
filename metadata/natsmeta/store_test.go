package natsmeta

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/metadata"
	"github.com/ebogdum/callfs/v2/natsrv/natstest"
)

func startStores(t *testing.T) []*Store {
	t.Helper()
	nodes := natstest.StartCluster(t, 3)
	for _, node := range nodes {
		natstest.WaitJetStream(t, node.Conn)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	js0, err := jetstream.New(nodes[0].Conn)
	if nil != err {
		t.Fatalf("jetstream context: %v", err)
	}
	if _, err := js0.CreateStream(ctx, jetstream.StreamConfig{
		Name:               StreamName,
		Subjects:           []string{Subject},
		Replicas:           3,
		Storage:            jetstream.FileStorage,
		Duplicates:         2 * time.Minute,
		AllowAtomicPublish: true,
	}); nil != err {
		t.Fatalf("create stream: %v", err)
	}

	cfg := config.NATSMetadataConfig{
		Replicas:     3,
		ApplyTimeout: 15 * time.Second,
		ReadBarrier:  config.NATSReadBarrierInfo,
		MissBarrier:  true,
		MaxMsgSize:   8 * config.MiB,
	}
	stores := make([]*Store, 0, len(nodes))
	for i, node := range nodes {
		js, err := jetstream.New(node.Conn)
		if nil != err {
			t.Fatalf("jetstream context n%d: %v", i, err)
		}
		store, err := Open(ctx, node.Conn, js, fmt.Sprintf("n%d", i), cfg, zap.NewNop())
		if nil != err {
			t.Fatalf("open store n%d: %v", i, err)
		}
		t.Cleanup(func() { _ = store.Close() })
		stores = append(stores, store)
	}
	return stores
}

func fileMeta(path, etag string) *metadata.Metadata {
	return &metadata.Metadata{Path: path, Name: path[1:], Type: "file", ETag: etag, Owner: "u1"}
}

func dirMeta(path string) *metadata.Metadata {
	return &metadata.Metadata{Path: path, Name: path[1:], Type: "directory", Owner: "u1"}
}

func syncAll(t *testing.T, stores []*Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for i, store := range stores {
		if err := store.barrier(ctx); nil != err {
			t.Fatalf("barrier n%d: %v", i, err)
		}
	}
}

func TestCreateOnOneNodeReadOnAnother(t *testing.T) {
	stores := startStores(t)
	ctx := context.Background()

	if err := stores[0].Create(ctx, fileMeta("/a", "e0")); nil != err {
		t.Fatalf("create: %v", err)
	}
	if err := stores[1].barrier(ctx); nil != err {
		t.Fatalf("barrier: %v", err)
	}
	md, err := stores[1].Get(ctx, "/a")
	if nil != err {
		t.Fatalf("get on n1: %v", err)
	}
	if "e0" != md.ETag {
		t.Fatalf("etag = %q, want e0", md.ETag)
	}
	if err := stores[2].Create(ctx, fileMeta("/a", "e1")); !errors.Is(err, metadata.ErrAlreadyExists) {
		t.Fatalf("duplicate create err = %v, want ErrAlreadyExists", err)
	}
}

func TestConcurrentUpdateIfMatchSingleWinner(t *testing.T) {
	stores := startStores(t)
	ctx := context.Background()

	if err := stores[0].Create(ctx, fileMeta("/c", "e0")); nil != err {
		t.Fatalf("create: %v", err)
	}
	syncAll(t, stores)

	errs := make([]error, len(stores))
	var wg sync.WaitGroup
	for i, store := range stores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = store.UpdateIfMatch(ctx, fileMeta("/c", fmt.Sprintf("w%d", i)), "e0")
		}()
	}
	wg.Wait()

	winners := 0
	for i, err := range errs {
		switch {
		case nil == err:
			winners++
		case errors.Is(err, metadata.ErrPrecondition):
		default:
			t.Fatalf("n%d unexpected error: %v", i, err)
		}
	}
	if 1 != winners {
		t.Fatalf("winners = %d, want 1 (errs: %v)", winners, errs)
	}
}

func TestRenameSubtree(t *testing.T) {
	stores := startStores(t)
	ctx := context.Background()

	for _, md := range []*metadata.Metadata{
		dirMeta("/d"),
		fileMeta("/d/f", "ef"),
		dirMeta("/d/s"),
		fileMeta("/d/s/g", "eg"),
	} {
		if err := stores[0].Create(ctx, md); nil != err {
			t.Fatalf("create %s: %v", md.Path, err)
		}
	}
	if err := stores[1].Rename(ctx, "/d", "/e"); nil != err {
		t.Fatalf("rename: %v", err)
	}
	syncAll(t, stores)

	for i, store := range stores {
		for _, path := range []string{"/e", "/e/f", "/e/s", "/e/s/g"} {
			if _, err := store.Get(ctx, path); nil != err {
				t.Fatalf("n%d get %s: %v", i, path, err)
			}
		}
		for _, path := range []string{"/d", "/d/f", "/d/s", "/d/s/g"} {
			if _, err := store.Get(ctx, path); !errors.Is(err, metadata.ErrNotFound) {
				t.Fatalf("n%d get %s err = %v, want ErrNotFound", i, path, err)
			}
		}
	}
}

func TestApplyBatchFailureLeavesStateUntouched(t *testing.T) {
	stores := startStores(t)
	ctx := context.Background()

	if err := stores[0].Create(ctx, fileMeta("/x", "ex")); nil != err {
		t.Fatalf("create: %v", err)
	}
	err := stores[2].ApplyBatch(ctx, []metadata.Op{
		{Kind: metadata.OpCreate, Metadata: fileMeta("/y", "ey")},
		{Kind: metadata.OpCreate, Metadata: fileMeta("/x", "dup")},
	})
	if !errors.Is(err, metadata.ErrAlreadyExists) {
		t.Fatalf("batch err = %v, want ErrAlreadyExists", err)
	}
	syncAll(t, stores)

	for i, store := range stores {
		if _, err := store.Get(ctx, "/y"); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("n%d get /y err = %v, want ErrNotFound", i, err)
		}
		md, getErr := store.Get(ctx, "/x")
		if nil != getErr || "ex" != md.ETag {
			t.Fatalf("n%d /x changed by failed batch: %v %v", i, md, getErr)
		}
		if stores[0].StateHash() != store.StateHash() {
			t.Fatalf("n%d state diverged from n0", i)
		}
	}

	if err := stores[1].ApplyBatch(ctx, []metadata.Op{
		{Kind: metadata.OpCreate, Metadata: fileMeta("/y", "ey")},
		{Kind: metadata.OpDelete, Path: "/x"},
	}); nil != err {
		t.Fatalf("batch: %v", err)
	}
	syncAll(t, stores)
	for i, store := range stores {
		if _, err := store.Get(ctx, "/y"); nil != err {
			t.Fatalf("n%d get /y: %v", i, err)
		}
		if _, err := store.Get(ctx, "/x"); !errors.Is(err, metadata.ErrNotFound) {
			t.Fatalf("n%d get /x err = %v, want ErrNotFound", i, err)
		}
	}
}

func TestStateHashEqualOnAllNodes(t *testing.T) {
	stores := startStores(t)
	ctx := context.Background()

	for i := 0; 30 > i; i++ {
		store := stores[i%len(stores)]
		if err := store.Create(ctx, fileMeta(fmt.Sprintf("/h%d", i), fmt.Sprintf("e%d", i))); nil != err {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if err := stores[1].Delete(ctx, "/h3"); nil != err {
		t.Fatalf("delete: %v", err)
	}
	if err := stores[2].Rename(ctx, "/h4", "/renamed"); nil != err {
		t.Fatalf("rename: %v", err)
	}
	syncAll(t, stores)

	want := stores[0].StateHash()
	for i, store := range stores[1:] {
		if want != store.StateHash() {
			t.Fatalf("state hash on n%d differs from n0", i+1)
		}
	}
}
