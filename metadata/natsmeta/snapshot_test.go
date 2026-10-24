package natsmeta

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"

	"github.com/ebogdum/callfs/v2/cluster"
	"github.com/ebogdum/callfs/v2/config"
	"github.com/ebogdum/callfs/v2/natsrv/natstest"
)

type snapshotCluster struct {
	nodes  []*natstest.Node
	stores []*Store
	js     jetstream.JetStream
	cfg    config.NATSMetadataConfig
}

func startSnapshotCluster(t *testing.T, retainCommands uint64, retainSnapshots int) *snapshotCluster {
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
		DenyDelete:         true,
		AllowAtomicPublish: true,
	}); nil != err {
		t.Fatalf("create stream: %v", err)
	}
	if _, err := js0.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:   SnapshotBucket,
		Storage:  jetstream.FileStorage,
		Replicas: 3,
	}); nil != err {
		t.Fatalf("create snapshot bucket: %v", err)
	}
	if _, err := js0.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:         cluster.BucketLeases,
		History:        1,
		LimitMarkerTTL: time.Second,
		Storage:        jetstream.MemoryStorage,
		Replicas:       3,
	}); nil != err {
		t.Fatalf("create lease bucket: %v", err)
	}

	sc := &snapshotCluster{
		nodes: nodes,
		js:    js0,
		cfg: config.NATSMetadataConfig{
			Replicas:        3,
			ApplyTimeout:    15 * time.Second,
			ReadBarrier:     config.NATSReadBarrierInfo,
			MissBarrier:     true,
			MaxMsgSize:      8 * config.MiB,
			RetainCommands:  retainCommands,
			RetainSnapshots: retainSnapshots,
		},
	}
	for i := range nodes {
		sc.stores = append(sc.stores, sc.open(t, i, fmt.Sprintf("n%d", i)))
	}
	return sc
}

