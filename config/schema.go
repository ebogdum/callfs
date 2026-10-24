package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

const envOnlyLogMode = "log.mode"

var (
	durationType      = reflect.TypeOf(time.Duration(0))
	durationSliceType = reflect.TypeOf([]time.Duration(nil))
	byteSizeType      = reflect.TypeOf(ByteSize(0))
)

type SchemaKey struct {
	Path         string   `json:"path"`
	Type         string   `json:"type"`
	Enum         []string `json:"enum"`
	Min          *float64 `json:"min"`
	Max          *float64 `json:"max"`
	Default      any      `json:"default"`
	RequiredWhen *string  `json:"required_when"`
	Secret       bool     `json:"secret"`
	Env          string   `json:"env"`
	Restart      string   `json:"restart"`
	Group        string   `json:"group"`
	DocsAnchor   string   `json:"docs_anchor"`
}

type keySpec struct {
	path         string
	typ          string
	group        string
	restart      string
	enum         []string
	min          *float64
	max          *float64
	secret       bool
	requiredWhen string
}

func bound(v float64) *float64 {
	return &v
}

var keySpecs = []keySpec{
	{"server.listen_addr", "hostport", "network", "restart", nil, nil, nil, false, ""},
	{"server.protocol", "enum", "network", "restart", []string{"https", "http", "auto"}, nil, nil, false, ""},
	{"server.external_url", "url", "network", "restart", nil, nil, nil, false, ""},
	{"server.cert_file", "path", "security", "restart", nil, nil, nil, false, ""},
	{"server.key_file", "path", "security", "restart", nil, nil, nil, false, ""},
	{"server.enable_quic", "bool", "network", "restart", nil, nil, nil, false, ""},
	{"server.quic_listen_addr", "hostport", "network", "restart", nil, nil, nil, false, ""},
	{"server.read_header_timeout", "duration", "limits", "restart", nil, nil, nil, false, ""},
	{"server.idle_timeout", "duration", "limits", "restart", nil, nil, nil, false, ""},
	{"server.read_timeout", "duration", "limits", "restart", nil, nil, nil, false, ""},
	{"server.write_timeout", "duration", "limits", "restart", nil, nil, nil, false, ""},
	{"server.transfer_timeout", "duration", "limits", "restart", nil, nil, nil, false, ""},
	{"server.file_op_timeout", "duration", "limits", "restart", nil, nil, nil, false, ""},
	{"server.metadata_op_timeout", "duration", "limits", "restart", nil, nil, nil, false, ""},
	{"server.metadata_cache_ttl", "duration", "limits", "runtime", nil, nil, nil, false, ""},
	{"server.metadata_cache_size", "int", "limits", "restart", nil, bound(0), nil, false, ""},
	{"server.trusted_proxies", "list", "network", "restart", nil, nil, nil, false, ""},
	{"server.rate_limit.link_generate_rps", "float", "limits", "runtime", nil, bound(0), nil, false, ""},
	{"server.rate_limit.link_generate_burst", "int", "limits", "runtime", nil, bound(1), nil, false, ""},
	{"server.rate_limit.max_active_links_per_user", "int", "limits", "restart", nil, bound(0), nil, false, ""},
	{"server.rate_limit.download_rps", "float", "limits", "runtime", nil, bound(0), nil, false, ""},
	{"server.rate_limit.download_burst", "int", "limits", "runtime", nil, bound(1), nil, false, ""},
	{"server.rate_limit.session_rps", "float", "limits", "runtime", nil, bound(0), nil, false, ""},
	{"server.rate_limit.session_burst", "int", "limits", "runtime", nil, bound(1), nil, false, ""},
	{"auth.api_keys", "list", "access", "auth_reload", nil, nil, nil, true, ""},
	{"auth.api_key_users", "map", "access", "auth_reload", nil, nil, nil, true, ""},
	{"auth.internal_proxy_secret", "secret", "security", "restart", nil, nil, nil, true, ""},
	{"auth.single_use_link_secret", "secret", "security", "restart", nil, nil, nil, true, ""},
	{"auth.admin_users", "list", "access", "auth_reload", nil, nil, nil, false, ""},
	{"log.level", "enum", "logging", "runtime", []string{"debug", "info", "warn", "error"}, nil, nil, false, ""},
	{"log.format", "enum", "logging", "restart", []string{"json", "text"}, nil, nil, false, ""},
	{"metrics.listen_addr", "hostport", "network", "restart", nil, nil, nil, false, ""},
	{"metrics.profiling", "bool", "network", "restart", nil, nil, nil, false, ""},
	{"backend.default_backend", "enum", "storage", "restart", []string{"localfs", "s3"}, nil, nil, false, ""},
	{"backend.localfs_root_path", "path", "storage", "restart", nil, nil, nil, false, ""},
	{"performance.mode", "enum", "storage", "restart", []string{"fast", "strict"}, nil, nil, false, ""},
	{"backend.localfs_sync", "enum", "storage", "restart", []string{"full", "barrier", "none"}, nil, nil, false, ""},
	{"backend.localfs_ack", "enum", "storage", "restart", []string{"memory", "disk"}, nil, nil, false, ""},
	{"backend.flush_workers", "int", "storage", "restart", nil, bound(0), bound(1024), false, ""},
	{"backend.flush_max_pending_bytes", "int", "storage", "restart", nil, bound(0), nil, false, ""},
	{"backend.flush_max_delay", "duration", "storage", "restart", nil, nil, nil, false, ""},
	{"backend.s3_access_key", "secret", "storage", "restart", nil, nil, nil, true, ""},
	{"backend.s3_secret_key", "secret", "storage", "restart", nil, nil, nil, true, ""},
	{"backend.s3_region", "string", "storage", "restart", nil, nil, nil, false, ""},
	{"backend.s3_bucket_name", "string", "storage", "restart", nil, nil, nil, false, ""},
	{"backend.s3_endpoint", "url", "storage", "restart", nil, nil, nil, false, ""},
	{"backend.s3_allow_insecure", "bool", "storage", "restart", nil, nil, nil, false, ""},
	{"backend.s3_server_side_encryption", "enum", "storage", "restart", []string{"AES256", "aws:kms", ""}, nil, nil, false, ""},
	{"backend.s3_acl", "string", "storage", "restart", nil, nil, nil, false, ""},
	{"backend.s3_kms_key_id", "string", "storage", "restart", nil, nil, nil, false, ""},
	{"backend.internal_proxy_skip_tls_verify", "bool", "security", "restart", nil, nil, nil, false, ""},
	{"metadata_store.type", "enum", "index", "restart", []string{"postgres", "sqlite", "redis", "nats"}, nil, nil, false, ""},
	{"metadata_store.dsn", "secret", "index", "restart", nil, nil, nil, true, ""},
	{"metadata_store.migration_dsn", "secret", "index", "restart", nil, nil, nil, true, ""},
	{"metadata_store.sqlite_path", "path", "index", "restart", nil, nil, nil, false, ""},
	{"metadata_store.redis_addr", "hostport", "index", "restart", nil, nil, nil, false, ""},
	{"metadata_store.redis_username", "string", "index", "restart", nil, nil, nil, false, ""},
	{"metadata_store.redis_password", "secret", "index", "restart", nil, nil, nil, true, ""},
	{"metadata_store.redis_db", "int", "index", "restart", nil, bound(0), bound(15), false, ""},
	{"metadata_store.redis_key_prefix", "string", "index", "restart", nil, nil, nil, false, ""},
	{"metadata_store.redis_tls", "bool", "index", "restart", nil, nil, nil, false, ""},
	{"metadata_store.redis_tls_ca_file", "path", "index", "restart", nil, nil, nil, false, ""},
	{"metadata_store.redis_tls_skip_verify", "bool", "index", "restart", nil, nil, nil, false, ""},
	{"metadata_store.postgres.max_open_conns", "int", "index", "restart", nil, nil, nil, false, ""},
	{"metadata_store.postgres.max_idle_conns", "int", "index", "restart", nil, nil, nil, false, ""},
	{"metadata_store.postgres.conn_max_lifetime", "duration", "index", "restart", nil, nil, nil, false, ""},
	{"nats.enabled", "bool", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.mode", "enum", "coordination", "restart", []string{"embedded", "external"}, nil, nil, false, "nats.enabled == true"},
	{"nats.server_name", "string", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.cluster_name", "string", "coordination", "restart", nil, nil, nil, false, "nats.enabled == true"},
	{"nats.client_listen", "hostport", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.cluster_listen", "hostport", "coordination", "restart", nil, nil, nil, false, "nats.mode == embedded"},
	{"nats.routes", "list", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.routes_from_peers", "bool", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.monitor_listen", "hostport", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.server_tags", "map", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.tls.cert_file", "path", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.tls.key_file", "path", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.tls.ca_file", "path", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.tls.client_cert_file", "path", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.tls.client_key_file", "path", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.auth.mode", "enum", "coordination", "restart", []string{"derived", "nkey", "creds"}, nil, nil, false, "nats.enabled == true"},
	{"nats.auth.nkey_seed_file", "path", "coordination", "restart", nil, nil, nil, false, "nats.auth.mode == nkey"},
	{"nats.auth.creds_file", "path", "coordination", "restart", nil, nil, nil, false, "nats.auth.mode == creds"},
	{"nats.auth.previous_secret", "secret", "coordination", "restart", nil, nil, nil, true, ""},
	{"nats.jetstream.store_dir", "path", "coordination", "restart", nil, nil, nil, false, "nats.mode == embedded"},
	{"nats.jetstream.max_memory", "bytes", "coordination", "restart", nil, bound(0), nil, false, ""},
	{"nats.jetstream.max_store", "bytes", "coordination", "restart", nil, bound(0), nil, false, ""},
	{"nats.jetstream.sync", "enum", "coordination", "restart", []string{"always", "interval"}, nil, nil, false, ""},
	{"nats.jetstream.sync_interval", "duration", "coordination", "restart", nil, nil, nil, false, "nats.jetstream.sync == interval"},
	{"nats.jetstream.unique_tag", "string", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.external.urls", "list", "coordination", "restart", nil, nil, nil, false, "nats.mode == external"},
	{"nats.external.js_domain", "string", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.external.tls_ca_file", "path", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.rpc_timeout", "duration", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.metadata.replicas", "int", "index", "restart", nil, bound(1), bound(5), false, ""},
	{"nats.metadata.apply_timeout", "duration", "index", "restart", nil, nil, nil, false, ""},
	{"nats.metadata.read_barrier", "enum", "index", "runtime", []string{"info", "publish"}, nil, nil, false, ""},
	{"nats.metadata.miss_barrier", "bool", "index", "restart", nil, nil, nil, false, ""},
	{"nats.metadata.snapshot_interval", "duration", "index", "restart", nil, nil, nil, false, ""},
	{"nats.metadata.snapshot_every", "int", "index", "restart", nil, bound(1), nil, false, ""},
	{"nats.metadata.retain_commands", "int", "index", "restart", nil, bound(1), nil, false, ""},
	{"nats.metadata.retain_snapshots", "int", "index", "restart", nil, bound(1), nil, false, ""},
	{"nats.metadata.max_msg_size", "bytes", "index", "restart", nil, bound(65536), bound(67108864), false, ""},
	{"nats.locks.replicas", "int", "coordination", "restart", nil, bound(1), bound(5), false, ""},
	{"nats.locks.ttl", "duration", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.locks.queue_stale", "duration", "coordination", "restart", nil, nil, nil, false, ""},
	{"nats.streams.replicas", "int", "coordination", "runtime", nil, bound(1), bound(5), false, ""},
	{"nats.streams.events_max_age", "duration", "coordination", "runtime", nil, nil, nil, false, ""},
	{"nats.streams.events_max_bytes", "bytes", "coordination", "runtime", nil, bound(0), nil, false, ""},
	{"nats.streams.audit_max_age", "duration", "coordination", "runtime", nil, nil, nil, false, ""},
	{"nats.streams.logs_level", "enum", "coordination", "runtime", []string{"debug", "info", "warn", "error"}, nil, nil, false, ""},
	{"nats.streams.logs_max_age", "duration", "coordination", "runtime", nil, nil, nil, false, ""},
	{"nats.streams.tasks_max_deliver", "int", "coordination", "runtime", nil, bound(1), nil, false, ""},
	{"nats.streams.tasks_backoff", "list", "coordination", "runtime", nil, nil, nil, false, ""},
	{"nats.streams.dlq_max_age", "duration", "coordination", "runtime", nil, nil, nil, false, ""},
	{"nats.bytes", "enum", "coordination", "restart", []string{"http", "chunked"}, nil, nil, false, ""},
	{"nats.counters", "bool", "coordination", "restart", nil, nil, nil, false, ""},
	{"dlm.type", "enum", "coordination", "restart", []string{"redis", "local", "nats"}, nil, nil, false, ""},
	{"dlm.redis_addr", "hostport", "coordination", "restart", nil, nil, nil, false, ""},
	{"dlm.redis_password", "secret", "coordination", "restart", nil, nil, nil, true, ""},
	{"dlm.wait_timeout", "duration", "coordination", "runtime", nil, nil, nil, false, ""},
	{"dlm.redis_pool_size", "int", "coordination", "restart", nil, nil, nil, false, ""},
	{"dlm.redis_min_idle_conns", "int", "coordination", "restart", nil, nil, nil, false, ""},
	{"dlm.redis_tls", "bool", "coordination", "restart", nil, nil, nil, false, ""},
	{"dlm.redis_tls_ca_file", "path", "coordination", "restart", nil, nil, nil, false, ""},
	{"dlm.redis_tls_skip_verify", "bool", "coordination", "restart", nil, nil, nil, false, ""},
	{"ha.replication_enabled", "bool", "storage", "restart", nil, nil, nil, false, ""},
	{"ha.replica_backend", "enum", "storage", "restart", []string{"localfs", "s3"}, nil, nil, false, "ha.replication_enabled == true"},
	{"ha.require_replica_success", "bool", "storage", "restart", nil, nil, nil, false, ""},
	{"instance_discovery.instance_id", "string", "coordination", "restart", nil, nil, nil, false, ""},
	{"instance_discovery.peer_endpoints", "map", "coordination", "restart", nil, nil, nil, false, ""},
	{"erasure.enabled", "bool", "storage", "restart", nil, nil, nil, false, ""},
	{"erasure.data_shards", "int", "storage", "restart", nil, bound(2), bound(32), false, ""},
	{"erasure.parity_shards", "int", "storage", "restart", nil, bound(1), bound(32), false, ""},
	{"erasure.shard_backend", "enum", "storage", "restart", []string{"localfs", "s3"}, nil, nil, false, ""},
	{"ui.enabled", "bool", "logging", "restart", nil, nil, nil, false, ""},
	{"ui.session_ttl", "duration", "limits", "runtime", nil, nil, nil, false, ""},
	{"ui.idle_timeout", "duration", "limits", "runtime", nil, nil, nil, false, ""},
	{"ui.allow_remote_restart", "bool", "logging", "restart", nil, nil, nil, false, ""},
	{"ui.metrics_sample_interval", "duration", "logging", "restart", nil, nil, nil, false, ""},
	{"ui.metrics_retention", "duration", "logging", "restart", nil, nil, nil, false, ""},
	{"ui.log_buffer_entries", "int", "logging", "restart", nil, bound(100), bound(1000000), false, ""},
	{"ui.audit_buffer_entries", "int", "logging", "restart", nil, bound(100), bound(1000000), false, ""},
	{"ui.event_buffer_entries", "int", "logging", "restart", nil, bound(100), bound(1000000), false, ""},
	{"trash.enabled", "bool", "storage", "runtime", nil, nil, nil, false, ""},
	{"trash.retention", "duration", "storage", "runtime", nil, nil, nil, false, ""},
}

var schemaByPath = buildSchemaIndex()

func buildSchemaIndex() map[string]SchemaKey {
	index := make(map[string]SchemaKey, len(keySpecs)+1)
	for _, key := range Schema() {
		index[key.Path] = key
	}
	return index
}

func envName(path string) string {
	return "CALLFS_" + strings.ToUpper(strings.ReplaceAll(path, ".", "__"))
}

func Schema() []SchemaKey {
	defaults := Flatten(DefaultAppConfig())
	keys := make([]SchemaKey, 0, len(keySpecs)+1)
	for _, spec := range keySpecs {
		key := SchemaKey{
			Path:       spec.path,
			Type:       spec.typ,
			Enum:       spec.enum,
			Min:        spec.min,
			Max:        spec.max,
			Secret:     spec.secret,
			Env:        envName(spec.path),
			Restart:    spec.restart,
			Group:      spec.group,
			DocsAnchor: "02-configuration.md#" + spec.path,
		}
		if spec.secret {
			key.Default = nil
		} else {
			key.Default = defaults[spec.path]
		}
		if "" != spec.requiredWhen {
			expr := spec.requiredWhen
			key.RequiredWhen = &expr
		}
		if nil == key.Enum {
			key.Enum = []string{}
		}
		keys = append(keys, key)
	}
	keys = append(keys, SchemaKey{
		Path:       envOnlyLogMode,
		Type:       "enum",
		Enum:       []string{"production", "development", "debug"},
		Default:    "production",
		Env:        "CALLFS_LOG_MODE",
		Restart:    "restart",
		Group:      "logging",
		DocsAnchor: "02-configuration.md#" + envOnlyLogMode,
	})
	return keys
}

func LookupKey(path string) (SchemaKey, bool) {
	key, ok := schemaByPath[path]
	return key, ok
}

func IsEnvOnly(path string) bool {
	return envOnlyLogMode == path
}

func Flatten(cfg AppConfig) map[string]any {
	out := make(map[string]any)
	flattenStruct(reflect.ValueOf(cfg), "", out)
	return out
}

func flattenStruct(v reflect.Value, prefix string, out map[string]any) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Tag.Get("koanf")
		if "" == name {
			continue
		}
		path := name
		if "" != prefix {
			path = prefix + "." + name
		}
		field := v.Field(i)
		if reflect.Struct == field.Kind() && durationType != field.Type() {
			flattenStruct(field, path, out)
			continue
		}
		out[path] = leafValue(field)
	}
}

func leafValue(v reflect.Value) any {
	if durationType == v.Type() {
		return time.Duration(v.Int()).String()
	}
	if byteSizeType == v.Type() {
		return ByteSize(v.Int()).String()
	}
	if durationSliceType == v.Type() {
		items := make([]string, v.Len())
		for i := range items {
			items[i] = time.Duration(v.Index(i).Int()).String()
		}
		return items
	}
	return v.Interface()
}

func fieldByPath(root reflect.Value, path string) (reflect.Value, error) {
	current := root
	for _, segment := range strings.Split(path, ".") {
		if reflect.Struct != current.Kind() {
			return reflect.Value{}, fmt.Errorf("unknown setting %s", path)
		}
		found := false
		t := current.Type()
		for i := 0; i < t.NumField(); i++ {
			if segment == t.Field(i).Tag.Get("koanf") {
				current = current.Field(i)
				found = true
				break
			}
		}
		if !found {
			return reflect.Value{}, fmt.Errorf("unknown setting %s", path)
		}
	}
	if reflect.Struct == current.Kind() && durationType != current.Type() {
		return reflect.Value{}, fmt.Errorf("unknown setting %s", path)
	}
	return current, nil
}

func SetByPath(cfg *AppConfig, path string, raw any) error {
	field, err := fieldByPath(reflect.ValueOf(cfg).Elem(), path)
	if nil != err {
		return err
	}
	if durationType == field.Type() {
		text, ok := raw.(string)
		if !ok {
			return fmt.Errorf("%s must be a duration such as 30s", path)
		}
		parsed, parseErr := time.ParseDuration(text)
		if nil != parseErr {
			return fmt.Errorf("%s must be a duration such as 30s", path)
		}
		field.SetInt(int64(parsed))
		return nil
	}
	if durationSliceType == field.Type() {
		items, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("%s must be a list of durations such as 30s", path)
		}
		parsed := make([]time.Duration, len(items))
		for i, item := range items {
			text, isText := item.(string)
			if !isText {
				return fmt.Errorf("%s must be a list of durations such as 30s", path)
			}
			value, parseErr := time.ParseDuration(text)
			if nil != parseErr {
				return fmt.Errorf("%s must be a list of durations such as 30s", path)
			}
			parsed[i] = value
		}
		field.Set(reflect.ValueOf(parsed))
		return nil
	}
	if byteSizeType == field.Type() {
		if number, isNumber := raw.(float64); isNumber {
			field.SetInt(int64(number))
			return nil
		}
	}
	buf, err := json.Marshal(raw)
	if nil != err {
		return fmt.Errorf("%s has an unsupported value", path)
	}
	target := reflect.New(field.Type())
	if decodeErr := json.Unmarshal(buf, target.Interface()); nil != decodeErr {
		return fmt.Errorf("%s has the wrong type", path)
	}
	field.Set(target.Elem())
	return nil
}

func CopyByPath(dst, src *AppConfig, path string) error {
	from, err := fieldByPath(reflect.ValueOf(src).Elem(), path)
	if nil != err {
		return err
	}
	to, err := fieldByPath(reflect.ValueOf(dst).Elem(), path)
	if nil != err {
		return err
	}
	to.Set(from)
	return nil
}

func CheckValue(path string, raw any) error {
	key, ok := LookupKey(path)
	if !ok {
		return fmt.Errorf("unknown setting %s", path)
	}
	if IsEnvOnly(path) {
		return fmt.Errorf("%s is set by environment", path)
	}
	if 0 < len(key.Enum) {
		text, isText := raw.(string)
		if !isText || !containsString(key.Enum, text) {
			return fmt.Errorf("%s must be one of: %s", path, strings.Join(key.Enum, ", "))
		}
	}
	if number, isNumber := raw.(float64); isNumber {
		if nil != key.Min && number < *key.Min {
			return fmt.Errorf("%s must be at least %v", path, *key.Min)
		}
		if nil != key.Max && number > *key.Max {
			return fmt.Errorf("%s must be at most %v", path, *key.Max)
		}
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if target == value {
			return true
		}
	}
	return false
}

func ValidateAppConfig(cfg *AppConfig) error {
	return validateConfig(cfg)
}
