package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/v2"
)

const RaftRemovedVersion = "v2.0.0"

var ErrRaftRemoved = errors.New("raft cluster mode was removed in " + RaftRemovedVersion + "; set metadata_store.type and dlm.type to nats and configure the nats section")

const (
	natsMaxReplicas   = 5
	natsMinMsgSize    = 64 * KiB
	natsMaxMsgSize    = 64 * MiB
	natsMinLockTTL    = time.Second
	natsMinQueueStale = time.Second
)

var commaListKeys = map[string]bool{
	"nats.routes":                true,
	"nats.external.urls":         true,
	"nats.streams.tasks_backoff": true,
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if "" != trimmed {
			out = append(out, trimmed)
		}
	}
	return out
}

func rejectLegacyRaft(k *koanf.Koanf) error {
	if k.Exists("raft") {
		return ErrRaftRemoved
	}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "CALLFS_RAFT_") || "CALLFS_RAFT" == name {
			return ErrRaftRemoved
		}
	}
	if strings.EqualFold("raft", strings.TrimSpace(k.String("metadata_store.type"))) {
		return ErrRaftRemoved
	}
	if strings.EqualFold("raft", strings.TrimSpace(k.String("dlm.type"))) {
		return ErrRaftRemoved
	}
	return nil
}

func remotePeerEndpoints(cfg *AppConfig) map[string]string {
	remote := make(map[string]string, len(cfg.InstanceDiscovery.PeerEndpoints))
	for id, endpoint := range cfg.InstanceDiscovery.PeerEndpoints {
		if id != cfg.InstanceDiscovery.InstanceID {
			remote[id] = endpoint
		}
	}
	return remote
}

func validateNATSConfig(cfg *AppConfig) error {
	n := &cfg.NATS
	if !n.Enabled {
		if 0 < len(remotePeerEndpoints(cfg)) {
			return fmt.Errorf("instance_discovery.peer_endpoints requires nats.enabled=true")
		}
		return nil
	}

	n.Mode = strings.ToLower(strings.TrimSpace(n.Mode))
	n.Auth.Mode = strings.ToLower(strings.TrimSpace(n.Auth.Mode))
	n.JetStream.Sync = strings.ToLower(strings.TrimSpace(n.JetStream.Sync))
	n.Metadata.ReadBarrier = strings.ToLower(strings.TrimSpace(n.Metadata.ReadBarrier))
	n.Streams.LogsLevel = strings.ToLower(strings.TrimSpace(n.Streams.LogsLevel))
	n.Bytes = strings.ToLower(strings.TrimSpace(n.Bytes))

	if "" == strings.TrimSpace(n.ServerName) {
		n.ServerName = cfg.InstanceDiscovery.InstanceID
	}
	if strings.ContainsAny(n.ServerName, " \t\r\n") {
		return fmt.Errorf("nats.server_name must not contain whitespace")
	}
	if "" == strings.TrimSpace(n.ClusterName) || strings.ContainsAny(n.ClusterName, " \t\r\n.*>") {
		return fmt.Errorf("nats.cluster_name is required and must not contain whitespace, '.', '*' or '>'")
	}

	switch n.Mode {
	case NATSModeEmbedded, NATSModeExternal:
	default:
		return fmt.Errorf("nats.mode must be one of: embedded, external")
	}

	switch n.Auth.Mode {
	case NATSAuthDerived:
		if "" != n.Auth.PreviousSecret {
			if len(n.Auth.PreviousSecret) < 16 {
				return fmt.Errorf("nats.auth.previous_secret must be at least 16 characters")
			}
			if n.Auth.PreviousSecret == cfg.Auth.InternalProxySecret {
				return fmt.Errorf("nats.auth.previous_secret must differ from auth.internal_proxy_secret")
			}
		}
	case NATSAuthNKey:
		if "" == n.Auth.NKeySeedFile {
			return fmt.Errorf("nats.auth.nkey_seed_file is required when nats.auth.mode=nkey")
		}
	case NATSAuthCreds:
		if "" == n.Auth.CredsFile {
			return fmt.Errorf("nats.auth.creds_file is required when nats.auth.mode=creds")
		}
	default:
		return fmt.Errorf("nats.auth.mode must be one of: derived, nkey, creds")
	}

	if err := validateNATSTLS(n); err != nil {
		return err
	}

	if NATSModeExternal == n.Mode {
		if err := validateNATSExternal(n); err != nil {
			return err
		}
	} else if err := validateNATSEmbedded(n); err != nil {
		return err
	}

	if 0 >= n.RPCTimeout {
		return fmt.Errorf("nats.rpc_timeout must be > 0")
	}
	if err := validateNATSMetadata(n); err != nil {
		return err
	}
	if err := validateNATSLocks(n); err != nil {
		return err
	}
	if err := validateNATSStreams(n); err != nil {
		return err
	}

	switch n.Bytes {
	case NATSBytesHTTP, NATSBytesChunked:
	default:
		return fmt.Errorf("nats.bytes must be one of: http, chunked")
	}

	for key := range n.ServerTags {
		if "" == strings.TrimSpace(key) || strings.ContainsAny(key, ": \t") {
			return fmt.Errorf("nats.server_tags: tag name %q is invalid", key)
		}
	}

	return nil
}