func (sc *snapshotCluster) open(t *testing.T, node int, self string) *Store {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	js, err := jetstream.New(sc.nodes[node].Conn)
	if nil != err {
		t.Fatalf("jetstream context %s: %v", self, err)
	}
	store, err := Open(ctx, sc.nodes[node].Conn, js, self, sc.cfg, zap.NewNop())
	if nil != err {
		t.Fatalf("open store %s: %v", self, err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func createFiles(t *testing.T, store *Store, prefix string, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; n > i; i++ {
		path := fmt.Sprintf("/%s%d", prefix, i)
		if err := store.Create(ctx, fileMeta(path, "e0")); nil != err {
			t.Fatalf("create %s: %v", path, err)
		}
	}
}

func TestSnapshotPurgeThenFreshOpenRestoresState(t *testing.T) {
	sc := startSnapshotCluster(t, 2, 3)
	ctx := context.Background()

	createFiles(t, sc.stores[0], "a", 10)
	syncAll(t, sc.stores)

	snap, err := sc.stores[0].TakeSnapshot(ctx)
	if nil != err {
		t.Fatalf("take snapshot: %v", err)
	}
	if sc.stores[0].AppliedSeq() != snap.Seq {
		t.Fatalf("snapshot seq = %d, want %d", snap.Seq, sc.stores[0].AppliedSeq())
	}
	if snapshotName(snap.Seq) != snap.ID {
		t.Fatalf("snapshot id = %q, want %q", snap.ID, snapshotName(snap.Seq))
	}

	info, err := sc.stores[0].LogInfo(ctx)
	if nil != err {
		t.Fatalf("log info: %v", err)
	}
	if snap.Seq-2 != info.FirstSeq {
		t.Fatalf("first seq after purge = %d, want %d", info.FirstSeq, snap.Seq-2)
	}
	if nil == info.LastSnapshot || snap.Seq != info.LastSnapshot.Seq {
		t.Fatalf("last snapshot = %+v, want seq %d", info.LastSnapshot, snap.Seq)
	}

	createFiles(t, sc.stores[1], "b", 3)
	syncAll(t, sc.stores)

	fresh := sc.open(t, 2, "n9")
	if err := fresh.barrier(ctx); nil != err {
		t.Fatalf("barrier fresh: %v", err)
	}
	if sc.stores[0].AppliedSeq() != fresh.AppliedSeq() {
		t.Fatalf("fresh applied seq = %d, want %d", fresh.AppliedSeq(), sc.stores[0].AppliedSeq())
	}
	if sc.stores[0].StateHash() != fresh.StateHash() {
		t.Fatalf("fresh state hash differs from n0")
	}
	if _, err := fresh.Get(ctx, "/a0"); nil != err {
		t.Fatalf("get /a0 on fresh store: %v", err)
	}
}

func TestCorruptNewestSnapshotFallsBack(t *testing.T) {
	sc := startSnapshotCluster(t, 1000, 3)
	ctx := context.Background()

	createFiles(t, sc.stores[0], "a", 5)
	syncAll(t, sc.stores)
	older, err := sc.stores[0].TakeSnapshot(ctx)
	if nil != err {
		t.Fatalf("take older snapshot: %v", err)
	}

	createFiles(t, sc.stores[0], "b", 5)
	syncAll(t, sc.stores)
	newest, err := sc.stores[0].TakeSnapshot(ctx)
	if nil != err {
		t.Fatalf("take newest snapshot: %v", err)
	}

	obj, err := sc.js.ObjectStore(ctx, SnapshotBucket)
	if nil != err {
		t.Fatalf("open snapshot bucket: %v", err)
	}
	current, err := obj.GetInfo(ctx, newest.ID)
	if nil != err {
		t.Fatalf("snapshot info: %v", err)
	}
	if _, err := obj.PutBytes(ctx, newest.ID, []byte(`{"corrupt":true}`)); nil != err {
		t.Fatalf("overwrite snapshot: %v", err)
	}
	if err := obj.UpdateMeta(ctx, newest.ID, current.ObjectMeta); nil != err {
		t.Fatalf("restore snapshot metadata: %v", err)
	}

	snaps, err := sc.stores[0].Snapshots(ctx)
	if nil != err {
		t.Fatalf("list snapshots: %v", err)
	}
	if 2 != len(snaps) || newest.Seq != snaps[0].Seq || older.Seq != snaps[1].Seq {
		t.Fatalf("snapshots = %+v, want seqs %d then %d", snaps, newest.Seq, older.Seq)
	}

	fresh := sc.open(t, 1, "n9")
	if err := fresh.barrier(ctx); nil != err {
		t.Fatalf("barrier fresh: %v", err)
	}
	if sc.stores[0].StateHash() != fresh.StateHash() {
		t.Fatalf("fresh state hash differs from n0 after fallback")
	}
	if _, err := fresh.Get(ctx, "/b4"); nil != err {
		t.Fatalf("get /b4 on fresh store: %v", err)
	}
}

func TestTakeSnapshotRetainAndLease(t *testing.T) {
	sc := startSnapshotCluster(t, 1000, 2)
	ctx := context.Background()

	if 2 != sc.stores[0].RetainSnapshots() {
		t.Fatalf("retain = %d, want 2", sc.stores[0].RetainSnapshots())
	}
	var taken []SnapshotInfo
	for round := 0; 3 > round; round++ {
		createFiles(t, sc.stores[round], fmt.Sprintf("r%d-", round), 2)
		syncAll(t, sc.stores)
		snap, err := sc.stores[round].TakeSnapshot(ctx)
		if nil != err {
			t.Fatalf("take snapshot round %d: %v", round, err)
		}
		taken = append(taken, snap)
	}

	snaps, err := sc.stores[1].Snapshots(ctx)
	if nil != err {
		t.Fatalf("list snapshots: %v", err)
	}
	if 2 != len(snaps) {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}
	if taken[2].Seq != snaps[0].Seq || taken[1].Seq != snaps[1].Seq {
		t.Fatalf("snapshots = %+v, want newest first %d, %d", snaps, taken[2].Seq, taken[1].Seq)
	}
	if snapshotName(snaps[0].Seq) != snaps[0].ID || 0 >= snaps[0].Bytes || snaps[0].CreatedAt.IsZero() {
		t.Fatalf("newest snapshot info incomplete: %+v", snaps[0])
	}

	kv, err := sc.js.KeyValue(ctx, cluster.BucketLeases)
	if nil != err {
		t.Fatalf("open lease bucket: %v", err)
	}
	rev, err := kv.Create(ctx, SnapshotLeaseKey, []byte("other"), jetstream.KeyTTL(time.Minute))
	if nil != err {
		t.Fatalf("hold lease: %v", err)
	}
	createFiles(t, sc.stores[0], "held", 1)
	if _, err := sc.stores[0].TakeSnapshot(ctx); !errors.Is(err, ErrSnapshotLeaseHeld) {
		t.Fatalf("take snapshot with held lease err = %v, want ErrSnapshotLeaseHeld", err)
	}
	if err := kv.Delete(ctx, SnapshotLeaseKey, jetstream.LastRevision(rev)); nil != err {
		t.Fatalf("release lease: %v", err)
	}
	snap, err := sc.stores[0].TakeSnapshot(ctx)
	if nil != err {
		t.Fatalf("take snapshot after release: %v", err)
	}
	if sc.stores[0].AppliedSeq() != snap.Seq {
		t.Fatalf("snapshot seq = %d, want %d", snap.Seq, sc.stores[0].AppliedSeq())
	}
}
