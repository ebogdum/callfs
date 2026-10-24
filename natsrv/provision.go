package natsrv

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/ebogdum/callfs/v2/config"
)

type Topology struct {
	Streams   []jetstream.StreamConfig
	KeyValues []jetstream.KeyValueConfig
	Objects   []jetstream.ObjectStoreConfig
}

func capReplicas(requested int, configured int) int {
	r := requested
	if configured < r {
		r = configured
	}
	if 1 > r {
		return 1
	}
	return r
}

func DesiredTopology(cfg config.NATSConfig, replicas int) Topology {
	metaReplicas := capReplicas(replicas, cfg.Metadata.Replicas)
	locksReplicas := capReplicas(replicas, cfg.Locks.Replicas)
	streamsReplicas := capReplicas(replicas, cfg.Streams.Replicas)

	streams := []jetstream.StreamConfig{
		{
			Name:               "CALLFS_META",
			Subjects:           []string{"callfs.meta.log"},
			Storage:            jetstream.FileStorage,
			Retention:          jetstream.LimitsPolicy,
			MaxMsgs:            -1,
			MaxBytes:           -1,
			Discard:            jetstream.DiscardNew,
			Duplicates:         2 * time.Minute,
			MaxMsgSize:         int32(cfg.Metadata.MaxMsgSize.Int64()),
			AllowDirect:        false,
			DenyDelete:         true,
			AllowAtomicPublish: true,
			Replicas:           metaReplicas,
		},
		{
			Name:       "CALLFS_CHANGES",
			Subjects:   []string{"callfs.changes.>"},
			Storage:    jetstream.FileStorage,
			Retention:  jetstream.LimitsPolicy,
			MaxAge:     7 * 24 * time.Hour,
			Duplicates: 2 * time.Minute,
			Replicas:   streamsReplicas,
		},
		{
			Name:     "CALLFS_EVENTS",
			Subjects: []string{"callfs.tel.events.>"},
			Storage:  jetstream.FileStorage,
			MaxAge:   cfg.Streams.EventsMaxAge,
			MaxBytes: cfg.Streams.EventsMaxBytes.Int64(),
			Replicas: streamsReplicas,
		},
		{
			Name:     "CALLFS_AUDIT",
			Subjects: []string{"callfs.tel.audit.>"},
			Storage:  jetstream.FileStorage,
			MaxAge:   cfg.Streams.AuditMaxAge,
			Replicas: streamsReplicas,
		},
		{
			Name:     "CALLFS_LOGS",
			Subjects: []string{"callfs.tel.logs.>"},
			Storage:  jetstream.MemoryStorage,
			MaxAge:   cfg.Streams.LogsMaxAge,
			Replicas: 1,
		},
		{
			Name:       "CALLFS_TASKS",
			Subjects:   []string{"callfs.tasks.>"},
			Storage:    jetstream.FileStorage,
			Retention:  jetstream.WorkQueuePolicy,
			Discard:    jetstream.DiscardNew,
			Duplicates: 10 * time.Minute,
			Replicas:   streamsReplicas,
		},
		{
			Name:     "CALLFS_DLQ",
			Subjects: []string{"callfs.dlq.>"},
			Storage:  jetstream.FileStorage,
			MaxAge:   cfg.Streams.DLQMaxAge,
			Replicas: streamsReplicas,
		},
	}

	keyValues := []jetstream.KeyValueConfig{
		{
			Bucket:   "CALLFS_NODES",
			TTL:      0,
			History:  1,
			Storage:  jetstream.FileStorage,
			Replicas: streamsReplicas,
		},
		{
			Bucket:         "CALLFS_LOCKS",
			History:        1,
			LimitMarkerTTL: time.Second,
			Storage:        jetstream.MemoryStorage,
			Replicas:       locksReplicas,
		},
		{
			Bucket:         "CALLFS_LEASES",
			History:        1,
			LimitMarkerTTL: time.Second,
			Storage:        jetstream.MemoryStorage,
			Replicas:       streamsReplicas,
		},
		{
			Bucket:   "CALLFS_JOBS",
			History:  1,
			Storage:  jetstream.FileStorage,
			Replicas: streamsReplicas,
		},
		{
			Bucket:   "CALLFS_UPLOADS",
			History:  1,
			Storage:  jetstream.FileStorage,
			Replicas: streamsReplicas,
		},
		{
			Bucket:   "CALLFS_CONFIG",
			History:  5,
			Storage:  jetstream.FileStorage,
			Replicas: streamsReplicas,
		},
		{
			Bucket:   "CALLFS_STATS",
			History:  1,
			Storage:  jetstream.FileStorage,
			Replicas: streamsReplicas,
		},
	}

	objects := []jetstream.ObjectStoreConfig{
		{
			Bucket:   "CALLFS_META_SNAP",
			Storage:  jetstream.FileStorage,
			Replicas: metaReplicas,
		},
	}

	return Topology{
		Streams:   streams,
		KeyValues: keyValues,
		Objects:   objects,
	}
}

func Provision(ctx context.Context, js jetstream.JetStream, top Topology) error {
	var failures []error
	for _, streamCfg := range top.Streams {
		if _, err := js.CreateOrUpdateStream(ctx, streamCfg); nil != err {
			failures = append(failures, fmt.Errorf("provision stream %s: %w", streamCfg.Name, err))
		}
	}
	for _, kvCfg := range top.KeyValues {
		if _, err := js.CreateOrUpdateKeyValue(ctx, kvCfg); nil != err {
			failures = append(failures, fmt.Errorf("provision key value %s: %w", kvCfg.Bucket, err))
		}
	}
	for _, objCfg := range top.Objects {
		if _, err := js.CreateOrUpdateObjectStore(ctx, objCfg); nil != err {
			failures = append(failures, fmt.Errorf("provision object store %s: %w", objCfg.Bucket, err))
		}
	}
	return errors.Join(failures...)
}