func validateNATSTLS(n *NATSConfig) error {
	t := n.TLS
	if ("" == t.CertFile) != ("" == t.KeyFile) {
		return fmt.Errorf("nats.tls.cert_file and nats.tls.key_file must be set together")
	}
	if ("" == t.ClientCertFile) != ("" == t.ClientKeyFile) {
		return fmt.Errorf("nats.tls.client_cert_file and nats.tls.client_key_file must be set together")
	}
	if "" != t.CAFile && "" == t.CertFile {
		return fmt.Errorf("nats.tls.ca_file requires nats.tls.cert_file and nats.tls.key_file")
	}
	return nil
}

func validateNATSEmbedded(n *NATSConfig) error {
	if NATSAuthCreds == n.Auth.Mode {
		return fmt.Errorf("nats.auth.mode=creds requires nats.mode=external")
	}
	if "" == n.ClusterListen {
		return fmt.Errorf("nats.cluster_listen is required when nats.mode=embedded")
	}
	if err := validateHostPort("nats.cluster_listen", n.ClusterListen); err != nil {
		return err
	}
	if !isLoopbackBindAddr(n.ClusterListen) {
		if "" == n.TLS.CAFile || "" == n.TLS.CertFile || "" == n.TLS.KeyFile {
			return fmt.Errorf("nats.tls.cert_file, nats.tls.key_file and nats.tls.ca_file are required when nats.cluster_listen is not a loopback address")
		}
	}
	if "" != n.ClientListen {
		if err := validateHostPort("nats.client_listen", n.ClientListen); err != nil {
			return err
		}
		if !isLoopbackBindAddr(n.ClientListen) && ("" == n.TLS.CertFile || "" == n.TLS.KeyFile) {
			return fmt.Errorf("nats.tls.cert_file and nats.tls.key_file are required when nats.client_listen is not a loopback address")
		}
	}
	if "" != n.MonitorListen {
		if err := validateHostPort("nats.monitor_listen", n.MonitorListen); err != nil {
			return err
		}
	}
	for _, route := range n.Routes {
		parsed, err := url.Parse(route)
		if nil != err {
			return fmt.Errorf("nats.routes: %q is not a valid URL: %w", route, err)
		}
		scheme := strings.ToLower(parsed.Scheme)
		if "nats-route" != scheme && "tls-route" != scheme {
			return fmt.Errorf("nats.routes: %q must use the nats-route:// or tls-route:// scheme", route)
		}
		if "" == parsed.Hostname() || "" == parsed.Port() {
			return fmt.Errorf("nats.routes: %q must include host and port", route)
		}
	}

	js := n.JetStream
	if "" == js.StoreDir {
		return fmt.Errorf("nats.jetstream.store_dir is required when nats.mode=embedded")
	}
	if 0 > js.MaxMemory {
		return fmt.Errorf("nats.jetstream.max_memory must be >= 0")
	}
	if 0 > js.MaxStore {
		return fmt.Errorf("nats.jetstream.max_store must be >= 0")
	}
	switch js.Sync {
	case NATSSyncAlways:
	case NATSSyncInterval:
		if 0 >= js.SyncInterval {
			return fmt.Errorf("nats.jetstream.sync_interval must be > 0 when nats.jetstream.sync=interval")
		}
	default:
		return fmt.Errorf("nats.jetstream.sync must be one of: always, interval")
	}
	return nil
}

