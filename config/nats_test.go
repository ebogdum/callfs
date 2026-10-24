package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const natsBaseHeader = `
server:
  listen_addr: ":8443"
  protocol: "http"
  external_url: "localhost:8443"
auth:
  api_key_users:
    alice: "alice-key-at-least-16-chars"
  internal_proxy_secret: "internal-secret-at-least-16"
  single_use_link_secret: "link-secret-at-least-16-ch"
instance_discovery:
  instance_id: "node-1"
`

func loadNATSConfig(t *testing.T, body string) (AppConfig, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(natsBaseHeader+body), 0o600); nil != err {
		t.Fatalf("write config: %v", err)
	}
	return LoadConfigFromFile(path)
}

const natsClusterBody = `
metadata_store:
  type: "nats"
dlm:
  type: "nats"
nats:
  enabled: true
`

func TestNATSDefaultsLoadAndValidate(t *testing.T) {
	cfg, err := loadNATSConfig(t, natsClusterBody)
	if nil != err {
		t.Fatalf("load: %v", err)
	}
	if "node-1" != cfg.NATS.ServerName {
		t.Errorf("server_name = %q, want instance id default", cfg.NATS.ServerName)
	}
	if NATSModeEmbedded != cfg.NATS.Mode || 256*MiB != cfg.NATS.JetStream.MaxMemory || 8*MiB != cfg.NATS.Metadata.MaxMsgSize {
		t.Errorf("unexpected defaults: %+v", cfg.NATS)
	}
	if 5 != len(cfg.NATS.Streams.TasksBackoff) || time.Second != cfg.NATS.Streams.TasksBackoff[0] {
		t.Errorf("tasks_backoff = %v", cfg.NATS.Streams.TasksBackoff)
	}
}

func TestNATSValuesFromFile(t *testing.T) {
	cfg, err := loadNATSConfig(t, natsClusterBody+`  server_tags: {zone: a}
  routes: ["nats-route://127.0.0.1:6223"]
  jetstream:
    max_memory: 512MiB
    max_store: 2GiB
    sync: "interval"
    sync_interval: 30s
  streams:
    tasks_backoff: [2s, 4s]
    tasks_max_deliver: 3
`)
	if nil != err {
		t.Fatalf("load: %v", err)
	}
	if 512*MiB != cfg.NATS.JetStream.MaxMemory || 2*GiB != cfg.NATS.JetStream.MaxStore {
		t.Errorf("byte sizes = %v %v", cfg.NATS.JetStream.MaxMemory, cfg.NATS.JetStream.MaxStore)
	}
	if "a" != cfg.NATS.ServerTags["zone"] || 1 != len(cfg.NATS.Routes) {
		t.Errorf("tags/routes = %v %v", cfg.NATS.ServerTags, cfg.NATS.Routes)
	}
	if 2 != len(cfg.NATS.Streams.TasksBackoff) || 4*time.Second != cfg.NATS.Streams.TasksBackoff[1] {
		t.Errorf("tasks_backoff = %v", cfg.NATS.Streams.TasksBackoff)
	}
}

func TestNATSEnvOverrides(t *testing.T) {
	t.Setenv("CALLFS_NATS__JETSTREAM__MAX_MEMORY", "1GiB")
	t.Setenv("CALLFS_NATS__ROUTES", "nats-route://10.0.0.2:6222, nats-route://10.0.0.3:6222")
	t.Setenv("CALLFS_NATS__STREAMS__TASKS_BACKOFF", "1s,3s")
	t.Setenv("CALLFS_NATS__LOCKS__TTL", "20s")
	t.Setenv("CALLFS_NATS__SERVER_TAGS__ZONE", "b")
	cfg, err := loadNATSConfig(t, natsClusterBody)
	if nil != err {
		t.Fatalf("load: %v", err)
	}
	if GiB != cfg.NATS.JetStream.MaxMemory {
		t.Errorf("max_memory = %v", cfg.NATS.JetStream.MaxMemory)
	}
	if 2 != len(cfg.NATS.Routes) || "nats-route://10.0.0.3:6222" != cfg.NATS.Routes[1] {
		t.Errorf("routes = %v", cfg.NATS.Routes)
	}
	if 2 != len(cfg.NATS.Streams.TasksBackoff) || 3*time.Second != cfg.NATS.Streams.TasksBackoff[1] {
		t.Errorf("tasks_backoff = %v", cfg.NATS.Streams.TasksBackoff)
	}
	if 20*time.Second != cfg.NATS.Locks.TTL || "b" != cfg.NATS.ServerTags["zone"] {
		t.Errorf("locks.ttl/tags = %v %v", cfg.NATS.Locks.TTL, cfg.NATS.ServerTags)
	}
}

func TestNATSValidationRejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AppConfig)
		want   string
	}{
		{"metadata nats without enabled", func(c *AppConfig) { c.NATS.Enabled = false; c.DLM.Type = "local" }, "metadata_store.type=nats requires nats.enabled"},
		{"dlm nats without enabled", func(c *AppConfig) {
			c.NATS.Enabled = false
			c.MetadataStore.Type = "sqlite"
			c.MetadataStore.SQLitePath = "x"
		}, "dlm.type=nats requires nats.enabled"},
		{"peers without nats", func(c *AppConfig) {
			c.NATS.Enabled = false
			c.MetadataStore.Type = "sqlite"
			c.MetadataStore.SQLitePath = "x"
			c.DLM.Type = "local"
			c.InstanceDiscovery.PeerEndpoints = map[string]string{"node-2": "https://node-2:8443"}
		}, "peer_endpoints requires nats.enabled"},
		{"dlm local with peers", func(c *AppConfig) {
			c.DLM.Type = "local"
			c.InstanceDiscovery.PeerEndpoints = map[string]string{"node-2": "https://node-2:8443"}
		}, "dlm.type=local cannot coordinate"},
		{"bad mode", func(c *AppConfig) { c.NATS.Mode = "cloud" }, "nats.mode"},
		{"bad auth mode", func(c *AppConfig) { c.NATS.Auth.Mode = "jwt" }, "nats.auth.mode"},
		{"nkey without seed", func(c *AppConfig) { c.NATS.Auth.Mode = NATSAuthNKey }, "nkey_seed_file"},
		{"creds without file", func(c *AppConfig) { c.NATS.Auth.Mode = NATSAuthCreds }, "creds_file"},
		{"short previous secret", func(c *AppConfig) { c.NATS.Auth.PreviousSecret = "short" }, "previous_secret"},
		{"non-loopback cluster without tls", func(c *AppConfig) { c.NATS.ClusterListen = "0.0.0.0:6222" }, "nats.tls"},
		{"non-loopback client without tls", func(c *AppConfig) { c.NATS.ClientListen = "0.0.0.0:4222" }, "nats.client_listen"},
		{"cert without key", func(c *AppConfig) { c.NATS.TLS.CertFile = "c.pem" }, "set together"},
		{"bad route scheme", func(c *AppConfig) { c.NATS.Routes = []string{"http://a:6222"} }, "nats.routes"},
		{"route without port", func(c *AppConfig) { c.NATS.Routes = []string{"nats-route://a"} }, "host and port"},
		{"external without urls", func(c *AppConfig) {
			c.NATS.Mode = NATSModeExternal
			c.NATS.Auth.Mode = NATSAuthCreds
			c.NATS.Auth.CredsFile = "x"
		}, "external.urls"},
		{"external derived auth", func(c *AppConfig) {
			c.NATS.Mode = NATSModeExternal
			c.NATS.External.URLs = []string{"nats://n:4222"}
		}, "nkey or creds"},
		{"sync interval zero", func(c *AppConfig) { c.NATS.JetStream.Sync = NATSSyncInterval; c.NATS.JetStream.SyncInterval = 0 }, "sync_interval"},
		{"bad sync", func(c *AppConfig) { c.NATS.JetStream.Sync = "never" }, "nats.jetstream.sync"},
		{"empty store dir", func(c *AppConfig) { c.NATS.JetStream.StoreDir = "" }, "store_dir"},
		{"metadata replicas", func(c *AppConfig) { c.NATS.Metadata.Replicas = 6 }, "metadata.replicas"},
		{"bad read barrier", func(c *AppConfig) { c.NATS.Metadata.ReadBarrier = "lazy" }, "read_barrier"},
		{"small msg size", func(c *AppConfig) { c.NATS.Metadata.MaxMsgSize = KiB }, "max_msg_size"},
		{"lock ttl", func(c *AppConfig) { c.NATS.Locks.TTL = time.Millisecond }, "locks.ttl"},
		{"queue stale", func(c *AppConfig) { c.NATS.Locks.QueueStale = time.Millisecond }, "queue_stale"},
		{"logs level", func(c *AppConfig) { c.NATS.Streams.LogsLevel = "trace" }, "logs_level"},
		{"backoff longer than deliveries", func(c *AppConfig) { c.NATS.Streams.TasksMaxDeliver = 5 }, "tasks_max_deliver"},
		{"bad bytes", func(c *AppConfig) { c.NATS.Bytes = "grpc" }, "nats.bytes"},
		{"rpc timeout", func(c *AppConfig) { c.NATS.RPCTimeout = 0 }, "rpc_timeout"},
		{"server name whitespace", func(c *AppConfig) { c.NATS.ServerName = "a b" }, "server_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultAppConfig()
			cfg.Auth.APIKeyUsers = map[string]string{"alice": "alice-key-at-least-16-chars"}
			cfg.Auth.InternalProxySecret = "internal-secret-at-least-16"
			cfg.Auth.SingleUseLinkSecret = "link-secret-at-least-16-ch"
			cfg.Server.Protocol = "http"
			cfg.InstanceDiscovery.InstanceID = "node-1"
			cfg.MetadataStore.Type = "nats"
			cfg.DLM.Type = "nats"
			cfg.NATS.Enabled = true
			tt.mutate(&cfg)
			err := ValidateAppConfig(&cfg)
			if nil == err {
				t.Fatalf("want error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestNATSNonLoopbackClusterAcceptedWithMutualTLS(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.Auth.APIKeyUsers = map[string]string{"alice": "alice-key-at-least-16-chars"}
	cfg.Auth.InternalProxySecret = "internal-secret-at-least-16"
	cfg.Auth.SingleUseLinkSecret = "link-secret-at-least-16-ch"
	cfg.Server.Protocol = "http"
	cfg.InstanceDiscovery.InstanceID = "node-1"
	cfg.MetadataStore.Type = "nats"
	cfg.DLM.Type = "nats"
	cfg.NATS.Enabled = true
	cfg.NATS.ClusterListen = "0.0.0.0:6222"
	cfg.NATS.TLS = NATSTLSConfig{CertFile: "c.pem", KeyFile: "k.pem", CAFile: "ca.pem"}
	if err := ValidateAppConfig(&cfg); nil != err {
		t.Fatalf("mutual tls cluster rejected: %v", err)
	}
}

func TestLegacyRaftKeyInFileFails(t *testing.T) {
	_, err := loadNATSConfig(t, `
metadata_store:
  type: "sqlite"
  dsn: "x"
dlm:
  type: "local"
raft:
  node_id: "n1"
`)
	if !errors.Is(err, ErrRaftRemoved) {
		t.Fatalf("err = %v, want ErrRaftRemoved", err)
	}
	want := "raft cluster mode was removed in " + RaftRemovedVersion + "; run 'callfs migrate config' then 'callfs migrate from-raft' (docs_markdown/07-clustering-distribution.md#migrating-from-raft)"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
}

func TestLegacyRaftEnvFails(t *testing.T) {
	for _, name := range []string{"CALLFS_RAFT__NODE_ID", "CALLFS_RAFT_NODE_ID"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "n1")
			_, err := loadNATSConfig(t, `
metadata_store:
  type: "sqlite"
  dsn: "x"
dlm:
  type: "local"
`)
			if !errors.Is(err, ErrRaftRemoved) {
				t.Fatalf("err = %v, want ErrRaftRemoved", err)
			}
		})
	}
}

