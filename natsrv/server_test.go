package natsrv

import (
	"context"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ebogdum/callfs/config"
)

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if nil != err {
		t.Fatalf("reserve loopback address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); nil != err {
		t.Fatalf("release loopback address: %v", err)
	}
	return addr
}

func testNATSConfig(t *testing.T) config.NATSConfig {
	t.Helper()
	return config.NATSConfig{
		Enabled:         true,
		Mode:            config.NATSModeEmbedded,
		ServerName:      "t1",
		ClusterName:     "callfs-test",
		ClusterListen:   freeLoopbackAddr(t),
		Routes:          []string{},
		RoutesFromPeers: true,
		ServerTags:      make(map[string]string),
		Auth: config.NATSAuthConfig{
			Mode: config.NATSAuthDerived,
		},
		JetStream: config.NATSJetStreamConfig{
			StoreDir:     t.TempDir(),
			MaxMemory:    256 * config.MiB,
			Sync:         config.NATSSyncInterval,
			SyncInterval: time.Minute,
		},
		External: config.NATSExternalConfig{
			URLs: []string{},
		},
		RPCTimeout: 10 * time.Second,
		Metadata: config.NATSMetadataConfig{
			Replicas:         3,
			ApplyTimeout:     10 * time.Second,
			ReadBarrier:      config.NATSReadBarrierInfo,
			MissBarrier:      true,
			SnapshotInterval: 5 * time.Minute,
			SnapshotEvery:    50000,
			RetainCommands:   100000,
			RetainSnapshots:  3,
			MaxMsgSize:       8 * config.MiB,
		},
		Locks: config.NATSLocksConfig{
			Replicas:   3,
			TTL:        15 * time.Second,
			QueueStale: 3 * time.Second,
		},
		Streams: config.NATSStreamsConfig{
			Replicas:        3,
			EventsMaxAge:    168 * time.Hour,
			EventsMaxBytes:  config.GiB,
			AuditMaxAge:     2160 * time.Hour,
			LogsLevel:       "warn",
			LogsMaxAge:      72 * time.Hour,
			TasksMaxDeliver: 8,
			TasksBackoff:    []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute},
			DLQMaxAge:       720 * time.Hour,
		},
		Bytes: config.NATSBytesHTTP,
	}
}

func TestStartProvisionAndAtomicBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := testNATSConfig(t)
	s, err := Start(ctx, cfg, "test-secret-0123456789abcdef", nil, zap.NewNop())
	if nil != err {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := s.Shutdown(shutdownCtx); nil != err {
			t.Errorf("shutdown: %v", err)
		}
	})

	top := DesiredTopology(cfg, 1)
	if err := Provision(ctx, s.JetStream(), top); nil != err {
		t.Fatalf("provision: %v", err)
	}
	if err := Provision(ctx, s.JetStream(), top); nil != err {
		t.Fatalf("provision again: %v", err)
	}

	if _, err := PublishAtomic(ctx, s.Conn(), "callfs.meta.log", [][]byte{[]byte("a"), []byte("b")}); nil != err {
		t.Fatalf("publish atomic batch: %v", err)
	}

	stream, err := s.JetStream().Stream(ctx, "CALLFS_META")
	if nil != err {
		t.Fatalf("lookup CALLFS_META: %v", err)
	}
	info, err := stream.Info(ctx)
	if nil != err {
		t.Fatalf("stream info: %v", err)
	}
	if 2 != info.State.Msgs {
		t.Fatalf("CALLFS_META messages: got %d, want 2", info.State.Msgs)
	}
}