func validateNATSExternal(n *NATSConfig) error {
	if 0 == len(n.External.URLs) {
		return fmt.Errorf("nats.external.urls is required when nats.mode=external")
	}
	for _, raw := range n.External.URLs {
		parsed, err := url.Parse(raw)
		if nil != err {
			return fmt.Errorf("nats.external.urls: %q is not a valid URL: %w", raw, err)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "nats", "tls", "ws", "wss":
		default:
			return fmt.Errorf("nats.external.urls: %q must use the nats://, tls://, ws:// or wss:// scheme", raw)
		}
		if "" == parsed.Hostname() {
			return fmt.Errorf("nats.external.urls: %q must include a host", raw)
		}
	}
	if NATSAuthDerived == n.Auth.Mode {
		return fmt.Errorf("nats.auth.mode must be nkey or creds when nats.mode=external")
	}
	return nil
}

func validateNATSMetadata(n *NATSConfig) error {
	m := n.Metadata
	if m.Replicas < 1 || natsMaxReplicas < m.Replicas {
		return fmt.Errorf("nats.metadata.replicas must be between 1 and %d", natsMaxReplicas)
	}
	if 0 >= m.ApplyTimeout {
		return fmt.Errorf("nats.metadata.apply_timeout must be > 0")
	}
	switch m.ReadBarrier {
	case NATSReadBarrierInfo, NATSReadBarrierPublish:
	default:
		return fmt.Errorf("nats.metadata.read_barrier must be one of: info, publish")
	}
	if 0 >= m.SnapshotInterval {
		return fmt.Errorf("nats.metadata.snapshot_interval must be > 0")
	}
	if 0 == m.SnapshotEvery {
		return fmt.Errorf("nats.metadata.snapshot_every must be > 0")
	}
	if 0 == m.RetainCommands {
		return fmt.Errorf("nats.metadata.retain_commands must be > 0")
	}
	if 1 > m.RetainSnapshots {
		return fmt.Errorf("nats.metadata.retain_snapshots must be >= 1")
	}
	if m.MaxMsgSize < natsMinMsgSize || natsMaxMsgSize < m.MaxMsgSize {
		return fmt.Errorf("nats.metadata.max_msg_size must be between %s and %s", natsMinMsgSize, natsMaxMsgSize)
	}
	return nil
}

func validateNATSLocks(n *NATSConfig) error {
	l := n.Locks
	if l.Replicas < 1 || natsMaxReplicas < l.Replicas {
		return fmt.Errorf("nats.locks.replicas must be between 1 and %d", natsMaxReplicas)
	}
	if l.TTL < natsMinLockTTL {
		return fmt.Errorf("nats.locks.ttl must be >= %s", natsMinLockTTL)
	}
	if l.QueueStale < natsMinQueueStale {
		return fmt.Errorf("nats.locks.queue_stale must be >= %s", natsMinQueueStale)
	}
	return nil
}

func validateNATSStreams(n *NATSConfig) error {
	s := n.Streams
	if s.Replicas < 1 || natsMaxReplicas < s.Replicas {
		return fmt.Errorf("nats.streams.replicas must be between 1 and %d", natsMaxReplicas)
	}
	if 0 >= s.EventsMaxAge {
		return fmt.Errorf("nats.streams.events_max_age must be > 0")
	}
	if 0 > s.EventsMaxBytes {
		return fmt.Errorf("nats.streams.events_max_bytes must be >= 0")
	}
	if 0 >= s.AuditMaxAge {
		return fmt.Errorf("nats.streams.audit_max_age must be > 0")
	}
	switch s.LogsLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("nats.streams.logs_level must be one of: debug, info, warn, error")
	}
	if 0 >= s.LogsMaxAge {
		return fmt.Errorf("nats.streams.logs_max_age must be > 0")
	}
	if 1 > s.TasksMaxDeliver {
		return fmt.Errorf("nats.streams.tasks_max_deliver must be >= 1")
	}
	for _, step := range s.TasksBackoff {
		if 0 >= step {
			return fmt.Errorf("nats.streams.tasks_backoff entries must be > 0")
		}
	}
	if 0 < len(s.TasksBackoff) && s.TasksMaxDeliver <= len(s.TasksBackoff) {
		return fmt.Errorf("nats.streams.tasks_max_deliver must be greater than the number of nats.streams.tasks_backoff entries")
	}
	if 0 >= s.DLQMaxAge {
		return fmt.Errorf("nats.streams.dlq_max_age must be > 0")
	}
	return nil
}

func validateHostPort(field, value string) error {
	_, port, err := net.SplitHostPort(value)
	if nil != err {
		return fmt.Errorf("%s must be host:port: %w", field, err)
	}
	if "" == port {
		return fmt.Errorf("%s must include a port", field)
	}
	return nil
}