func TestLegacyRaftTypesFail(t *testing.T) {
	bodies := map[string]string{
		"metadata_store": "metadata_store:\n  type: \"raft\"\ndlm:\n  type: \"local\"\n",
		"dlm":            "metadata_store:\n  type: \"sqlite\"\n  dsn: \"x\"\ndlm:\n  type: \"raft\"\n",
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			_, err := loadNATSConfig(t, body)
			if !errors.Is(err, ErrRaftRemoved) {
				t.Fatalf("err = %v, want ErrRaftRemoved", err)
			}
		})
	}
}

func TestLegacyRaftTypeFailsInValidateAppConfig(t *testing.T) {
	cfg := DefaultAppConfig()
	cfg.MetadataStore.Type = "raft"
	if err := validateMetadataStoreConfig(&cfg); !errors.Is(err, ErrRaftRemoved) {
		t.Fatalf("err = %v, want ErrRaftRemoved", err)
	}
	cfg.DLM.Type = "raft"
	if err := validateDLMConfig(&cfg); !errors.Is(err, ErrRaftRemoved) {
		t.Fatalf("err = %v, want ErrRaftRemoved", err)
	}
}

func TestByteSizeParseAndFormat(t *testing.T) {
	tests := []struct {
		in   string
		want ByteSize
	}{
		{"256MiB", 256 * MiB},
		{"1GiB", GiB},
		{"1.5KiB", 1536},
		{"2MB", 2000000},
		{"512", 512},
		{"0", 0},
	}
	for _, tt := range tests {
		got, err := ParseByteSize(tt.in)
		if nil != err || tt.want != got {
			t.Errorf("ParseByteSize(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
	if _, err := ParseByteSize("lots"); nil == err {
		t.Error("ParseByteSize(lots) accepted")
	}
	if "256MiB" != (256 * MiB).String() {
		t.Errorf("String() = %s", 256*MiB)
	}
}

func TestSetByPathNATSTypes(t *testing.T) {
	cfg := DefaultAppConfig()
	if err := SetByPath(&cfg, "nats.jetstream.max_memory", "512MiB"); nil != err || 512*MiB != cfg.NATS.JetStream.MaxMemory {
		t.Errorf("max_memory = %v, %v", cfg.NATS.JetStream.MaxMemory, err)
	}
	if err := SetByPath(&cfg, "nats.metadata.max_msg_size", float64(1048576)); nil != err || MiB != cfg.NATS.Metadata.MaxMsgSize {
		t.Errorf("max_msg_size = %v, %v", cfg.NATS.Metadata.MaxMsgSize, err)
	}
	if err := SetByPath(&cfg, "nats.streams.tasks_backoff", []any{"1s", "2s"}); nil != err || 2 != len(cfg.NATS.Streams.TasksBackoff) || 2*time.Second != cfg.NATS.Streams.TasksBackoff[1] {
		t.Errorf("tasks_backoff = %v, %v", cfg.NATS.Streams.TasksBackoff, err)
	}
	if err := SetByPath(&cfg, "nats.streams.tasks_backoff", []any{1}); nil == err {
		t.Error("numeric backoff entry accepted")
	}
}

func TestSchemaNATSRows(t *testing.T) {
	for path, restart := range map[string]string{
		"nats.enabled":               "restart",
		"nats.streams.dlq_max_age":   "runtime",
		"nats.metadata.read_barrier": "runtime",
		"nats.metadata.replicas":     "restart",
		"nats.auth.previous_secret":  "restart",
		"nats.jetstream.max_memory":  "restart",
		"nats.streams.tasks_backoff": "runtime",
	} {
		key, ok := LookupKey(path)
		if !ok {
			t.Errorf("%s missing from schema", path)
			continue
		}
		if restart != key.Restart {
			t.Errorf("%s restart = %s, want %s", path, key.Restart, restart)
		}
	}
	if key, _ := LookupKey("nats.metadata.max_msg_size"); "index" != key.Group || "8MiB" != key.Default {
		t.Errorf("max_msg_size group/default = %s / %v", key.Group, key.Default)
	}
	if key, _ := LookupKey("nats.auth.previous_secret"); !key.Secret || nil != key.Default {
		t.Errorf("previous_secret must be secret with nil default")
	}
	if _, ok := LookupKey("raft.node_id"); ok {
		t.Error("raft.node_id still in schema")
	}
	if key, _ := LookupKey("metadata_store.type"); !containsString(key.Enum, "nats") || containsString(key.Enum, "raft") {
		t.Errorf("metadata_store.type enum = %v", key.Enum)
	}
	if key, _ := LookupKey("dlm.type"); !containsString(key.Enum, "nats") || containsString(key.Enum, "raft") {
		t.Errorf("dlm.type enum = %v", key.Enum)
	}
}
